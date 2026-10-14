package store

import "fmt"

// Entry is a key/value pair exported from or imported into a Store.
type Entry struct {
	Key     string
	Value   []byte
	Version uint64
}

// ShardCount returns the number of shards, for iterating with ExportShard.
func (s *Store) ShardCount() int {
	return len(s.shards)
}

// ExportShard returns a copy of every entry in shard i. Exporting one shard
// at a time bounds the extra memory a snapshot needs by the size of the
// largest shard rather than the whole store, and only blocks writers to
// that shard while it is copied.
func (s *Store) ExportShard(i int) []Entry {
	sh := s.shards[i]
	sh.mu.RLock()
	defer sh.mu.RUnlock()

	out := make([]Entry, 0, len(sh.items))
	for k, e := range sh.items {
		out = append(out, Entry{Key: k, Value: clone(e.value), Version: e.version})
	}
	return out
}

// Import loads entries into the store, preserving their versions and
// overwriting existing keys. Every entry is validated before any is
// applied, so an invalid entry leaves the store unchanged. Import takes
// ownership of the entries' Value slices.
func (s *Store) Import(entries []Entry) error {
	for _, e := range entries {
		if err := s.validate(e.Key, e.Value); err != nil {
			return fmt.Errorf("import %q: %w", e.Key, err)
		}
	}
	for _, e := range entries {
		sh := s.shardFor(e.Key)
		sh.mu.Lock()
		sh.items[e.Key] = &entry{value: e.Value, version: e.Version}
		sh.mu.Unlock()
	}
	return nil
}
