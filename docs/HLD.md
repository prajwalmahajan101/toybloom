# HLD — Dynamic (Scalable) Bloom Filter Service

**Status:** Draft · **Date:** 2026-07-16 · Companion to [PRD](./PRD.md), [LLD](./LLD.md), [RFC](./RFC.md)

## 1. Overview
A Go service exposing a Scalable Bloom Filter (SBF) over Valkey. `k` and `m` are
derived per stage from `n` and `p`; the filter chains new, larger stages as it
fills so the compounded false-positive rate stays ≤ `p`. Delivered as a core
library plus a thin Gin REST wrapper, fully instrumented with OpenTelemetry.

## 2. Architecture
```
                 ┌──────────────────────────────────────────┐
   HTTP clients ─┤ Gin REST (/v1)  ── internal/api           │
                 │      │ handlers, validation, envelopes     │
                 │      ▼                                     │
                 │ SBF core ── internal/bloom                 │
                 │  sizing · double-hash · add/exists · grow  │
                 │      │ BitStore interface                  │
                 │      ▼                                     │
                 │ Storage ── internal/store                  │
                 │  ValkeyStore (primary) · MemcachedStore    │
                 └──────┬───────────────────────────┬────────┘
                        │ SETBIT/GETBIT/HSET/INCR/Lua │ OTLP
                        ▼                             ▼
                    ┌────────┐              ┌────────────────────┐
                    │ Valkey │              │ OTel Collector     │
                    └────────┘              └───┬────────┬───────┘
                                    metrics │  traces │  logs │
                                            ▼         ▼       ▼
                                     Prometheus    Tempo    Loki
                                            └────────┴───────┘
                                                     ▼
                                                  Grafana (UI)
```

## 3. Components
| Component | Package | Responsibility |
|-----------|---------|----------------|
| REST API | `internal/api` | Gin router, request validation, JSON error envelope, correlation IDs. |
| SBF core | `internal/bloom` | Sizing math, double hashing, Add/Exists, stage growth. Storage-agnostic. |
| Storage | `internal/store` | `BitStore` interface + Valkey (primary) & Memcached (limited) impls. |
| Observability | `internal/obs` | OTel tracer/meter/logger providers, metric instruments, OTLP exporters. |
| Entrypoint | `cmd/server` | Wire config → store → core → api → OTel; start HTTP server. |
| Local stack | `deploy/`, `docker-compose.yml` | Valkey + app + OTel Collector + Prometheus + Tempo + Loki + Grafana. |

## 4. Data flow
**Add(name, item):** API → core loads filter meta → hash item once → compute `k`
positions in current stage → group by shard → `BitStore.SetBits` (pipelined
SETBIT per shard) → `INCR fill_count` → if full, Lua-guarded stage append.

**Exists(name, item):** API → core → for each stage newest→oldest: compute `k`
positions → `BitStore.GetBits` (pipelined GETBIT) → if all set, return true
(short-circuit). Member iff present in **any** stage. No false negatives.

## 5. Scalability & sharding
- Single Valkey string caps at 512MB (~4.29B bits). Each stage bitmap is sharded
  into `2^25`-bit (4MB) chunks keyed `bf:{name}:s{i}:sh{j}`.
- Growth: stage `i` capacity = `n·2^i`; error budget tightened by `r=0.9` so the
  chain's total FPP ≤ `p`.

## 6. Concurrency
- Bit sets are atomic per SETBIT; a multi-bit Add is pipelined (idempotent — a
  duplicate Add re-sets the same bits harmlessly).
- Stage append is guarded by a Lua compare-and-append on `stage_count` so
  concurrent writers cannot create duplicate stages.

## 7. Observability (single OTLP pipeline)
- App emits one OTLP/gRPC stream (metrics + logs + traces) to the OTel Collector,
  which exports to Prometheus (metrics), Tempo (traces), Loki (logs).
- **RED** metrics per endpoint (rate, errors, latency histogram → p50/p95/p99
  with trace exemplars) plus **bloom** metrics (items added, stage count, fill
  ratio, estimated FPP, Valkey op duration).
- Structured logs carry `trace_id`/`span_id`; Grafana cross-links logs↔traces↔metrics.
- p99 SLO panel + alert (< 200ms).

## 8. Deployment (local dev)
`docker-compose up` starts: `valkey`, `bloomfilter` (the app), `otel-collector`,
`prometheus`, `tempo`, `loki`, `grafana`. Grafana datasources and the two
dashboards (RED, Bloom internals) are auto-provisioned from `deploy/grafana/`.

## 9. Failure modes
| Failure | Handling |
|---------|----------|
| Valkey unavailable | Ops return explicit errors (no silent fallback); circuit-breaker/timeout on the client. |
| Collector down | App buffers/drops telemetry per OTel exporter policy; request path unaffected. |
| Stage-append race | Lua guard ensures exactly one stage created. |
| Oversized filter | Sharding keeps every key < 512MB; no single-key ceiling hit. |
