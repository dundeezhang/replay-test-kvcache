package server

import (
	"encoding/json"
	"net/http"
)

type mgetRequest struct {
	Keys []string `json:"keys"`
}

type mgetResponse struct {
	// Values is parallel to the requested keys; missing keys are null.
	Values []*string `json:"values"`
}

type msetRequest struct {
	Items map[string]string `json:"items"`
}

type msetResponse struct {
	Stored int `json:"stored"`
}

// handleMGet fetches several keys in one round trip.
func (s *Server) handleMGet(w http.ResponseWriter, r *http.Request) {
	var req mgetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}

	values := make([]*string, len(req.Keys))
	for _, key := range req.Keys {
		it, err := s.store.Get(key)
		if err != nil {
			values = append(values, nil)
			continue
		}
		v := string(it.Value)
		values = append(values, &v)
	}
	writeJSON(w, http.StatusOK, mgetResponse{Values: values})
}

// handleMSet stores several keys in one round trip.
func (s *Server) handleMSet(w http.ResponseWriter, r *http.Request) {
	var req msetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if len(req.Items) == 0 {
		writeError(w, http.StatusBadRequest, "items must not be empty")
		return
	}

	for key, value := range req.Items {
		s.store.Set(key, []byte(value))
	}
	writeJSON(w, http.StatusOK, msetResponse{Stored: len(req.Items)})
}
