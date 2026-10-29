package prorab

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"oma/internal/apperr"
	"oma/internal/events"
	"oma/internal/id"
	"oma/internal/project"
)

// DefaultForemanID — демонстрационный исполнитель по умолчанию.
const DefaultForemanID = "foreman_demo_01"

// PreflightReport — результат автоматической проверки комплектности.
type PreflightReport struct {
	ProjectID string   `json:"project_id"`
	Status    string   `json:"status"` // ready | needs_data
	Questions []string `json:"questions"`
	CheckedAt string   `json:"checked_at"`
}

// ForemanWithReasons — кандидат с объяснимыми причинами соответствия.
type ForemanWithReasons struct {
	Foreman Foreman  `json:"foreman"`
	Reasons []string `json:"match_reasons"`
}

// StageDecisionRecord — решение владельца по конкретной версии отчёта.
type StageDecisionRecord struct {
	ID                  string    `json:"id"`
	StageID             string    `json:"stage_id"`
	ReportID            string    `json:"report_id"`
	Decision            string    `json:"decision"` // accept | request_changes
	AcceptanceConfirmed bool      `json:"acceptance_confirmed"`
	Comment             string    `json:"comment"`
	Author              string    `json:"author"`
	CreatedAt           time.Time `json:"created_at"`
}

// Service — бизнес-логика ОМА.Прораб.
type Service struct {
	projects     *project.Service
	events       *events.Log
	foremen      []Foreman
	materialHash func(string) string

	mu             sync.Mutex
	reviews        map[string]*Review
	activeReview   map[string]string // project|foreman|scope -> review id
	plans          map[string]*WorkPlan
	planByProject  map[string][]string
	stages         map[string]*Stage
	reports        map[string]*Report
	reportsByStage map[string][]string
	decisions      []StageDecisionRecord
}

// NewService собирает домен Прораб.
func NewService(p *project.Service, ev *events.Log, foremen []Foreman) *Service {
	return &Service{
		projects:       p,
		events:         ev,
		foremen:        foremen,
		reviews:        map[string]*Review{},
		activeReview:   map[string]string{},
		plans:          map[string]*WorkPlan{},
		planByProject:  map[string][]string{},
		stages:         map[string]*Stage{},
		reports:        map[string]*Report{},
		reportsByStage: map[string][]string{},
	}
}

// ListForemen отбирает кандидатов и объясняет причины соответствия.
func (s *Service) ListForemen(speciality, region string) []ForemanWithReasons {
	out := make([]ForemanWithReasons, 0, len(s.foremen))
	for _, f := range s.foremen {
		if speciality != "" && !containsFold(f.Specialities, speciality) {
			continue
		}
		if region != "" && !strings.EqualFold(f.Region, region) {
			continue
		}
		reasons := make([]string, 0, 3)
		if speciality != "" {
			reasons = append(reasons, "специализация включает "+speciality)
		}
		if region != "" {
			reasons = append(reasons, "работает в регионе "+region)
		}
		if f.Verified {
			reasons = append(reasons, "сведения профиля подтверждены")
		} else {
			reasons = append(reasons, "профиль помечен как демо и не подтверждён")
		}
		out = append(out, ForemanWithReasons{Foreman: f, Reasons: reasons})
	}
	return out
}

// GetForeman возвращает профиль по идентификатору.
func (s *Service) GetForeman(foremanID string) (Foreman, error) {
	for _, f := range s.foremen {
		if f.ID == foremanID {
			return f, nil
		}
	}
	return Foreman{}, apperr.NotFound("исполнитель не найден")
}

