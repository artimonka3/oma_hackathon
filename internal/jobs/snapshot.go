package jobs

import (
	"encoding/json"
)

type snapshot struct {
	Jobs  map[string]*Job
	Order []string
}

func (q *Queue) Export() (json.RawMessage, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return json.Marshal(snapshot{Jobs: q.jobs, Order: q.order})
}
func (q *Queue) Import(raw json.RawMessage) error {
	var v snapshot
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.jobs = v.Jobs
	q.order = v.Order
	for _, j := range q.jobs {
		if j.State == StateRunning {
			j.State = StateQueued
		}
	}
	return nil
}
