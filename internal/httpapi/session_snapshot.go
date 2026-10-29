package httpapi

import "encoding/json"

func (s *SessionStore) Export() (json.RawMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return json.Marshal(s.byToken)
}
func (s *SessionStore) Import(raw json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return json.Unmarshal(raw, &s.byToken)
}
