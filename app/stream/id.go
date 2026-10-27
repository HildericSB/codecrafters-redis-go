package stream

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

type ID struct {
	msTime    int
	seqNumber int
}

func (s ID) String() string {
	return fmt.Sprintf("%d-%d", s.msTime, s.seqNumber)
}

// Compare returns -1 if id < other, 0 if equal, +1 if id > other.
func (s ID) Compare(other ID) int {
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

// ParseID parses a literal "<ms>-<seq>" stream ID.
func ParseID(s string) (ID, error) {
	msStr, seqStr, hasSeq := strings.Cut(s, "-")
	if !hasSeq {
		return ID{}, fmt.Errorf("malformed stream ID : %q", s)
	}

	ms, err := strconv.Atoi(msStr)
	if err != nil {
		return ID{}, fmt.Errorf("can't parse ms : %q : %w", msStr, err)
	}

	seq, err := strconv.Atoi(seqStr)
	if err != nil {
		return ID{}, fmt.Errorf("can't parse seq : %q : %w", seqStr, err)
	}

	return ID{msTime: ms, seqNumber: seq}, nil
}

// ParseRangeID parses an XRANGE bound: "-", "+", or a stream ID.
func ParseRangeID(s string) (ID, error) {
	switch s {
	case "-":
		return ID{}, nil
	case "+":
		return ID{msTime: math.MaxInt, seqNumber: math.MaxInt}, nil
	}
	return ParseID(s)
}

// nextID turns an XADD id argument ("*", "<ms>-*" or "<ms>-<seq>")
// into a concrete ID, using last to generate sequence numbers.
func nextID(id string, last *ID) (ID, error) {
	if id == "*" {
		msTime := int(time.Now().UnixMilli())
		if last != nil {
			msTime = max(msTime, last.msTime)
		}
		return autoSeq(msTime, last), nil
	}

	msStr, seqStr, hasSeq := strings.Cut(id, "-")
	if !hasSeq {
		return ID{}, fmt.Errorf("invalid stream ID %q", id)
	}

	msTime, err := strconv.Atoi(msStr)
	if err != nil {
		return ID{}, fmt.Errorf("invalid ms time %q: %w", msStr, err)
	}

	if seqStr == "*" {
		return autoSeq(msTime, last), nil
	}

	seqNumber, err := strconv.Atoi(seqStr)
	if err != nil {
		return ID{}, fmt.Errorf("invalid sequence number %q: %w", seqStr, err)
	}

	return ID{msTime: msTime, seqNumber: seqNumber}, nil
}

func autoSeq(msTime int, last *ID) ID {
	switch {
	case last != nil && last.msTime == msTime:
		return ID{msTime: msTime, seqNumber: last.seqNumber + 1}
	case msTime == 0:
		return ID{msTime: msTime, seqNumber: 1}
	default:
		return ID{msTime: msTime, seqNumber: 0}
	}
}
