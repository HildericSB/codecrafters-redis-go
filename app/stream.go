package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/codecrafters-io/redis-starter-go/app/resp"
)

func (s *Server) handleType(args []resp.RESP) ([]byte, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("TYPE cmd requires at leat 1 argument")
	}

	key := args[0].String()
	entry := s.entries[key]

	if entry == nil {
		return resp.EncodeSimpleString("none"), nil
	}

	switch entry.val.(type) {
	case string:
		return resp.EncodeSimpleString("string"), nil
	case *stream:
		return resp.EncodeSimpleString("stream"), nil
	default:
		return resp.EncodeSimpleString("undefined"), nil
	}
}

// stream keeps entries in ascending id order
type stream struct {
	entries []streamEntry
}

type streamEntry struct {
	id     streamID
	fields []kv
}

type streamID struct {
	msTime    int
	seqNumber int
}

type kv struct {
	k string
	v string
}

func (s *Server) handleXADD(args []resp.RESP) ([]byte, error) {
	if len(args) < 4 {
		return nil, fmt.Errorf("XADD cmd requires at leat 4 argument")
	}

	if (len(args)-2)%2 != 0 {
		return nil, fmt.Errorf("XADD cmd requires arguments in pair")
	}

	key := args[0].String()
	id := args[1].String()
	entry := s.entries[key]
	var st *stream

	if entry != nil {
		v, ok := entry.val.(*stream)
		if !ok {
			return nil, fmt.Errorf("can't XADD into something else than a stream")
		}
		st = v
	} else {
		st = &stream{}
		entry = &Entry{val: st}
		s.entries[key] = entry
	}

	// Verify id
	if id == "0-0" {
		return nil, fmt.Errorf("ERR The ID specified in XADD must be greater than 0-0")
	}

	newStreamID, err := stringToStreamID(id)
	if err != nil {
		return nil, fmt.Errorf("ERR Parsing new stream id")
	}

	if len(st.entries) >= 1 {
		last := st.entries[len(st.entries)-1]
		if last.id.msTime > newStreamID.msTime || (last.id.msTime == newStreamID.msTime && last.id.seqNumber >= newStreamID.seqNumber) {
			return nil, fmt.Errorf("ERR The ID specified in XADD is equal or smaller than the target stream top item")
		}
	}

	se := streamEntry{id: *newStreamID, fields: []kv{}}
	for i := 2; i < len(args); i += 2 {
		k := args[i].String()
		v := args[i+1].String()

		se.fields = append(se.fields, kv{k: k, v: v})
	}

	st.entries = append(st.entries, se)

	return resp.EncodeBulkString(&id), nil
}

func stringToStreamID(id string) (*streamID, error) {
	split := strings.Split(id, "-")
	if len(split) != 2 {
		return nil, fmt.Errorf("error splitting id %s", split)
	}

	msTime, err := strconv.Atoi(split[0])
	if err != nil {
		return nil, fmt.Errorf("error converting msTime %s", split[0])
	}

	seqNumber, err := strconv.Atoi(split[1])
	if err != nil {
		return nil, fmt.Errorf("error converting seqNumber %s", split[1])
	}

	return &streamID{msTime: msTime, seqNumber: seqNumber}, nil
}
