package server

import (
	"net/http"
	"strings"
	"testing"
)

func TestMetricsEndpoint(t *testing.T) {
	_, ts := newTestServer(t)
	do(t, http.MethodPut, ts.URL+"/v1/keys/a", "1")
	do(t, http.MethodGet, ts.URL+"/v1/keys/a", "")
	do(t, http.MethodGet, ts.URL+"/v1/keys/missing", "")

	resp, body := do(t, http.MethodGet, ts.URL+"/metrics", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /metrics = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain; version=0.0.4") {
		t.Fatalf("Content-Type = %q", ct)
	}

	for _, want := range []string{
		`kvcache_http_requests_total{method="PUT",path="/v1/keys/a",code="204"} 1`,
		`kvcache_http_requests_total{method="GET",path="/v1/keys/a",code="200"} 1`,
		`kvcache_http_requests_total{method="GET",path="/v1/keys/missing",code="404"} 1`,
		"# TYPE kvcache_http_request_duration_seconds histogram",
		"kvcache_keys 1",
		"kvcache_shards 64",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics output missing %q", want)
		}
	}
}
