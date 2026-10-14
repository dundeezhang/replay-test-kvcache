package server

import (
	"net/http"

	"github.com/dundeezhang/replay-test-kvcache/internal/snapshot"
)

// Snapshotter is the part of snapshot.Manager the admin API uses.
type Snapshotter interface {
	SnapshotNow() (snapshot.Info, error)
}

// SetSnapshotter enables the snapshot admin endpoints. It must be called
// before the server starts handling requests.
func (s *Server) SetSnapshotter(sn Snapshotter) {
	s.snapshots = sn
}

func (s *Server) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	if s.snapshots == nil {
		writeError(w, http.StatusServiceUnavailable, "snapshots are not enabled")
		return
	}
	info, err := s.snapshots.SnapshotNow()
	if err != nil {
		s.logger.Printf("admin: snapshot failed: %v", err)
		writeError(w, http.StatusInternalServerError, "snapshot failed")
		return
	}
	writeJSON(w, http.StatusOK, info)
}
