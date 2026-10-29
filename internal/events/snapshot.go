package events

import (
	"encoding/json"
)

type snapshot struct {
	Next map[string]int64
	Seen map[string]bool
	All  []Event
}

func (l *Log) Export() (json.RawMessage, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return json.Marshal(snapshot{Next: l.next, Seen: l.seen, All: l.all})
}
func (l *Log) Import(raw json.RawMessage) error {
	var v snapshot
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.next = v.Next
	l.seen = v.Seen
	l.all = v.All
	return nil
}
