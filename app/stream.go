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
	default:
		conn.Write(resp.EncodeSimpleString("undefined"))
	}

	return nil
}
