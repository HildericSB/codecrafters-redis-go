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
	case map[string][]kv:
		conn.Write(resp.EncodeSimpleString("stream"))
	default:
		conn.Write(resp.EncodeSimpleString("undefined"))
	}

	return nil
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
	var streams map[string][]kv

	if entry != nil {
		if v, ok := entry.val.(map[string][]kv); ok {
			streams = v
		} else {
			return fmt.Errorf("can't XADD into something else than a stream")
		}
	} else {
		streams = make(map[string][]kv)
		entry = &Entry{val: streams}
		s.entries[key] = entry
	}

	for i := 2; i < len(args); i += 2 {
		field := args[i].String()
		value := args[i+1].String()
		streams[id] = append(streams[id], kv{k: field, v: value})
	}

	conn.Write(resp.EncodeBulkString(&id))

	return nil
}
