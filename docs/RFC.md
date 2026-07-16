# RFC — Dynamic (Scalable) Bloom Filter over Valkey

**Status:** Proposed · **Author:** Prajwal Mahajan · **Date:** 2026-07-16
Companion to [PRD](./PRD.md), [HLD](./HLD.md), [LLD](./LLD.md)

## 1. Summary
Build a probabilistic membership service that, given `n` and `p`, provisions a
bloom filter with dynamically derived `k`/`m`, grows automatically past `n` while
keeping the false-positive rate ≤ `p`, stores bits in Valkey, and ships with
first-class OpenTelemetry observability. Deliver as a Go library + Gin REST API.

## 2. Motivation
Exact "seen before?" checks over 100M–1B+ keys are memory- and latency-heavy. A
bloom filter trades a tunable false-positive rate for O(k) sub-ms checks and tiny
storage. A *classic* bloom filter degrades once inserts exceed `n`; a **Scalable
Bloom Filter** preserves the target `p` as data grows. We want this as a shared,
observable service rather than re-implemented per team.

## 3. Proposal
- **Algorithm:** Scalable Bloom Filter (chained fixed-size stages), each stage
  sized from `n`,`p` with `m = ceil(-n·ln(p₀)/ln2²)`, `k = round((m/n)·ln2)`.
- **Growth:** tightening ratio `r=0.9`, growth ratio `s=2`; new stage when full.
- **Hashing:** double hashing (Kirsch–Mitzenmacher) from one xxhash128 digest.
- **Storage:** pluggable `BitStore`; Valkey primary (native SETBIT/GETBIT,
  pipelined, Lua-guarded stage append). Bitmaps sharded into 4MB keys.
- **API:** Gin `/v1` — create / add / check / stats / delete; multi-tenant.
- **Observability:** OTLP → OTel Collector → Prometheus/Tempo/Loki → Grafana;
  RED + bloom metrics, p99 SLO, traces + structured logs; `docker-compose` stack.

## 4. Alternatives considered
| Option | Why not (for v1) |
|--------|------------------|
| Classic fixed-size bloom filter | FPP blows past `p` once inserts exceed `n`; caller must know size upfront. |
| Counting Bloom Filter | Supports deletes but ~4× memory and no native Valkey bit-counter path; deletes are out of scope. |
| Cuckoo filter | Supports deletes and good locality, but more complex, and Valkey bit ops map cleanly to bloom. Revisit if deletes become required. |
| Valkey Bloom module (`valkey-bloom`) | Off-the-shelf, but less control over sharding/observability and couples us to a module; we want the algorithm in-app and pluggable storage. |
| Redis instead of Valkey | Equivalent bit ops; Valkey chosen as the open, license-clean default. Client abstraction keeps this swappable. |
| Memcached as primary | No native bit ops → racy blob RMW; kept only as a documented, limited secondary. |
| Direct metric scrape (no Collector) | Simpler but couples app to each backend and splits the pipeline; Collector gives one OTLP path and easy rerouting. |

## 5. Drawbacks / risks
- **No deletes** — reset means drop & rebuild. Acceptable per PRD; revisit with a
  Counting/Cuckoo variant if a real need appears.
- **Estimated FPP is an estimate** — derived from fill counts, not measured truth.
- **Operational surface** — the full LGTM stack adds containers; mitigated by
  keeping it to the `docker-compose` dev stack, prod wiring is deployment-specific.
- **Stage-append contention** under very high write concurrency — mitigated by the
  Lua compare-and-append; worst case a writer reloads and retries.

## 6. Rollout
1. Core library (sizing, hashing, sharding, SBF) with unit tests.
2. ValkeyStore + integration tests (testcontainers).
3. Gin REST wrapper.
4. OTel instrumentation.
5. `docker-compose` + Grafana/Prometheus/Tempo/Loki provisioning.
6. Load test to validate p99 < 200ms and measured FPP ≤ p.

## 7. Open questions
- Do we need per-filter configurable `r`/`s`, or are the `0.9`/`2` defaults enough?
- Should `bloom_false_positive_checks_total` be wired only in test harnesses, or
  exposed via an optional "known-truth" side channel in prod?
- Auth/rate-limiting: handle at an upstream gateway (assumed) or add in-service later?

## 8. Decision
Proceeding with the proposal above under a guided, milestone-by-milestone
hand-build (M0–M10; see [LLD](./LLD.md) and the milestone map). ADRs will record
the SBF, Valkey, sharding, double-hashing, and OTel/LGTM choices in `docs/adr/`.
