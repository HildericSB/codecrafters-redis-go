package main

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/codecrafters-io/redis-starter-go/app/resp"
)

func (s *Server) handleEcho(args []resp.RESP) ([]byte, error) {
	return resp.EncodeBulkString(resp.Ptr(string(args[0].Data))), nil
}

func (s *Server) handlePing() ([]byte, error) {
	return resp.EncodeSimpleString("PONG"), nil
}

func (s *Server) handleSet(args []resp.RESP) ([]byte, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("SET cmd requires at least 2 arguments")
	}
	key := args[0].String()
	val := args[1].String()

	expiry, err := parseExpiry(args)
	if err != nil {
		return nil, err
	}

	s.entries[key] = &Entry{val: val, expirationDate: expiry}
	return resp.EncodeSimpleString("OK"), nil
}

func (s *Server) handleGet(args []resp.RESP) ([]byte, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("GET cmd requires at least 1 argument")
	}

	key := args[0].String()
	entry := s.entries[key]

	if entry == nil {
		return []byte("$-1\r\n"), nil
	}

	value, ok := entry.val.(string)
	if !ok {
		return nil, fmt.Errorf("GET not supported for this type of entry : %T", entry.val)
	}

	if !entry.expirationDate.IsZero() && entry.expirationDate.Before(time.Now()) {
		delete(s.entries, key)
		return []byte("$-1\r\n"), nil
	}

	return resp.EncodeBulkString(&value), nil
}

func (s *Server) handleRpush(args []resp.RESP) ([]byte, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("RPUSH cmd requires at least 2 arguments")
	}

	key := args[0].String()
	entry := s.entries[key]

	if entry == nil {
		entry = &Entry{val: []string{}}
		s.entries[key] = entry
	}

	values, ok := entry.val.([]string)
	if !ok {
		return nil, fmt.Errorf("entry with key %s is not a list", key)
	}

	for _, item := range args[1:] {
		values = append(values, item.String())
	}

	entry.val = values
	s.entries[key] = entry

	s.mu.Lock()
	if len(s.waiters[key]) != 0 {
		val := values[0]
		entry.val = values[1:]
		s.waiters[key][0] <- val
		s.waiters[key] = s.waiters[key][1:]

	}
	s.mu.Unlock()

	return resp.EncodeInteger(len(values)), nil
}

func (s *Server) handleLpush(args []resp.RESP) ([]byte, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("LPUSH cmd requires at least 2 arguments")
	}

	key := args[0].String()
	entry := s.entries[key]

	if entry == nil {
		entry = &Entry{val: []string{}}
		s.entries[key] = entry
	}

	values, ok := entry.val.([]string)
	if !ok {
		return nil, fmt.Errorf("entry with key %s is not a list", key)
	}

	for _, item := range args[1:] {
		values = append([]string{item.String()}, values...)
	}

	entry.val = values
	s.entries[key] = entry
	return resp.EncodeInteger(len(values)), nil
}

func (s *Server) handleLrange(args []resp.RESP) ([]byte, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("LRANGE cmd requires at least 3 arguments")
	}

	key := args[0].String()
	entry := s.entries[key]
	startIndex := args[1].Int()
	endIndex := args[2].Int()

	if entry == nil {
		return []byte("*0\r\n"), nil
	}

	values, ok := entry.val.([]string)
	if !ok {
		return nil, fmt.Errorf("entry with key %s is not a list", key)
	}

	if startIndex < 0 {
		startIndex = max(startIndex+len(values), 0)
	}
	if endIndex < 0 {
		endIndex = max(endIndex+len(values), 0)
	}
	res := []string{}
	for i := startIndex; i < len(values) && i <= endIndex; i++ {
		res = append(res, values[i])
	}
	return resp.EncodeArray(res), nil
}

func (s *Server) handleLlen(args []resp.RESP) ([]byte, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("LLEN cmd requires at least 1 parameters")
	}
	key := args[0].String()
	entry := s.entries[key]

	if entry == nil {
		return resp.EncodeInteger(0), nil
	}

	values, ok := entry.val.([]string)
	if !ok {
		return nil, fmt.Errorf("entry with key %s is not a list", key)
	}

	return resp.EncodeInteger(len(values)), nil
}

func (s *Server) handleLpop(args []resp.RESP) ([]byte, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("LPOP cmd requires at least 1 parameters")
	}

	key := args[0].String()
	entry := s.entries[key]

	if entry == nil {
		return []byte("$-1\r\n"), nil
	}

	values, ok := entry.val.([]string)
	if !ok {
		return nil, fmt.Errorf("entry with key %s is not a list", key)
	}

	withCount := len(args) == 2
	endIndex := 1
	if withCount {
		endIndex = min(args[1].Int(), len(values))
	}

	elems := values[0:endIndex]
	entry.val = values[endIndex:]

	if len(entry.val.([]string)) == 0 {
		delete(s.entries, key)
	}

	if !withCount {
		return resp.EncodeBulkString(&elems[0]), nil
	}
	return resp.EncodeArray(elems), nil
}

func (s *Server) handleBLPOP(args []resp.RESP) ([]byte, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("BLPOP cmd requires at least 2 parameters")
	}

	s.mu.Lock()

	key := args[0].String()
	entry := s.entries[key]
	timeoutSecs := args[1].Float()

	if entry != nil {
		values, ok := entry.val.([]string)
		if !ok {
			s.mu.Unlock()
			return nil, fmt.Errorf("entry with key %s is not a list", key)
		}

		if len(values) > 0 {
			elem := values[0]
			entry.val = values[1:]
			s.mu.Unlock()
			return resp.EncodeArray([]string{key, elem}), nil
		}

	}

	ch := make(chan string, 1)
	s.waiters[key] = append(s.waiters[key], ch)
	s.mu.Unlock()

	var timeoutCh <-chan time.Time
	if timeoutSecs > 0 {
		timer := time.NewTimer(time.Duration(timeoutSecs * float64(time.Second)))
		defer timer.Stop()
		timeoutCh = timer.C
	}

	select {
	case val := <-ch:
		return resp.EncodeArray([]string{key, val}), nil
	case <-timeoutCh:
		s.mu.Lock()
		idx := slices.Index(s.waiters[key], ch)
		if idx != -1 {
			// still waiting — genuinely timed out, nobody sent anything
			s.waiters[key] = slices.Delete(s.waiters[key], idx, idx+1)
			s.mu.Unlock()
			return resp.EncodeArray(nil), nil
		}
		// a pusher already claimed us right as the timer fired
		s.mu.Unlock()
		val := <-ch
		return resp.EncodeArray([]string{key, val}), nil
	}
}

func parseExpiry(args []resp.RESP) (time.Time, error) {
	if len(args) <= 2 {
		return time.Time{}, nil
	}

	option := strings.ToUpper(args[2].String())
	switch option {
	case "PX":
		if len(args) < 4 {
			return time.Time{}, fmt.Errorf("PX requires a value")
		}
		return time.Now().Add(time.Millisecond * time.Duration(args[3].Int())), nil
	default:
		return time.Time{}, fmt.Errorf("unknown option: %s", option)
	}
}
