package httpapi

import (
	"net/http"

	"oma/internal/apperr"
)

// GET /healthz — проверка живости.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": "oma-app"})
}

// requireServiceSecret проверяет секрет сервисного клиента магазина
// (маршруты /internal/v1/oma/* доступны только сервису).
func (s *Server) requireServiceSecret(r *http.Request) error {
	if s.cfg.ServiceSecret == "" {
		return nil
	}
	if r.Header.Get("X-Service-Secret") != s.cfg.ServiceSecret {
		return apperr.Forbidden("неверный секрет сервисного клиента")
	}
	return nil
}

// POST /internal/v1/oma/checkout-authorizations — серверная авторизация
// оформления. Каркас фиксирует контракт: проверка владельца, котировки,
// режима исполнения и подтверждений будет реализована на этапе G ТЗ.
func (s *Server) handleCheckoutAuthorization(w http.ResponseWriter, r *http.Request) {
	if err := s.requireServiceSecret(r); err != nil {
		respondError(w, r, err)
		return
	}
	var in struct {
		QuoteID    string `json:"quote_id"`
		CartID     string `json:"cart_id"`
		ProjectID  string `json:"project_id"`
		TotalMinor int64  `json:"total_minor"`
		Currency   string `json:"currency"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":     "authorized",
		"quote_id":   in.QuoteID,
		"project_id": in.ProjectID,
		"note":       "каркас: полная проверка подтверждений и отпечатка состава — этап G ТЗ",
	})
}

// POST /internal/v1/oma/events — входящие события магазина.
// Повторный идентификатор события обрабатывается один раз.
func (s *Server) handleOmaEvent(w http.ResponseWriter, r *http.Request) {
	if err := s.requireServiceSecret(r); err != nil {
		respondError(w, r, err)
		return
	}
	var in struct {
		EventID   string         `json:"event_id"`
		Type      string         `json:"type"`
		Aggregate string         `json:"aggregate"`
		ID        string         `json:"id"`
		Version   int64          `json:"version"`
		Data      map[string]any `json:"data"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	projectID := ""
	if in.Data != nil {
		if pid, ok := in.Data["project_id"].(string); ok {
			projectID = pid
		}
	}
	accepted := s.cfg.Events.IngestExternal(in.EventID, in.Type, in.Aggregate, in.ID, projectID, in.Data)
	writeJSON(w, http.StatusOK, map[string]any{"accepted": accepted, "event_id": in.EventID})
}
