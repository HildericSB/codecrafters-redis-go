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
			out, err := s.handleCmd(val)
			if err != nil {
				fmt.Println("handling connection failed : ", err)
				conn.Write(resp.EncodeSimpleError("ERR " + err.Error()))
				continue
			}
			conn.Write(out)
		}
	}

}

func (s *Server) handleCmd(r resp.RESP) ([]byte, error) {
	cmd := strings.ToUpper(r.Items[0].String())
	args := r.Items[1:]

	switch cmd {
	case "ECHO":
		return s.handleEcho(args)
	case "PING":
		return s.handlePing()
	case "SET":
		return s.handleSet(args)
	case "GET":
		return s.handleGet(args)
	case "RPUSH":
		return s.handleRpush(args)
	case "LRANGE":
		return s.handleLrange(args)
	case "LPUSH":
		return s.handleLpush(args)
	case "LLEN":
		return s.handleLlen(args)
	case "LPOP":
		return s.handleLpop(args)
	case "BLPOP":
		return s.handleBLPOP(args)
	case "TYPE":
		return s.handleType(args)
	case "XADD":
		return s.handleXADD(args)
	case "XRANGE":
		return s.handleXRANGE(args)
	default:
		return nil, fmt.Errorf("Unknown cmd : %v", cmd)
	}
}