// Preflight проверяет обязательные вводные, единицы и полноту подготовки
// к обращению (раздел 4.1 ТЗ). Существенные пропуски возвращаются
// списком вопросов.
func (s *Service) Preflight(ctx context.Context, actor project.UserID, pid string) (*PreflightReport, error) {
	p, err := s.projects.GetForActor(ctx, actor, project.ProjectID(pid))
	if err != nil {
		return nil, err
	}
	rooms, err := s.projects.RoomsOf(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	var questions []string
	if strings.TrimSpace(p.Goal) == "" {
		questions = append(questions, "Опишите цель проекта")
	}
	if p.ExecutionMode != project.ModeForeman {
		questions = append(questions, "Выберите сценарий выполнения с прорабом")
	}
	if len(rooms) == 0 {
		questions = append(questions, "Добавьте хотя бы одно помещение")
	}
	for _, r := range rooms {
		if !r.DimensionsConfirmed {
			questions = append(questions, "Подтвердите размеры помещения "+r.Name)
		}
	}
	report := &PreflightReport{
		ProjectID: pid,
		Questions: questions,
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if len(questions) > 0 {
		report.Status = "needs_data"
	} else {
		report.Status = "ready"
	}
	s.events.Append(pid, "preflight", "preflight.checked", string(actor), map[string]any{"status": report.Status})
	return report, nil
}

// inputHash — каноническое представление области работ: цель, режим,
// подтверждённая геометрия помещений. Правила канонизации версионируются.
func inputHash(p *project.Project, rooms []project.Room) string {
	parts := []string{"v1", p.Goal, p.ExecutionMode}
	ids := make([]string, 0, len(rooms))
	byID := map[string]project.Room{}
	for _, r := range rooms {
		ids = append(ids, string(r.ID))
		byID[string(r.ID)] = r
	}
	sort.Strings(ids)
	for _, rid := range ids {
		r := byID[rid]
		parts = append(parts, fmt.Sprintf("%s:%d:%d:%d:%t", r.Name, r.WidthMM, r.LengthMM, r.HeightMM, r.DimensionsConfirmed))
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])
}

func isActiveReview(st ReviewStatus) bool {
	switch st {
	case ReviewQueued, ReviewInReview, ReviewNeedsInfo, ReviewNeedsVisit, ReviewChangesProposed:
		return true
	}
	return false
}

// CreateReviewInput — вход адресного запроса проверки.
type CreateReviewInput struct {
	ForemanID string
	Scope     string
}

// CreateReview создаёт или возвращает существующую активную карточку
// проверки по выбранной области и исполнителю: повторная отправка того же
// состава не создаёт дубли (раздел 4.1 ТЗ).
func (s *Service) CreateReview(ctx context.Context, actor project.UserID, pid string, in CreateReviewInput) (*Review, bool, error) {
	report, err := s.Preflight(ctx, actor, pid)
	if err != nil {
		return nil, false, err
	}
	if report.Status != "ready" {
		return nil, false, apperr.NeedsData("проект не готов к проверке", report.Questions)
	}
	if _, err := s.GetForeman(in.ForemanID); err != nil {
		return nil, false, err
	}
	if strings.TrimSpace(in.Scope) == "" {
		in.Scope = "project"
	}
	p0, _ := s.projects.InternalProject(ctx, project.ProjectID(pid))
	rooms0, _ := s.projects.RoomsOf(ctx, p0.ID)
	current := s.currentHash(p0, rooms0)
	key := pid + "|" + in.ForemanID + "|" + in.Scope
	s.mu.Lock()
	if existingID, ok := s.activeReview[key]; ok {
		if r, ok := s.reviews[existingID]; ok && isActiveReview(r.Status) && r.InputHash == current && r.Status != ReviewNeedsInfo && r.Status != ReviewNeedsVisit {
			cp := *r
			s.mu.Unlock()
			return &cp, false, nil
		}
	}
	if previous := s.reviews[s.activeReview[key]]; previous != nil {
		previous.Status = ReviewSuperseded
	}
	p, err := s.projects.InternalProject(ctx, project.ProjectID(pid))
	if err != nil {
		s.mu.Unlock()
		return nil, false, err
	}
	rooms, err := s.projects.RoomsOf(ctx, p.ID)
	if err != nil {
		s.mu.Unlock()
		return nil, false, err
	}
	r := &Review{
		ID:        id.New("rev"),
		ProjectID: pid,
		ForemanID: in.ForemanID,
		Scope:     in.Scope,
		Status:    ReviewInReview,
		InputHash: s.currentHash(p, rooms),
		HashRules: "v1",
		Version:   1,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	s.reviews[r.ID] = r
	s.activeReview[key] = r.ID
	s.mu.Unlock()
	s.events.Append(pid, "review", "review.created", string(actor),
		map[string]any{"review_id": r.ID, "foreman_id": r.ForemanID, "scope": r.Scope})
	return r, true, nil
}

// ListReviews возвращает карточки проверок проекта.
func (s *Service) ListReviews(pid string) []Review {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Review, 0, 4)
	for _, r := range s.reviews {
		if r.ProjectID == pid {
			out = append(out, *r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

func (s *Service) review(reviewID string) (*Review, error) {
	r, ok := s.reviews[reviewID]
	if !ok {
		return nil, apperr.NotFound("проверка не найдена")
	}
	return r, nil
}

// ReviewDecisionInput — решение прораба по карточке проверки.
type ReviewDecisionInput struct {
	Action      string // approve_current | propose_changes | request_info | request_visit | decline
	Comment     string
	ChangedSKUs []string
}

// ReviewDecision применяет решение исполнителя с проверкой роли
// и допустимости перехода (раздел 4.2 ТЗ).
func (s *Service) ReviewDecision(ctx context.Context, actor project.UserID, reviewID string, in ReviewDecisionInput) (*Review, error) {
	cp0, e := s.GetReview(reviewID)
	if e != nil {
		return nil, e
	}
	p0, e := s.projects.InternalProject(ctx, project.ProjectID(cp0.ProjectID))
	if e != nil {
		return nil, e
	}
	rooms0, _ := s.projects.RoomsOf(ctx, p0.ID)
	if cp0.InputHash != s.currentHash(p0, rooms0) {
		return nil, apperr.NeedsData("Комплект изменился. Заказчик должен отправить его заново", nil)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.review(reviewID)
	if err != nil {
		return nil, err
	}
	if string(actor) != r.ForemanID {
		return nil, apperr.Forbidden("решение по проверке принимает только назначенный исполнитель")
	}
	var target ReviewStatus
	switch in.Action {
	case "approve_current":
		target = ReviewApproved
	case "propose_changes":
		target = ReviewChangesProposed
	case "request_info":
		target = ReviewNeedsInfo
	case "request_visit":
		target = ReviewNeedsVisit
	case "decline":
		target = ReviewDeclined
	default:
		return nil, apperr.InvalidInput("неизвестное действие: "+in.Action, nil)
	}
	if !CanReviewTransition(r.Status, target) {
		return nil, apperr.InvalidTransition(fmt.Sprintf("переход %s -> %s недопустим", r.Status, target))
	}
	if target == ReviewChangesProposed {
		proposed := r.InputHash
		if len(in.ChangedSKUs) > 0 {
			skus := append([]string(nil), in.ChangedSKUs...)
			sort.Strings(skus)
			sum := sha256.Sum256([]byte(r.InputHash + "|" + strings.Join(skus, ",")))
			proposed = hex.EncodeToString(sum[:])
		}
		r.Proposal = &Proposal{Comment: in.Comment, ChangedSKUs: in.ChangedSKUs, ProposedHash: proposed}
	}
	r.Comment = in.Comment
	r.Status = target
	r.Version++
	r.UpdatedAt = time.Now().UTC()
	s.events.Append(r.ProjectID, "review", "review."+string(target), string(actor), map[string]any{"review_id": r.ID})
	cp := *r
	return &cp, nil
}

// AcceptProposal принимает точную предложенную версию: подтверждение
// становится действующим только при совпадении итогового отпечатка
// с предложенным (раздел 4.2 ТЗ).
func (s *Service) AcceptProposal(ctx context.Context, actor project.UserID, reviewID string) (*Review, error) {
	s.mu.Lock()
	r, err := s.review(reviewID)
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	if r.Status != ReviewChangesProposed {
		s.mu.Unlock()
		return nil, apperr.InvalidTransition("принять можно только предложенные правки (changes_proposed)")
	}
	cp := *r
	s.mu.Unlock()

	p, err := s.projects.GetForActor(ctx, actor, project.ProjectID(cp.ProjectID))
	if err != nil {
		return nil, err
	}
	if p.OwnerID != actor {
		return nil, apperr.Forbidden("предложенные правки принимает владелец проекта")
	}
	rooms, err := s.projects.RoomsOf(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	current := s.currentHash(p, rooms)
	s.mu.Lock()
	defer s.mu.Unlock()
	r, _ = s.review(reviewID)
	if cp.Proposal == nil || current != cp.Proposal.ProposedHash {
		return nil, apperr.NeedsData(
			"состав изменился либо предложение отсутствует; требуется новый раунд проверки",
			[]string{"Проверьте актуальный состав и повторите проверку"})
	}
	r.Status = ReviewApproved
	r.Version++
	r.UpdatedAt = time.Now().UTC()
	s.events.Append(r.ProjectID, "review", "review.proposal_accepted", string(actor), map[string]any{"review_id": r.ID})
	out := *r
	return &out, nil
}

// StageInput — этап в предложении плана.
type StageInput struct {
	Title    string
	RoomID   string
	Weight   int
	Required bool
}

// CreatePlanInput — предложение плана работ.
type CreatePlanInput struct {
	ForemanID string
	Stages    []StageInput
}

// CreateWorkPlan создаёт предложение плана: этапы формируются в состоянии
// planned и становятся действующими после подтверждения владельцем.
func (s *Service) CreateWorkPlan(ctx context.Context, actor project.UserID, pid string, in CreatePlanInput) (*WorkPlan, error) {
	p, err := s.projects.GetForActor(ctx, actor, project.ProjectID(pid))
	if err != nil {
		return nil, err
	}
	if len(in.Stages) == 0 {
		return nil, apperr.InvalidInput("добавьте хотя бы один этап в план", nil)
	}
	foremanID := in.ForemanID
	if foremanID == "" {
		foremanID = DefaultForemanID
	}
	if _, err := s.GetForeman(foremanID); err != nil {
		return nil, err
	}
	if p.OwnerID != actor && string(actor) != foremanID {
		return nil, apperr.Forbidden("план предлагает назначенный исполнитель или владелец проекта")
	}
	for _, st := range in.Stages {
		if strings.TrimSpace(st.Title) == "" {
			return nil, apperr.InvalidInput("название этапа обязательно", nil)
		}
	}
	plan := &WorkPlan{
		ID:        id.New("plan"),
		ProjectID: pid,
		ForemanID: foremanID,
		Version:   1,
		CreatedAt: time.Now().UTC(),
	}
	s.mu.Lock()
	s.plans[plan.ID] = plan
	s.planByProject[pid] = append(s.planByProject[pid], plan.ID)
	for _, st := range in.Stages {
		title := strings.TrimSpace(st.Title)
		if title == "" {
			s.mu.Unlock()
			return nil, apperr.InvalidInput("название этапа обязательно", nil)
		}
		weight := st.Weight
		if weight <= 0 {
			weight = 1
		}
		stage := &Stage{
			ID:        id.New("stg"),
			ProjectID: pid,
			PlanID:    plan.ID,
			RoomID:    st.RoomID,
			Title:     title,
			Required:  st.Required,
			Weight:    weight,
			Status:    StagePlanned,
		}
		plan.StageIDs = append(plan.StageIDs, stage.ID)
		s.stages[stage.ID] = stage
	}
	s.mu.Unlock()
	s.events.Append(pid, "plan", "plan.proposed", string(actor), map[string]any{"plan_id": plan.ID, "foreman_id": foremanID})
	return plan, nil
}

// ConfirmWorkPlan подтверждает план владельцем; после этого этапы
// можно запускать.
func (s *Service) ConfirmWorkPlan(ctx context.Context, actor project.UserID, planID string) (*WorkPlan, error) {
	s.mu.Lock()
	plan, ok := s.plans[planID]
	if !ok {
		s.mu.Unlock()
		return nil, apperr.NotFound("план работ не найден")
	}
	cp := *plan
	s.mu.Unlock()
	p, err := s.projects.GetForActor(ctx, actor, project.ProjectID(cp.ProjectID))
	if err != nil {
		return nil, err
	}
	if p.OwnerID != actor {
		return nil, apperr.Forbidden("план подтверждает владелец проекта")
	}
	s.mu.Lock()
	plan.Confirmed = true
	plan.Version++
	s.mu.Unlock()
	s.events.Append(cp.ProjectID, "plan", "plan.confirmed", string(actor), map[string]any{"plan_id": planID})
	out := *plan
	return &out, nil
}

// ListWorkPlans возвращает планы проекта с этапами.
type PlanView struct {
	Plan   *WorkPlan `json:"plan"`
	Stages []Stage   `json:"stages"`
}

// ListWorkPlans формирует снимок планов и этапов.
func (s *Service) ListWorkPlans(pid string) ([]PlanView, []Stage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []PlanView
	var stages []Stage
	for _, planID := range s.planByProject[pid] {
		plan := s.plans[planID]
		view := PlanView{Plan: plan}
		for _, sid := range plan.StageIDs {
			st := s.stages[sid]
			view.Stages = append(view.Stages, *st)
			stages = append(stages, *st)
		}
		out = append(out, view)
	}
	return out, stages
}

// AcceptanceProgress считает процент приёмки по согласованным весам:
// 100 * сумма весов accepted / сумма весов действующих обязательных этапов
// (раздел 5 технической спецификации).
func (s *Service) AcceptanceProgress(pid string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	num, denom := 0, 0
	for _, planID := range s.planByProject[pid] {
		plan := s.plans[planID]
		if !plan.Confirmed {
			continue
		}
		for _, sid := range plan.StageIDs {
			st := s.stages[sid]
			if st.Status == StageCancelled || !st.Required {
				continue
			}
			denom += st.Weight
			if st.Status == StageAccepted {
				num += st.Weight
			}
		}
	}
	if denom == 0 {
		return 0
	}
	return 100 * num / denom
}

func (s *Service) stage(stageID string) (*Stage, error) {
	st, ok := s.stages[stageID]
	if !ok {
		return nil, apperr.NotFound("этап не найден")
	}
	return st, nil
}

// StartStage начинает допустимый этап: план должен быть подтверждён,
// а действие выполняет назначенный прораб.
func (s *Service) StartStage(ctx context.Context, actor project.UserID, stageID string) (*Stage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.stage(stageID)
	if err != nil {
		return nil, err
	}
	plan, ok := s.plans[st.PlanID]
	if !ok || !plan.Confirmed {
		return nil, apperr.InvalidTransition("план работ не подтверждён владельцем")
	}
	if string(actor) != plan.ForemanID {
		return nil, apperr.Forbidden("этап начинает назначенный прораб")
	}
	if !CanStageTransition(st.Status, StageInProgress) {
		return nil, apperr.InvalidTransition(fmt.Sprintf("переход %s -> %s недопустим", st.Status, StageInProgress))
	}
	st.Status = StageInProgress
	st.Version++
	s.events.Append(st.ProjectID, "stage", "stage.started", string(actor), map[string]any{"stage_id": st.ID})
	cp := *st
	return &cp, nil
}

// ReportInput — публикация отчёта прораба.
type ReportInput struct {
	Items    []string
	Comment  string
	PhotoIDs []string
}

// SubmitReport публикует отчёт: этап переходит в submitted, версия
// отчёта сохраняется, исправленный отчёт связывается с предыдущим.
func (s *Service) SubmitReport(ctx context.Context, actor project.UserID, stageID string, in ReportInput) (*Report, *Stage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.stage(stageID)
	if err != nil {
		return nil, nil, err
	}
	plan, ok := s.plans[st.PlanID]
	if !ok || string(actor) != plan.ForemanID {
		return nil, nil, apperr.Forbidden("отчёт публикует назначенный прораб")
	}
	if st.Status != StageInProgress && st.Status != StageChangesRequested {
		return nil, nil, apperr.InvalidTransition(fmt.Sprintf("отчёт можно сдать только на этапах %s или %s", StageInProgress, StageChangesRequested))
	}
	if len(in.Items) == 0 && strings.TrimSpace(in.Comment) == "" {
		return nil, nil, apperr.InvalidInput("заполните выполненные пункты или пояснение отчёта", nil)
	}
	report := &Report{
		ID:        id.New("rep"),
		StageID:   stageID,
		Version:   int64(len(s.reportsByStage[stageID]) + 1),
		Items:     in.Items,
		Comment:   in.Comment,
		PhotoIDs:  in.PhotoIDs,
		Author:    string(actor),
		CreatedAt: time.Now().UTC(),
	}
	s.reports[report.ID] = report
	s.reportsByStage[stageID] = append(s.reportsByStage[stageID], report.ID)
	st.CurrentReportID = report.ID
	st.Status = StageSubmitted
	st.Version++
	s.events.Append(st.ProjectID, "stage", "stage.report_submitted", string(actor),
		map[string]any{"stage_id": stageID, "report_id": report.ID, "report_version": report.Version})
	stcp := *st
	return report, &stcp, nil
}

// StageDecisionInput — решение владельца по конкретной версии отчёта
// (тело из раздела 7.5 технической спецификации).
type StageDecisionInput struct {
	ExpectedVersion     int64
	ReportID            string
	Decision            string // accept | request_changes
	AcceptanceConfirmed bool
	Comment             string
	IdempotencyKey      string
}

// StageDecision применяет решение: только владелец, только по текущей
// версии отчёта, приёмка требует явного подтверждения; прораб,
// мастер или посредник не могут принять этап.
func (s *Service) StageDecision(ctx context.Context, actor project.UserID, stageID string, in StageDecisionInput) (*Stage, error) {
	s.mu.Lock()
	st, err := s.stage(stageID)
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	cp := *st
	s.mu.Unlock()

	p, err := s.projects.GetForActor(ctx, actor, project.ProjectID(cp.ProjectID))
	if err != nil {
		return nil, err
	}
	if p.OwnerID != actor {
		return nil, apperr.Forbidden("этап принимает только владелец проекта")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st, _ = s.stage(stageID)
	if in.ExpectedVersion != st.Version {
		return nil, apperr.VersionConflict("ожидается версия " + strconv.FormatInt(in.ExpectedVersion, 10) + ", текущая " + strconv.FormatInt(st.Version, 10))
	}
	if st.Status != StageSubmitted {
		return nil, apperr.InvalidTransition("этап не сдан на приёмку")
	}
	reportID := in.ReportID
	if reportID == "" {
		reportID = st.CurrentReportID
	}
	if reportID != st.CurrentReportID {
		return nil, apperr.New("REPORT_CHANGED", "решение должно относиться к текущей версии отчёта", nil)
	}
	var target StageStatus
	switch in.Decision {
	case "accept":
		if !in.AcceptanceConfirmed {
			return nil, apperr.InvalidInput("требуется явное подтверждение приёмки: acceptance_confirmed=true", nil)
		}
		target = StageAccepted
	case "request_changes":
		if strings.TrimSpace(in.Comment) == "" {
			return nil, apperr.InvalidInput("укажите замечание (comment)", nil)
		}
		target = StageChangesRequested
	default:
		return nil, apperr.InvalidInput("decision должен быть accept или request_changes", nil)
	}
	if !CanStageTransition(st.Status, target) {
		return nil, apperr.InvalidTransition(fmt.Sprintf("переход %s -> %s недопустим", st.Status, target))
	}
	st.Status = target
	st.Version++
	s.decisions = append(s.decisions, StageDecisionRecord{
		ID:                  id.New("dec"),
		StageID:             stageID,
		ReportID:            reportID,
		Decision:            in.Decision,
		AcceptanceConfirmed: in.AcceptanceConfirmed,
		Comment:             in.Comment,
		Author:              string(actor),
		CreatedAt:           time.Now().UTC(),
	})
	s.events.Append(st.ProjectID, "stage", "stage."+string(target), string(actor),
		map[string]any{"stage_id": stageID, "report_id": reportID, "comment": in.Comment})
	out := *st
	return &out, nil
}

// StageReports возвращает историю версий отчётов этапа: старые версии
// сохраняются и не перезаписываются.
func (s *Service) StageReports(stageID string) []Report {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Report
	for _, rid := range s.reportsByStage[stageID] {
		out = append(out, *s.reports[rid])
	}
	return out
}

// GetStage возвращает этап по идентификатору.
func (s *Service) GetStage(stageID string) (*Stage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.stage(stageID)
	if err != nil {
		return nil, err
	}
	cp := *st
	return &cp, nil
}

// GetReview возвращает карточку проверки.
func (s *Service) GetReview(reviewID string) (*Review, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.review(reviewID)
	if err != nil {
		return nil, err
	}
	cp := *r
	return &cp, nil
}

func containsFold(items []string, needle string) bool {
	for _, it := range items {
		if strings.EqualFold(it, needle) {
			return true
		}
	}
	return false
}

func (s *Service) SetMaterialHash(fn func(string) string) { s.materialHash = fn }
func (s *Service) currentHash(p *project.Project, rooms []project.Room) string {
	h := inputHash(p, rooms)
	if s.materialHash != nil {
		h += s.materialHash(string(p.ID))
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(h)))
}
func (s *Service) ValidReview(ctx context.Context, pid, foreman string) bool {
	p, e := s.projects.InternalProject(ctx, project.ProjectID(pid))
	if e != nil {
		return false
	}
	rooms, _ := s.projects.RoomsOf(ctx, p.ID)
	hash := s.currentHash(p, rooms)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, v := range s.reviews {
		if v.ProjectID == pid && v.ForemanID == foreman && v.Status == ReviewApproved && v.InputHash == hash {
			return true
		}
	}
	return false
}
