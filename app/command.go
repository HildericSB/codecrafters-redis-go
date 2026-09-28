package main

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/codecrafters-io/redis-starter-go/app/resp"
)

func (s *Server) handleEcho(args []string) ([]byte, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("ECHO cmd requires at least 1 argument")
	}
	return resp.BulkString(args[0]).Encode(), nil
}

func (s *Server) handlePing() ([]byte, error) {
	return resp.SimpleString("PONG").Encode(), nil
}

func (s *Server) handleSet(args []string) ([]byte, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("SET cmd requires at least 2 arguments")
	}
	key := args[0]
	val := args[1]

	expiry, err := parseExpiry(args)
	if err != nil {
		return nil, err
	}

	s.entries[key] = &Entry{val: val, expirationDate: expiry}
	return resp.SimpleString("OK").Encode(), nil
}

func (s *Server) handleGet(args []string) ([]byte, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("GET cmd requires at least 1 argument")
	}

	key := args[0]
	entry := s.entries[key]

	if entry == nil {
		return resp.NullBulk{}.Encode(), nil
	}

	value, ok := entry.val.(string)
	if !ok {
		return nil, fmt.Errorf("GET not supported for this type of entry : %T", entry.val)
	}

	if !entry.expirationDate.IsZero() && entry.expirationDate.Before(time.Now()) {
		delete(s.entries, key)
		return resp.NullBulk{}.Encode(), nil
	}

	return resp.BulkString(value).Encode(), nil
}

func (s *Server) handleRpush(args []string) ([]byte, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("RPUSH cmd requires at least 2 arguments")
	}

	key := args[0]
	entry := s.entries[key]

	if entry == nil {
		entry = &Entry{val: []string{}}
		s.entries[key] = entry
	}

	values, ok := entry.val.([]string)
	if !ok {
		return nil, fmt.Errorf("entry with key %s is not a list", key)
	}

	values = append(values, args[1:]...)

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

	return resp.Integer(len(values)).Encode(), nil
}

func (s *Server) handleLpush(args []string) ([]byte, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("LPUSH cmd requires at least 2 arguments")
	}

	key := args[0]
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
		values = append([]string{item}, values...)
	}

	entry.val = values
	s.entries[key] = entry
	return resp.Integer(len(values)).Encode(), nil
}

func (s *Server) handleLrange(args []string) ([]byte, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("LRANGE cmd requires at least 3 arguments")
	}

	key := args[0]
	entry := s.entries[key]
	startIndex, err := parseInt(args[1])
	if err != nil {
		return nil, err
	}
	endIndex, err := parseInt(args[2])
	if err != nil {
		return nil, err
	}

	if entry == nil {
		return resp.Array{}.Encode(), nil
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
	return resp.BulkStrings(res).Encode(), nil
}

func (s *Server) handleLlen(args []string) ([]byte, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("LLEN cmd requires at least 1 parameters")
	}
	key := args[0]
	entry := s.entries[key]

	if entry == nil {
		return resp.Integer(0).Encode(), nil
	}

	values, ok := entry.val.([]string)
	if !ok {
		return nil, fmt.Errorf("entry with key %s is not a list", key)
	}

	return resp.Integer(len(values)).Encode(), nil
}

func (s *Server) handleLpop(args []string) ([]byte, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("LPOP cmd requires at least 1 parameters")
	}

	key := args[0]
	entry := s.entries[key]

	if entry == nil {
		return resp.NullBulk{}.Encode(), nil
	}

	values, ok := entry.val.([]string)
	if !ok {
		return nil, fmt.Errorf("entry with key %s is not a list", key)
	}

	withCount := len(args) == 2
	endIndex := 1
	if withCount {
		count, err := parseInt(args[1])
		if err != nil {
			return nil, err
		}
		endIndex = min(count, len(values))
	}

	elems := values[0:endIndex]
	entry.val = values[endIndex:]

	if len(entry.val.([]string)) == 0 {
		delete(s.entries, key)
	}

	if !withCount {
		return resp.BulkString(elems[0]).Encode(), nil
	}
	return resp.BulkStrings(elems).Encode(), nil
}

func (s *Server) handleBLPOP(args []string) ([]byte, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("BLPOP cmd requires at least 2 parameters")
	}

	timeoutSecs, err := strconv.ParseFloat(args[1], 64)
	if err != nil {
		return nil, fmt.Errorf("timeout is not a float or out of range")
	}

	s.mu.Lock()

	key := args[0]
	entry := s.entries[key]

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
			return resp.BulkStrings([]string{key, elem}).Encode(), nil
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
		return resp.BulkStrings([]string{key, val}).Encode(), nil
	case <-timeoutCh:
		s.mu.Lock()
		idx := slices.Index(s.waiters[key], ch)
		if idx != -1 {
			// still waiting — genuinely timed out, nobody sent anything
			s.waiters[key] = slices.Delete(s.waiters[key], idx, idx+1)
			s.mu.Unlock()
			return resp.NullArray{}.Encode(), nil
		}
		// a pusher already claimed us right as the timer fired
		s.mu.Unlock()
		val := <-ch
		return resp.BulkStrings([]string{key, val}).Encode(), nil
	}
}

func parseExpiry(args []string) (time.Time, error) {
	if len(args) <= 2 {
		return time.Time{}, nil
	}

	option := strings.ToUpper(args[2])
	switch option {
	case "PX":
		if len(args) < 4 {
			return time.Time{}, fmt.Errorf("PX requires a value")
		}
		ms, err := parseInt(args[3])
		if err != nil {
			return time.Time{}, err
		}
		return time.Now().Add(time.Millisecond * time.Duration(ms)), nil
	default:
		return time.Time{}, fmt.Errorf("unknown option: %s", option)
	}
}

// parseInt returns the same error as Redis when an argument is not an integer
func parseInt(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("value is not an integer or out of range")
	}
	return n, nil
}
