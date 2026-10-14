// Command kvcache runs a single in-memory cache node.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/dundeezhang/replay-test-kvcache/internal/config"
	"github.com/dundeezhang/replay-test-kvcache/internal/server"
	"github.com/dundeezhang/replay-test-kvcache/internal/snapshot"
	"github.com/dundeezhang/replay-test-kvcache/internal/store"
)

func main() {
	logger := log.New(os.Stderr, "kvcache ", log.LstdFlags|log.Lmicroseconds)

	cfg, err := config.Load()
	if err != nil {
		logger.Fatalf("load config: %v", err)
	}

	st := store.New(store.Options{
		Shards:        cfg.ShardCount,
		MaxKeyBytes:   cfg.MaxKeyBytes,
		MaxValueBytes: cfg.MaxValueBytes,
	})

	api := server.New(st, cfg, logger)

	if cfg.SnapshotDir != "" {
		snaps, err := snapshot.NewManager(st, snapshot.Options{
			Dir:      cfg.SnapshotDir,
			Interval: cfg.SnapshotInterval,
			Retain:   cfg.SnapshotRetain,
			Logger:   logger,
		})
		if err != nil {
			logger.Fatalf("snapshots: %v", err)
		}

		info, found, err := snaps.Restore()
		switch {
		case err != nil:
			logger.Fatalf("snapshots: restore: %v", err)
		case !found:
			logger.Printf("snapshots: none in %s, starting empty", cfg.SnapshotDir)
		default:
			logger.Printf("snapshots: restored %d keys from %s (taken %s)", info.Records, info.Path, info.Created)
		}

		api.SetSnapshotter(snaps)
		snaps.Start()
	}

	srv := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      api.Handler(),
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	logger.Printf("listening on %s (shards=%d)", cfg.ListenAddr, cfg.ShardCount)
	if err := srv.ListenAndServe(); err != nil {
		logger.Fatalf("serve: %v", err)
	}
}
