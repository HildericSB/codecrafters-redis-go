package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/codecrafters-io/redis-starter-go/app/resp"
)

type Server struct {
	entries map[string]*Entry
	mu      sync.Mutex
	waiters map[string][]chan string
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
		waiters: make(map[string][]chan string),
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
	args := r.Items[1:]

	switch cmd {
	case "ECHO":
		return s.handleEcho(args, conn)
	case "PING":
		return s.handlePing(conn)
	case "SET":
		return s.handleSet(args, conn)
	case "GET":
		return s.handleGet(args, conn)
	case "RPUSH":
		return s.handleRpush(args, conn)
	case "LRANGE":
		return s.handleLrange(args, conn)
	case "LPUSH":
		return s.handleLpush(args, conn)
	case "LLEN":
		return s.handleLlen(args, conn)
	case "LPOP":
		return s.handleLpop(args, conn)
	case "BLPOP":
		return s.handleBLPOP(args, conn)
	default:
		return fmt.Errorf("Unknown cmd : %v", cmd)
	}
}

func (s *Server) handleEcho(args []resp.RESP, conn net.Conn) error {
	conn.Write(resp.EncodeBulkString(resp.Ptr(string(args[0].Data))))
	return nil
}

func (s *Server) handlePing(conn net.Conn) error {
	conn.Write(resp.EncodeSimpleString("PONG"))
	return nil
}

func (s *Server) handleSet(args []resp.RESP, conn net.Conn) error {
	if len(args) < 2 {
		return fmt.Errorf("SET cmd requires at least 2 arguments")
	}
	key := args[0].String()
	val := args[1].String()

	expiry, err := parseExpiry(args)
	if err != nil {
		return err
	}

	s.entries[key] = &Entry{val: val, expirationDate: expiry}
	conn.Write(resp.EncodeSimpleString("OK"))
	return nil
}

func (s *Server) handleGet(args []resp.RESP, conn net.Conn) error {
	if len(args) < 1 {
		return fmt.Errorf("GET cmd requires at least 1 argument")
	}

	key := args[0].String()
	entry := s.entries[key]

	if entry == nil {
		conn.Write([]byte("$-1\r\n"))
		return nil
	}

	value, ok := entry.val.(string)
	if !ok {
		return fmt.Errorf("GET not supported for this type of entry : %T", entry.val)
	}

	if !entry.expirationDate.IsZero() && entry.expirationDate.Before(time.Now()) {
		delete(s.entries, key)
		conn.Write([]byte("$-1\r\n"))
		return nil
	}

	conn.Write(resp.EncodeBulkString(&value))
	return nil
}

func (s *Server) handleRpush(args []resp.RESP, conn net.Conn) error {
	if len(args) < 2 {
		return fmt.Errorf("RPUSH cmd requires at least 2 arguments")
	}

	key := args[0].String()
	entry := s.entries[key]

	if entry == nil {
		entry = &Entry{val: []string{}}
		s.entries[key] = entry
	}

	values, ok := entry.val.([]string)
	if !ok {
		return fmt.Errorf("entry with key %s is not a list", key)
	}

	for _, item := range args[1:] {
		values = append(values, item.String())
	}

	entry.val = values
	s.entries[key] = entry

	s.mu.Lock()

	if len(s.waiters[key]) != 0 {
		val := values[0]
		entry.val = values[1:]
		s.waiters[key][0] <- val
		s.waiters[key] = s.waiters[key][1:]

	}
	s.mu.Unlock()

	conn.Write(resp.EncodeInteger(len(values)))

	return nil
}

func (s *Server) handleLpush(args []resp.RESP, conn net.Conn) error {
	if len(args) < 2 {
		return fmt.Errorf("LPUSH cmd requires at least 2 arguments")
	}

	key := args[0].String()
	entry := s.entries[key]

	if entry == nil {
		entry = &Entry{val: []string{}}
		s.entries[key] = entry
	}

	if values, ok := entry.val.([]string); ok {
		for _, item := range args[1:] {
			values = append([]string{item.String()}, values...)
		}

		entry.val = values
		s.entries[key] = entry
		conn.Write(resp.EncodeInteger(len(values)))
	} else {
		return fmt.Errorf("entry with key %s is not a list", key)
	}
	return nil
}

