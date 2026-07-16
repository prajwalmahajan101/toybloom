# Roadmap

Guided, milestone-by-milestone hand-build of the dynamic (scalable) bloom filter
service. Each milestone is one testable chunk: hand-written, verified, committed
before the next begins. Bottom-up — pure logic first, storage next, transport and
observability last.

**Legend:** ☐ pending · ◐ in progress · ☑ done

## Phase 1 — Core library (no Valkey, no HTTP)
| # | Milestone | Deliverable | Acceptance | Status |
|---|-----------|-------------|------------|--------|
| M0 | Project init | `go.mod`, deps (gin, xxhash, valkey-go), folder layout | `go build ./...` clean | ☑ |
| M1 | Sizing math | `internal/bloom/sizing.go` — `OptimalM/K`, `StageError`, `StageCapacity` | unit tests match self-check table | ☑ |
| M2 | Double hashing | `internal/bloom/hash.go` — `hashParts`, `positions` | k positions, each `< m`, `h2==0` guarded | ☑ |
| M3 | Sharding | `internal/bloom/shard.go` — `ShardBits`, `shardFor`, `groupByShard` | round-trip index↔(shard,offset) | ☐ |
| M4 | BitStore interface | `internal/store/store.go` — interface only | compiles; documented contract | ☐ |
| M5 | SBF core | `internal/bloom/filter.go` — `New/Load/Add/Exists/Stats/Drop`, stage growth | works against an in-memory fake store | ☐ |

## Phase 2 — Storage
| # | Milestone | Deliverable | Acceptance | Status |
|---|-----------|-------------|------------|--------|
| M6 | ValkeyStore | `internal/store/valkey.go` — pipelined SETBIT/GETBIT, HSET/HGETALL, INCR, Lua stage-append | integration test vs. real Valkey (testcontainers) | ☐ |

## Phase 3 — Transport
| # | Milestone | Deliverable | Acceptance | Status |
|---|-----------|-------------|------------|--------|
| M7 | Gin REST | `cmd/server`, `internal/api` — 5 endpoints, error envelope, validation | endpoint tests green; curl flow works | ☐ |

## Phase 4 — Observability & ops
| # | Milestone | Deliverable | Acceptance | Status |
|---|-----------|-------------|------------|--------|
| M8 | OTel instrumentation | `internal/obs` — tracer/meter/logger, `otelgin`, RED + bloom metrics | traces + metrics + logs emitted via OTLP | ☐ |
| M9 | Local stack | `docker-compose.yml`, `deploy/` — collector, prometheus, tempo, loki, grafana | `docker-compose up`; Grafana dashboards live | ☐ |
| M10 | Integration & ADRs | end-to-end tests, load test, ADRs finalized | measured FPP ≤ p; p99 < 200ms; zero false negatives | ☐ |

## Definition of done (v1)
- Measured false-positive rate ≤ configured `p` across stage growth.
- p99 Add/Exists latency < 200ms under representative load.
- Zero false negatives in integration tests.
- Grafana shows live RED + bloom metrics out of the box.
- Every architectural decision recorded in [ADR/](./ADR/README.md); each phase
  logged in [Journal/](./Journal/README.md).

## References
See [PRD](./PRD.md) for requirements, [HLD](./HLD.md)/[LLD](./LLD.md) for design,
[RFC](./RFC.md) for the proposal and alternatives.
