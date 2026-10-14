# kvcache

kvcache is a small in-memory key/value cache with an HTTP API. It is meant to
sit next to a service as a fast, disposable lookaside cache: no replication,
no persistence, and no clustering.

## Design

- Keys are partitioned across a fixed number of shards (`KVCACHE_SHARDS`),
  each guarded by its own `sync.RWMutex`, so writers to unrelated keys rarely
  contend.
- Values are opaque byte strings. Every write bumps a per-key version that is
  returned in the `ETag` header.
- The store copies values on the way in and on the way out, so callers never
  share memory with the cache.

## Observability

`GET /metrics` serves Prometheus text-format metrics without any client
library dependency:

- `kvcache_http_requests_total{method,path,code}`: completed requests.
- `kvcache_http_request_duration_seconds{path}`: latency histogram with
  buckets from 100µs to 1s.
- `kvcache_keys`, `kvcache_shards`, `kvcache_uptime_seconds`: gauges sampled at
  scrape time.

## Building

```sh
make build      # produces bin/kvcache
make test       # unit tests
make race       # unit tests with the race detector
```

Requires Go 1.22 or newer (the router uses method and wildcard patterns).

## Configuration

All settings come from environment variables.

| Variable | Default | Description |
| --- | --- | --- |
| `KVCACHE_LISTEN_ADDR` | `:7070` | Address the HTTP API listens on. |
| `KVCACHE_SHARDS` | `64` | Number of shards; must be a power of two. |
| `KVCACHE_MAX_VALUE_BYTES` | `1048576` | Largest accepted value. |
| `KVCACHE_MAX_KEY_BYTES` | `250` | Longest accepted key. |
| `KVCACHE_JANITOR_INTERVAL` | `1s` | Time between background expiry sweeps (minimum `10ms`). |
| `KVCACHE_JANITOR_SAMPLE_SIZE` | `20` | Entries examined per shard per sweep pass. |
| `KVCACHE_READ_TIMEOUT` | `5s` | Per-request read timeout. |
| `KVCACHE_WRITE_TIMEOUT` | `5s` | Per-request write timeout. |
| `KVCACHE_LOG_REQUESTS` | `false` | Log one line per request. |

## API

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/healthz` | Liveness probe. |
| `GET` | `/metrics` | Prometheus metrics in text format. |
| `GET` | `/v1/stats` | Key count, approximate bytes, and shard count. |
| `GET` | `/v1/keys?prefix=&limit=` | List keys in lexical order. |
| `GET` | `/v1/keys/{key}` | Fetch a value. Returns `404` if missing. |
| `PUT` | `/v1/keys/{key}?ttl=` | Store the request body as the value, optionally expiring after `ttl` seconds. |
| `DELETE` | `/v1/keys/{key}` | Remove a key. |
| `GET` | `/v1/keys/{key}/ttl` | Remaining lifetime in seconds; `-1` if the key never expires. |
| `PUT` | `/v1/keys/{key}/ttl` | Set a new TTL on an existing key: `{"ttl_seconds": 30}`. |
| `DELETE` | `/v1/keys/{key}/ttl` | Remove a key's expiry. |

Example:

```sh
curl -X PUT --data-binary 'teal' localhost:7070/v1/keys/color
curl localhost:7070/v1/keys/color
```

## Expiry

Keys written with a TTL stop being visible as soon as the TTL elapses: reads
return `404`, they are excluded from key listings, and a later `PUT` starts a
fresh version history at `1`. A plain `PUT` without `ttl` clears any existing
expiry.

Memory for expired keys is reclaimed in two ways:

1. **Lazily**, when a read touches an expired key.
2. **Actively**, by a background janitor. Every `KVCACHE_JANITOR_INTERVAL` it
   samples `KVCACHE_JANITOR_SAMPLE_SIZE` entries from each shard and deletes
   the expired ones. If more than a quarter of a sample had expired it samples
   that shard again, up to 16 passes, so a burst of simultaneous expiries is
   cleaned up quickly without one shard starving the others.

`GET /v1/stats` reports `expiring`, the number of keys that carry a TTL,
including expired keys the janitor has not reached yet.

```sh
curl -X PUT --data-binary 'abc' 'localhost:7070/v1/keys/session:42?ttl=900'
curl localhost:7070/v1/keys/session:42/ttl
# {"key":"session:42","ttl_seconds":900,"expires_at":"2024-03-01T12:15:00Z"}
```

## Development

Run `make lint race` before sending a change. New endpoints need a handler test
in `internal/server`, and store changes need a concurrent test that passes under
`-race`.
