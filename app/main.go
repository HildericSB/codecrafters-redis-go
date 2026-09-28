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
		args, err := resp.ReadCommand(reader)
		if err != nil {
			// Client send EOF when finished
			if err != io.EOF {
				fmt.Println("error reading RESP command :", err)
			}
			return
		}

		fmt.Print(args)

		out, err := s.handleCmd(args)
		if err != nil {
			fmt.Println("handling command failed : ", err)
			conn.Write(resp.SimpleError("ERR " + err.Error()).Encode())
			continue
		}

		conn.Write(out)
	}

}

func (s *Server) handleCmd(cmd []string) ([]byte, error) {
	name := strings.ToUpper(cmd[0])
	cmdArgs := cmd[1:]

	switch name {
	case "ECHO":
		return s.handleEcho(cmdArgs)
	case "PING":
		return s.handlePing()
	case "SET":
		return s.handleSet(cmdArgs)
	case "GET":
		return s.handleGet(cmdArgs)
	case "RPUSH":
		return s.handleRpush(cmdArgs)
	case "LRANGE":
		return s.handleLrange(cmdArgs)
	case "LPUSH":
		return s.handleLpush(cmdArgs)
	case "LLEN":
		return s.handleLlen(cmdArgs)
	case "LPOP":
		return s.handleLpop(cmdArgs)
	case "BLPOP":
		return s.handleBLPOP(cmdArgs)
	case "TYPE":
		return s.handleType(cmdArgs)
	case "XADD":
		return s.handleXADD(cmdArgs)
	case "XRANGE":
		return s.handleXRANGE(cmdArgs)
	default:
		return nil, fmt.Errorf("Unknown cmd : %v", cmd)
	}
}
