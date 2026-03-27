package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
)

// Various RESP kinds
type Type byte

const (
	Integer Type = ':'
	String  Type = '+' // Simple string erminated by CRLF
	Bulk    Type = '$' // A bulk string represents a single binary string. $<length>\r\n<data>\r\n
	Array   Type = '*'
	Error   Type = '-'
)

type RESP struct {
	Type  Type
	Data  []byte
	Items []RESP
	Count int
}

func (r RESP) String() string {
	if r.Type == Array {
		var res string
		for _, e := range r.Items {
			res = "[array]\n" + res + e.String() + "\n"
		}
		return res
	}

	return string(r.Data)
}

// Ensures gofmt doesn't remove the "net" and "os" imports in stage 1 (feel free to remove this!)
var _ = net.Listen
var _ = os.Exit

func main() {
	l, err := net.Listen("tcp", "0.0.0.0:6379")
	if err != nil {
		fmt.Println("Failed to bind to port 6379")
		os.Exit(1)
	}
	defer l.Close()

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
		resp, err := readRESP(reader)
		if err != nil {
			// Client send EOF when finished
			if err != io.EOF {
				fmt.Println("error reading RESP:", err)
			}
			return
		}

		fmt.Print(resp)

		// if resp.Type == String || resp.Type == Bulk {
		// 	cmd := strings.ToUpper(string(resp.Data))
		// 	if cmd == "PING" {
		// 		conn.Write([]byte("+PONG\r\n"))
		// 	}
		// }

		if resp.Type == Array {
			cmd := strings.ToUpper(string(resp.Items[0].Data))
			if cmd == "ECHO" {
				conn.Write(fmt.Appendf(nil, "$%d\r\n%s\r\n", len(resp.Items[1].Data), resp.Items[1].Data))
			}

			if cmd == "PING" {
				conn.Write([]byte("+PONG\r\n"))
			}
		}
	}

}

// *2\r\n$4\r\nECHO\r\n$3\r\nhey\r\n
// readRESP : use a reader to return a RESP
func readRESP(reader *bufio.Reader) (RESP, error) {
	line, _, err := reader.ReadLine() // readline remove crlf

	// fmt.Printf("Reading line : %s \n", line)

	if err != nil {
		return RESP{}, err
	}

	t := Type(line[0])
	data := line[1:]

	switch t {
	case String:
		return RESP{Type: t, Data: data}, nil

	case Array:
		count, err := strconv.Atoi(string(data))
		if err != nil {
			return RESP{}, fmt.Errorf("invalid array count: %w", err)
		}
		items := make([]RESP, count)
		for i := range items {
			items[i], err = readRESP(reader)
			if err != nil {
				return RESP{}, err
			}
		}
		return RESP{Type: t, Count: count, Items: items}, nil
	case Bulk:
		n, err := strconv.Atoi(string(data))
		if err != nil {
			return RESP{}, fmt.Errorf("invalid bulk length: %w", err)
		}
		// n bytes of data + \r\n
		buff := make([]byte, n+2)
		if _, err := io.ReadFull(reader, buff); err != nil {
			return RESP{}, err
		}
		return RESP{Type: t, Data: buff[:n]}, nil

	default:
		return RESP{}, fmt.Errorf("Unknown resp type")
	}
}
