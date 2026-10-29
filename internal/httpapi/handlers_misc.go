package httpapi

import (
	"net/http"
	"strings"
	"time"

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
	// Standalone clone carts use a separate demo identifier; project carts require an active payment permit.
	if in.ProjectID != "" && !strings.HasPrefix(in.ProjectID, "prj_") {
		writeJSON(w, 200, map[string]string{"status": "authorized", "mode": "standalone_shop_demo"})
		return
	}
	value, ok := s.permits.Load(in.QuoteID)
	if !ok {
		respondError(w, r, apperr.New("CHECKOUT_DECLINED", "Подтвердите проектный заказ через ОМА.Подбор", nil))
		return
	}
	permit := value.(checkoutPermit)
	if permit.ProjectID != in.ProjectID || permit.CartID != in.CartID || permit.TotalMinor != in.TotalMinor || permit.Currency != in.Currency || time.Now().After(permit.ExpiresAt) || permit.Hash != s.cfg.Soberi.ReviewHash(in.ProjectID) {
		respondError(w, r, apperr.New("CHECKOUT_DECLINED", "Котировка изменилась", nil))
		return
	}
	writeJSON(w, 200, map[string]any{"status": "authorized", "quote_id": in.QuoteID})

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
	if accepted && s.cfg.Save != nil {
		go func() { s.mu.Lock(); defer s.mu.Unlock(); _ = s.cfg.Save() }()
	}
	writeJSON(w, http.StatusOK, map[string]any{"accepted": accepted, "event_id": in.EventID})
}

type checkoutPermit struct {
	ProjectID, CartID, Currency, Hash string
	TotalMinor                        int64
	ExpiresAt                         time.Time
}
