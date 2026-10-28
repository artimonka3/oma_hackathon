package httpapi

import (
	"net/http"

	"oma/internal/media"
	"oma/internal/project"
	"oma/internal/soberi"
)

func metaFor(pid string, sess Session, kind, filename, contentType string) media.Meta {
	return media.Meta{
		ProjectID:   pid,
		Author:      sess.UserID,
		Kind:        kind,
		Filename:    filename,
		ContentType: contentType,
	}
}

// POST /api/v1/projects/{id}/design-jobs — запустить подготовку вариантов.
func (s *Server) handleStartDesignJob(w http.ResponseWriter, r *http.Request, sess Session) {
	job, err := s.cfg.Soberi.StartDesignJob(r.Context(), project.UserID(sess.UserID), r.PathValue("project_id"))
	if err != nil {
		respondError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

// GET /api/v1/jobs/{id} — состояние и результат задания.
func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request, sess Session) {
	job, err := s.cfg.Jobs.Get(r.PathValue("job_id"))
	if err != nil {
		respondError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

// POST /api/v1/projects/{id}/design-selections — подтвердить выбранную версию.
func (s *Server) handleSelectDesign(w http.ResponseWriter, r *http.Request, sess Session) {
	var in struct {
		VariantID string `json:"variant_id"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	sel, err := s.cfg.Soberi.SelectDesign(r.Context(), project.UserID(sess.UserID), r.PathValue("project_id"), in.VariantID)
	if err != nil {
		respondError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, sel)
}

// POST /api/v1/projects/{id}/specifications — рассчитать спецификацию.
func (s *Server) handleCalculateSpec(w http.ResponseWriter, r *http.Request, sess Session) {
	spec, err := s.cfg.Soberi.CalculateSpecification(r.Context(), project.UserID(sess.UserID), r.PathValue("project_id"))
	if err != nil {
		respondError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, spec)
}

// POST /api/v1/projects/{id}/cart-sync — создать/обновить связанные строки
// торговой корзины. Повторный вызов обновляет строки по связям, не удваивая
// покупку.
func (s *Server) handleCartSync(w http.ResponseWriter, r *http.Request, sess Session) {
	res, err := s.cfg.Soberi.CartSync(r.Context(), project.UserID(sess.UserID), r.PathValue("project_id"))
	if err != nil {
		respondError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// Compile-time проверка использования доменных типов в тонких обработчиках.
var _ = soberi.SelectedDesign{}
