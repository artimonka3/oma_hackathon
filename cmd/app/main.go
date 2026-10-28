// Команда app запускает основной Go-сервер ОМА: общие данные (Project),
// ОМА.Собери, ОМА.Прораб и /internal маршруты для магазина.
//
// Минимальный запуск без реального API ОМА:
//
//	OMA_API_BASE_URL=http://localhost:8090 go run ./cmd/app   # с симулятором Mock OMA
//	go run ./cmd/app                                            # со встроенным Fake
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"oma/fixtures"
	"oma/internal/ai"
	"oma/internal/checkpoint"
	"oma/internal/events"
	"oma/internal/httpapi"
	"oma/internal/integrations/oma"
	"oma/internal/jobs"
	"oma/internal/media"
	"oma/internal/mockoma"
	"oma/internal/project"
	"oma/internal/prorab"
	"oma/internal/soberi"
	webui "oma/web"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

type config struct {
	Addr             string
	OMA_APIBaseURL   string
	MediaStoragePath string
	DemoMode         bool
	ServiceSecret    string
	AIMode           string
}

func loadConfig() config {
	return config{
		Addr:             envOr("APP_ADDR", ":8080"),
		OMA_APIBaseURL:   os.Getenv("OMA_API_BASE_URL"),
		MediaStoragePath: envOr("MEDIA_STORAGE_PATH", "./var/media"),
		DemoMode:         envOr("DEMO_MODE", "true") == "true",
		ServiceSecret:    os.Getenv("OMA_SERVICE_SECRET"),
		AIMode:           envOr("AI_MODE", "demo"),
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := loadConfig()

	// Границы модулей фиксируются один раз; обработчики и домены
	// получают только нужные им зависимости.
	eventLog := events.NewLog()
	repo := project.NewMemoryRepo()
	projectSvc := project.NewService(repo, eventLog)
	commerce := mockoma.New(mockoma.Options{Products: fixtures.Products()})

	var shop oma.Client
	var merchant oma.Commerce = commerce
	if cfg.OMA_APIBaseURL != "" {
		client, err := oma.NewHTTPClient(cfg.OMA_APIBaseURL)
		if err != nil {
			return err
		}
		shop = client
		merchant = client
		log.Printf("oma: магазин через HTTP API %s (Mock OMA или реальное API ОМА)", cfg.OMA_APIBaseURL)
	} else {
		shop = mockoma.Adapter{Service: commerce}
		log.Printf("oma: реальный API не задан — используется встроенный симулятор (минимальный запуск)")
	}

	queue := jobs.NewQueue()
	provider, err := ai.FromEnv()
	if err != nil {
		return err
	}
	soberiSvc := soberi.NewService(projectSvc, shop, queue, provider, eventLog)
	prorabSvc := prorab.NewService(projectSvc, eventLog, fixtures.Foremen())
	prorabSvc.SetMaterialHash(soberiSvc.ReviewHash)
	sessions := httpapi.NewSessionStore()
	appState := &httpapi.ApplicationState{Projects: map[string]*httpapi.ProjectMeta{}}
	store := &checkpoint.Store{Path: envOr("OMA_STATE_PATH", "./var/state.json"), Components: map[string]checkpoint.Component{"projects": repo, "soberi": soberiSvc, "prorab": prorabSvc, "events": eventLog, "jobs": queue, "commerce": commerce, "sessions": sessions}, Extra: appState}
	save := store.Save
	storageMode := "file"
	var ready func(context.Context) error
	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		store.Path = os.Getenv("OMA_IMPORT_STATE_PATH")
		db, err := checkpoint.OpenPostgres(ctx, databaseURL, store)
		if err != nil {
			return err
		}
		defer db.Close()
		if err = db.Load(); err != nil {
			return err
		}
		save = func() error {
			if err := db.Save(); err != nil {
				log.Printf("PostgreSQL: сохранение не удалось, перезапуск для восстановления состояния")
				stop()
				return err
			}
			return nil
		}
		ready = db.Ping
		storageMode = "postgres"
	} else if err := store.Load(); err != nil {
		return err
	}

	if remote, ok := shop.(*oma.HTTPClient); ok {
		storageReady := ready
		ready = func(ctx context.Context) error {
			if storageReady != nil {
				if e := storageReady(ctx); e != nil {
					return e
				}
			}
			return remote.Ready(ctx)
		}
	}
	// Обработка сохранённых заданий в этом же процессе; cmd/worker
	// подключается к общей очереди после этапа A (PostgreSQL).
	processor := jobs.NewProcessor(queue, map[string]jobs.Handler{
		"design_variants": soberiSvc.HandleDesignJob,
	})
	go processor.Run(ctx, 300*time.Millisecond)

	storage, err := media.NewLocalStorage(cfg.MediaStoragePath)
	if err != nil {
		return err
	}

	srv := httpapi.NewServer(httpapi.Config{
		DemoMode:      cfg.DemoMode,
		ServiceSecret: cfg.ServiceSecret,
		Sessions:      sessions,
		State:         appState, Save: save, Static: webui.Handler(),
		Projects:    projectSvc,
		Soberi:      soberiSvc,
		Prorab:      prorabSvc,
		Oma:         shop,
		Jobs:        queue,
		Events:      eventLog,
		Media:       storage,
		Commerce:    merchant,
		AIModel:     envOr("AI_MODEL", ai.DefaultModel),
		StorageMode: storageMode, Ready: ready, AIMode: cfg.AIMode, ShopBaseURL: cfg.OMA_APIBaseURL,
	})

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("app: HTTP API слушает %s (demo_mode=%v, ai_mode=%s)", cfg.Addr, cfg.DemoMode, cfg.AIMode)
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	}
}
