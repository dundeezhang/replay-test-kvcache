package snapshot

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dundeezhang/replay-test-kvcache/internal/store"
)

const (
	filePrefix = "snapshot-"
	fileSuffix = ".kvs"
)

// Options configures a Manager.
type Options struct {
	// Dir holds snapshot files. It is created if it does not exist.
	Dir string

	// Interval is the time between periodic snapshots. Zero selects five
	// minutes.
	Interval time.Duration

	// Retain is how many snapshot files to keep. Zero selects three.
	Retain int

	// Logger receives progress and error messages. Nil discards them.
	Logger *log.Logger
}

// Info describes a snapshot that was written or restored.
type Info struct {
	Path     string        `json:"path"`
	Records  uint64        `json:"records"`
	Bytes    int64         `json:"bytes"`
	Created  time.Time     `json:"created"`
	Duration time.Duration `json:"duration_ns"`
}

// Manager periodically snapshots a store to disk and restores the newest
// readable snapshot at startup.
type Manager struct {
	store *store.Store
	opts  Options

	mu   sync.Mutex // serializes snapshots and guards last
	last Info

	startOnce sync.Once
	stopOnce  sync.Once
	started   atomic.Bool
	stop      chan struct{}
	done      chan struct{}
}

// NewManager validates opts, creates the snapshot directory and returns a
// stopped Manager.
func NewManager(st *store.Store, opts Options) (*Manager, error) {
	if opts.Dir == "" {
		return nil, errors.New("snapshot: Dir is required")
	}
	if opts.Interval <= 0 {
		opts.Interval = 5 * time.Minute
	}
	if opts.Retain <= 0 {
		opts.Retain = 3
	}
	if opts.Logger == nil {
		opts.Logger = log.New(io.Discard, "", 0)
	}
	if err := os.MkdirAll(opts.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("snapshot: create %s: %w", opts.Dir, err)
	}
	return &Manager{
		store: st,
		opts:  opts,
		stop:  make(chan struct{}),
		done:  make(chan struct{}),
	}, nil
}

// SnapshotNow writes a snapshot immediately and prunes old snapshots
// beyond Retain. Concurrent calls are serialized.
func (m *Manager) SnapshotNow() (Info, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	start := time.Now()
	created := start.UTC()
	name := fmt.Sprintf("%s%d%s", filePrefix, created.UnixNano(), fileSuffix)
	path := filepath.Join(m.opts.Dir, name)

	records, size, err := WriteFile(path, created, func(enc *Encoder) error {
		for i := 0; i < m.store.ShardCount(); i++ {
			for _, e := range m.store.ExportShard(i) {
				if err := enc.Encode(Record{Key: e.Key, Value: e.Value, Version: e.Version}); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return Info{}, fmt.Errorf("snapshot: write %s: %w", path, err)
	}

	info := Info{Path: path, Records: records, Bytes: size, Created: created, Duration: time.Since(start)}
	m.last = info
	if err := m.prune(); err != nil {
		m.opts.Logger.Printf("snapshot: prune: %v", err)
	}
	return info, nil
}

// Last returns the most recent snapshot written by this Manager, or the
// zero Info if none has been written yet.
func (m *Manager) Last() Info {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.last
}

// Restore loads the newest readable snapshot in Dir into the store. found
// is false if the directory contains no snapshots at all. A snapshot that
// fails its checksum is skipped in favour of the next older one; Restore
// fails only if every snapshot is unreadable.
func (m *Manager) Restore() (info Info, found bool, err error) {
	paths, err := m.list()
	if err != nil {
		return Info{}, false, err
	}
	if len(paths) == 0 {
		return Info{}, false, nil
	}

	var errs []error
	for i := len(paths) - 1; i >= 0; i-- {
		path := paths[i]
		start := time.Now()

		f, err := os.Open(path)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		defer f.Close()

		entries, created, err := readAll(f)
		if err != nil {
			m.opts.Logger.Printf("snapshot: skipping %s: %v", path, err)
			errs = append(errs, fmt.Errorf("%s: %w", filepath.Base(path), err))
			continue
		}
		if err := m.store.Import(entries); err != nil {
			return Info{}, true, fmt.Errorf("snapshot: import %s: %w", path, err)
		}

		info = Info{Path: path, Records: uint64(len(entries)), Created: created, Duration: time.Since(start)}
		if fi, err := f.Stat(); err == nil {
			info.Bytes = fi.Size()
		}
		return info, true, nil
	}
	return Info{}, true, fmt.Errorf("snapshot: no readable snapshot in %s: %w", m.opts.Dir, errors.Join(errs...))
}

// Start begins periodic snapshots. Calling Start more than once has no
// additional effect.
func (m *Manager) Start() {
	m.startOnce.Do(func() {
		m.started.Store(true)
		go m.run()
	})
}

// Stop halts periodic snapshots and waits for one in progress to finish.
// It does not write a final snapshot; call SnapshotNow for that.
func (m *Manager) Stop() {
	m.stopOnce.Do(func() { close(m.stop) })
	if m.started.Load() {
		<-m.done
	}
}

func (m *Manager) run() {
	defer close(m.done)
	ticker := time.NewTicker(m.opts.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stop:
			return
		case <-ticker.C:
			info, err := m.SnapshotNow()
			if err != nil {
				m.opts.Logger.Printf("%v", err)
				continue
			}
			m.opts.Logger.Printf("snapshot: wrote %d records (%d bytes) to %s in %s",
				info.Records, info.Bytes, info.Path, info.Duration.Round(time.Millisecond))
		}
	}
}

// list returns snapshot paths in Dir, oldest first. File names embed a
// fixed-width nanosecond timestamp, so lexical order is chronological.
func (m *Manager) list() ([]string, error) {
	dirents, err := os.ReadDir(m.opts.Dir)
	if err != nil {
		return nil, fmt.Errorf("snapshot: list %s: %w", m.opts.Dir, err)
	}
	var paths []string
	for _, de := range dirents {
		name := de.Name()
		if de.IsDir() || !strings.HasPrefix(name, filePrefix) || !strings.HasSuffix(name, fileSuffix) {
			continue
		}
		paths = append(paths, filepath.Join(m.opts.Dir, name))
	}
	sort.Strings(paths)
	return paths, nil
}

// prune removes the oldest snapshots so that at most Retain remain.
func (m *Manager) prune() error {
	paths, err := m.list()
	if err != nil {
		return err
	}
	var errs []error
	for len(paths) > m.opts.Retain {
		if err := os.Remove(paths[0]); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
		paths = paths[1:]
	}
	return errors.Join(errs...)
}

// readAll decodes every record in r and verifies the trailer before
// returning, so a corrupt snapshot never reaches the store.
func readAll(r io.Reader) ([]store.Entry, time.Time, error) {
	dec, err := NewDecoder(r)
	if err != nil {
		return nil, time.Time{}, err
	}
	var entries []store.Entry
	for {
		rec, err := dec.Next()
		if errors.Is(err, io.EOF) {
			return entries, dec.Created(), nil
		}
		if err != nil {
			return nil, time.Time{}, err
		}
		entries = append(entries, store.Entry{Key: rec.Key, Value: rec.Value, Version: rec.Version})
	}
}
