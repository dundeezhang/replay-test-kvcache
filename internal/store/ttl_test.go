package store

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newClockedStore(c *fakeClock, shards int) *Store {
	s := New(Options{Shards: shards})
	s.now = c.Now
	return s
}

func TestSetWithTTLExpires(t *testing.T) {
	clk := newFakeClock()
	s := newClockedStore(clk, 4)

	if _, err := s.SetWithTTL("session", []byte("abc"), 10*time.Second); err != nil {
		t.Fatal(err)
	}

	clk.Advance(9 * time.Second)
	if _, err := s.Get("session"); err != nil {
		t.Fatalf("Get before expiry: %v", err)
	}

	clk.Advance(time.Second)
	if _, err := s.Get("session"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get at expiry err = %v, want ErrNotFound", err)
	}
	if n := s.Len(); n != 0 {
		t.Fatalf("Len after lazy expiry = %d, want 0", n)
	}
}

func TestTTLReportsRemaining(t *testing.T) {
	clk := newFakeClock()
	s := newClockedStore(clk, 4)

	s.SetWithTTL("a", []byte("1"), time.Minute)
	s.Set("b", []byte("2"))

	clk.Advance(15 * time.Second)

	if got, err := s.TTL("a"); err != nil || got != 45*time.Second {
		t.Fatalf("TTL(a) = %s, %v; want 45s", got, err)
	}
	if got, err := s.TTL("b"); err != nil || got != NoExpiry {
		t.Fatalf("TTL(b) = %s, %v; want NoExpiry", got, err)
	}
	if _, err := s.TTL("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("TTL(missing) err = %v", err)
	}
}

func TestExpireAndPersist(t *testing.T) {
	clk := newFakeClock()
	s := newClockedStore(clk, 4)
	s.Set("k", []byte("v"))

	if err := s.Expire("k", 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.TTL("k"); got != 5*time.Second {
		t.Fatalf("TTL after Expire = %s", got)
	}
	if err := s.Persist("k"); err != nil {
		t.Fatal(err)
	}
	clk.Advance(time.Hour)
	if _, err := s.Get("k"); err != nil {
		t.Fatalf("Get after Persist: %v", err)
	}
	if err := s.Expire("nope", time.Second); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Expire(missing) err = %v", err)
	}
}

func TestSetClearsExistingTTL(t *testing.T) {
	clk := newFakeClock()
	s := newClockedStore(clk, 4)

	s.SetWithTTL("k", []byte("v1"), time.Second)
	s.Set("k", []byte("v2"))
	clk.Advance(time.Minute)

	it, err := s.Get("k")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(it.Value) != "v2" || it.Version != 2 {
		t.Fatalf("Get = %q v%d", it.Value, it.Version)
	}
}

func TestOverwriteExpiredStartsNewVersion(t *testing.T) {
	clk := newFakeClock()
	s := newClockedStore(clk, 4)

	s.SetWithTTL("k", []byte("old"), time.Second)
	s.SetWithTTL("k", []byte("old2"), time.Second)
	clk.Advance(2 * time.Second)

	v, err := s.Set("k", []byte("new"))
	if err != nil {
		t.Fatal(err)
	}
	if v != 1 {
		t.Fatalf("version after overwriting expired entry = %d, want 1", v)
	}
}

func TestKeysSkipsExpired(t *testing.T) {
	clk := newFakeClock()
	s := newClockedStore(clk, 4)
	for i := 0; i < 5; i++ {
		s.SetWithTTL(fmt.Sprintf("tmp:%d", i), []byte("x"), time.Second)
	}
	s.Set("tmp:keep", []byte("x"))
	clk.Advance(time.Second)

	if got := s.Keys("tmp:", 0); len(got) != 1 || got[0] != "tmp:keep" {
		t.Fatalf("Keys = %v, want [tmp:keep]", got)
	}
}

func TestNegativeTTLRejected(t *testing.T) {
	s := New(Options{Shards: 4})
	if _, err := s.SetWithTTL("k", []byte("v"), -time.Second); !errors.Is(err, ErrInvalidTTL) {
		t.Fatalf("SetWithTTL err = %v", err)
	}
	s.Set("k", []byte("v"))
	if err := s.Expire("k", -time.Second); !errors.Is(err, ErrInvalidTTL) {
		t.Fatalf("Expire err = %v", err)
	}
}

func TestStatsCountsExpiring(t *testing.T) {
	s := New(Options{Shards: 4})
	s.SetWithTTL("a", []byte("1"), time.Hour)
	s.SetWithTTL("b", []byte("2"), time.Hour)
	s.Set("c", []byte("3"))

	if st := s.Stats(); st.Keys != 3 || st.Expiring != 2 {
		t.Fatalf("Stats = %+v", st)
	}
}
