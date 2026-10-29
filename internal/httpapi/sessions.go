package httpapi

import (
	"net/http"
	"strings"
	"sync"

	"oma/internal/apperr"
	"oma/internal/id"
	"oma/internal/project"
)

// Демо-участники минимального запуска: смена ролей работает только
// при DEMO_MODE (раздел 9 ТЗ); в обычном развёртывании требуется
// нормальная сессия и подключение торговых учётных записей.
const (
	ParticipantClient   = "client"
	ParticipantForeman  = "foreman"
	ParticipantMediator = "mediator"
)

// Session — явно демонстрационная сессия выбранного тестового участника.
type Session struct {
	Token       string
	UserID      string
	Role        project.Role
	DisplayName string
}

var demoParticipants = map[string]Session{
	ParticipantClient:   {UserID: "client_demo", Role: project.RoleOwner, DisplayName: "Демо-клиент"},
	ParticipantForeman:  {UserID: "foreman_demo_01", Role: project.RoleForeman, DisplayName: "Демо-прораб"},
	ParticipantMediator: {UserID: "mediator_demo", Role: project.RoleMediator, DisplayName: "Демо-посредник"},
}

// SessionStore — хранилище активных демо-сессий.
type SessionStore struct {
	mu      sync.RWMutex
	byToken map[string]Session
}

// NewSessionStore создаёт пустое хранилище сессий.
func NewSessionStore() *SessionStore { return &SessionStore{byToken: map[string]Session{}} }

// Create создаёт демо-сессию выбранного участника.
func (s *SessionStore) Create(participant string) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	base, ok := demoParticipants[strings.TrimSpace(participant)]
	if !ok && strings.HasPrefix(participant, "foreman_demo_0") && (participant >= "foreman_demo_01" && participant <= "foreman_demo_08" && len(participant) == len("foreman_demo_01")) {
		base = Session{UserID: participant, Role: project.RoleForeman, DisplayName: "Демо-прораб"}
		ok = true
	}
	if !ok {
		return Session{}, apperr.InvalidInput(
			"неизвестный участник; доступны client, foreman, mediator", nil)
	}
	sess := base
	sess.Token = id.New("sess")
	s.byToken[sess.Token] = sess
	return sess, nil
}

// Authenticate разрешает сессию по заголовку Authorization: Bearer.
func (s *SessionStore) Authenticate(r *http.Request) (Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return Session{}, apperr.Forbidden("требуется заголовок Authorization: Bearer <token> (создайте POST /api/v1/demo/session)")
	}
	token := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	sess, ok := s.byToken[token]
	if !ok {
		return Session{}, apperr.Forbidden("сессия не найдена")
	}
	return sess, nil
}
