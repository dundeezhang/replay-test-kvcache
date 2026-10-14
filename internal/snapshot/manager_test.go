package snapshot

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/dundeezhang/replay-test-kvcache/internal/store"
)

func newManager(t *testing.T, st *store.Store, opts Options) *Manager {
	t.Helper()
	m, err := NewManager(st, opts)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(m.Stop)
	return m
}

func TestSnapshotAndRestore(t *testing.T) {
	dir := t.TempDir()
	src := store.New(store.Options{Shards: 4})
	for i := 0; i < 100; i++ {
		src.Set(fmt.Sprintf("k%03d", i), []byte(fmt.Sprintf("v%d", i)))
	}
	src.Set("k000", []byte("updated"))

	info, err := newManager(t, src, Options{Dir: dir}).SnapshotNow()
	if err != nil {
		t.Fatalf("SnapshotNow: %v", err)
	}
	if info.Records != 100 || info.Bytes <= 0 {
		t.Fatalf("info = %+v", info)
	}

	dst := store.New(store.Options{Shards: 8})
	got, found, err := newManager(t, dst, Options{Dir: dir}).Restore()
	if err != nil || !found {
		t.Fatalf("Restore = %+v, %v, %v", got, found, err)
	}
	if got.Path != info.Path || got.Records != 100 {
		t.Fatalf("restored %+v, wrote %+v", got, info)
	}
	if n := dst.Len(); n != 100 {
		t.Fatalf("Len = %d, want 100", n)
	}
	it, err := dst.Get("k000")
	if err != nil || string(it.Value) != "updated" || it.Version != 2 {
		t.Fatalf("Get(k000) = %q v%d, %v", it.Value, it.Version, err)
	}
}

func TestRestoreEmptyDir(t *testing.T) {
	st := store.New(store.Options{Shards: 4})
	_, found, err := newManager(t, st, Options{Dir: t.TempDir()}).Restore()
	if err != nil || found {
		t.Fatalf("Restore on empty dir = found %v, err %v", found, err)
	}
}

func TestRestoreSkipsCorruptNewest(t *testing.T) {
	dir := t.TempDir()
	src := store.New(store.Options{Shards: 4})
	m := newManager(t, src, Options{Dir: dir})

	src.Set("a", []byte("1"))
	older, err := m.SnapshotNow()
	if err != nil {
		t.Fatal(err)
	}
	src.Set("b", []byte("2"))
	newer, err := m.SnapshotNow()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(newer.Path, newer.Bytes-3); err != nil {
		t.Fatal(err)
	}

	dst := store.New(store.Options{Shards: 4})
	got, found, err := newManager(t, dst, Options{Dir: dir}).Restore()
	if err != nil || !found {
		t.Fatalf("Restore = %v, %v", found, err)
	}
	if got.Path != older.Path {
		t.Fatalf("restored %s, want fallback to %s", got.Path, older.Path)
	}
	if _, err := dst.Get("b"); err == nil {
		t.Fatal("key from the corrupt snapshot was restored")
	}
}

func TestPruneKeepsNewest(t *testing.T) {
	dir := t.TempDir()
	st := store.New(store.Options{Shards: 4})
	st.Set("k", []byte("v"))
	m := newManager(t, st, Options{Dir: dir, Retain: 2})

	var written []string
	for i := 0; i < 4; i++ {
		info, err := m.SnapshotNow()
		if err != nil {
			t.Fatal(err)
		}
		written = append(written, info.Path)
	}

	left, err := m.list()
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 2 || left[0] != written[2] || left[1] != written[3] {
		t.Fatalf("remaining = %v, want last two of %v", left, written)
	}
}

func TestPeriodicSnapshots(t *testing.T) {
	dir := t.TempDir()
	st := store.New(store.Options{Shards: 4})
	st.Set("k", []byte("v"))
	m := newManager(t, st, Options{Dir: dir, Interval: 10 * time.Millisecond})
	m.Start()

	deadline := time.Now().Add(2 * time.Second)
	for m.Last().Path == "" {
		if time.Now().After(deadline) {
			t.Fatal("no periodic snapshot was written")
		}
		time.Sleep(5 * time.Millisecond)
	}
	m.Stop()
	m.Stop()
}
