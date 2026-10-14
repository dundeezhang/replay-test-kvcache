// Package config loads kvcache runtime settings from the environment.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds the runtime settings for a kvcache node.
type Config struct {
	// ListenAddr is the host:port the HTTP API binds to.
	ListenAddr string

	// ShardCount is the number of independent lock domains in the store.
	// It must be a power of two so shard selection can use a bit mask.
	ShardCount int

	// MaxValueBytes caps the size of a single stored value.
	MaxValueBytes int

	// MaxKeyBytes caps the length of a key.
	MaxKeyBytes int

	// JanitorInterval is how often the background janitor samples shards
	// for expired keys, and JanitorSampleSize is how many entries it
	// examines per shard per pass.
	JanitorInterval   time.Duration
	JanitorSampleSize int

	// ReadTimeout and WriteTimeout bound a single HTTP request.
	ReadTimeout  time.Duration
	WriteTimeout time.Duration

	// LogRequests enables per-request access logging.
	LogRequests bool
}

// Default returns the configuration used when no overrides are set.
func Default() Config {
	return Config{
		ListenAddr: ":7070",

		ShardCount:    64,
		MaxValueBytes: 1 << 20,
		MaxKeyBytes:   250,

		JanitorInterval:   time.Second,
		JanitorSampleSize: 20,

		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,

		LogRequests: false,
	}
}

// Load builds a Config from KVCACHE_* environment variables layered on top
// of Default, then validates it.
func Load() (Config, error) {
	return load(os.LookupEnv)
}

type lookupFunc func(key string) (string, bool)

func load(lookup lookupFunc) (Config, error) {
	cfg := Default()

	if v, ok := lookup("KVCACHE_LISTEN_ADDR"); ok && v != "" {
		cfg.ListenAddr = v
	}
	if err := intVar(lookup, "KVCACHE_SHARDS", &cfg.ShardCount); err != nil {
		return Config{}, err
	}
	if err := intVar(lookup, "KVCACHE_MAX_VALUE_BYTES", &cfg.MaxValueBytes); err != nil {
		return Config{}, err
	}
	if err := intVar(lookup, "KVCACHE_MAX_KEY_BYTES", &cfg.MaxKeyBytes); err != nil {
		return Config{}, err
	}
	if err := durationVar(lookup, "KVCACHE_JANITOR_INTERVAL", &cfg.JanitorInterval); err != nil {
		return Config{}, err
	}
	if err := intVar(lookup, "KVCACHE_JANITOR_SAMPLE_SIZE", &cfg.JanitorSampleSize); err != nil {
		return Config{}, err
	}
	if err := durationVar(lookup, "KVCACHE_READ_TIMEOUT", &cfg.ReadTimeout); err != nil {
		return Config{}, err
	}
	if err := durationVar(lookup, "KVCACHE_WRITE_TIMEOUT", &cfg.WriteTimeout); err != nil {
		return Config{}, err
	}
	if err := boolVar(lookup, "KVCACHE_LOG_REQUESTS", &cfg.LogRequests); err != nil {
		return Config{}, err
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate reports whether the configuration is usable.
func (c Config) Validate() error {
	if c.ShardCount <= 0 || c.ShardCount&(c.ShardCount-1) != 0 {
		return fmt.Errorf("config: KVCACHE_SHARDS must be a positive power of two, got %d", c.ShardCount)
	}
	if c.JanitorInterval < 10*time.Millisecond {
		return fmt.Errorf("config: KVCACHE_JANITOR_INTERVAL must be at least 10ms, got %s", c.JanitorInterval)
	}
	if c.JanitorSampleSize <= 0 {
		return fmt.Errorf("config: KVCACHE_JANITOR_SAMPLE_SIZE must be positive, got %d", c.JanitorSampleSize)
	}
	if c.MaxValueBytes <= 0 {
		return fmt.Errorf("config: KVCACHE_MAX_VALUE_BYTES must be positive, got %d", c.MaxValueBytes)
	}
	if c.MaxKeyBytes <= 0 {
		return fmt.Errorf("config: KVCACHE_MAX_KEY_BYTES must be positive, got %d", c.MaxKeyBytes)
	}
	if c.ReadTimeout <= 0 || c.WriteTimeout <= 0 {
		return fmt.Errorf("config: request timeouts must be positive")
	}
	return nil
}

func intVar(lookup lookupFunc, name string, dst *int) error {
	v, ok := lookup(name)
	if !ok || v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fmt.Errorf("config: %s: %w", name, err)
	}
	*dst = n
	return nil
}

func durationVar(lookup lookupFunc, name string, dst *time.Duration) error {
	v, ok := lookup(name)
	if !ok || v == "" {
		return nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fmt.Errorf("config: %s: %w", name, err)
	}
	*dst = d
	return nil
}

func boolVar(lookup lookupFunc, name string, dst *bool) error {
	v, ok := lookup(name)
	if !ok || v == "" {
		return nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fmt.Errorf("config: %s: %w", name, err)
	}
	*dst = b
	return nil
}
