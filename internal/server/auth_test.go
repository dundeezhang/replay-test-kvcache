package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dundeezhang/replay-test-kvcache/internal/config"
	"github.com/dundeezhang/replay-test-kvcache/internal/store"
)

func TestAuthEnforcedWhenTokenConfigured(t *testing.T) {
	cfg := config.Default()
	cfg.AuthToken = "test-token"
	srv := New(store.New(store.Options{Shards: 4}), cfg, nil)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	if resp, _ := do(t, http.MethodGet, ts.URL+"/healthz", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("/healthz without token = %d, want 200", resp.StatusCode)
	}
	if resp, _ := do(t, http.MethodGet, ts.URL+"/v1/stats", ""); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("/v1/stats without token = %d, want 401", resp.StatusCode)
	}

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/v1/stats", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer test-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/v1/stats with token = %d, want 200", resp.StatusCode)
	}
}
