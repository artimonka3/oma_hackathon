package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestChatProviderContractAndValidation(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		status        int
		ok            bool
	}{{"valid", `{"variants":[{"id":"v1","title":"Белые стены","summary":"Матовая краска","skus":["PAINT-MATT-WHITE"]}],"questions":[]}`, 200, true}, {"invented-sku", `{"variants":[{"id":"v1","title":"Нет в каталоге","skus":["INVENTED"]}]}`, 200, false}, {"empty", "{\"variants\":[]}", 200, false}, {"invalid-json", "hello", 200, false}, {"upstream-error", "secret-key", 401, false}} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer test-key" {
					t.Error("bad endpoint/auth")
				}
				var body map[string]any
				json.NewDecoder(r.Body).Decode(&body)
				if body["model"] != DefaultModel || body["max_tokens"] != float64(4096) || body["reasoning_effort"] != "low" {
					t.Error("bad cost controls")
				}
				w.WriteHeader(tc.status)
				if tc.status != 200 {
					w.Write([]byte(tc.content))
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": tc.content}, "finish_reason": "stop"}}})
			}))
			defer srv.Close()
			provider, e := NewChatProvider(srv.URL+"/v1", "test-key", DefaultModel, 2*time.Second, 4096)
			if e != nil {
				t.Fatal(e)
			}
			draft, e := provider.PrepareVariants(context.Background(), PrepareRequest{Goal: "Покрасить стены", AllowedSKUs: []string{"PAINT-MATT-WHITE"}})
			if (e == nil) != tc.ok {
				t.Fatalf("got %v", e)
			}
			if tc.ok && (draft.Mode != "live" || draft.Model != DefaultModel) {
				t.Fatal(draft)
			}
			if e != nil && strings.Contains(e.Error(), "secret-key") {
				t.Fatal("upstream body leaked")
			}
			if calls != 1 {
				t.Fatal("unexpected billed retry")
			}
		})
	}
}
func TestLiveRequiresKey(t *testing.T) {
	t.Setenv("AI_MODE", "live")
	t.Setenv("AI_API_KEY", "")
	if _, e := FromEnv(); e == nil {
		t.Fatal("live mode without a key")
	}
}
