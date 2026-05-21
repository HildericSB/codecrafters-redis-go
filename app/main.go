package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"github.com/codecrafters-io/redis-starter-go/app/resp"
)

type Server struct {
	entries map[string]*Entry
}

type Entry struct {
	val            any
	expirationDate time.Time
}

func main() {
	l, err := net.Listen("tcp", "0.0.0.0:6379")
	if err != nil {
		fmt.Println("Failed to bind to port 6379")
		os.Exit(1)
	}
	defer l.Close()

	s := Server{
		entries: make(map[string]*Entry),
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			fmt.Println("Error accepting connection: ", err.Error())
			os.Exit(1)
		}

		go s.handleConnection(conn)
	}

}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)

	for {
		val, err := resp.ReadRESP(reader)
		if err != nil {
			// Client send EOF when finished
			if err != io.EOF {
				fmt.Println("error reading RESP:", err)
			}
			return
		}

		fmt.Print(val)

		if val.Type == resp.Array {
			s.handleCmd(val, conn)
		}
	}

}

func (s *Server) handleCmd(r resp.RESP, conn net.Conn) error {
	cmd := strings.ToUpper(r.Items[0].String())

	switch cmd {
	case "ECHO":
		conn.Write(resp.EncodeBulkString(string(r.Items[1].Data)))

	case "PING":
		conn.Write(resp.EncodeSimpleString("PONG"))

	case "SET":
		if r.Count < 2 {
			return fmt.Errorf("SET cmd requires at least 1 parameters")
		}
		key := r.Items[1].String()
		val := r.Items[2].String()

		expiry, err := parseExpiry(r)
		if err != nil {
			return err
		}

		v := string(val)

		s.entries[key] = &Entry{val: v, expirationDate: expiry}
		conn.Write(resp.EncodeSimpleString("OK"))

	case "GET":
		if r.Count < 2 {
			return fmt.Errorf("GET cmd requires at least 1 parameter")
		}

		key := r.Items[1].String()
		entry := s.entries[key]
		var stringVal string

		if entry == nil {
			conn.Write([]byte("$-1\r\n"))
			return nil
		}

		if value, ok := entry.val.(string); !ok {
			return fmt.Errorf("GET not supported for this type of entry : %T", entry.val)
		} else {
			stringVal = value
		}

		if !entry.expirationDate.IsZero() && entry.expirationDate.Before(time.Now()) {
			delete(s.entries, key)
			conn.Write([]byte("$-1\r\n"))
			return nil
		}

		conn.Write(resp.EncodeBulkString(stringVal))

	case "RPUSH":
		if r.Count < 3 {
			return fmt.Errorf("RPUSH cmd requires at least 2 parameter")
		}

		key := r.Items[1].String()
		entry := s.entries[key]

		if entry == nil {
			entry = &Entry{val: []string{}}
			s.entries[key] = entry
		}

		if values, ok := entry.val.([]string); ok {
			for _, item := range r.Items[2:] {
				values = append(values, item.String())
			}

			entry.val = values
			s.entries[key] = entry
			conn.Write(resp.EncodeInteger(len(values)))
		} else {
			return fmt.Errorf("entry with key %s is not a list", key)
		}

	case "LRANGE":
		// 	# List items from index 2 to 4
		// > LRANGE list_key 2 4
		// 1) "c"
		// 2) "d"
		// 3) "e"
		if r.Count < 4 {
			return fmt.Errorf("LRANGE cmd requires at least 3 parameter")
		}

		key := r.Items[1].String()
		entry := s.entries[key]
		startIndex := r.Items[2].Int()
		endIndex := r.Items[3].Int()

		if entry == nil {
			conn.Write([]byte("*0\r\n"))
			return nil
		}

		var res []resp.RESP
		if values, ok := entry.val.([]string); ok {
			if startIndex < 0 {
				startIndex = max(startIndex+len(values), 0)

			}
			if endIndex < 0 {
				endIndex = max(endIndex+len(values), 0)
			}
			for i := startIndex; i < len(values) && i <= endIndex; i++ {
				res = append(res, resp.RESP{Type: resp.Bulk, Data: []byte(values[i])})
			}

			conn.Write(resp.EncodeArray(res))
		} else {
			return fmt.Errorf("entry with key %s is not a list", key)
		}

	default:
		return fmt.Errorf("Unknown cmd : %v", cmd)
	}

	return nil
}

func parseExpiry(r resp.RESP) (time.Time, error) {
	if r.Count <= 3 {
		return time.Time{}, nil
	}

	option := strings.ToUpper(r.Items[3].String())
	switch option {
	case "PX":
		if len(r.Items) < 5 {
			return time.Time{}, fmt.Errorf("PX requires a value")
		}
		return time.Now().Add(time.Millisecond * time.Duration(r.Items[4].Int())), nil
	default:
		return time.Time{}, fmt.Errorf("unknown option: %s", option)
	}
}
