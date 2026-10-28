package project

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"oma/internal/apperr"
	"oma/internal/events"
	"oma/internal/id"
)

// Repository — граница хранения агрегатов проекта.
// Каркасная реализация в памяти; этап A ТЗ заменяет её на PostgreSQL
// с той же сигнатурой.
type Repository interface {
	SaveProject(ctx context.Context, p *Project) error
	GetProject(ctx context.Context, pid ProjectID) (*Project, error)
	SaveRoom(ctx context.Context, r *Room) error
	ListRooms(ctx context.Context, pid ProjectID) ([]Room, error)
	SaveLot(ctx context.Context, l *InventoryLot) error
	GetLot(ctx context.Context, lotID string) (*InventoryLot, error)
	ListLots(ctx context.Context, pid ProjectID) ([]InventoryLot, error)
}

// MemoryRepo — хранилище в памяти процесса.
type MemoryRepo struct {
	mu       sync.Mutex
	projects map[ProjectID]*Project
	rooms    map[ProjectID][]Room
	lots     map[ProjectID][]*InventoryLot
}

// NewMemoryRepo создаёт пустое хранилище.
func NewMemoryRepo() *MemoryRepo {
	return &MemoryRepo{
		projects: map[ProjectID]*Project{},
		rooms:    map[ProjectID][]Room{},
		lots:     map[ProjectID][]*InventoryLot{},
	}
}

func (r *MemoryRepo) SaveProject(ctx context.Context, p *Project) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.projects[p.ID] = p
	return nil
}

func (r *MemoryRepo) GetProject(ctx context.Context, pid ProjectID) (*Project, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.projects[pid]
	if !ok {
		return nil, apperr.NotFound("проект не найден")
	}
	return p, nil
}

func (r *MemoryRepo) SaveRoom(ctx context.Context, room *Room) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rooms[room.ProjectID] = append(r.rooms[room.ProjectID], *room)
	return nil
}

func (r *MemoryRepo) ListRooms(ctx context.Context, pid ProjectID) ([]Room, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Room, len(r.rooms[pid]))
	copy(out, r.rooms[pid])
	return out, nil
}

func (r *MemoryRepo) SaveLot(ctx context.Context, l *InventoryLot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lots[l.ProjectID] = append(r.lots[l.ProjectID], l)
	return nil
}

func (r *MemoryRepo) GetLot(ctx context.Context, lotID string) (*InventoryLot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, lots := range r.lots {
		for _, l := range lots {
			if l.ID == lotID {
				return l, nil
			}
		}
	}
	return nil, apperr.NotFound("партия не найдена")
}

func (r *MemoryRepo) ListLots(ctx context.Context, pid ProjectID) ([]InventoryLot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]InventoryLot, 0, len(r.lots[pid]))
	for _, l := range r.lots[pid] {
		out = append(out, *l)
	}
	return out, nil
}

// CreateInput — вход создания проекта.
type CreateInput struct {
	Goal          string
	BudgetMinor   int64
	ExecutionMode string
}

// UpdateInput — вход PATCH: передаётся ожидаемая версия агрегата.
type UpdateInput struct {
	Goal            *string
	BudgetMinor     *int64
	ExecutionMode   *string
	ExpectedVersion int64
}

// AddRoomInput — вход добавления помещения.
type AddRoomInput struct {
	Name                        string
	WidthMM, LengthMM, HeightMM int
	DimensionsConfirmed         bool
}

// HistorySuggestion — кандидат из истории покупок для подтверждения.
type HistorySuggestion struct {
	SKU, Title, Unit, Amount, PurchasedAt string
}

// InventoryInput — вход ручной партии материала.
type InventoryInput struct {
	SKU, Title, Amount, Unit string
}

// ProjectState — снимок проекта с помещениями и доступными
// следующими действиями (по разделу 7.2 ТЗ).
type ProjectState struct {
	Project     *Project `json:"project"`
	Rooms       []Room   `json:"rooms"`
	NextActions []string `json:"next_actions"`
}

