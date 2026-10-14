package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dundeezhang/replay-test-kvcache/internal/config"
	"github.com/dundeezhang/replay-test-kvcache/internal/snapshot"
	"github.com/dundeezhang/replay-test-kvcache/internal/store"
)

type fakeSnapshotter struct {
	err error
}

func (f *fakeSnapshotter) SnapshotNow() (snapshot.Info, error) {
	if f.err != nil {
		return snapshot.Info{}, f.err
	}
	return snapshot.Info{
		Path:    "/var/lib/kvcache/snapshot-1709294400000000000.kvs",
		Records: 3,
		Bytes:   128,
		Created: time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC),
	}, nil
}

func newAdminTestServer(t *testing.T, sn Snapshotter) *httptest.Server {
	t.Helper()
	srv := New(store.New(store.Options{Shards: 4}), config.Default(), nil)
	if sn != nil {
		srv.SetSnapshotter(sn)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func TestSnapshotEndpointDisabled(t *testing.T) {
	ts := newAdminTestServer(t, nil)
	resp, _ := do(t, http.MethodPost, ts.URL+"/v1/admin/snapshot", "")
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("POST = %d, want 503", resp.StatusCode)
	}
}

func TestSnapshotEndpoint(t *testing.T) {
	ts := newAdminTestServer(t, &fakeSnapshotter{})

	resp, body := do(t, http.MethodPost, ts.URL+"/v1/admin/snapshot", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST = %d %s", resp.StatusCode, body)
	}
	var info snapshot.Info
	if err := json.Unmarshal([]byte(body), &info); err != nil {
		t.Fatal(err)
	}
	if info.Records != 3 || !strings.HasSuffix(info.Path, ".kvs") {
		t.Fatalf("info = %+v", info)
	}
}
