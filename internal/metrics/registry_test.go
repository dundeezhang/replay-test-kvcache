package metrics

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestHistogramBuckets(t *testing.T) {
	h := NewHistogram([]float64{10, 0.1, 1})
	for _, v := range []float64{0.05, 0.5, 0.7, 3, 42} {
		h.Observe(v)
	}

	cumulative, sum, count := h.snapshot()
	want := []uint64{1, 3, 4, 5}
	for i := range want {
		if cumulative[i] != want[i] {
			t.Fatalf("cumulative = %v, want %v", cumulative, want)
		}
	}
	if count != 5 {
		t.Errorf("count = %d, want 5", count)
	}
	if sum < 46.24 || sum > 46.26 {
		t.Errorf("sum = %v, want 46.25", sum)
	}
}

func TestRegistryWriteTo(t *testing.T) {
	r := NewRegistry()
	r.IncRequest("GET", "/v1/keys/a", 200)
	r.IncRequest("GET", "/v1/keys/a", 200)
	r.IncRequest("PUT", "/v1/keys/a", 204)
	r.ObserveLatency("/v1/keys/a", 300*time.Microsecond)
	r.RegisterGauge("kvcache_keys", "Number of keys.", func() float64 { return 7 })

	var buf bytes.Buffer
	n, err := r.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(buf.Len()) {
		t.Errorf("WriteTo returned %d, wrote %d bytes", n, buf.Len())
	}

	out := buf.String()
	for _, want := range []string{
		"# TYPE kvcache_http_requests_total counter",
		`kvcache_http_requests_total{method="GET",path="/v1/keys/a",code="200"} 2`,
		`kvcache_http_requests_total{method="PUT",path="/v1/keys/a",code="204"} 1`,
		`kvcache_http_request_duration_seconds_bucket{path="/v1/keys/a",le="0.00025"} 0`,
		`kvcache_http_request_duration_seconds_bucket{path="/v1/keys/a",le="0.0005"} 1`,
		`kvcache_http_request_duration_seconds_bucket{path="/v1/keys/a",le="+Inf"} 1`,
		`kvcache_http_request_duration_seconds_count{path="/v1/keys/a"} 1`,
		"# TYPE kvcache_keys gauge",
		"kvcache_keys 7",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}

	if strings.Index(out, `method="GET"`) > strings.Index(out, `method="PUT"`) {
		t.Error("request series are not sorted")
	}
}

func TestEscapeLabel(t *testing.T) {
	got := escapeLabel("a\"b\\c\nd")
	want := `a\"b\\c\nd`
	if got != want {
		t.Fatalf("escapeLabel = %q, want %q", got, want)
	}
}
