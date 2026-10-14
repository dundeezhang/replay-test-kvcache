// Package store implements the sharded in-memory map behind kvcache.
package store

import (
	"errors"
	"sort"
	"strings"
	"sync"
)

var (
	// ErrNotFound is returned when a key does not exist.
	ErrNotFound = errors.New("store: key not found")
	// ErrEmptyKey is returned for zero-length keys.
	ErrEmptyKey = errors.New("store: empty key")
	// ErrKeyTooLarge is returned when a key exceeds Options.MaxKeyBytes.
	ErrKeyTooLarge = errors.New("store: key too large")
	// ErrValueTooLarge is returned when a value exceeds Options.MaxValueBytes.
	ErrValueTooLarge = errors.New("store: value too large")
)

// DefaultShards is used when Options.Shards is zero.
const DefaultShards = 64

// Options configures a Store.
type Options struct {
	// Shards is the number of lock domains. It must be a power of two;
	// zero selects DefaultShards.
	Shards int

	// MaxKeyBytes and MaxValueBytes bound individual entries. Zero means
	// unlimited.
	MaxKeyBytes   int
	MaxValueBytes int
}

// Store is a concurrency-safe key/value map partitioned into shards so
// that writers to unrelated keys do not contend on a single lock.
type Store struct {
	shards []*shard
	mask   uint32
	opts   Options
}

type shard struct {
	mu    sync.RWMutex
	items map[string]*entry
}

type entry struct {
	value   []byte
	version uint64
}

// Item is a value together with its version. Versions start at 1 and
// increase on every overwrite of the same key.
type Item struct {
	Value   []byte
	Version uint64
}

// New returns an empty Store.
func New(opts Options) *Store {
	if opts.Shards <= 0 {
		opts.Shards = DefaultShards
	}
	if opts.Shards&(opts.Shards-1) != 0 {
		panic("store: shard count must be a power of two")
	}
	s := &Store{
		shards: make([]*shard, opts.Shards),
		mask:   uint32(opts.Shards - 1),
		opts:   opts,
	}
	for i := range s.shards {
		s.shards[i] = &shard{items: make(map[string]*entry)}
	}
	return s
}

// Get returns a copy of the value stored under key.
func (s *Store) Get(key string) (Item, error) {
	sh := s.shardFor(key)
	sh.mu.RLock()
	defer sh.mu.RUnlock()

	e, ok := sh.items[key]
	if !ok {
		return Item{}, ErrNotFound
	}
	return Item{Value: clone(e.value), Version: e.version}, nil
}

// Set stores a copy of value under key, replacing any existing value, and
// returns the new version.
func (s *Store) Set(key string, value []byte) (uint64, error) {
	if err := s.validate(key, value); err != nil {
		return 0, err
	}
	v := clone(value)

	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()

	if e, ok := sh.items[key]; ok {
		e.value = v
		e.version++
		return e.version, nil
	}
	sh.items[key] = &entry{value: v, version: 1}
	return 1, nil
}

// Delete removes key and reports whether it was present.
func (s *Store) Delete(key string) bool {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()

	if _, ok := sh.items[key]; !ok {
		return false
	}
	delete(sh.items, key)
	return true
}

// Len returns the number of keys across all shards.
func (s *Store) Len() int {
	n := 0
	for _, sh := range s.shards {
		sh.mu.RLock()
		n += len(sh.items)
		sh.mu.RUnlock()
	}
	return n
}

// Keys returns the keys that start with prefix in lexical order, at most
// limit of them. A limit <= 0 means no limit.
func (s *Store) Keys(prefix string, limit int) []string {
	var out []string
	for _, sh := range s.shards {
		sh.mu.RLock()
		for k := range sh.items {
			if strings.HasPrefix(k, prefix) {
				out = append(out, k)
			}
		}
		sh.mu.RUnlock()
	}
	sort.Strings(out)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Stats is a point-in-time summary of the store.
type Stats struct {
	Keys   int   `json:"keys"`
	Bytes  int64 `json:"bytes"`
	Shards int   `json:"shards"`
}

// Stats walks every shard and reports key and byte counts. Byte counts
// include key and value lengths but not map or entry overhead.
func (s *Store) Stats() Stats {
	st := Stats{Shards: len(s.shards)}
	for _, sh := range s.shards {
		sh.mu.RLock()
		st.Keys += len(sh.items)
		for k, e := range sh.items {
			st.Bytes += int64(len(k) + len(e.value))
		}
		sh.mu.RUnlock()
	}
	return st
}

func (s *Store) shardFor(key string) *shard {
	return s.shards[hashKey(key)&s.mask]
}

// hashKey is 32-bit FNV-1a, inlined to avoid allocating a hash.Hash32 on
// every lookup.
func hashKey(key string) uint32 {
	const (
		offset32 = 2166136261
		prime32  = 16777619
	)
	h := uint32(offset32)
	for i := 0; i < len(key); i++ {
		h ^= uint32(key[i])
		h *= prime32
	}
	return h
}

func (s *Store) validate(key string, value []byte) error {
	switch {
	case key == "":
		return ErrEmptyKey
	case s.opts.MaxKeyBytes > 0 && len(key) > s.opts.MaxKeyBytes:
		return ErrKeyTooLarge
	case s.opts.MaxValueBytes > 0 && len(value) > s.opts.MaxValueBytes:
		return ErrValueTooLarge
	}
	return nil
}

func clone(b []byte) []byte {
	if b == nil {
		return nil
	}
	out := make([]byte, len(b))
	copy(out, b)
	return out
}
