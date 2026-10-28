package httpapi

import (
	"context"
	"io"
	"net/http"
	"oma/internal/apperr"
	"oma/internal/media"
	"oma/internal/project"
)

type mediaReader interface {
	Open(context.Context, string) (media.Meta, io.ReadCloser, error)
}

func (s *Server) handleMedia(w http.ResponseWriter, r *http.Request, sess Session) {
	store, ok := s.cfg.Media.(mediaReader)
	if !ok {
		respondError(w, r, apperr.NotFound("Фото недоступно"))
		return
	}
	m, f, e := store.Open(r.Context(), r.PathValue("media_id"))
	if e != nil {
		respondError(w, r, e)
		return
	}
	defer f.Close()
	if _, e = s.cfg.Projects.GetForActor(r.Context(), project.UserID(sess.UserID), project.ProjectID(m.ProjectID)); e != nil {
		respondError(w, r, e)
		return
	}
	w.Header().Set("Content-Type", m.ContentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", "attachment")
	w.Header().Set("Cache-Control", "private, no-store")
	io.Copy(w, f)
}
func (s *Server) validateReportMedia(r *http.Request, sess Session, stageID string, ids []string) error {
	st, e := s.cfg.Prorab.GetStage(stageID)
	if e != nil {
		return e
	}
	if _, e = s.cfg.Projects.GetForActor(r.Context(), project.UserID(sess.UserID), project.ProjectID(st.ProjectID)); e != nil {
		return e
	}
	store, ok := s.cfg.Media.(mediaReader)
	for _, id := range ids {
		if !ok {
			return apperr.InvalidInput("Фото недоступно", nil)
		}
		m, f, e := store.Open(r.Context(), id)
		if e != nil {
			return e
		}
		f.Close()
		if m.ProjectID != st.ProjectID {
			return apperr.Forbidden("Фото относится к другому проекту")
		}
	}
	return nil
}