func (s *Server) handleLrange(args []resp.RESP, conn net.Conn) error {
	if len(args) < 3 {
		return fmt.Errorf("LRANGE cmd requires at least 3 arguments")
	}

	key := args[0].String()
	entry := s.entries[key]
	startIndex := args[1].Int()
	endIndex := args[2].Int()

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
	return nil
}

func (s *Server) handleLlen(args []resp.RESP, conn net.Conn) error {
	if len(args) < 1 {
		return fmt.Errorf("LLEN cmd requires at least 1 parameters")
	}
	key := args[0].String()
	entry := s.entries[key]

	if entry == nil {
		conn.Write(resp.EncodeInteger(0))
		return nil
	}

	if values, ok := entry.val.([]string); ok {
		conn.Write(resp.EncodeInteger(len(values)))
	} else {
		return fmt.Errorf("entry with key %s is not a list", key)
	}

	return nil
}

func (s *Server) handleLpop(args []resp.RESP, conn net.Conn) error {
	if len(args) < 1 {
		return fmt.Errorf("LPOP cmd requires at least 1 parameters")
	}

	key := args[0].String()
	entry := s.entries[key]

	if entry == nil {
		conn.Write([]byte("$-1\r\n"))
		return nil
	}

	values, ok := entry.val.([]string)
	if !ok {
		return fmt.Errorf("entry with key %s is not a list", key)
	}

	withCount := len(args) == 2
	endIndex := 1
	if withCount {
		endIndex = min(args[1].Int(), len(values))
	}

	elems := values[0:endIndex]
	entry.val = values[endIndex:]

	if len(entry.val.([]string)) == 0 {
		delete(s.entries, key)
	}

	if !withCount {
		conn.Write(resp.EncodeBulkString(&elems[0]))
	} else {
		var res []resp.RESP
		for _, v := range elems {
			res = append(res, resp.RESP{Type: resp.Bulk, Data: []byte(v)})
		}
		conn.Write(resp.EncodeArray(res))
	}

	return nil
}

func (s *Server) handleBLPOP(args []resp.RESP, conn net.Conn) error {
	if len(args) < 2 {
		return fmt.Errorf("BLPOP cmd requires at least 2 parameters")
	}

	key := args[0].String()
	entry := s.entries[key]
	// timeoutSecs := args[1]

	if entry != nil {
		values, ok := entry.val.([]string)
		if !ok {
			return fmt.Errorf("entry with key %s is not a list", key)
		}

		elem := values[0]
		entry.val = values[1:]
		conn.Write(resp.EncodeArray([]resp.RESP{
			{Type: resp.Bulk, Data: []byte(key)},
			{Type: resp.Bulk, Data: []byte(elem)},
		}))

	}

	ch := make(chan string, 1)
	s.mu.Lock()
	s.waiters[key] = append(s.waiters[key], ch)
	s.mu.Unlock()

	select {
	case val := <-ch:
		conn.Write(resp.EncodeArray([]resp.RESP{
			{Type: resp.Bulk, Data: []byte(key)},
			{Type: resp.Bulk, Data: []byte(val)},
		}))
	}

	return nil
}

func parseExpiry(args []resp.RESP) (time.Time, error) {
	if len(args) <= 2 {
		return time.Time{}, nil
	}

	option := strings.ToUpper(args[2].String())
	switch option {
	case "PX":
		if len(args) < 4 {
			return time.Time{}, fmt.Errorf("PX requires a value")
		}
		return time.Now().Add(time.Millisecond * time.Duration(args[3].Int())), nil
	default:
		return time.Time{}, fmt.Errorf("unknown option: %s", option)
	}
}
