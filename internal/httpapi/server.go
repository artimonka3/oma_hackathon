// Package httpapi — маршруты, сессии и конверты ошибок основного сервера.
//
// Обработчики намеренно тонкие: разобрать запрос, вызвать доменный сервис,
// записать ответ. Вся бизнес-логика (переходы состояний, проверка прав,
// пересчёты) находится в internal/project, internal/soberi, internal/prorab.
package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"

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
	AIModel       string
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
	Commerce      oma.Commerce
	State         *ApplicationState
	Save          func() error
	Static        http.Handler
	StorageMode   string
	Ready         func(context.Context) error
	AIMode        string
	ShopBaseURL   string
}

// Server — HTTP API основного Go-приложения.
type Server struct {
	cfg     Config
	mux     *http.ServeMux
	mu      sync.Mutex
	permits sync.Map
}

// NewServer собирает сервер и регистрирует маршруты API v1
// (раздел 7 технической спецификации).
func NewServer(cfg Config) *Server {
	if cfg.State == nil {
		cfg.State = &ApplicationState{Projects: map[string]*ProjectMeta{}}
	}
	if cfg.State.Projects == nil {
		cfg.State.Projects = map[string]*ProjectMeta{}
	}
	s := &Server{cfg: cfg, mux: http.NewServeMux()}
	s.routes()
	if u, e := url.Parse(cfg.ShopBaseURL); e == nil && u.Host != "" {
		proxy := http.StripPrefix("/shop", httputil.NewSingleHostReverseProxy(u))
		for _, method := range []string{"GET", "POST", "OPTIONS"} {
			s.mux.Handle(method+" /shop/", proxy)
		}
	}
	return s
}

// Handler возвращает цепочку middleware поверх маршрутизатора.
func (s *Server) Handler() http.Handler {
	return recoverer(requestID(logger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			s.mux.ServeHTTP(w, r)
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		buf := httptest.NewRecorder()
		s.mux.ServeHTTP(buf, r)
		if s.cfg.Save != nil && buf.Code < 400 {
			if err := s.cfg.Save(); err != nil {
				respondError(w, r, err)
				return
			}
		}
		for key, vals := range buf.Header() {
			for _, v := range vals {
				w.Header().Add(key, v)
			}
		}
		w.WriteHeader(buf.Code)
		_, _ = w.Write(buf.Body.Bytes())
	}))))
}

// auth оборачивает обработчик проверкой сессии.
func (s *Server) auth(next func(http.ResponseWriter, *http.Request, Session)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess, err := s.cfg.Sessions.Authenticate(r)
		if err != nil {
			respondError(w, r, err)
			return
		}

		pid := r.PathValue("project_id")
		if pid != "" && r.Method != "GET" && r.Method != "HEAD" {
			path := r.URL.Path
			material := strings.HasSuffix(path, "/room") || strings.HasSuffix(path, "/rooms") || strings.HasSuffix(path, "/inventory") || strings.HasSuffix(path, "/inventory-suggestions") || strings.HasSuffix(path, "/design-jobs") || strings.HasSuffix(path, "/design-selections") || strings.HasSuffix(path, "/specifications") || strings.HasSuffix(path, "/cart-sync") || strings.HasSuffix(path, "/cart") || r.Method == "PATCH" && path == "/api/v1/projects/"+pid
			if material {
				if _, err := s.owner(r, sess, pid); err != nil {
					respondError(w, r, err)
					return
				}
				m := s.meta(pid)
				if m.Order != "" || m.Finished {
					respondError(w, r, apperr.New("INVALID_TRANSITION", "Заказ уже оформлен. Для нового объёма создайте отдельный проект", nil))
					return
				}
			}
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

	if s.cfg.Static != nil {
		m.Handle("GET /", s.cfg.Static)
	}
	// Application endpoints reuse the existing domains.
	m.HandleFunc("GET /api/v1/projects", s.auth(s.handleProjects))
	m.HandleFunc("GET /api/v1/catalog", s.auth(s.handleCatalog))
	m.HandleFunc("GET /api/v1/catalog/{sku}", s.auth(s.handleCatalogProduct))
	m.HandleFunc("GET /api/v1/projects/{project_id}/application", s.auth(s.handleApplication))
	m.HandleFunc("PUT /api/v1/projects/{project_id}/room", s.auth(s.handleRoomPut))
	m.HandleFunc("PUT /api/v1/projects/{project_id}/inventory", s.auth(s.handleInventoryPut))
	m.HandleFunc("PATCH /api/v1/projects/{project_id}/cart", s.auth(s.handleSetPacks))
	m.HandleFunc("PUT /api/v1/projects/{project_id}/foreman", s.auth(s.handleAssign))
	m.HandleFunc("GET /api/v1/projects/{project_id}/analysis", s.auth(s.handleAnalysis))
	m.HandleFunc("POST /api/v1/projects/{project_id}/checkout", s.auth(s.handlePrepareCheckout))
	m.HandleFunc("POST /api/v1/projects/{project_id}/pay", s.auth(s.handlePay))
	m.HandleFunc("POST /api/v1/projects/{project_id}/demo-delivery", s.auth(s.handleDelivery))
	m.HandleFunc("POST /api/v1/projects/{project_id}/default-plan", s.auth(s.handleDefaultPlan))
	m.HandleFunc("POST /api/v1/projects/{project_id}/finish", s.auth(s.handleFinish))
	m.HandleFunc("GET /api/v1/media/{media_id}", s.auth(s.handleMedia))
	m.HandleFunc("GET /api/v1/config", s.handlePublicConfig)
	m.HandleFunc("GET /readyz", s.handleReady)
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
