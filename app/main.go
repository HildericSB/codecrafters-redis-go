package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"strings"

	"github.com/codecrafters-io/redis-starter-go/app/resp"
)

var values map[string]string

func main() {
	l, err := net.Listen("tcp", "0.0.0.0:6379")
	if err != nil {
		fmt.Println("Failed to bind to port 6379")
		os.Exit(1)
	}
	defer l.Close()

	values = make(map[string]string)

	for {
		conn, err := l.Accept()
		if err != nil {
			fmt.Println("Error accepting connection: ", err.Error())
			os.Exit(1)
		}

		go handleConnection(conn)
	}

}

func handleConnection(conn net.Conn) {
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
			handleCmd(val, conn)
		}
	}

}

func handleCmd(r resp.RESP, conn net.Conn) error {
	cmd := strings.ToUpper(string(r.Items[0].Data))

	switch cmd {
	case "ECHO":
		conn.Write(fmt.Appendf(nil, "$%d\r\n%s\r\n", len(r.Items[1].Data), r.Items[1].Data))

	case "PING":
		conn.Write([]byte("+PONG\r\n"))

	case "SET":
		if len(r.Items) < 2 {
			return fmt.Errorf("SET cmd requires 2 parameters")
		}
		key := string(r.Items[1].Data)

		value := string(r.Items[2].Data)
		values[key] = value

		conn.Write([]byte("+OK\r\n"))
	case "GET":
		if len(r.Items) < 1 {
			return fmt.Errorf("GET cmd requires 1 parameter")
		}
		key := string(r.Items[1].Data)
		value := values[key]
		conn.Write(fmt.Appendf(nil, "$%d\r\n%s\r\n", len(value), value))

	default:
		return fmt.Errorf("Unknown cmd : %v", cmd)
	}

	return nil
}
