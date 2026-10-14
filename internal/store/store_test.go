package store

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func newTestStore() *Store {
	return New(Options{Shards: 8, MaxKeyBytes: 32, MaxValueBytes: 64})
}

func TestSetGetDelete(t *testing.T) {
	s := newTestStore()

	if _, err := s.Get("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(missing) err = %v, want ErrNotFound", err)
	}

	v, err := s.Set("greeting", []byte("hello"))
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	if v != 1 {
		t.Fatalf("first version = %d, want 1", v)
	}

	it, err := s.Get("greeting")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(it.Value) != "hello" || it.Version != 1 {
		t.Fatalf("Get = %q v%d", it.Value, it.Version)
	}

	if v, _ := s.Set("greeting", []byte("hi")); v != 2 {
		t.Fatalf("overwrite version = %d, want 2", v)
	}

	if !s.Delete("greeting") {
		t.Fatal("Delete returned false for an existing key")
	}
	if s.Delete("greeting") {
		t.Fatal("Delete returned true for a missing key")
	}
}

func TestGetReturnsCopy(t *testing.T) {
	s := newTestStore()
	buf := []byte("abc")
	if _, err := s.Set("k", buf); err != nil {
		t.Fatal(err)
	}
	buf[0] = 'X'

	it, _ := s.Get("k")
	if string(it.Value) != "abc" {
		t.Fatalf("store aliased the caller's buffer: %q", it.Value)
	}
	it.Value[1] = 'Y'
	again, _ := s.Get("k")
	if string(again.Value) != "abc" {
		t.Fatalf("store aliased the returned buffer: %q", again.Value)
	}
}

func TestValidation(t *testing.T) {
	s := newTestStore()
	cases := []struct {
		key   string
		value []byte
		want  error
	}{
		{"", []byte("x"), ErrEmptyKey},
		{strings.Repeat("k", 33), []byte("x"), ErrKeyTooLarge},
		{"k", make([]byte, 65), ErrValueTooLarge},
	}
	for _, tc := range cases {
		if _, err := s.Set(tc.key, tc.value); !errors.Is(err, tc.want) {
			t.Errorf("Set(len(key)=%d, len(value)=%d) = %v, want %v", len(tc.key), len(tc.value), err, tc.want)
		}
	}
}

func TestKeysPrefixAndLimit(t *testing.T) {
	s := newTestStore()
	for _, k := range []string{"user:3", "user:1", "user:2", "session:1"} {
		if _, err := s.Set(k, []byte("v")); err != nil {
			t.Fatal(err)
		}
	}
	got := s.Keys("user:", 0)
	want := []string{"user:1", "user:2", "user:3"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("Keys = %v, want %v", got, want)
	}
	if got := s.Keys("user:", 2); len(got) != 2 || got[0] != "user:1" {
		t.Fatalf("Keys with limit = %v", got)
	}
}

func TestStats(t *testing.T) {
	s := newTestStore()
	s.Set("ab", []byte("cde"))
	s.Set("f", []byte("gh"))

	st := s.Stats()
	if st.Keys != 2 || st.Bytes != 8 || st.Shards != 8 {
		t.Fatalf("Stats = %+v", st)
	}
}

func TestConcurrentAccess(t *testing.T) {
	s := New(Options{Shards: 4})
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				key := fmt.Sprintf("k%d", i%50)
				s.Set(key, []byte(fmt.Sprintf("%d-%d", w, i)))
				s.Get(key)
				if i%7 == 0 {
					s.Delete(key)
				}
			}
		}(w)
	}
	wg.Wait()
	if n := s.Len(); n > 50 {
		t.Fatalf("Len = %d, want <= 50", n)
	}
}
