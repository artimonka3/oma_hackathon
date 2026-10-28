// Package prorab — домен ОМА.Прораб: исполнители, проверки комплекта,
// планы работ, отчёты, решения и приёмка (раздел 5 продуктовой спецификации).
// Все переходы состояний проверяются доменом; AI и обработчики не меняют
// состояния напрямую.
package prorab

import "time"

// Foreman — профиль исполнителя. Демо-профили помечены признаком demo.
type Foreman struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Specialities []string `json:"specialities"`
	Region       string   `json:"region"`
	Availability string   `json:"availability"`
	TeamSize     int      `json:"team_size"`
	Conditions   string   `json:"conditions"`
	Verified     bool     `json:"verified"`
	Demo         bool     `json:"demo"`
}

// ReviewStatus — состояние проверки комплекта (раздел 3 ТЗ).
type ReviewStatus string

const (
	ReviewQueued          ReviewStatus = "queued"
	ReviewInReview        ReviewStatus = "in_review"
	ReviewNeedsInfo       ReviewStatus = "needs_info"
	ReviewNeedsVisit      ReviewStatus = "needs_visit"
	ReviewChangesProposed ReviewStatus = "changes_proposed"
	ReviewApproved        ReviewStatus = "approved"
	ReviewDeclined        ReviewStatus = "declined"
	ReviewExpired         ReviewStatus = "expired"
	ReviewSuperseded      ReviewStatus = "superseded"
	ReviewCancelled       ReviewStatus = "cancelled"
)

var reviewTransitions = map[ReviewStatus][]ReviewStatus{
	ReviewQueued:          {ReviewInReview, ReviewCancelled},
	ReviewInReview:        {ReviewNeedsInfo, ReviewNeedsVisit, ReviewChangesProposed, ReviewApproved, ReviewDeclined, ReviewExpired, ReviewSuperseded},
	ReviewNeedsInfo:       {ReviewInReview, ReviewCancelled},
	ReviewNeedsVisit:      {ReviewInReview, ReviewCancelled},
	ReviewChangesProposed: {ReviewApproved, ReviewSuperseded, ReviewCancelled},
	ReviewApproved:        {ReviewSuperseded},
	ReviewDeclined:        {},
	ReviewExpired:         {},
	ReviewSuperseded:      {},
	ReviewCancelled:       {},
}

// CanReviewTransition проверяет допустимость перехода проверки.
func CanReviewTransition(from, to ReviewStatus) bool {
	for _, t := range reviewTransitions[from] {
		if t == to {
			return true
		}
	}
	return false
}

// StageStatus — состояние этапа работ (раздел 3 ТЗ).
type StageStatus string

const (
	StagePlanned          StageStatus = "planned"
	StageInProgress       StageStatus = "in_progress"
	StageSubmitted        StageStatus = "submitted"
	StageChangesRequested StageStatus = "changes_requested"
	StageAccepted         StageStatus = "accepted"
	StagePaused           StageStatus = "paused"
	StageCancelled        StageStatus = "cancelled"
)

var stageTransitions = map[StageStatus][]StageStatus{
	StagePlanned:          {StageInProgress, StagePaused, StageCancelled},
	StageInProgress:       {StageSubmitted, StagePaused, StageCancelled},
	StageSubmitted:        {StageAccepted, StageChangesRequested, StagePaused, StageCancelled},
	StageChangesRequested: {StageSubmitted, StageCancelled},
	StagePaused:           {StageInProgress, StageCancelled},
	StageAccepted:         {},
	StageCancelled:        {},
}

// CanStageTransition проверяет допустимость перехода этапа.
func CanStageTransition(from, to StageStatus) bool {
	for _, t := range stageTransitions[from] {
		if t == to {
			return true
		}
	}
	return false
}

// Proposal — точная предложенная версия состава с подтверждением для неё.
type Proposal struct {
	Comment      string   `json:"comment"`
	ChangedSKUs  []string `json:"changed_skus"`
	ProposedHash string   `json:"proposed_hash"`
}

// Review — адресная карточка проверки комплекта: одна активная карточка
// на выбранную область и исполнителя.
type Review struct {
	ID        string       `json:"id"`
	ProjectID string       `json:"project_id"`
	ForemanID string       `json:"foreman_id"`
	Scope     string       `json:"scope"`
	Status    ReviewStatus `json:"status"`
	InputHash string       `json:"review_input_hash"`
	HashRules string       `json:"hash_rules"`
	Proposal  *Proposal    `json:"proposal,omitempty"`
	Comment   string       `json:"comment"`
	Version   int64        `json:"version"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

// Stage — этап работ действующего плана.
type Stage struct {
	ID              string      `json:"id"`
	ProjectID       string      `json:"project_id"`
	PlanID          string      `json:"plan_id"`
	RoomID          string      `json:"room_id"`
	Title           string      `json:"title"`
	Required        bool        `json:"required"`
	Weight          int         `json:"weight"`
	Status          StageStatus `json:"status"`
	CurrentReportID string      `json:"current_report_id"`
	Version         int64       `json:"version"`
}

// Report — версия отчёта прораба; привязан к этапу, хранит автора и версию.
type Report struct {
	ID        string    `json:"id"`
	StageID   string    `json:"stage_id"`
	Version   int64     `json:"version"`
	Items     []string  `json:"items"`
	Comment   string    `json:"comment"`
	PhotoIDs  []string  `json:"photo_ids"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"created_at"`
}

// WorkPlan — предложение плана работ. Становится действующим после
// подтверждения владельцем.
type WorkPlan struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	ForemanID string    `json:"foreman_id"`
	StageIDs  []string  `json:"stage_ids"`
	Confirmed bool      `json:"confirmed"`
	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
}
