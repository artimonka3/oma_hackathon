package project

import (
	"encoding/json"
)

type snapshot struct {
	Projects map[ProjectID]*Project
	Rooms    map[ProjectID][]Room
	Lots     map[ProjectID][]*InventoryLot
}

func (r *MemoryRepo) Export() (json.RawMessage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return json.Marshal(snapshot{Projects: r.projects, Rooms: r.rooms, Lots: r.lots})
}
func (r *MemoryRepo) Import(raw json.RawMessage) error {
	var v snapshot
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.projects = v.Projects
	r.rooms = v.Rooms
	r.lots = v.Lots
	return nil
}
