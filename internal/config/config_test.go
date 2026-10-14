package config

import (
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) lookupFunc {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := load(env(nil))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg != Default() {
		t.Fatalf("got %+v, want defaults %+v", cfg, Default())
	}
}

func TestLoadOverrides(t *testing.T) {
	cfg, err := load(env(map[string]string{
		"KVCACHE_LISTEN_ADDR":     "127.0.0.1:9000",
		"KVCACHE_SHARDS":          "16",
		"KVCACHE_MAX_VALUE_BYTES": "4096",
		"KVCACHE_READ_TIMEOUT":    "250ms",
		"KVCACHE_LOG_REQUESTS":    "true",
	}))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.ListenAddr != "127.0.0.1:9000" {
		t.Errorf("ListenAddr = %q", cfg.ListenAddr)
	}
	if cfg.ShardCount != 16 {
		t.Errorf("ShardCount = %d", cfg.ShardCount)
	}
	if cfg.MaxValueBytes != 4096 {
		t.Errorf("MaxValueBytes = %d", cfg.MaxValueBytes)
	}
	if cfg.ReadTimeout != 250*time.Millisecond {
		t.Errorf("ReadTimeout = %s", cfg.ReadTimeout)
	}
	if !cfg.LogRequests {
		t.Errorf("LogRequests = false, want true")
	}
}

func TestLoadRejectsInvalid(t *testing.T) {
	cases := map[string]map[string]string{
		"non power of two shards":    {"KVCACHE_SHARDS": "12"},
		"zero shards":                {"KVCACHE_SHARDS": "0"},
		"garbage int":                {"KVCACHE_MAX_KEY_BYTES": "lots"},
		"garbage duration":           {"KVCACHE_WRITE_TIMEOUT": "soon"},
		"garbage bool":               {"KVCACHE_LOG_REQUESTS": "yes please"},
		"janitor interval too small": {"KVCACHE_JANITOR_INTERVAL": "1ms"},
		"janitor sample size zero":   {"KVCACHE_JANITOR_SAMPLE_SIZE": "0"},
	}
	for name, vars := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := load(env(vars))
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.HasPrefix(err.Error(), "config: ") {
				t.Errorf("error %q is missing the package prefix", err)
			}
		})
	}
}
