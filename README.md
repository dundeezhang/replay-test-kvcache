# kvcache

kvcache is a small in-memory key/value cache with an HTTP API. It is meant to
sit next to a service as a fast lookaside cache: no replication and no
clustering. Optional on-disk snapshots let a restarted node come back warm.

## Design

- Keys are partitioned across a fixed number of shards (`KVCACHE_SHARDS`),
  each guarded by its own `sync.RWMutex`, so writers to unrelated keys rarely
  contend.
- Values are opaque byte strings. Every write bumps a per-key version that is
  returned in the `ETag` header.
- The store copies values on the way in and on the way out, so callers never
  share memory with the cache.

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
| `KVCACHE_READ_TIMEOUT` | `5s` | Per-request read timeout. |
| `KVCACHE_WRITE_TIMEOUT` | `5s` | Per-request write timeout. |
| `KVCACHE_LOG_REQUESTS` | `false` | Log one line per request. |
| `KVCACHE_SNAPSHOT_DIR` | unset | Directory for snapshots. Snapshots are disabled when unset. |
| `KVCACHE_SNAPSHOT_INTERVAL` | `5m` | Time between periodic snapshots (minimum `1s`). |
| `KVCACHE_SNAPSHOT_RETAIN` | `3` | Number of snapshot files to keep. |

## Persistence

When `KVCACHE_SNAPSHOT_DIR` is set, kvcache writes the whole store to
`snapshot-<unix-nanos>.kvs` every `KVCACHE_SNAPSHOT_INTERVAL` and keeps the newest
`KVCACHE_SNAPSHOT_RETAIN` files. Each file is written to a temporary name and
renamed into place, and ends with a CRC-32C checksum.

At startup the newest snapshot is loaded before the API starts listening. A
snapshot that fails its checksum is skipped and the next older one is tried.
Shards are exported one at a time, so taking a snapshot only briefly blocks
writers to the shard being copied.

Writes made after the last snapshot are lost on restart; treat snapshots as a
warm start, not durable storage.

## API

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/healthz` | Liveness probe. |
| `GET` | `/v1/stats` | Key count, approximate bytes, and shard count. |
| `POST` | `/v1/admin/snapshot` | Write a snapshot now. `503` if snapshots are disabled. |
| `GET` | `/v1/keys?prefix=&limit=` | List keys in lexical order. |
| `GET` | `/v1/keys/{key}` | Fetch a value. Returns `404` if missing. |
| `PUT` | `/v1/keys/{key}` | Store the request body as the value. |
| `DELETE` | `/v1/keys/{key}` | Remove a key. |

Example:

```sh
curl -X PUT --data-binary 'teal' localhost:7070/v1/keys/color
curl localhost:7070/v1/keys/color
```

## Development

Run `make lint race` before sending a change. New endpoints need a handler test
in `internal/server`, and store changes need a concurrent test that passes under
`-race`.
