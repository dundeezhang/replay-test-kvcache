// Command kvcache runs a single in-memory cache node.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/dundeezhang/replay-test-kvcache/internal/config"
	"github.com/dundeezhang/replay-test-kvcache/internal/server"
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

	janitor := store.NewJanitor(st, store.JanitorOptions{
		Interval:   cfg.JanitorInterval,
		SampleSize: cfg.JanitorSampleSize,
		Logger:     logger,
	})
	janitor.Start()

	api := server.New(st, cfg, logger)

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
