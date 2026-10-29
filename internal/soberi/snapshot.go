package soberi

import (
	"encoding/json"
	"oma/internal/ai"
)

type snapshot struct {
	Drafts     map[string]ai.Draft
	Selections map[string]SelectedDesign
	Specs      map[string]*Specification
	Versions   map[string]int64
}

func (s *Service) Export() (json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return json.Marshal(snapshot{Drafts: s.drafts, Selections: s.selections, Specs: s.specs, Versions: s.specVers})
}
func (s *Service) Import(raw json.RawMessage) error {
	var v snapshot
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.drafts = v.Drafts
	s.selections = v.Selections
	s.specs = v.Specs
	s.specVers = v.Versions
	return nil
}
