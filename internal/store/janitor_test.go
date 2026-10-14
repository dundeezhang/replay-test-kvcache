package store

import (
	"fmt"
	"testing"
	"time"
)

func fill(t *testing.T, s *Store, prefix string, n int, ttl time.Duration) {
	t.Helper()
	for i := 0; i < n; i++ {
		if _, err := s.SetWithTTL(fmt.Sprintf("%s%d", prefix, i), []byte("v"), ttl); err != nil {
			t.Fatal(err)
		}
	}
}

func TestJanitorSweepRemovesExpired(t *testing.T) {
	clk := newFakeClock()
	s := newClockedStore(clk, 8)
	fill(t, s, "tmp:", 100, time.Second)
	fill(t, s, "keep:", 10, 0)
	clk.Advance(2 * time.Second)

	j := NewJanitor(s, JanitorOptions{SampleSize: 1000})
	j.sweep()

	if n := s.Len(); n != 10 {
		t.Fatalf("Len after sweep = %d, want 10", n)
	}
	st := j.Stats()
	if st.Sweeps != 1 || st.Removed != 100 || st.Examined < 110 {
		t.Fatalf("Stats = %+v", st)
	}
}

func TestJanitorRepeatsWhileShardIsMostlyExpired(t *testing.T) {
	clk := newFakeClock()
	s := newClockedStore(clk, 1)
	fill(t, s, "tmp:", 200, time.Second)
	clk.Advance(2 * time.Second)

	j := NewJanitor(s, JanitorOptions{SampleSize: 20, MaxPassesPerShard: 16})
	j.sweep()

	if n := s.Len(); n != 0 {
		t.Fatalf("Len after sweep = %d, want 0", n)
	}
}

func TestJanitorMaxPassesBoundsWork(t *testing.T) {
	clk := newFakeClock()
	s := newClockedStore(clk, 1)
	fill(t, s, "tmp:", 1000, time.Second)
	clk.Advance(2 * time.Second)

	j := NewJanitor(s, JanitorOptions{SampleSize: 10, MaxPassesPerShard: 3})
	j.sweep()

	if got := j.Stats().Removed; got != 30 {
		t.Fatalf("Removed = %d, want 30", got)
	}
	if n := s.Len(); n != 970 {
		t.Fatalf("Len = %d, want 970", n)
	}
}

func TestJanitorLeavesLiveKeys(t *testing.T) {
	clk := newFakeClock()
	s := newClockedStore(clk, 4)
	fill(t, s, "live:", 50, time.Hour)
	clk.Advance(time.Minute)

	j := NewJanitor(s, JanitorOptions{})
	j.sweep()

	if n := s.Len(); n != 50 {
		t.Fatalf("Len = %d, want 50", n)
	}
}

func TestJanitorStartStop(t *testing.T) {
	clk := newFakeClock()
	s := newClockedStore(clk, 4)
	fill(t, s, "tmp:", 40, time.Second)
	clk.Advance(2 * time.Second)

	j := NewJanitor(s, JanitorOptions{Interval: 5 * time.Millisecond, SampleSize: 100})
	j.Start()
	j.Start()

	deadline := time.Now().Add(2 * time.Second)
	for s.Len() > 0 {
		if time.Now().After(deadline) {
			t.Fatalf("janitor did not reclaim keys; Len = %d", s.Len())
		}
		time.Sleep(5 * time.Millisecond)
	}

	j.Stop()
	j.Stop()
}

func TestJanitorStopWithoutStart(t *testing.T) {
	j := NewJanitor(New(Options{Shards: 1}), JanitorOptions{})
	done := make(chan struct{})
	go func() {
		j.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Stop blocked on a janitor that was never started")
	}
}
