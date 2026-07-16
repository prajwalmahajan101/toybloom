# PRD — Dynamic (Scalable) Bloom Filter Service

**Status:** Draft · **Owner:** Prajwal Mahajan · **Date:** 2026-07-16

## 1. Problem
Membership checks ("have we seen X?") over very large sets are expensive to do
exactly (DB lookups, full sets in memory). We need a probabilistic membership
service that answers in sub-millisecond time, uses little memory, and lets the
caller pick a tolerated false-positive rate — while continuing to hold that rate
even as the dataset grows beyond its initial estimate.

## 2. Goal
Given `n` (expected number of items) and `p` (target false-positive
probability), provision a bloom filter whose number of hash functions `k` and
bit-array size `m` are **derived dynamically** from `n` and `p`, backed by
Valkey for bit storage, and expose it as a Go library plus a Gin REST API.

## 3. Users
- **Backend services** that need dedup / "seen before" checks (crawlers, event
  pipelines, fraud/abuse pre-filters, cache-miss guards).
- **Platform/SRE** who operate the service and need SLO-grade observability.

## 4. Functional requirements
| ID | Requirement |
|----|-------------|
| F1 | Create a named filter from `{name, n, p}`; `k` and `m` derived automatically. |
| F2 | Add an item to a filter. |
| F3 | Check membership of an item (no false negatives; false positives ≤ `p`). |
| F4 | Filter **grows automatically** past `n` while keeping overall FPP ≤ `p` (Scalable Bloom Filter). |
| F5 | Report filter stats: stages, `m`, `k`, fill count/ratio, estimated live FPP. |
| F6 | Delete/drop a filter. |
| F7 | Support many independent named filters in one deployment (multi-tenant). |
| F8 | Pluggable storage backend; **Valkey** is the primary/first implementation. |

## 5. Non-functional requirements
| ID | Requirement |
|----|-------------|
| N1 | **p99 latency < 200ms** for Add and Exists (user-facing budget). |
| N2 | Scale to **100M–1B+ items** per filter (bit array sharded across Valkey keys). |
| N3 | Concurrent writers safe (atomic bit ops; guarded stage growth). |
| N4 | Full observability: OpenTelemetry **metrics + logs + traces**, surfaced in Grafana (Prometheus/Tempo/Loki). RED metrics + bloom-internal metrics. |
| N5 | One-command local stack via `docker-compose` (app + Valkey + full LGTM). |
| N6 | Deterministic membership: **zero false negatives** guaranteed. |

## 6. Success metrics
- Measured false-positive rate ≤ configured `p` across stage growth (verified in tests).
- p99 Add/Exists latency < 200ms under representative load.
- Zero false negatives observed in integration tests.
- Grafana dashboards show live RED + bloom metrics out of the box.

## 7. Out of scope (YAGNI)
- Item **deletion** (use Counting Bloom Filter later if needed; reset = drop & rebuild).
- Cross-region replication / HA of Valkey (rely on Valkey's own clustering).
- Authn/z on the REST API (assumed behind an internal gateway for v1).
- Persistence guarantees beyond Valkey's own durability config.

## 8. Assumptions
- Callers can tolerate a bounded false-positive rate.
- Valkey is reachable with low latency (same cluster/VPC).
- Item keys are arbitrary byte strings.

## 9. References
- Almeida et al., *Scalable Bloom Filters* (2007).
- Kirsch & Mitzenmacher, *Less Hashing, Same Performance: Building a Better Bloom Filter* (2006).
