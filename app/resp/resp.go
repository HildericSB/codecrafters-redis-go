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
	TypeInteger Type = ':'
	TypeString  Type = '+' // Simple string erminated by CRLF
	TypeBulk    Type = '$' // A bulk string represents a single binary string. $<length>\r\n<data>\r\n
	TypeArray   Type = '*'
	TypeError   Type = '-'
)

// Value is anything that can be written as RESP
type Value interface {
	Encode() []byte
}

type SimpleString string
type SimpleError string
type BulkString string
type Integer int
type Array []Value

// NullBulk encodes as a null bulk string ($-1)
type NullBulk struct{}

// NullArray encodes as a null array (*-1)
type NullArray struct{}

func (s SimpleString) Encode() []byte {
	return fmt.Appendf(nil, "+%s\r\n", s)
}

func (e SimpleError) Encode() []byte {
	return fmt.Appendf(nil, "-%s\r\n", e)
}

func (s BulkString) Encode() []byte {
	return fmt.Appendf(nil, "$%d\r\n%s\r\n", len(s), s)
}

func (i Integer) Encode() []byte {
	return fmt.Appendf(nil, ":%d\r\n", i)
}

func (a Array) Encode() []byte {
	buf := fmt.Appendf(nil, "*%d\r\n", len(a))
	for _, e := range a {
		buf = append(buf, e.Encode()...)
	}
	return buf
}

func (NullBulk) Encode() []byte {
	return []byte("$-1\r\n")
}

func (NullArray) Encode() []byte {
	return []byte("*-1\r\n")
}

// BulkStrings wraps each string as a BulkString
func BulkStrings(ss []string) Array {
	a := make(Array, 0, len(ss))
	for _, s := range ss {
		a = append(a, BulkString(s))
	}
	return a
}

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

func ReadValue(r *bufio.Reader) (Value, error) {
	line, _, err := r.ReadLine() // readline remove crlf
	if err != nil {
		return nil, err
	}

	if len(line) == 0 {
		return nil, fmt.Errorf("empty line")
	}

	switch line[0] {
	case '+':
		return SimpleString(line[1:]), nil
	case '$':
		n, err := strconv.Atoi(string(line[1:]))
		if err != nil {
			return nil, fmt.Errorf("cant read bulk string size : %w", err)
		}
		if n == -1 {
			return NullBulk{}, nil
		}
		if n < -1 {
			return nil, fmt.Errorf("invalid size: %d", n)
		}
		buff := make([]byte, n+2)
		if _, err := io.ReadFull(r, buff); err != nil {
			return nil, err
		}
		return BulkString(buff[:n]), nil
	case ':':
		r, err := strconv.Atoi(string(line[1:]))
		if err != nil {
			return nil, fmt.Errorf("invalid integer: %w", err)
		}
		return Integer(r), nil
	case '-':
		return SimpleError(line[1:]), nil
	case '*':
		size, err := strconv.Atoi(string(line[1:]))
		res := make([]Value, 0, size)
		if err != nil {
			return nil, fmt.Errorf("invalid array size: %w", err)
		}
		for i := 0; i < int(size); i++ {
			v, err := ReadValue(r)
			if err != nil {
				return nil, err
			}
			res = append(res, v)
		}
		return Array(res), nil
	}

	return nil, nil
}

func ReadCommand(reader *bufio.Reader) ([]string, error) {
	value, err := ReadValue(reader)
	if err != nil {
		return nil, err
	}

	cmds, ok := value.(Array)

	if !ok {
		return nil, fmt.Errorf("command should be an array, got %T", value)
	}

	if len(cmds) == 0 {
		return nil, fmt.Errorf("command should have at least one element")
	}

	res := make([]string, 0, len(cmds))
	for _, c := range cmds {
		s, ok := c.(BulkString)
		if !ok {
			return nil, fmt.Errorf("command args should be a bulk string, got %T", c)
		}
		res = append(res, string(s))
	}

	return res, nil
}
