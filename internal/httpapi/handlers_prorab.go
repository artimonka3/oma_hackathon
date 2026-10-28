package httpapi

import (
	"net/http"

	"oma/internal/apperr"
	"oma/internal/project"
	"oma/internal/prorab"
)

// GET /api/v1/foremen — кандидаты с причинами соответствия.
func (s *Server) handleListForemen(w http.ResponseWriter, r *http.Request, sess Session) {
	foremen := s.cfg.Prorab.ListForemen(r.URL.Query().Get("speciality"), r.URL.Query().Get("region"))
	writeJSON(w, http.StatusOK, map[string]any{"foremen": foremen, "demo": true})
}

// GET /api/v1/foremen/{id} — профиль, команда и условия исполнителя.
func (s *Server) handleGetForeman(w http.ResponseWriter, r *http.Request, sess Session) {
	f, err := s.cfg.Prorab.GetForeman(r.PathValue("foreman_id"))
	if err != nil {
		respondError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"foreman": f, "demo": f.Demo})
}

// POST /api/v1/projects/{id}/purchase-preflight — проверить полноту
// подготовки к обращению.
func (s *Server) handlePurchasePreflight(w http.ResponseWriter, r *http.Request, sess Session) {
	report, err := s.cfg.Prorab.Preflight(r.Context(), project.UserID(sess.UserID), r.PathValue("project_id"))
	if err != nil {
		respondError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// POST /api/v1/projects/{id}/reviews — создать или вернуть существующий
// адресный запрос проверки (идемпотентно по области и исполнителю).
func (s *Server) handleCreateReview(w http.ResponseWriter, r *http.Request, sess Session) {
	var in struct {
		ForemanID string `json:"foreman_id"`
		Scope     string `json:"scope"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.ForemanID == "" {
		in.ForemanID = prorab.DefaultForemanID
	}
	pid := r.PathValue("project_id")
	if _, e := s.owner(r, sess, pid); e != nil {
		respondError(w, r, e)
		return
	}
	if s.cfg.Commerce != nil {
		if s.meta(pid).Foreman != in.ForemanID {
			respondError(w, r, apperr.InvalidInput("Выберите этого прораба для проекта", nil))
			return
		}
		sp, e := s.cfg.Soberi.LatestSpecification(pid)
		if e != nil || sp.SourceHash != s.cfg.Soberi.BasisHash(r.Context(), pid) {
			respondError(w, r, apperr.NeedsData("Пересчитайте комплект перед отправкой", nil))
			return
		}
	}
	review, created, err := s.cfg.Prorab.CreateReview(r.Context(), project.UserID(sess.UserID), r.PathValue("project_id"), prorab.CreateReviewInput{
		ForemanID: in.ForemanID,
		Scope:     in.Scope,
	})
	if err != nil {
		respondError(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, map[string]any{"review": review, "created": created, "idempotency_key": r.Header.Get("Idempotency-Key")})
}

// GET /api/v1/projects/{id}/reviews — карточки проверок проекта.
func (s *Server) handleListReviews(w http.ResponseWriter, r *http.Request, sess Session) {
	pid := r.PathValue("project_id")
	if _, err := s.cfg.Projects.GetForActor(r.Context(), project.UserID(sess.UserID), project.ProjectID(pid)); err != nil {
		respondError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reviews": s.cfg.Prorab.ListReviews(pid)})
}

// POST /api/v1/reviews/{id}/decisions — решение прораба с точными вводными.
func (s *Server) handleReviewDecision(w http.ResponseWriter, r *http.Request, sess Session) {
	var in struct {
		Action      string   `json:"action"`
		Comment     string   `json:"comment"`
		ChangedSKUs []string `json:"changed_skus"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	review, err := s.cfg.Prorab.ReviewDecision(r.Context(), project.UserID(sess.UserID), r.PathValue("review_id"), prorab.ReviewDecisionInput{
		Action:      in.Action,
		Comment:     in.Comment,
		ChangedSKUs: in.ChangedSKUs,
	})
	if err != nil {
		respondError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, review)
}

// POST /api/v1/reviews/{id}/proposal-acceptance — принять точную
// предложенную версию.
func (s *Server) handleAcceptProposal(w http.ResponseWriter, r *http.Request, sess Session) {
	review, err := s.cfg.Prorab.AcceptProposal(r.Context(), project.UserID(sess.UserID), r.PathValue("review_id"))
	if err != nil {
		respondError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, review)
}

// POST /api/v1/projects/{id}/work-plans — создать предложение плана работ.
func (s *Server) handleCreateWorkPlan(w http.ResponseWriter, r *http.Request, sess Session) {
	var in struct {
		ForemanID string `json:"foreman_id"`
		Stages    []struct {
			Title    string `json:"title"`
			RoomID   string `json:"room_id"`
			Weight   int    `json:"weight"`
			Required bool   `json:"required"`
		} `json:"stages"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	stages := make([]prorab.StageInput, 0, len(in.Stages))
	for _, st := range in.Stages {
		stages = append(stages, prorab.StageInput{
			Title:    st.Title,
			RoomID:   st.RoomID,
			Weight:   st.Weight,
			Required: st.Required,
		})
	}
	plan, err := s.cfg.Prorab.CreateWorkPlan(r.Context(), project.UserID(sess.UserID), r.PathValue("project_id"), prorab.CreatePlanInput{
		ForemanID: in.ForemanID,
		Stages:    stages,
	})
	if err != nil {
		respondError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, plan)
}

// GET /api/v1/projects/{id}/work-plans — планы, этапы и прогресс приёмки.
func (s *Server) handleListWorkPlans(w http.ResponseWriter, r *http.Request, sess Session) {
	pid := r.PathValue("project_id")
	if _, err := s.cfg.Projects.GetForActor(r.Context(), project.UserID(sess.UserID), project.ProjectID(pid)); err != nil {
		respondError(w, r, err)
		return
	}
	plans, _ := s.cfg.Prorab.ListWorkPlans(pid)
	writeJSON(w, http.StatusOK, map[string]any{
		"plans":              plans,
		"acceptance_percent": s.cfg.Prorab.AcceptanceProgress(pid),
	})
}

// POST /api/v1/work-plans/{id}/confirmation — подтвердить план владельцем.
func (s *Server) handleConfirmWorkPlan(w http.ResponseWriter, r *http.Request, sess Session) {
	plan, err := s.cfg.Prorab.ConfirmWorkPlan(r.Context(), project.UserID(sess.UserID), r.PathValue("plan_id"))
	if err != nil {
		respondError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

// POST /api/v1/stages/{id}/start — начать допустимый этап.
func (s *Server) handleStartStage(w http.ResponseWriter, r *http.Request, sess Session) {
	if s.cfg.Commerce != nil {
		st, e := s.cfg.Prorab.GetStage(r.PathValue("stage_id"))
		if e != nil {
			respondError(w, r, e)
			return
		}
		if e = s.readyToStart(r.Context(), st.ProjectID, st.ID); e != nil {
			respondError(w, r, e)
			return
		}
	}

	stage, err := s.cfg.Prorab.StartStage(r.Context(), project.UserID(sess.UserID), r.PathValue("stage_id"))
	if err != nil {
		respondError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, stage)
}

// POST /api/v1/stages/{id}/reports — опубликовать отчёт.
func (s *Server) handleSubmitReport(w http.ResponseWriter, r *http.Request, sess Session) {
	var in struct {
		Items    []string `json:"items"`
		Comment  string   `json:"comment"`
		PhotoIDs []string `json:"photo_ids"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := s.validateReportMedia(r, sess, r.PathValue("stage_id"), in.PhotoIDs); err != nil {
		respondError(w, r, err)
		return
	}
	report, stage, err := s.cfg.Prorab.SubmitReport(r.Context(), project.UserID(sess.UserID), r.PathValue("stage_id"), prorab.ReportInput{
		Items:    in.Items,
		Comment:  in.Comment,
		PhotoIDs: in.PhotoIDs,
	})
	if err != nil {
		respondError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"report": report, "stage": stage, "idempotency_key": r.Header.Get("Idempotency-Key")})
}

// GET /api/v1/stages/{id}/reports — история версий отчётов этапа.
func (s *Server) handleListStageReports(w http.ResponseWriter, r *http.Request, sess Session) {
	stage, err := s.cfg.Prorab.GetStage(r.PathValue("stage_id"))
	if err != nil {
		respondError(w, r, err)
		return
	}
	if _, err := s.cfg.Projects.GetForActor(r.Context(), project.UserID(sess.UserID), project.ProjectID(stage.ProjectID)); err != nil {
		respondError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reports": s.cfg.Prorab.StageReports(stage.ID)})
}

// POST /api/v1/stages/{id}/decisions — принять текущий отчёт либо
// направить замечания (тело из раздела 7.5 ТЗ).
func (s *Server) handleStageDecision(w http.ResponseWriter, r *http.Request, sess Session) {
	var in struct {
		ExpectedVersion     int64  `json:"expected_version"`
		ReportID            string `json:"report_id"`
		Decision            string `json:"decision"`
		AcceptanceConfirmed bool   `json:"acceptance_confirmed"`
		Comment             string `json:"comment"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	stage, err := s.cfg.Prorab.StageDecision(r.Context(), project.UserID(sess.UserID), r.PathValue("stage_id"), prorab.StageDecisionInput{
		ExpectedVersion:     in.ExpectedVersion,
		ReportID:            in.ReportID,
		Decision:            in.Decision,
		AcceptanceConfirmed: in.AcceptanceConfirmed,
		Comment:             in.Comment,
		IdempotencyKey:      r.Header.Get("Idempotency-Key"),
	})
	if err != nil {
		respondError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, stage)
}
