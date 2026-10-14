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

## Building

```sh
make build      # produces bin/kvcache
make test       # unit tests
make race       # unit tests with the race detector
```

Requires Go 1.22 or newer (the router uses method and wildcard patterns).

## Authentication

When `KVCACHE_TOKEN` is set, every request except `GET /healthz` must carry
`Authorization: Bearer <token>`; anything else gets `401` with a
`WWW-Authenticate` challenge. Leave it unset only on a trusted network.

To rotate the token without downtime:

1. Set `KVCACHE_TOKEN_NEXT` to the new token and restart; both are accepted.
2. Move clients over to the new token.
3. Promote it to `KVCACHE_TOKEN`, unset `KVCACHE_TOKEN_NEXT`, and restart.

```sh
curl -H "Authorization: Bearer $KVCACHE_TOKEN" localhost:7070/v1/stats
```

## Configuration

All settings come from environment variables.

| Variable | Default | Description |
| --- | --- | --- |
| `KVCACHE_LISTEN_ADDR` | `:7070` | Address the HTTP API listens on. |
| `KVCACHE_TOKEN` | unset | Bearer token required on every request except `/healthz`. |
| `KVCACHE_TOKEN_NEXT` | unset | Second accepted token, used while rotating `KVCACHE_TOKEN`. |
| `KVCACHE_SHARDS` | `64` | Number of shards; must be a power of two. |
| `KVCACHE_MAX_VALUE_BYTES` | `1048576` | Largest accepted value. |
| `KVCACHE_MAX_KEY_BYTES` | `250` | Longest accepted key. |
| `KVCACHE_READ_TIMEOUT` | `5s` | Per-request read timeout. |
| `KVCACHE_WRITE_TIMEOUT` | `5s` | Per-request write timeout. |
| `KVCACHE_LOG_REQUESTS` | `false` | Log one line per request. |

## API

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/healthz` | Liveness probe. |
| `GET` | `/v1/stats` | Key count, approximate bytes, and shard count. |
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
