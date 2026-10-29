package httpapi

import (
	"net/http"

	"oma/internal/apperr"
	"oma/internal/project"
)

// POST /api/v1/demo/session — создать явно демонстрационную сессию.
func (s *Server) handleDemoSession(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.DemoMode {
		respondError(w, r, apperr.Forbidden("демо-сессии отключены: DEMO_MODE=false"))
		return
	}
	var in struct {
		Participant string `json:"participant"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	sess, err := s.cfg.Sessions.Create(in.Participant)
	if err != nil {
		respondError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"token":   sess.Token,
		"user_id": sess.UserID,
		"role":    sess.Role,
		"name":    sess.DisplayName,
		"demo":    true,
		"note":    "демо-сессия; в обычном развёртывании требуется нормальная сессия",
	})
}

// POST /api/v1/projects — создать проект.
func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request, sess Session) {
	var in struct {
		Goal          string `json:"goal"`
		BudgetMinor   int64  `json:"budget_minor"`
		ExecutionMode string `json:"execution_mode"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	p, err := s.cfg.Projects.Create(r.Context(), project.UserID(sess.UserID), project.CreateInput{
		Goal:          in.Goal,
		BudgetMinor:   in.BudgetMinor,
		ExecutionMode: in.ExecutionMode,
	})
	if err != nil {
		respondError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

// GET /api/v1/projects/{id} — проект с доступными следующими действиями.
func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request, sess Session) {
	state, err := s.cfg.Projects.GetState(r.Context(), project.UserID(sess.UserID), project.ProjectID(r.PathValue("project_id")))
	if err != nil {
		respondError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

// PATCH /api/v1/projects/{id} — обновить запрос и условия по версии.
func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request, sess Session) {
	var in struct {
		Goal            *string `json:"goal"`
		BudgetMinor     *int64  `json:"budget_minor"`
		ExecutionMode   *string `json:"execution_mode"`
		ExpectedVersion int64   `json:"expected_version"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	p, err := s.cfg.Projects.Update(r.Context(), project.UserID(sess.UserID), project.ProjectID(r.PathValue("project_id")), project.UpdateInput{
		Goal:            in.Goal,
		BudgetMinor:     in.BudgetMinor,
		ExecutionMode:   in.ExecutionMode,
		ExpectedVersion: in.ExpectedVersion,
	})
	if err != nil {
		respondError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// POST /api/v1/projects/{id}/rooms — добавить помещение.
func (s *Server) handleAddRoom(w http.ResponseWriter, r *http.Request, sess Session) {
	var in struct {
		Name                string `json:"name"`
		WidthMM             int    `json:"width_mm"`
		LengthMM            int    `json:"length_mm"`
		HeightMM            int    `json:"height_mm"`
		DimensionsConfirmed bool   `json:"dimensions_confirmed"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	room, err := s.cfg.Projects.AddRoom(r.Context(), project.UserID(sess.UserID), project.ProjectID(r.PathValue("project_id")), project.AddRoomInput{
		Name:                in.Name,
		WidthMM:             in.WidthMM,
		LengthMM:            in.LengthMM,
		HeightMM:            in.HeightMM,
		DimensionsConfirmed: in.DimensionsConfirmed,
	})
	if err != nil {
		respondError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, room)
}

// GET /api/v1/projects/{id}/rooms — список помещений.
func (s *Server) handleListRooms(w http.ResponseWriter, r *http.Request, sess Session) {
	rooms, err := s.cfg.Projects.ListRoomsForActor(r.Context(), project.UserID(sess.UserID), project.ProjectID(r.PathValue("project_id")))
	if err != nil {
		respondError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rooms": rooms})
}

// POST /api/v1/projects/{id}/captures — загрузить фото, план или захват.
func (s *Server) handleAddCapture(w http.ResponseWriter, r *http.Request, sess Session) {
	pid := r.PathValue("project_id")
	if _, err := s.cfg.Projects.GetForActor(r.Context(), project.UserID(sess.UserID), project.ProjectID(pid)); err != nil {
		respondError(w, r, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		respondError(w, r, apperr.InvalidInput("ожидается multipart/form-data с полем file", nil))
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		respondError(w, r, apperr.InvalidInput("поле file обязательно", nil))
		return
	}
	defer file.Close()
	kind := r.FormValue("kind")
	if kind == "" {
		kind = "photo"
	}
	meta, err := s.cfg.Media.Save(r.Context(), metaFor(pid, sess, kind, hdr.Filename, hdr.Header.Get("Content-Type")), file)
	if err != nil {
		respondError(w, r, err)
		return
	}
	if err := s.cfg.Projects.NoteCapture(r.Context(), project.UserID(sess.UserID), project.ProjectID(pid), meta.ID, kind); err != nil {
		respondError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, meta)
}

// POST /api/v1/projects/{id}/inventory-suggestions — кандидаты из истории.
func (s *Server) handleInventorySuggestions(w http.ResponseWriter, r *http.Request, sess Session) {
	pid := r.PathValue("project_id")
	items, err := s.cfg.Oma.PurchaseHistory(r.Context(), "trade_"+sess.UserID)
	if err != nil {
		respondError(w, r, err)
		return
	}
	sugg := make([]project.HistorySuggestion, 0, len(items))
	for _, it := range items {
		sugg = append(sugg, project.HistorySuggestion{
			SKU: it.SKU, Title: it.Title, Unit: it.Unit, Amount: it.Amount, PurchasedAt: it.PurchasedAt,
		})
	}
	lots, err := s.cfg.Projects.SuggestInventory(r.Context(), project.UserID(sess.UserID), project.ProjectID(pid), sugg)
	if err != nil {
		respondError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"lots": lots, "note": "позиция со статусом suggested не уменьшает новую закупку"})
}

// POST /api/v1/projects/{id}/inventory — подтвердить партию или добавить свою.
func (s *Server) handleAddInventory(w http.ResponseWriter, r *http.Request, sess Session) {
	var in struct {
		LotID  string `json:"lot_id"`
		SKU    string `json:"sku"`
		Title  string `json:"title"`
		Amount string `json:"amount"`
		Unit   string `json:"unit"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	pid := r.PathValue("project_id")
	if in.LotID != "" {
		lot, err := s.cfg.Projects.ConfirmInventory(r.Context(), project.UserID(sess.UserID), project.ProjectID(pid), in.LotID, in.Amount)
		if err != nil {
			respondError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, lot)
		return
	}
	lot, err := s.cfg.Projects.AddInventory(r.Context(), project.UserID(sess.UserID), project.ProjectID(pid), project.InventoryInput{
		SKU: in.SKU, Title: in.Title, Amount: in.Amount, Unit: in.Unit,
	})
	if err != nil {
		respondError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, lot)
}

// GET /api/v1/projects/{id}/inventory — партии проекта.
func (s *Server) handleListInventory(w http.ResponseWriter, r *http.Request, sess Session) {
	lots, err := s.cfg.Projects.ListInventory(r.Context(), project.UserID(sess.UserID), project.ProjectID(r.PathValue("project_id")))
	if err != nil {
		respondError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"lots": lots})
}

// GET /api/v1/projects/{id}/events — разрешённый поток событий проекта.
func (s *Server) handleProjectEvents(w http.ResponseWriter, r *http.Request, sess Session) {
	pid := r.PathValue("project_id")
	if _, err := s.cfg.Projects.GetForActor(r.Context(), project.UserID(sess.UserID), project.ProjectID(pid)); err != nil {
		respondError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": s.cfg.Events.ByProject(pid)})
}
