package main

import (
	"fmt"
	"net"

	"github.com/codecrafters-io/redis-starter-go/app/resp"
)

func (s *Server) handleType(args []resp.RESP, conn net.Conn) error {
	if len(args) < 1 {
		return fmt.Errorf("TYPE cmd requires at leat 1 argument")
	}

	key := args[0].String()
	entry := s.entries[key]

	if entry == nil {
		conn.Write(resp.EncodeSimpleString("none"))
		return nil
	}

	switch entry.val.(type) {
	case string:
		conn.Write(resp.EncodeSimpleString("string"))
	case *stream:
		conn.Write(resp.EncodeSimpleString("stream"))
	default:
		conn.Write(resp.EncodeSimpleString("undefined"))
	}

	return nil
}

// stream keeps entries in ascending id order
type stream struct {
	entries []streamEntry
}

type streamEntry struct {
	id     string
	fields []kv
}

type kv struct {
	k string
	v string
}

func (s *Server) handleXADD(args []resp.RESP, conn net.Conn) error {
	if len(args) < 4 {
		return fmt.Errorf("XADD cmd requires at leat 4 argument")
	}

	if (len(args)-2)%2 != 0 {
		return fmt.Errorf("XADD cmd requires arguments in pair")
	}

	key := args[0].String()
	id := args[1].String()
	entry := s.entries[key]
	var st *stream

	if entry != nil {
		if v, ok := entry.val.(*stream); ok {
			st = v
		} else {
			return fmt.Errorf("can't XADD into something else than a stream")
		}
	} else {
		st = &stream{}
		entry = &Entry{val: st}
		s.entries[key] = entry
	}

	se := streamEntry{id: id, fields: []kv{}}
	for i := 2; i < len(args); i += 2 {
		k := args[i].String()
		v := args[i+1].String()

		se.fields = append(se.fields, kv{k: k, v: v})
	}

	st.entries = append(st.entries, se)

	conn.Write(resp.EncodeBulkString(&id))

	return nil
}
