package resp

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
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

// readRESP : use a reader to return a RESP
func ReadRESP(reader *bufio.Reader) (RESP, error) {
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
			items[i], err = ReadRESP(reader)
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
