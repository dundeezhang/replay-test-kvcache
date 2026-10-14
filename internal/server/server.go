// Package server exposes the store over a small HTTP/JSON API.
package server

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/dundeezhang/replay-test-kvcache/internal/config"
	"github.com/dundeezhang/replay-test-kvcache/internal/store"
)

// Server routes HTTP requests to a Store.
type Server struct {
	store     *store.Store
	cfg       config.Config
	mux       *http.ServeMux
	logger    *log.Logger
	startedAt time.Time
}

// New builds a Server. A nil logger discards log output.
func New(st *store.Store, cfg config.Config, logger *log.Logger) *Server {
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}
	s := &Server{
		store:     st,
		cfg:       cfg,
		mux:       http.NewServeMux(),
		logger:    logger,
		startedAt: time.Now(),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("GET /v1/stats", s.handleStats)
	s.mux.HandleFunc("GET /v1/keys", s.handleListKeys)
	s.mux.HandleFunc("GET /v1/keys/{key}", s.handleGet)
	s.mux.HandleFunc("PUT /v1/keys/{key}", s.handleSet)
	s.mux.HandleFunc("DELETE /v1/keys/{key}", s.handleDelete)
}

// Handler returns the root HTTP handler with middleware applied.
func (s *Server) Handler() http.Handler {
	var h http.Handler = s.mux
	if s.cfg.LogRequests {
		h = s.logRequests(h)
	}
	return h
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"uptime": time.Since(s.startedAt).Round(time.Second).String(),
	})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.store.Stats())
}

func (s *Server) handleListKeys(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := 1000
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = n
	}
	keys := s.store.Keys(q.Get("prefix"), limit)
	if keys == nil {
		keys = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": keys})
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	it, err := s.store.Get(r.PathValue("key"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("ETag", etag(it.Version))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(it.Value)
}

func (s *Server) handleSet(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, int64(s.cfg.MaxValueBytes)))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "value too large")
			return
		}
		writeError(w, http.StatusBadRequest, "read body: "+err.Error())
		return
	}
	version, err := s.store.Set(key, body)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	w.Header().Set("ETag", etag(version))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	if !s.store.Delete(r.PathValue("key")) {
		writeError(w, http.StatusNotFound, store.ErrNotFound.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		s.logger.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Microsecond))
	})
}

// statusRecorder captures the status code written by a handler.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func etag(version uint64) string {
	return `"` + strconv.FormatUint(version, 10) + `"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, store.ErrEmptyKey), errors.Is(err, store.ErrKeyTooLarge):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, store.ErrValueTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}
