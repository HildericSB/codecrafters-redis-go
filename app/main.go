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
	values map[string]Entry
}

type Entry struct {
	val            string
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
		values: make(map[string]Entry),
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
	cmd := strings.ToUpper(string(r.Items[0].Data))

	switch cmd {
	case "ECHO":
		conn.Write(resp.EncodeBulkString(string(r.Items[1].Data)))

	case "PING":
		conn.Write(resp.EncodeSimpleString("PONG"))

	case "SET":
		if r.Count < 2 {
			return fmt.Errorf("SET cmd requires at least 2 parameters")
		}
		key := r.Items[1].String()
		value := r.Items[2].String()
		expiry := time.Time{}

		// If two other args given
		if r.Count > 3 {
			cmd2 := strings.ToUpper(r.Items[3].String())
			switch cmd2 {
			case "PX":
				if len(r.Items) < 4 {
					return fmt.Errorf("PX cmd requires a value")
				}
				expiry = time.Now().Add(time.Millisecond * time.Duration(r.Items[4].Int()))
			}
		}

		redisval := Entry{
			val:            value,
			expirationDate: expiry,
		}

		s.values[key] = redisval
		conn.Write(resp.EncodeSimpleString("OK"))

	case "GET":
		if len(r.Items) < 1 {
			return fmt.Errorf("GET cmd requires at least 1 parameter")
		}
		key := r.Items[1].String()
		value := s.values[key]

		if !value.expirationDate.IsZero() && value.expirationDate.Before(time.Now()) {
			//fmt.Printf("expired, now %v, expiration date: %v", time.Now(), value.expirationDate)
			conn.Write(resp.EncodeBulkString(""))
			return nil
		}

		fmt.Printf("key : %v , value: %v", key, value.val)
		conn.Write(resp.EncodeBulkString(value.val))
	default:
		return fmt.Errorf("Unknown cmd : %v", cmd)
	}

	return nil
}
