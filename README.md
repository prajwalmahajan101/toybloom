# bloomfilter — Dynamic (Scalable) Bloom Filter over Valkey

A probabilistic membership service. Give it `n` (expected items) and `p` (target
false-positive probability); it provisions a bloom filter with **dynamically
derived** `k` (hash functions) and `m` (bits), stores the bits in **Valkey**, and
**grows automatically** past `n` while keeping the false-positive rate ≤ `p`
(Scalable Bloom Filter). Shipped as a Go library plus a thin **Gin** REST API,
fully observable with **OpenTelemetry** (metrics + logs + traces) in **Grafana**.

> Module: `github.com/prajwalmahajan101/bloomfilter`

## Why
Exact "have we seen X?" checks over 100M–1B+ keys are slow and memory-heavy. A
bloom filter answers in O(k) sub-millisecond time with a tunable false-positive
rate and tiny storage — and this one holds that rate even as the dataset grows.

## Features
- Dynamic `k`/`m` from `n`,`p` (optimal sizing).
- Scalable Bloom Filter: auto-chains larger stages; compounded FPP ≤ `p`.
- Double hashing (Kirsch–Mitzenmacher) over one xxhash128 digest.
- Valkey-backed bit storage, sharded across 4MB keys for 1B+ scale.
- Pluggable `BitStore` (Valkey primary; Memcached limited/secondary).
- Multi-tenant: many independent named filters.
- Full OTel observability, RED + bloom metrics, p99 SLO, Grafana dashboards.
- Zero false negatives.

## Quickstart (local stack)
```bash
docker-compose up          # app + Valkey + OTel Collector + Prometheus + Tempo + Loki + Grafana
```
- API:        http://localhost:8080
- Grafana:    http://localhost:3000  (RED + Bloom dashboards auto-provisioned)
- Prometheus: http://localhost:9090

## API (v1)
| Method | Path | Body | Result |
|--------|------|------|--------|
| POST | `/v1/filters` | `{name, n, p}` | create filter → `201` |
| POST | `/v1/filters/:name/items` | `{value}` | add item |
| GET | `/v1/filters/:name/items/:value` | — | `{exists: bool}` |
| GET | `/v1/filters/:name` | — | stats (stages, m, k, fill, est. FPP) |
| DELETE | `/v1/filters/:name` | — | drop filter |

```bash
# create, add, check
curl -X POST localhost:8080/v1/filters -d '{"name":"seen","n":1000000,"p":0.01}'
curl -X POST localhost:8080/v1/filters/seen/items -d '{"value":"user-42"}'
curl localhost:8080/v1/filters/seen/items/user-42        # {"exists":true}
```

## Benchmarks
Load-tested with [k6](https://k6.io) against the full compose stack (ramping to
20 VUs, ~50/50 Add/Exists on a 1M-capacity filter). SLO: `p(99) < 200ms`,
`errors < 0.1%`. Best 3 of 5 runs (2026-09-03):

| Run | p99 | p95 | median | throughput | errors |
|-----|-----|-----|--------|------------|--------|
| best | **14.68ms** | 8.87ms | 4.16ms | 3411 req/s | 0.00% |
| 2nd | 14.97ms | 9.94ms | 4.39ms | 3196 req/s | 0.00% |
| 3rd | 18.21ms | 12.20ms | 5.26ms | 2667 req/s | 0.00% |

Best-3 mean p99 ≈ **15.95ms** — ~12.5× under the 200ms budget; 1.2M+ total
requests across 5 runs, 0 failed, 0 false negatives. Full benchmark (all 5 runs)
in [`test/load/RESULTS.md`](./test/load/RESULTS.md); run it with `make load`.

## Layout
```
cmd/server        Gin REST entrypoint (+ OTel bootstrap)
internal/bloom    SBF core (sizing, hashing, add/exists, stage growth)
internal/store    BitStore interface + valkey + memcached
internal/api      Gin handlers, envelopes, validation
internal/obs      OTel providers + metric instruments
deploy            docker-compose provisioning (collector, prometheus, tempo, loki, grafana)
docs              PRD / HLD / LLD / RFC, ADRs, phase journal
```

## Development

Enable the pre-commit quality gate once per clone:

```bash
git config core.hooksPath .githooks
```

On every commit it runs `gofmt` (staged Go files), `go vet ./...`, `go test ./...`,
and `golangci-lint run` (skipped if not installed). Install golangci-lint from
https://golangci-lint.run/welcome/install/.

## Documentation
See [`docs/`](./docs/README.md) — [PRD](./docs/PRD.md), [HLD](./docs/HLD.md),
[LLD](./docs/LLD.md), [RFC](./docs/RFC.md), [ADRs](./docs/ADR/README.md),
[Journal](./docs/Journal/README.md).

## Status
Early, guided hand-build (milestones M0–M10). Not yet released.
