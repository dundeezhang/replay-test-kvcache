package store

import (
	"log"
	"sync"
	"time"
)

// JanitorOptions tunes background expiry.
type JanitorOptions struct {
	// Interval is the time between sweeps. Zero selects one second.
	Interval time.Duration

	// SampleSize is how many entries are examined per shard per pass.
	// Zero selects 20, matching Redis' active-expire default.
	SampleSize int

	// RepeatThreshold is the fraction of expired samples above which the
	// janitor immediately samples the same shard again. Zero selects 0.25.
	RepeatThreshold float64

	// MaxPassesPerShard bounds the repeat loop so that one shard full of
	// expired keys cannot starve the others. Zero selects 16.
	MaxPassesPerShard int

	// Logger receives a line for every sweep that removed keys. Nil
	// disables logging.
	Logger *log.Logger
}

// JanitorStats are cumulative counters since the janitor was created.
type JanitorStats struct {
	Sweeps   uint64        `json:"sweeps"`
	Examined uint64        `json:"examined"`
	Removed  uint64        `json:"removed"`
	LastRun  time.Duration `json:"last_run_ns"`
}

// Janitor actively removes expired keys. Without it, an expired key is
// only reclaimed when something reads it, so write-once keys with a TTL
// would hold memory forever.
type Janitor struct {
	store *Store
	opts  JanitorOptions

	mu      sync.Mutex
	stats   JanitorStats
	started bool
	stop    chan struct{}
	done    chan struct{}
}

// NewJanitor returns a stopped janitor for s.
func NewJanitor(s *Store, opts JanitorOptions) *Janitor {
	if opts.Interval <= 0 {
		opts.Interval = time.Second
	}
	if opts.SampleSize <= 0 {
		opts.SampleSize = 20
	}
	if opts.RepeatThreshold <= 0 {
		opts.RepeatThreshold = 0.25
	}
	if opts.MaxPassesPerShard <= 0 {
		opts.MaxPassesPerShard = 16
	}
	return &Janitor{
		store: s,
		opts:  opts,
		stop:  make(chan struct{}),
		done:  make(chan struct{}),
	}
}

// Start launches the sweep loop. Calling Start more than once has no
// additional effect.
func (j *Janitor) Start() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.started {
		return
	}
	j.started = true
	go j.run()
}

// Stop halts the sweep loop and waits for an in-progress sweep to finish.
// It is safe to call Stop on a janitor that was never started.
func (j *Janitor) Stop() {
	j.mu.Lock()
	started := j.started
	j.mu.Unlock()
	if !started {
		return
	}
	select {
	case <-j.stop:
	default:
		close(j.stop)
	}
	<-j.done
}

// Stats returns a copy of the cumulative janitor counters.
func (j *Janitor) Stats() JanitorStats {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.stats
}

func (j *Janitor) run() {
	defer close(j.done)
	ticker := time.NewTicker(j.opts.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-j.stop:
			return
		case <-ticker.C:
			j.sweep()
		}
	}
}

// sweep visits every shard once, re-sampling a shard while the fraction of
// expired samples stays above RepeatThreshold. This is the same adaptive
// strategy Redis uses: cheap when little has expired, aggressive when a
// burst of keys expires together.
func (j *Janitor) sweep() {
	start := time.Now()
	now := j.store.now()
	var examined, removed uint64

	for _, sh := range j.store.shards {
		for pass := 0; pass < j.opts.MaxPassesPerShard; pass++ {
			e, r := sh.evictExpired(now, j.opts.SampleSize)
			examined += uint64(e)
			removed += uint64(r)
			if e == 0 || float64(r)/float64(e) <= j.opts.RepeatThreshold {
				break
			}
		}
	}

	elapsed := time.Since(start)
	j.mu.Lock()
	j.stats.Sweeps++
	j.stats.Examined += examined
	j.stats.Removed += removed
	j.stats.LastRun = elapsed
	j.mu.Unlock()

	if removed > 0 && j.opts.Logger != nil {
		j.opts.Logger.Printf("janitor: removed %d expired keys (examined %d) in %s", removed, examined, elapsed)
	}
}
