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
	return string(r.Data)
}

func (r RESP) Int() int {
	x, _ := strconv.ParseInt(r.String(), 10, 64)
	return int(x)
}

func (r RESP) Float() float64 {
	x, _ := strconv.ParseFloat(r.String(), 10)
	return x
}

func (r RESP) Bytes() []byte {
	return r.Data
}

func Ptr(s string) *string { return &s }

func EncodeSimpleString(str string) []byte {
	return fmt.Appendf(nil, "+%s\r\n", str)
}

func EncodeInteger(val int) []byte {
	return fmt.Appendf(nil, ":%d\r\n", val)
}

func EncodeBulkString(str *string) []byte {
	if str == nil {
		return []byte("$-1\r\n")
	}
	return fmt.Appendf(nil, "$%d\r\n%s\r\n", len(*str), *str)
}

func EncodeArray(items []string) []byte {
	if items == nil {
		return []byte("*-1\r\n")
	}
	res := "*" + strconv.Itoa(len(items)) + "\r\n"
	for _, s := range items {
		res += string(EncodeBulkString(Ptr(s)))
	}
	return []byte(res)
}

func EncodeRESPArray(array []RESP) []byte {
	res := "*" + strconv.Itoa(len(array)) + "\r\n"
	for _, resp := range array {
		switch resp.Type {
		case String:
			res += string(EncodeSimpleString(resp.String()))
		case Integer:
			res += string(EncodeInteger(resp.Int()))
		case Bulk:
			res += string(EncodeBulkString(Ptr(resp.String())))
		case Array:
			res += string(EncodeRESPArray(resp.Items))
		}
	}
	return []byte(res)
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
