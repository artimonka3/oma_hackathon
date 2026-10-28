// Package project — домен общих данных продукта: пользователи, проекты,
// помещения, медиа-связи, инвентарь и версии (раздел 2.2 технической
// спецификации). Бизнес-правила живут в Service; обработчики HTTP
// только разбирают запрос и возвращают ответ.
package project

import "time"

// UserID — идентификатор пользователя.
type UserID string

// ProjectID — идентификатор проекта.
type ProjectID string

// RoomID — идентификатор помещения.
type RoomID string

// Role — роль участника проекта.
type Role string

const (
	RoleOwner    Role = "owner"
	RoleForeman  Role = "foreman"
	RoleMaster   Role = "master"
	RoleMediator Role = "mediator"
)

// Participant — участник проекта с ролью.
type Participant struct {
	UserID UserID `json:"user_id"`
	Role   Role   `json:"role"`
}

// ExecutionMode — режим исполнения проекта.
const (
	ModeSelf    = "self"
	ModeForeman = "foreman"
)

// Project — агрегат проекта: одна задача от запроса до истории.
type Project struct {
	ID            ProjectID     `json:"id"`
	OwnerID       UserID        `json:"owner_id"`
	Goal          string        `json:"goal"`
	BudgetMinor   int64         `json:"budget_minor"`
	Currency      string        `json:"currency"`
	ExecutionMode string        `json:"execution_mode"` // self | foreman
	Participants  []Participant `json:"participants"`
	RoomIDs       []RoomID      `json:"room_ids"`
	Version       int64         `json:"version"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

// Room — помещение проекта. Размеры заданы в миллиметрах.
type Room struct {
	ID                  RoomID    `json:"id"`
	ProjectID           ProjectID `json:"project_id"`
	Name                string    `json:"name"`
	WidthMM             int       `json:"width_mm"`
	LengthMM            int       `json:"length_mm"`
	HeightMM            int       `json:"height_mm"`
	DimensionsConfirmed bool      `json:"dimensions_confirmed"`
}
