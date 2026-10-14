package store

import (
	"errors"
	"time"
)

// NoExpiry is the TTL reported for keys that never expire.
const NoExpiry time.Duration = -1

// ErrInvalidTTL is returned when a negative TTL is supplied.
var ErrInvalidTTL = errors.New("store: ttl must not be negative")

// expired reports whether e is no longer visible at now.
func (e *entry) expired(now time.Time) bool {
	return !e.expiresAt.IsZero() && !now.Before(e.expiresAt)
}

// SetWithTTL stores a copy of value under key and arranges for it to
// expire after ttl. A ttl of zero means the key never expires. Writing
// over an expired entry starts a fresh version history at 1.
func (s *Store) SetWithTTL(key string, value []byte, ttl time.Duration) (uint64, error) {
	if ttl < 0 {
		return 0, ErrInvalidTTL
	}
	if err := s.validate(key, value); err != nil {
		return 0, err
	}
	v := clone(value)

	now := s.now()
	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = now.Add(ttl)
	}

	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()

	if e, ok := sh.items[key]; ok && !e.expired(now) {
		e.value = v
		e.expiresAt = expiresAt
		e.version++
		return e.version, nil
	}
	sh.items[key] = &entry{value: v, version: 1, expiresAt: expiresAt}
	return 1, nil
}

// Expire sets a new TTL on an existing key, measured from now. A ttl of
// zero removes the expiry, which is equivalent to Persist.
func (s *Store) Expire(key string, ttl time.Duration) error {
	if ttl < 0 {
		return ErrInvalidTTL
	}
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()

	now := s.now()
	e, ok := sh.items[key]
	if !ok || e.expired(now) {
		return ErrNotFound
	}
	if ttl == 0 {
		e.expiresAt = time.Time{}
	} else {
		e.expiresAt = now.Add(ttl)
	}
	return nil
}

// Persist removes any expiry from key.
func (s *Store) Persist(key string) error {
	return s.Expire(key, 0)
}

// TTL returns the remaining lifetime of key, or NoExpiry if the key never
// expires. Expired keys are reported as ErrNotFound.
func (s *Store) TTL(key string) (time.Duration, error) {
	sh := s.shardFor(key)
	sh.mu.RLock()
	e, ok := sh.items[key]
	sh.mu.RUnlock()
	if !ok {
		return 0, ErrNotFound
	}

	if e.expiresAt.IsZero() {
		return NoExpiry, nil
	}
	remaining := e.expiresAt.Sub(s.now())
	if remaining <= 0 {
		return 0, ErrNotFound
	}
	return remaining, nil
}

// evictExpired examines up to limit entries of sh and deletes the expired
// ones. It returns how many entries were examined and how many were
// removed. A limit <= 0 scans the whole shard. Go's randomized map
// iteration order means repeated calls sample different entries.
func (sh *shard) evictExpired(now time.Time, limit int) (examined, removed int) {
	sh.mu.Lock()
	defer sh.mu.Unlock()

	for k, e := range sh.items {
		if limit > 0 && examined >= limit {
			break
		}
		examined++
		if e.expired(now) {
			delete(sh.items, k)
			removed++
		}
	}
	return examined, removed
}
