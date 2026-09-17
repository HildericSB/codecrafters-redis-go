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
			err = s.handleCmd(val, conn)
			if err != nil {
				fmt.Println("handling connection failed : %w", err)
			}
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
