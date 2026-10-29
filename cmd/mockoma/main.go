// Команда mockoma запускает симулятор HTTP API магазина ОМА — отдельный
// процесс с отдельным хранилищем торговых данных (раздел 1.1 ТЗ).
// Реальное API ОМА отсутствует: каталог, история, корзины, котировки,
// оплата-симуляция и доставка помечены признаком demo.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"oma/fixtures"
	"oma/internal/apperr"
	"oma/internal/checkpoint"
	"oma/internal/integrations/oma"
	"oma/internal/mockoma"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	addr := envOr("MOCKOMA_ADDR", ":8090")
	appBase := os.Getenv("OMA_APP_BASE_URL") // например http://localhost:8080; пусто — без обратных вызовов
	secret := os.Getenv("OMA_SERVICE_SECRET")

	// MOCKOMA_PRODUCTS_FILE — каталог из файла (например, скрап oma-site/data/products.json);
	// без переменной используется демонстрационный fixtures.Products().
	products, productSource, err := loadProducts(os.Getenv("MOCKOMA_PRODUCTS_FILE"))
	if err != nil {
		return err
	}

	svc := mockoma.New(mockoma.Options{
		Products:       products,
		LegacyProducts: fixtures.Products(),
		AppBaseURL:     appBase,
		ServiceSecret:  secret,
	})
	source := &checkpoint.Store{Path: os.Getenv("MOCKOMA_STATE_PATH"), Namespace: "oma_shop", ImportNamespace: "public", Components: map[string]checkpoint.Component{"commerce": svc}, Extra: &map[string]any{}}
	var save func() error
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		db, e := checkpoint.OpenPostgres(ctx, dsn, source)
		if e != nil {
			return e
		}
		defer db.Close()
		if e = db.Load(); e != nil {
			return e
		}
		save = db.Save
	} else if source.Path != "" {
		if e := source.Load(); e != nil {
			return e
		}
		save = source.Save
	}
	mux := http.NewServeMux()
	registerRoutes(mux, svc)
	registerSite(mux, os.Getenv("MOCKOMA_SITE_DIR"))

	server := &http.Server{Addr: addr, Handler: enableCORS(logRequests(persistShop(mux, save))), ReadHeaderTimeout: 5 * time.Second}
	errCh := make(chan error, 1)
	go func() {
		log.Printf("mockoma: симулятор API магазина слушает %s (callbacks=%s, каталог=%s, %d товаров)",
			addr, appBaseOr(appBase), productSource, len(products))
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

func appBaseOr(v string) string {
	if v == "" {
		return "off"
	}
	return v
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// loadProducts читает каталог из JSON-файла либо возвращает демонстрационные фикстуры.
func loadProducts(path string) ([]oma.Product, string, error) {
	if path == "" {
		return fixtures.Products(), "fixtures", nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("mockoma: каталог: %w", err)
	}
	var products []oma.Product
	if err := json.Unmarshal(raw, &products); err != nil {
		return nil, "", fmt.Errorf("mockoma: каталог %s: %w", path, err)
	}
	if len(products) == 0 {
		return nil, "", fmt.Errorf("mockoma: каталог %s пуст", path)
	}
	return products, path, nil
}

// registerSite раздаёт статичную витрину (oma-site/site), если каталог задан.
func registerSite(mux *http.ServeMux, dir string) {
	if dir == "" {
		return
	}
	fs := http.FileServer(http.Dir(dir))
	mux.Handle("GET /", fs)
}

// enableCORS разрешает кросс-доменные запросы к витрине, открытой отдельно от API.
func enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s (%s)", r.Method, r.URL.Path, time.Since(start))
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func respondErr(w http.ResponseWriter, err error) {
	var ae *apperr.Error
	if !errors.As(err, &ae) {
		ae = &apperr.Error{Code: "INTERNAL", Message: "внутренняя ошибка симулятора"}
	}
	status := http.StatusInternalServerError
	switch ae.Code {
	case "INVALID_INPUT":
		status = http.StatusBadRequest
	case "NOT_FOUND":
		status = http.StatusNotFound
	case "QUOTE_EXPIRED", "CHECKOUT_DECLINED", "INVALID_TRANSITION":
		status = http.StatusConflict
	}
	writeJSON(w, status, map[string]any{"code": ae.Code, "message": ae.Message})
}

func registerRoutes(mux *http.ServeMux, svc *mockoma.Service) {
	mux.HandleFunc("GET /oma-api/v1/projects/{project_id}/cart", func(w http.ResponseWriter, r *http.Request) {
		v, e := svc.CartForProject(r.PathValue("project_id"))
		if e != nil {
			respondErr(w, e)
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("GET /oma-api/v1/quotes/{quote_id}", func(w http.ResponseWriter, r *http.Request) {
		v, e := svc.Quote(r.PathValue("quote_id"))
		if e != nil {
			respondErr(w, e)
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("GET /oma-api/v1/checkout-sessions/{checkout_id}", func(w http.ResponseWriter, r *http.Request) {
		v, e := svc.Checkout(r.PathValue("checkout_id"))
		if e != nil {
			respondErr(w, e)
			return
		}
		writeJSON(w, 200, v)
	})

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": "mock-oma", "demo": true})
	})
	mux.HandleFunc("GET /oma-api/v1/products", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"products": svc.Search(r.URL.Query().Get("q"), r.URL.Query().Get("category")),
			"demo":     true,
		})
	})
	mux.HandleFunc("GET /oma-api/v1/products/{sku}", func(w http.ResponseWriter, r *http.Request) {
		p, err := svc.Get(r.PathValue("sku"))
		if err != nil {
			respondErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, p)
	})
	mux.HandleFunc("GET /oma-api/v1/categories", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"categories": svc.Categories(), "demo": true})
	})
	mux.HandleFunc("GET /oma-api/v1/purchase-history", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"items": svc.History(r.URL.Query().Get("account")), "demo": true})
	})
	mux.HandleFunc("POST /oma-api/v1/carts/sync", func(w http.ResponseWriter, r *http.Request) {
		var req oma.SyncRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondErr(w, apperr.InvalidInput("некорректное тело запроса: "+err.Error(), nil))
			return
		}
		res, err := svc.SyncCart(req)
		if err != nil {
			respondErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("GET /oma-api/v1/carts/{cart_id}", func(w http.ResponseWriter, r *http.Request) {
		cart, err := svc.GetCart(r.PathValue("cart_id"))
		if err != nil {
			respondErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, cart)
	})
	mux.HandleFunc("POST /oma-api/v1/carts/{cart_id}/quotes", func(w http.ResponseWriter, r *http.Request) {
		quote, err := svc.CreateQuote(r.PathValue("cart_id"))
		if err != nil {
			respondErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, quote)
	})
	mux.HandleFunc("POST /oma-api/v1/checkout-sessions", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			QuoteID string `json:"quote_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.QuoteID == "" {
			respondErr(w, apperr.InvalidInput("укажите quote_id", nil))
			return
		}
		sess, err := svc.CreateCheckout(in.QuoteID)
		if err != nil {
			respondErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, sess)
	})
	mux.HandleFunc("POST /oma-api/v1/checkout-sessions/{session_id}/pay", func(w http.ResponseWriter, r *http.Request) {
		order, err := svc.Pay(r.Context(), r.PathValue("session_id"))
		if err != nil {
			respondErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, order)
	})
	mux.HandleFunc("GET /oma-api/v1/orders/{order_id}", func(w http.ResponseWriter, r *http.Request) {
		order, err := svc.GetOrder(r.PathValue("order_id"))
		if err != nil {
			respondErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, order)
	})
	mux.HandleFunc("POST /oma-api/v1/demo/orders/{order_id}/events", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Action  string `json:"action"`
			Comment string `json:"comment"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			respondErr(w, apperr.InvalidInput("некорректное тело запроса", nil))
			return
		}
		order, err := svc.DemoOrderEvent(r.Context(), r.PathValue("order_id"), in.Action, in.Comment)
		if err != nil {
			respondErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, order)
	})
}

func persistShop(next http.Handler, save func() error) http.Handler {
	var mu sync.Mutex
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" || !strings.HasPrefix(r.URL.Path, "/oma-api/") {
			next.ServeHTTP(w, r)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		buf := httptest.NewRecorder()
		next.ServeHTTP(buf, r)
		if save != nil && buf.Code < 400 {
			if e := save(); e != nil {
				writeJSON(w, 503, map[string]string{"code": "STORAGE_UNAVAILABLE", "message": "Магазин не смог сохранить операцию"})
				return
			}
		}
		for k, v := range buf.Header() {
			w.Header()[k] = v
		}
		w.WriteHeader(buf.Code)
		w.Write(buf.Body.Bytes())
	})
}
