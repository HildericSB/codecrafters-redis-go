package main

import (
	"fmt"
	"strings"

	"github.com/codecrafters-io/redis-starter-go/app/resp"
	"github.com/codecrafters-io/redis-starter-go/app/stream"
)

// getStream returns the stream stored at key, nil if the key doesn't exist,
// or an error if the key holds another type.
func (s *Server) getStream(key string) (*stream.Stream, error) {
	entry := s.entries[key]
	if entry == nil {
		return nil, nil
	}

	st, ok := entry.val.(*stream.Stream)
	if !ok {
		return nil, fmt.Errorf("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
	return st, nil
}

func (s *Server) handleXADD(args []string) ([]byte, error) {
	if len(args) < 4 {
		return nil, fmt.Errorf("XADD cmd requires at leat 4 argument")
	}

	if (len(args)-2)%2 != 0 {
		return nil, fmt.Errorf("XADD cmd requires arguments in pair")
	}

	key := args[0]
	st, err := s.getStream(key)
	if err != nil {
		return nil, err
	}

	fields := make([]stream.Field, 0, (len(args)-2)/2)
	for i := 2; i < len(args); i += 2 {
		fields = append(fields, stream.Field{Key: args[i], Value: args[i+1]})
	}

	isNew := st == nil
	if isNew {
		st = &stream.Stream{}
	}

	id, err := st.Add(args[1], fields)
	if err != nil {
		return nil, err
	}

	// only store a new stream once its first entry is valid
	if isNew {
		s.entries[key] = &Entry{val: st}
	}

	return resp.BulkString(id.String()).Encode(), nil
}

func (s *Server) handleXRANGE(args []string) ([]byte, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("XRANGE cmd requires at leat 3 argument")
	}

	key := args[0]

	startID, err := stream.ParseRangeID(args[1])
	if err != nil {
		return nil, fmt.Errorf("invalid start ID: %w", err)
	}
	endID, err := stream.ParseRangeID(args[2])
	if err != nil {
		return nil, fmt.Errorf("invalid end ID: %w", err)
	}

	st, err := s.getStream(key)
	if err != nil {
		return nil, err
	}
	if st == nil {
		return resp.Array{}.Encode(), nil
	}

	return encodeEntries(st.Between(startID, endID)).Encode(), nil
}

func (s *Server) handleXREAD(args []string) ([]byte, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("XREAD cmd requires at leat 3 argument")
	}

	option := strings.ToUpper(args[0])
	if option != "STREAMS" {
		return nil, fmt.Errorf("only stream option supported for now")
	}

	pairs := args[1:]
	n := len(pairs)
	if n%2 != 0 {
		return nil, fmt.Errorf("Unbalanced 'xread' list of streams: for each stream key an ID must be specified.")
	}

	keys := pairs[:n/2]
	ids := pairs[n/2:]

	startIDs := make([]stream.ID, 0, len(ids))
	for _, id := range ids {
		startID, err := stream.ParseID(id)
		if err != nil {
			return nil, fmt.Errorf("invalid ID %q: %w", id, err)
		}
		startIDs = append(startIDs, startID)
	}

	var streams resp.Array
	for i, key := range keys {
		st, err := s.getStream(key)
		if err != nil {
			return nil, err
		}
		if st == nil {
			continue
		}

		selected := st.After(startIDs[i])
		if len(selected) == 0 {
			continue
		}

		streams = append(streams, resp.Array{resp.BulkString(key), encodeEntries(selected)})
	}

	if len(streams) == 0 {
		return resp.NullArray{}.Encode(), nil
	}

	return streams.Encode(), nil
}

/*
 *  Helpers
 */
func encodeEntries(entries []stream.Entry) resp.Array {
	res := make(resp.Array, 0, len(entries))
	for _, e := range entries {
		fieldItems := make(resp.Array, 0, 2*len(e.Fields))
		for _, f := range e.Fields {
			fieldItems = append(fieldItems, resp.BulkString(f.Key), resp.BulkString(f.Value))
		}

		res = append(res, resp.Array{resp.BulkString(e.ID.String()), fieldItems})
	}

	return res
}
