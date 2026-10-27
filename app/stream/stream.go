package stream

import (
	"fmt"
)

// Stream keeps entries in ascending id order
type Stream struct {
	entries []Entry
}

type Entry struct {
	ID     ID
	Fields []Field
}

type Field struct {
	Key   string
	Value string
}

// Add appends a new entry. idArg is the raw XADD id ("*", "<ms>-*" or "<ms>-<seq>").
// It returns the id given to the entry.
func (st *Stream) Add(idArg string, fields []Field) (ID, error) {
	last := st.lastID()

	id, err := nextID(idArg, last)
	if err != nil {
		return ID{}, err
	}

	if id == (ID{}) {
		return ID{}, fmt.Errorf("The ID specified in XADD must be greater than 0-0")
	}

	if last != nil && id.Compare(*last) <= 0 {
		return ID{}, fmt.Errorf("The ID specified in XADD is equal or smaller than the target stream top item")
	}

	st.entries = append(st.entries, Entry{ID: id, Fields: fields})
	return id, nil
}

// After returns the entries with id > after.
func (st *Stream) After(id ID) []Entry {
	for i, e := range st.entries {
		if e.ID.Compare(id) > 0 {
			return st.entries[i:]
		}
	}
	return nil
}

// Between returns the entries with start <= id <= end.
func (st *Stream) Between(start, end ID) []Entry {
	var selected []Entry
	for _, e := range st.entries {
		if e.ID.Compare(start) < 0 {
			continue
		}
		if e.ID.Compare(end) > 0 {
			break
		}
		selected = append(selected, e)
	}
	return selected
}

// lastID returns the id of the newest entry, or nil if the stream is empty.
func (st *Stream) lastID() *ID {
	if len(st.entries) == 0 {
		return nil
	}
	return &st.entries[len(st.entries)-1].ID
}
