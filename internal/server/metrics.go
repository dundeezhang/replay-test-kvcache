package server

import (
	"net/http"
	"time"
)

// instrument records a request count and latency for every request that
// reaches the router, including 404s and 405s.
func (s *Server) instrument(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		s.metrics.IncRequest(r.Method, r.URL.Path, rec.status)
		s.metrics.ObserveLatency(r.URL.Path, time.Since(start))
	})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	if _, err := s.metrics.WriteTo(w); err != nil {
		s.logger.Printf("metrics: write response: %v", err)
	}
}

func (s *Server) registerGauges() {
	s.metrics.RegisterGauge("kvcache_keys", "Number of keys currently stored.", func() float64 {
		return float64(s.store.Len())
	})
	s.metrics.RegisterGauge("kvcache_shards", "Number of store shards.", func() float64 {
		return float64(s.cfg.ShardCount)
	})
	s.metrics.RegisterGauge("kvcache_uptime_seconds", "Seconds since the server started.", func() float64 {
		return time.Since(s.startedAt).Seconds()
	})
}
