package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dundeezhang/replay-test-kvcache/internal/config"
	"github.com/dundeezhang/replay-test-kvcache/internal/store"
)

func newTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	cfg := config.Default()
	cfg.MaxValueBytes = 64
	st := store.New(store.Options{
		Shards:        4,
		MaxKeyBytes:   cfg.MaxKeyBytes,
		MaxValueBytes: cfg.MaxValueBytes,
	})
	srv := New(st, cfg, nil)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return srv, ts
}

func do(t *testing.T, method, url, body string) (*http.Response, string) {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, string(b)
}

func TestHealth(t *testing.T) {
	_, ts := newTestServer(t)
	resp, body := do(t, http.MethodGet, ts.URL+"/healthz", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if !strings.Contains(body, `"status":"ok"`) {
		t.Fatalf("body = %s", body)
	}
}

func TestKeyLifecycle(t *testing.T) {
	_, ts := newTestServer(t)
	url := ts.URL + "/v1/keys/color"

	if resp, _ := do(t, http.MethodGet, url, ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET missing = %d, want 404", resp.StatusCode)
	}

	resp, _ := do(t, http.MethodPut, url, "teal")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT = %d, want 204", resp.StatusCode)
	}
	if got := resp.Header.Get("ETag"); got != `"1"` {
		t.Fatalf("PUT ETag = %s, want \"1\"", got)
	}

	resp, body := do(t, http.MethodGet, url, "")
	if resp.StatusCode != http.StatusOK || body != "teal" {
		t.Fatalf("GET = %d %q", resp.StatusCode, body)
	}

	if resp, _ := do(t, http.MethodDelete, url, ""); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE = %d, want 204", resp.StatusCode)
	}
	if resp, _ := do(t, http.MethodDelete, url, ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("second DELETE = %d, want 404", resp.StatusCode)
	}
}

func TestSetRejectsOversizedValue(t *testing.T) {
	_, ts := newTestServer(t)
	resp, _ := do(t, http.MethodPut, ts.URL+"/v1/keys/big", strings.Repeat("x", 65))
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("PUT oversized = %d, want 413", resp.StatusCode)
	}
}

func TestListKeys(t *testing.T) {
	_, ts := newTestServer(t)
	for _, k := range []string{"a:1", "a:2", "b:1"} {
		do(t, http.MethodPut, ts.URL+"/v1/keys/"+k, "v")
	}

	resp, body := do(t, http.MethodGet, ts.URL+"/v1/keys?prefix=a:", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list = %d", resp.StatusCode)
	}
	var out struct{ Keys []string }
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Keys) != 2 || out.Keys[0] != "a:1" || out.Keys[1] != "a:2" {
		t.Fatalf("keys = %v", out.Keys)
	}

	if resp, _ := do(t, http.MethodGet, ts.URL+"/v1/keys?limit=-1", ""); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad limit = %d, want 400", resp.StatusCode)
	}
}
