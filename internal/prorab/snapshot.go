package prorab

import (
	"encoding/json"
)

type snapshot struct {
	Reviews   map[string]*Review
	Active    map[string]string
	Plans     map[string]*WorkPlan
	ByProject map[string][]string
	Stages    map[string]*Stage
	Reports   map[string]*Report
	ByStage   map[string][]string
	Decisions []StageDecisionRecord
}

func (s *Service) Export() (json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return json.Marshal(snapshot{Reviews: s.reviews, Active: s.activeReview, Plans: s.plans, ByProject: s.planByProject, Stages: s.stages, Reports: s.reports, ByStage: s.reportsByStage, Decisions: s.decisions})
}
func (s *Service) Import(raw json.RawMessage) error {
	var v snapshot
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reviews = v.Reviews
	s.activeReview = v.Active
	s.plans = v.Plans
	s.planByProject = v.ByProject
	s.stages = v.Stages
	s.reports = v.Reports
	s.reportsByStage = v.ByStage
	s.decisions = v.Decisions
	return nil
}
