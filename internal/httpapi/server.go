// Package httpapi — маршруты, сессии и конверты ошибок основного сервера.
//
// Обработчики намеренно тонкие: разобрать запрос, вызвать доменный сервис,
// записать ответ. Вся бизнес-логика (переходы состояний, проверка прав,
// пересчёты) находится в internal/project, internal/soberi, internal/prorab.
package httpapi

import (
	"net/http"

	"oma/internal/apperr"
	"oma/internal/events"
	"oma/internal/integrations/oma"
	"oma/internal/jobs"
	"oma/internal/media"
	"oma/internal/project"
	"oma/internal/prorab"
	"oma/internal/soberi"
)

func notImplementedErr(name string) error {
	return apperr.NotImplemented("операция «" + name + "» входит в следующие этапы ТЗ (раздел 10)")
}

// Config — зависимости сервера; границы модулей фиксируются здесь.
type Config struct {
	DemoMode      bool
	ServiceSecret string
	Sessions      *SessionStore
	Projects      *project.Service
	Soberi        *soberi.Service
	Prorab        *prorab.Service
	Oma           oma.Client
	Jobs          *jobs.Queue
	Events        *events.Log
	Media         media.Storage
}

// Server — HTTP API основного Go-приложения.
type Server struct {
	cfg Config
	mux *http.ServeMux
}

// NewServer собирает сервер и регистрирует маршруты API v1
// (раздел 7 технической спецификации).
func NewServer(cfg Config) *Server {
	s := &Server{cfg: cfg, mux: http.NewServeMux()}
	s.routes()
	return s
}

// Handler возвращает цепочку middleware поверх маршрутизатора.
func (s *Server) Handler() http.Handler {
	return recoverer(requestID(logger(s.mux)))
}

// auth оборачивает обработчик проверкой сессии.
func (s *Server) auth(next func(http.ResponseWriter, *http.Request, Session)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess, err := s.cfg.Sessions.Authenticate(r)
		if err != nil {
			respondError(w, r, err)
			return
		}
		next(w, r, sess)
	}
}

// stub регистрирует маршрут будущих этапов ТЗ с кодом NOT_IMPLEMENTED,
// чтобы поверхность API совпадала со спецификацией уже сейчас.
func (s *Server) stub(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		respondError(w, r, notImplementedErr(name))
	}
}

func (s *Server) routes() {
	m := s.mux

	// Служебные.
	m.HandleFunc("GET /healthz", s.handleHealth)
	m.HandleFunc("POST /internal/v1/oma/checkout-authorizations", s.handleCheckoutAuthorization)
	m.HandleFunc("POST /internal/v1/oma/events", s.handleOmaEvent)

	// Общие данные (7.2).
	m.HandleFunc("POST /api/v1/demo/session", s.handleDemoSession)
	m.HandleFunc("POST /api/v1/projects", s.auth(s.handleCreateProject))
	m.HandleFunc("GET /api/v1/projects/{project_id}", s.auth(s.handleGetProject))
	m.HandleFunc("PATCH /api/v1/projects/{project_id}", s.auth(s.handleUpdateProject))
	m.HandleFunc("POST /api/v1/projects/{project_id}/rooms", s.auth(s.handleAddRoom))
	m.HandleFunc("GET /api/v1/projects/{project_id}/rooms", s.auth(s.handleListRooms))
	m.HandleFunc("POST /api/v1/projects/{project_id}/captures", s.auth(s.handleAddCapture))
	m.HandleFunc("POST /api/v1/projects/{project_id}/inventory-suggestions", s.auth(s.handleInventorySuggestions))
	m.HandleFunc("POST /api/v1/projects/{project_id}/inventory", s.auth(s.handleAddInventory))
	m.HandleFunc("GET /api/v1/projects/{project_id}/inventory", s.auth(s.handleListInventory))
	m.HandleFunc("GET /api/v1/projects/{project_id}/events", s.auth(s.handleProjectEvents))

	// Собери (7.2).
	m.HandleFunc("POST /api/v1/projects/{project_id}/design-jobs", s.auth(s.handleStartDesignJob))
	m.HandleFunc("GET /api/v1/jobs/{job_id}", s.auth(s.handleGetJob))
	m.HandleFunc("POST /api/v1/projects/{project_id}/design-selections", s.auth(s.handleSelectDesign))
	m.HandleFunc("POST /api/v1/projects/{project_id}/specifications", s.auth(s.handleCalculateSpec))
	m.HandleFunc("POST /api/v1/projects/{project_id}/cart-sync", s.auth(s.handleCartSync))

	// Прораб: подбор и проверка перед оплатой (7.3).
	m.HandleFunc("GET /api/v1/foremen", s.auth(s.handleListForemen))
	m.HandleFunc("GET /api/v1/foremen/{foreman_id}", s.auth(s.handleGetForeman))
	m.HandleFunc("POST /api/v1/projects/{project_id}/purchase-preflight", s.auth(s.handlePurchasePreflight))
	m.HandleFunc("POST /api/v1/projects/{project_id}/reviews", s.auth(s.handleCreateReview))
	m.HandleFunc("GET /api/v1/projects/{project_id}/reviews", s.auth(s.handleListReviews))
	m.HandleFunc("POST /api/v1/reviews/{review_id}/decisions", s.auth(s.handleReviewDecision))
	m.HandleFunc("POST /api/v1/reviews/{review_id}/proposal-acceptance", s.auth(s.handleAcceptProposal))

	// Прораб: план, отчёты и приёмка (7.3).
	m.HandleFunc("POST /api/v1/projects/{project_id}/work-plans", s.auth(s.handleCreateWorkPlan))
	m.HandleFunc("GET /api/v1/projects/{project_id}/work-plans", s.auth(s.handleListWorkPlans))
	m.HandleFunc("POST /api/v1/work-plans/{plan_id}/confirmation", s.auth(s.handleConfirmWorkPlan))
	m.HandleFunc("POST /api/v1/stages/{stage_id}/start", s.auth(s.handleStartStage))
	m.HandleFunc("POST /api/v1/stages/{stage_id}/reports", s.auth(s.handleSubmitReport))
	m.HandleFunc("GET /api/v1/stages/{stage_id}/reports", s.auth(s.handleListStageReports))
	m.HandleFunc("POST /api/v1/stages/{stage_id}/decisions", s.auth(s.handleStageDecision))

	// Маршруты следующих этапов ТЗ (раздел 10): обозначены в API,
	// возвращают NOT_IMPLEMENTED с понятным сообщением.
	for _, rt := range [][2]string{
		{"POST /api/v1/projects/{project_id}/engagements", "назначение исполнителя"},
		{"POST /api/v1/engagements/{engagement_id}/confirmation", "подтверждение назначения"},
		{"POST /api/v1/projects/{project_id}/changes", "изменение объёма работ"},
		{"POST /api/v1/changes/{change_id}/decisions", "решение по изменению"},
		{"POST /api/v1/projects/{project_id}/disputes", "обращение к посреднику"},
		{"POST /api/v1/disputes/{dispute_id}/messages", "сообщение в обращении"},
		{"POST /api/v1/disputes/{dispute_id}/proposals", "предложение урегулирования"},
		{"POST /api/v1/disputes/{dispute_id}/decisions", "решение по урегулированию"},
		{"POST /api/v1/projects/{project_id}/completion", "завершение проекта"},
	} {
		m.HandleFunc(rt[0], s.stub(rt[1]))
	}
}
