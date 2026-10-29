package httpapi

import (
	"context"
	"net/http"

	"oma/internal/ai"
	"time"
)

func (s *Server) handlePublicConfig(w http.ResponseWriter, r *http.Request) {
	model := s.cfg.AIModel
	if model == "" {
		model = ai.DefaultModel
	}
	mode := s.cfg.AIMode
	if mode == "" {
		mode = "demo"
	}
	writeJSON(w, 200, map[string]any{"ai_mode": mode, "ai_model": model, "storage": s.cfg.StorageMode, "shop_connected": s.cfg.ShopBaseURL != "", "demo": s.cfg.DemoMode})
}
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Ready != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if s.cfg.Ready(ctx) != nil {
			writeJSON(w, 503, map[string]any{"status": "unavailable", "storage": s.cfg.StorageMode})
			return
		}
	}
	writeJSON(w, 200, map[string]any{"status": "ok", "storage": s.cfg.StorageMode})
}
