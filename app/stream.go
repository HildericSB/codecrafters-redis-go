package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

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

func (s streamID) String() string {
	return fmt.Sprintf("%d-%d", s.msTime, s.seqNumber)
}

// Compare returns -1 if id < other, 0 if equal, +1 if id > other.
func (s streamID) Compare(other streamID) int {
	// compare msTime first
	if s.msTime < other.msTime {
		return -1
	}
	if s.msTime > other.msTime {
		return 1
	}
	if s.seqNumber < other.seqNumber {
		return -1
	}
	if s.seqNumber > other.seqNumber {
		return 1
	}
	return 0
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
	}

	// Verify id
	if id == "0-0" {
		return nil, fmt.Errorf("The ID specified in XADD must be greater than 0-0")
	}

	var last *streamID
	if len(st.entries) >= 1 {
		last = &st.entries[len(st.entries)-1].id
	}

	newStreamID, err := stringToStreamID(id, last)
	if err != nil {
		return nil, fmt.Errorf("Parsing new stream id, %w", err)
	}

	if last != nil && (last.msTime > newStreamID.msTime || (last.msTime == newStreamID.msTime && last.seqNumber >= newStreamID.seqNumber)) {
		return nil, fmt.Errorf("The ID specified in XADD is equal or smaller than the target stream top item")
	}

	se := streamEntry{id: newStreamID, fields: []kv{}}
	for i := 2; i < len(args); i += 2 {
		k := args[i].String()
		v := args[i+1].String()

		se.fields = append(se.fields, kv{k: k, v: v})
	}

	st.entries = append(st.entries, se)

	s.entries[key] = entry

	idStr := se.id.String()
	return resp.EncodeBulkString(&idStr), nil
}

func stringToStreamID(id string, last *streamID) (streamID, error) {
	if id == "*" {
		msTime := int(time.Now().UnixMilli())
		if last != nil {
			msTime = max(msTime, last.msTime)
		}
		return autoSeq(msTime, last), nil
	}

	split := strings.Split(id, "-")
	if len(split) != 2 {
		return streamID{}, fmt.Errorf("error splitting id %q", id)
	}

	msTime, err := strconv.Atoi(split[0])
	if err != nil {
		return streamID{}, fmt.Errorf("error converting msTime %s", split[0])
	}

	if split[1] == "*" {
		return autoSeq(msTime, last), nil
	}

	seqNumber, err := strconv.Atoi(split[1])
	if err != nil {
		return streamID{}, fmt.Errorf("error converting seqNumber %q: %w", split[1], err)
	}

	return streamID{msTime: msTime, seqNumber: seqNumber}, nil
}

func autoSeq(msTime int, last *streamID) streamID {
	switch {
	case last != nil && last.msTime == msTime:
		return streamID{msTime: msTime, seqNumber: last.seqNumber + 1}
	case msTime == 0:
		return streamID{msTime: msTime, seqNumber: 1}
	default:
		return streamID{msTime: msTime, seqNumber: 0}
	}
}

func (s *Server) handleXRANGE(args []resp.RESP) ([]byte, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("XRANGE cmd requires at leat 3 argument")
	}

	key := args[0].String()
	startID, err := stringToStreamID(args[1].String(), nil)
	if err != nil {
		return nil, fmt.Errorf("error parsing streamID 1")
	}
	endID, err := stringToStreamID(args[2].String(), nil)
	if err != nil {
		return nil, fmt.Errorf("error parsing streamID 2")
	}

	entry := s.entries[key]

	if entry == nil {
		return nil, fmt.Errorf("entry can't be null")
	}

	var st *stream
	var ok bool
	if st, ok = entry.val.(*stream); !ok {
		return nil, fmt.Errorf("entry must be a *stream")
	}

	var selected []streamEntry
	for _, e := range st.entries {
		if e.id.Compare(startID) == -1 {
			continue
		}
		if e.id.Compare(endID) == 1 {
			break
		}
		selected = append(selected, e)
	}

	entries := make([]resp.RESP, len(selected))
	for i, e := range selected {
		idStr := e.id.String()

		fieldsItems := make([]resp.RESP, 0, len(e.fields)*2)
		for _, f := range e.fields {
			fieldsItems = append(fieldsItems,
				resp.RESP{Type: resp.Bulk, Data: []byte(f.k)},
				resp.RESP{Type: resp.Bulk, Data: []byte(f.v)},
			)
		}

		entries[i] = resp.RESP{
			Type: resp.Array,
			Items: []resp.RESP{
				{Type: resp.Bulk, Data: []byte(idStr)},
				{Type: resp.Array, Items: fieldsItems},
			},
		}
	}

	return resp.EncodeRESPArray(entries), nil

}
