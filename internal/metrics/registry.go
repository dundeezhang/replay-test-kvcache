// Package metrics is a small, dependency-free metrics registry that renders
// the Prometheus text exposition format (version 0.0.4).
package metrics

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// GaugeFunc is sampled on every scrape.
type GaugeFunc func() float64

type requestKey struct {
	method string
	path   string
	code   int
}

type gauge struct {
	help string
	fn   GaugeFunc
}

// Registry holds the process' HTTP and gauge metrics.
type Registry struct {
	mu        sync.RWMutex
	requests  map[requestKey]uint64
	latencies map[string]*Histogram
	gauges    map[string]gauge
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		requests:  make(map[requestKey]uint64),
		latencies: make(map[string]*Histogram),
		gauges:    make(map[string]gauge),
	}
}

// IncRequest counts one completed HTTP request.
func (r *Registry) IncRequest(method, path string, code int) {
	k := requestKey{method: method, path: path, code: code}
	r.mu.RLock()
	r.requests[k]++
	r.mu.RUnlock()
}

// ObserveLatency records the duration of one request to path.
func (r *Registry) ObserveLatency(path string, d time.Duration) {
	r.mu.RLock()
	h, ok := r.latencies[path]
	r.mu.RUnlock()

	if !ok {
		r.mu.Lock()
		if h, ok = r.latencies[path]; !ok {
			h = NewHistogram(DefaultLatencyBuckets)
			r.latencies[path] = h
		}
		r.mu.Unlock()
	}
	h.Observe(d.Seconds())
}

// RegisterGauge adds a gauge that is sampled on every scrape. Registering
// the same name again replaces the earlier function.
func (r *Registry) RegisterGauge(name, help string, fn GaugeFunc) {
	r.mu.Lock()
	r.gauges[name] = gauge{help: help, fn: fn}
	r.mu.Unlock()
}

// WriteTo renders every metric in the Prometheus text format. Series are
// sorted so that output is stable between scrapes.
func (r *Registry) WriteTo(w io.Writer) (int64, error) {
	cw := &countingWriter{w: w}

	r.mu.RLock()
	defer r.mu.RUnlock()

	keys := make([]requestKey, 0, len(r.requests))
	for k := range r.requests {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.path != b.path {
			return a.path < b.path
		}
		if a.method != b.method {
			return a.method < b.method
		}
		return a.code < b.code
	})
	fmt.Fprintln(cw, "# HELP kvcache_http_requests_total Completed HTTP requests.")
	fmt.Fprintln(cw, "# TYPE kvcache_http_requests_total counter")
	for _, k := range keys {
		fmt.Fprintf(cw, "kvcache_http_requests_total{method=\"%s\",path=\"%s\",code=\"%d\"} %d\n",
			escapeLabel(k.method), escapeLabel(k.path), k.code, r.requests[k])
	}

	paths := make([]string, 0, len(r.latencies))
	for p := range r.latencies {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	fmt.Fprintln(cw, "# HELP kvcache_http_request_duration_seconds HTTP request latency.")
	fmt.Fprintln(cw, "# TYPE kvcache_http_request_duration_seconds histogram")
	for _, p := range paths {
		h := r.latencies[p]
		cumulative, sum, count := h.snapshot()
		label := escapeLabel(p)
		for i, upper := range h.upper {
			fmt.Fprintf(cw, "kvcache_http_request_duration_seconds_bucket{path=\"%s\",le=\"%s\"} %d\n",
				label, formatFloat(upper), cumulative[i])
		}
		fmt.Fprintf(cw, "kvcache_http_request_duration_seconds_bucket{path=\"%s\",le=\"+Inf\"} %d\n", label, count)
		fmt.Fprintf(cw, "kvcache_http_request_duration_seconds_sum{path=\"%s\"} %s\n", label, formatFloat(sum))
		fmt.Fprintf(cw, "kvcache_http_request_duration_seconds_count{path=\"%s\"} %d\n", label, count)
	}

	names := make([]string, 0, len(r.gauges))
	for n := range r.gauges {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		g := r.gauges[n]
		fmt.Fprintf(cw, "# HELP %s %s\n", n, g.help)
		fmt.Fprintf(cw, "# TYPE %s gauge\n", n)
		fmt.Fprintf(cw, "%s %s\n", n, formatFloat(g.fn()))
	}

	return cw.n, cw.err
}

var labelEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)

func escapeLabel(v string) string {
	return labelEscaper.Replace(v)
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// countingWriter tracks bytes written and remembers the first error so
// WriteTo can satisfy io.WriterTo without checking every Fprintf.
type countingWriter struct {
	w   io.Writer
	n   int64
	err error
}

func (c *countingWriter) Write(p []byte) (int, error) {
	if c.err != nil {
		return 0, c.err
	}
	n, err := c.w.Write(p)
	c.n += int64(n)
	c.err = err
	return n, err
}
