package httpapi

import (
	"context"
	"log"
	"net/http"
	"time"

	"oma/internal/id"
)

type ctxKey int

const ctxRequestID ctxKey = 0

// requestID назначает идентификатор запроса и возвращает его в заголовке.
func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := r.Header.Get("X-Request-Id")
		if rid == "" {
			rid = id.New("req")
		}
		w.Header().Set("X-Request-Id", rid)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxRequestID, rid)))
	})
}

func requestIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxRequestID).(string); ok {
		return v
	}
	return ""
}

// logger фиксирует вызовы API.
func logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s (%s)", r.Method, r.URL.Path, time.Since(start))
	})
}

// recoverer не даёт панике оборвать соединение без конверта ошибки.
func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("httpapi: panic: %v", rec)
				writeJSON(w, http.StatusInternalServerError, map[string]any{
					"code":       "INTERNAL",
					"message":    "внутренняя ошибка сервера",
					"details":    map[string]any{},
					"request_id": requestIDFrom(r.Context()),
				})
			}
		}()
		next.ServeHTTP(w, r)
	})
}
