package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/dundeezhang/replay-test-kvcache/internal/store"
)

// maxTTLSeconds caps client-supplied TTLs at ten years. Anything longer is
// almost certainly a unit mistake (milliseconds sent as seconds).
const maxTTLSeconds = 10 * 365 * 24 * 60 * 60

type ttlResponse struct {
	Key string `json:"key"`
	// TTLSeconds is -1 for keys that never expire.
	TTLSeconds int64 `json:"ttl_seconds"`
	// ExpiresAt is omitted for keys that never expire.
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type expireRequest struct {
	TTLSeconds int64 `json:"ttl_seconds"`
}

func (s *Server) handleGetTTL(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	ttl, err := s.store.TTL(key)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	resp := ttlResponse{Key: key, TTLSeconds: -1}
	if ttl != store.NoExpiry {
		// Round up so that a key with 300ms left reports 1 rather than 0,
		// which most clients would read as "already expired".
		resp.TTLSeconds = int64((ttl + time.Second - 1) / time.Second)
		at := time.Now().Add(ttl).UTC().Truncate(time.Millisecond)
		resp.ExpiresAt = &at
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleSetTTL(w http.ResponseWriter, r *http.Request) {
	var req expireRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if req.TTLSeconds <= 0 || req.TTLSeconds > maxTTLSeconds {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("ttl_seconds must be between 1 and %d; use DELETE to remove an expiry", maxTTLSeconds))
		return
	}

	ttl := time.Duration(req.TTLSeconds) * time.Second
	if err := s.store.Expire(r.PathValue("key"), ttl); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePersist(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Persist(r.PathValue("key")); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
