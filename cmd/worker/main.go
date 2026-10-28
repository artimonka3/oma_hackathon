// Команда worker обрабатывает сохранённые задания (AI, подготовка медиа,
// доставка событий). В каркасе очередь живёт в памяти процесса, поэтому
// worker демонстрирует цикл обработки и health-сервер; после этапа A
// (PostgreSQL, раздел 10 ТЗ) он подключается к общей очереди app
// без изменения контрактов.
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"oma/internal/ai"
	"oma/internal/apperr"
	"oma/internal/jobs"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	addr := envOr("WORKER_ADDR", ":8081")

	queue := jobs.NewQueue()
	processor := jobs.NewProcessor(queue, map[string]jobs.Handler{
		"design_variants": func(ctx context.Context, job *jobs.Job) (any, error) {
			req, ok := job.Input.(ai.PrepareRequest)
			if !ok {
				return nil, apperr.InvalidInput("задание содержит некорректный вход", nil)
			}
			return ai.NewStub().PrepareVariants(ctx, req)
		},
	})
	go processor.Run(ctx, time.Second)

	// Демонстрационная задача: worker реально проходит цикл обработки.
	seed := queue.Enqueue("design_variants", "demo", ai.PrepareRequest{
		ProjectID: "demo",
		Goal:      "Демонстрация цикла worker при минимальном запуске",
	})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		job, _ := queue.Get(seed.ID)
		state := "unknown"
		if job != nil {
			state = string(job.State)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok", "service": "worker", "demo_job_state": state,
			"note": "общая очередь на PostgreSQL подключается на этапе A ТЗ",
		})
	})

	server := &http.Server{Addr: addr, Handler: mux}
	errCh := make(chan error, 1)
	go func() {
		log.Printf("worker: слушает %s, обрабатывает сохранённые задания", addr)
		errCh <- server.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
