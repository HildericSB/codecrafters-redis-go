package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/codecrafters-io/redis-starter-go/app/resp"
)

func (s *Server) handleType(args []string) ([]byte, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("TYPE cmd requires at leat 1 argument")
	}

	key := args[0]
	entry := s.entries[key]

	if entry == nil {
		return resp.SimpleString("none").Encode(), nil
	}

	switch entry.val.(type) {
	case string:
		return resp.SimpleString("string").Encode(), nil
	case *stream:
		return resp.SimpleString("stream").Encode(), nil
	default:
		return resp.SimpleString("undefined").Encode(), nil
	}
}

// stream keeps entries in ascending id order
type stream struct {
	entries []streamEntry
}

func (st *stream) after(id streamID) []streamEntry {
	for i, e := range st.entries {
		if e.id.Compare(id) == 1 {
			return st.entries[i:]
		}
	}
	return nil
}

// between returns the entries with start <= id <= end.
func (st *stream) between(start, end streamID) []streamEntry {
	var selected []streamEntry
	for _, e := range st.entries {
		if e.id.Compare(start) < 0 {
			continue
		}
		if e.id.Compare(end) > 0 {
			break
		}
		selected = append(selected, e)
	}
	return selected
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

func (s *Server) handleXADD(args []string) ([]byte, error) {
	if len(args) < 4 {
		return nil, fmt.Errorf("XADD cmd requires at leat 4 argument")
	}

	if (len(args)-2)%2 != 0 {
		return nil, fmt.Errorf("XADD cmd requires arguments in pair")
	}

	key := args[0]
	id := args[1]
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

	var last *streamID
	if len(st.entries) >= 1 {
		last = &st.entries[len(st.entries)-1].id
	}

	newStreamID, err := nextStreamID(id, last)

	if err != nil {
		return nil, err
	}

	if newStreamID == (streamID{}) {
		return nil, fmt.Errorf("The ID specified in XADD must be greater than 0-0")
	}

	if last != nil && newStreamID.Compare(*last) <= 0 {
		return nil, fmt.Errorf("The ID specified in XADD is equal or smaller than the target stream top item")

	}

	se := streamEntry{id: newStreamID, fields: []kv{}}
	for i := 2; i < len(args); i += 2 {
		k := args[i]
		v := args[i+1]

		se.fields = append(se.fields, kv{k: k, v: v})
	}

	st.entries = append(st.entries, se)

	s.entries[key] = entry

	return resp.BulkString(se.id.String()).Encode(), nil
}

// nextStreamID turns an XADD id argument ("*", "<ms>-*" or "<ms>-<seq>")
// into a concrete streamID, using last to generate sequence numbers.
func nextStreamID(id string, last *streamID) (streamID, error) {
	if id == "*" {
		msTime := int(time.Now().UnixMilli())
		if last != nil {
			msTime = max(msTime, last.msTime)
		}
		return autoSeq(msTime, last), nil
	}

	msStr, seqStr, hasSeq := strings.Cut(id, "-")
	if !hasSeq {
		return streamID{}, fmt.Errorf("invalid stream ID %q", id)
	}

	msTime, err := strconv.Atoi(msStr)
	if err != nil {
		return streamID{}, fmt.Errorf("invalid ms time %q: %w", msStr, err)
	}

	if seqStr == "*" {
		return autoSeq(msTime, last), nil
	}

	seqNumber, err := strconv.Atoi(seqStr)
	if err != nil {
		return streamID{}, fmt.Errorf("invalid sequence number %q: %w", seqStr, err)
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

func (s *Server) handleXRANGE(args []string) ([]byte, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("XRANGE cmd requires at leat 3 argument")
	}

	key := args[0]

	startID, err := parseRangeID(args[1])
	if err != nil {
		return nil, fmt.Errorf("invalid start ID: %w", err)
	}
	endID, err := parseRangeID(args[2])
	if err != nil {
		return nil, fmt.Errorf("invalid end ID: %w", err)
	}

	entry := s.entries[key]
	if entry == nil {
		return resp.Array{}.Encode(), nil
	}

	st, ok := entry.val.(*stream)
	if !ok {
		return nil, fmt.Errorf("entry must be a *stream")
	}

	return encodeEntries(st.between(startID, endID)).Encode(), nil
}

// parseStreamID parses a literal "<ms>-<seq>" stream ID.
func parseStreamID(s string) (streamID, error) {
	msStr, seqStr, hasSeq := strings.Cut(s, "-")
	if !hasSeq {
		return streamID{}, fmt.Errorf("malformed stream ID : %q", s)
	}

	ms, err := strconv.Atoi(msStr)
	if err != nil {
		return streamID{}, fmt.Errorf("can't parse ms : %q : %w", msStr, err)
	}

	seq, err := strconv.Atoi(seqStr)
	if err != nil {
		return streamID{}, fmt.Errorf("can't parse seq : %q : %w", seqStr, err)
	}

	return streamID{msTime: ms, seqNumber: seq}, nil
}

// parseRangeID parses an XRANGE bound: "-", "+", or a stream ID.
func parseRangeID(s string) (streamID, error) {
	switch s {
	case "-":
		return streamID{}, nil
	case "+":
		return streamID{msTime: math.MaxInt, seqNumber: math.MaxInt}, nil
	}
	return parseStreamID(s)
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

	startIDs := make([]streamID, 0, len(ids))
	for _, id := range ids {
		startID, err := parseStreamID(id)
		if err != nil {
			return nil, fmt.Errorf("invalid ID %q: %w", id, err)
		}
		startIDs = append(startIDs, startID)
	}

	var streams resp.Array
	for i, key := range keys {
		entry := s.entries[key]
		if entry == nil {
			continue
		}

		st, ok := entry.val.(*stream)
		if !ok {
			return nil, fmt.Errorf("WRONGTYPE Operation against a key holding the wrong kind of value")
		}

		selected := st.after(startIDs[i])
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

func encodeEntries(entries []streamEntry) resp.Array {
	res := make(resp.Array, 0, len(entries))
	for _, e := range entries {
		fieldItems := resp.Array{}
		for _, f := range e.fields {
			fieldItems = append(fieldItems, resp.BulkString(f.k), resp.BulkString(f.v))
		}

		res = append(res, resp.Array{resp.BulkString(e.id.String()), fieldItems})
	}

	return res
}
