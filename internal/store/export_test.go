package store

import (
	"errors"
	"fmt"
	"testing"
)

func exportAll(s *Store) []Entry {
	var all []Entry
	for i := 0; i < s.ShardCount(); i++ {
		all = append(all, s.ExportShard(i)...)
	}
	return all
}

func TestExportImportRoundTrip(t *testing.T) {
	src := New(Options{Shards: 4})
	for i := 0; i < 50; i++ {
		src.Set(fmt.Sprintf("k%d", i), []byte{byte(i)})
	}
	src.Set("k7", []byte("again"))

	all := exportAll(src)
	if len(all) != 50 {
		t.Fatalf("exported %d entries, want 50", len(all))
	}

	dst := New(Options{Shards: 16})
	if err := dst.Import(all); err != nil {
		t.Fatalf("Import: %v", err)
	}
	if n := dst.Len(); n != 50 {
		t.Fatalf("Len = %d, want 50", n)
	}
	it, err := dst.Get("k7")
	if err != nil || string(it.Value) != "again" || it.Version != 2 {
		t.Fatalf("Get(k7) = %q v%d, %v", it.Value, it.Version, err)
	}
}

func TestImportIsAllOrNothing(t *testing.T) {
	s := New(Options{Shards: 1, MaxValueBytes: 4})
	err := s.Import([]Entry{
		{Key: "ok", Value: []byte("1"), Version: 1},
		{Key: "big", Value: []byte("12345"), Version: 1},
	})
	if !errors.Is(err, ErrValueTooLarge) {
		t.Fatalf("Import err = %v, want ErrValueTooLarge", err)
	}
	if n := s.Len(); n != 0 {
		t.Fatalf("Len after failed import = %d, want 0", n)
	}
}