// Service — бизнес-логика домена Project.
type Service struct {
	repo   Repository
	events *events.Log
}

// NewService собирает сервис на границах репозитория и журнала событий.
func NewService(repo Repository, ev *events.Log) *Service {
	return &Service{repo: repo, events: ev}
}

// Create создаёт проект: цель обязательна, режим исполнения по умолчанию self.
func (s *Service) Create(ctx context.Context, owner UserID, in CreateInput) (*Project, error) {
	goal := strings.TrimSpace(in.Goal)
	if goal == "" {
		return nil, apperr.InvalidInput("укажите цель проекта (goal)", nil)
	}
	if in.BudgetMinor < 0 {
		return nil, apperr.InvalidInput("бюджет не может быть отрицательным", nil)
	}
	mode := in.ExecutionMode
	if mode == "" {
		mode = ModeSelf
	}
	if mode != ModeSelf && mode != ModeForeman {
		return nil, apperr.InvalidInput("execution_mode должен быть self или foreman", nil)
	}
	now := time.Now().UTC()
	p := &Project{
		ID:            ProjectID(id.New("prj")),
		OwnerID:       owner,
		Goal:          goal,
		BudgetMinor:   in.BudgetMinor,
		Currency:      "BYN",
		ExecutionMode: mode,
		Participants:  []Participant{{UserID: owner, Role: RoleOwner}},
		Version:       1,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.repo.SaveProject(ctx, p); err != nil {
		return nil, err
	}
	s.events.Append(string(p.ID), "project", "project.created", string(owner), nil)
	return p, nil
}

// ensureAccess проверяет, что actor — владелец или участник проекта.
// Проверка выполняется сервером для каждого идентификатора.
func ensureAccess(p *Project, actor UserID) error {
	if p.OwnerID == actor {
		return nil
	}
	for _, pt := range p.Participants {
		if pt.UserID == actor {
			return nil
		}
	}
	return apperr.Forbidden("нет доступа к проекту")
}

// GetForActor возвращает проект с проверкой доступа.
func (s *Service) GetForActor(ctx context.Context, actor UserID, pid ProjectID) (*Project, error) {
	p, err := s.repo.GetProject(ctx, pid)
	if err != nil {
		return nil, err
	}
	if err := ensureAccess(p, actor); err != nil {
		return nil, err
	}
	return p, nil
}

// InternalProject возвращает агрегат другим доменам без проверки доступа:
// доступ уже проверен вызывающим сервисом для своего actor.
func (s *Service) InternalProject(ctx context.Context, pid ProjectID) (*Project, error) {
	return s.repo.GetProject(ctx, pid)
}

// RoomsOf возвращает помещения проекта другим доменам.
func (s *Service) RoomsOf(ctx context.Context, pid ProjectID) ([]Room, error) {
	return s.repo.ListRooms(ctx, pid)
}

// GetState формирует снимок проекта и вычисляет следующие действия.
func (s *Service) GetState(ctx context.Context, actor UserID, pid ProjectID) (*ProjectState, error) {
	p, err := s.GetForActor(ctx, actor, pid)
	if err != nil {
		return nil, err
	}
	rooms, err := s.repo.ListRooms(ctx, pid)
	if err != nil {
		return nil, err
	}
	return &ProjectState{Project: p, Rooms: rooms, NextActions: nextActions(p, rooms)}, nil
}

// nextActions — доменное правило доступных следующих шагов.
func nextActions(p *Project, rooms []Room) []string {
	actions := []string{}
	if len(rooms) == 0 {
		return append(actions, "add_rooms")
	}
	for _, r := range rooms {
		if !r.DimensionsConfirmed {
			return append(actions, "confirm_dimensions")
		}
	}
	if p.ExecutionMode == ModeForeman {
		return append(actions, "purchase_preflight")
	}
	return append(actions, "organize_execution")
}

// Update применяет изменения по ожидаемой версии; несовпадение
// даёт VERSION_CONFLICT (раздел 7.1 ТЗ).
func (s *Service) Update(ctx context.Context, actor UserID, pid ProjectID, in UpdateInput) (*Project, error) {
	p, err := s.GetForActor(ctx, actor, pid)
	if err != nil {
		return nil, err
	}
	if p.Version != in.ExpectedVersion {
		return nil, apperr.VersionConflict(
			"ожидается версия " + strconv.FormatInt(in.ExpectedVersion, 10) + ", текущая " + strconv.FormatInt(p.Version, 10))
	}
	if in.Goal != nil {
		goal := strings.TrimSpace(*in.Goal)
		if goal == "" {
			return nil, apperr.InvalidInput("цель проекта не может быть пустой", nil)
		}
		p.Goal = goal
	}
	if in.BudgetMinor != nil {
		if *in.BudgetMinor < 0 {
			return nil, apperr.InvalidInput("бюджет не может быть отрицательным", nil)
		}
		p.BudgetMinor = *in.BudgetMinor
	}
	if in.ExecutionMode != nil {
		if *in.ExecutionMode != ModeSelf && *in.ExecutionMode != ModeForeman {
			return nil, apperr.InvalidInput("execution_mode должен быть self или foreman", nil)
		}
		p.ExecutionMode = *in.ExecutionMode
	}
	p.Version++
	p.UpdatedAt = time.Now().UTC()
	if err := s.repo.SaveProject(ctx, p); err != nil {
		return nil, err
	}
	s.events.Append(string(p.ID), "project", "project.updated", string(actor), nil)
	return p, nil
}

// AddRoom добавляет помещение; правка границ проекта — только владельцем.
func (s *Service) AddRoom(ctx context.Context, actor UserID, pid ProjectID, in AddRoomInput) (*Room, error) {
	p, err := s.GetForActor(ctx, actor, pid)
	if err != nil {
		return nil, err
	}
	if p.OwnerID != actor {
		return nil, apperr.Forbidden("добавлять помещения может только владелец проекта")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, apperr.InvalidInput("укажите название помещения", nil)
	}
	if in.WidthMM <= 0 || in.LengthMM <= 0 || in.HeightMM <= 0 {
		return nil, apperr.InvalidInput("размеры должны быть положительными (в мм)", nil)
	}
	room := &Room{
		ID:                  RoomID(id.New("room")),
		ProjectID:           pid,
		Name:                name,
		WidthMM:             in.WidthMM,
		LengthMM:            in.LengthMM,
		HeightMM:            in.HeightMM,
		DimensionsConfirmed: in.DimensionsConfirmed,
	}
	if err := s.repo.SaveRoom(ctx, room); err != nil {
		return nil, err
	}
	p.RoomIDs = append(p.RoomIDs, room.ID)
	p.Version++
	p.UpdatedAt = time.Now().UTC()
	if err := s.repo.SaveProject(ctx, p); err != nil {
		return nil, err
	}
	s.events.Append(string(pid), "room", "room.added", string(actor), map[string]any{"room_id": room.ID, "name": name})
	return room, nil
}

// ListRoomsForActor возвращает помещения с проверкой доступа.
func (s *Service) ListRoomsForActor(ctx context.Context, actor UserID, pid ProjectID) ([]Room, error) {
	if _, err := s.GetForActor(ctx, actor, pid); err != nil {
		return nil, err
	}
	return s.repo.ListRooms(ctx, pid)
}

// NoteCapture фиксирует связь сохранённого медиа с проектом.
func (s *Service) NoteCapture(ctx context.Context, actor UserID, pid ProjectID, mediaID, kind string) error {
	if _, err := s.GetForActor(ctx, actor, pid); err != nil {
		return err
	}
	s.events.Append(string(pid), "capture", "capture.added", string(actor),
		map[string]any{"media_id": mediaID, "kind": kind})
	return nil
}

// SuggestInventory создаёт кандидатов из истории покупок: до подтверждения
// позиция не уменьшает новую закупку, повторная подсказка не дублируется.
func (s *Service) SuggestInventory(ctx context.Context, actor UserID, pid ProjectID, items []HistorySuggestion) ([]InventoryLot, error) {
	if _, err := s.GetForActor(ctx, actor, pid); err != nil {
		return nil, err
	}
	existing, err := s.repo.ListLots(ctx, pid)
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for _, l := range existing {
		if l.Status == LotSuggested {
			known[l.SKU] = true
		}
	}
	var out []InventoryLot
	for _, it := range items {
		if it.SKU == "" || known[it.SKU] {
			continue
		}
		known[it.SKU] = true
		lot := &InventoryLot{
			ID:        id.New("lot"),
			ProjectID: pid,
			SKU:       it.SKU,
			Title:     it.Title,
			Amount:    it.Amount,
			Unit:      it.Unit,
			Status:    LotSuggested,
			Source:    "history",
			CreatedAt: time.Now().UTC(),
		}
		if err := s.repo.SaveLot(ctx, lot); err != nil {
			return nil, err
		}
		out = append(out, *lot)
	}
	s.events.Append(string(pid), "inventory", "inventory.suggested", string(actor), map[string]any{"count": len(out)})
	return out, nil
}

// AddInventory добавляет собственную партию как подтвержденную.
func (s *Service) AddInventory(ctx context.Context, actor UserID, pid ProjectID, in InventoryInput) (*InventoryLot, error) {
	p, err := s.GetForActor(ctx, actor, pid)
	if err != nil {
		return nil, err
	}
	if p.OwnerID != actor {
		return nil, apperr.Forbidden("партию подтверждает владелец проекта")
	}
	if strings.TrimSpace(in.SKU) == "" || strings.TrimSpace(in.Amount) == "" {
		return nil, apperr.InvalidInput("укажите артикул и количество материала", nil)
	}
	lot := &InventoryLot{
		ID:        id.New("lot"),
		ProjectID: pid,
		OwnerID:   actor,
		SKU:       in.SKU,
		Title:     in.Title,
		Amount:    in.Amount,
		Unit:      in.Unit,
		Status:    LotConfirmed,
		Source:    "manual",
		CreatedAt: time.Now().UTC(),
	}
	if err := s.repo.SaveLot(ctx, lot); err != nil {
		return nil, err
	}
	s.events.Append(string(pid), "inventory", "inventory.confirmed", string(actor), map[string]any{"lot_id": lot.ID})
	return lot, nil
}

// ConfirmInventory переводит предложенную партию в подтвержденную;
// фиксируется подтверждённый доступный объём.
func (s *Service) ConfirmInventory(ctx context.Context, actor UserID, pid ProjectID, lotID, amount string) (*InventoryLot, error) {
	p, err := s.GetForActor(ctx, actor, pid)
	if err != nil {
		return nil, err
	}
	if p.OwnerID != actor {
		return nil, apperr.Forbidden("партию подтверждает владелец проекта")
	}
	lot, err := s.repo.GetLot(ctx, lotID)
	if err != nil {
		return nil, err
	}
	if lot.ProjectID != pid {
		return nil, apperr.Forbidden("партия принадлежит другому проекту")
	}
	if amount != "" {
		lot.Amount = amount
	}
	lot.Status = LotConfirmed
	lot.OwnerID = actor
	s.events.Append(string(pid), "inventory", "inventory.confirmed", string(actor), map[string]any{"lot_id": lot.ID})
	return lot, nil
}

// ListInventory возвращает партии проекта с проверкой доступа.
func (s *Service) ListInventory(ctx context.Context, actor UserID, pid ProjectID) ([]InventoryLot, error) {
	if _, err := s.GetForActor(ctx, actor, pid); err != nil {
		return nil, err
	}
	return s.repo.ListLots(ctx, pid)
}
