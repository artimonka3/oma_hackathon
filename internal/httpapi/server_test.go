package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestShopProxyRoutesRegisterWithStaticUI(t *testing.T) {
	s := NewServer(Config{DemoMode: true, Static: http.NotFoundHandler(), ShopBaseURL: "http://127.0.0.1:8090"})
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/config", nil))
	if w.Code != 200 {
		t.Fatalf("config: %d %s", w.Code, w.Body.String())
	}
}
