# 0008 — Per-filter gauges with a bounded cardinality cap

**Status:** Accepted · **Date:** 2026-09-05 · **Amends:** [0007](./0007-otel-instrumentation-boundaries.md)

## Context
ADR 0007 (decision #2) kept filter names out of metric labels entirely, on the
grounds that names are user-supplied and unbounded, and deferred the fill-ratio
and estimated-FPP gauges to M9 because they "need a live-filter registry to
observe correctly." M9 (the local stack) is the point where those gauges become
useful — Grafana's Bloom dashboard is built around them. But a per-filter
saturation/FPP gauge is *inherently* per-filter: it must carry a `filter` label,
which is exactly the unbounded label ADR 0007 avoided. That tension has to be
resolved rather than left implicit.

Building the gauges also surfaced a latent correctness bug: `Add` incremented an
atomic `:fill` counter and updated the in-process stage, but `Load` reconstructed
`Stage.Fill` from the stage-meta *hash* field, which is only seeded at creation
and never updated. Any fresh `Load` — the `GET /stats` endpoint and the gauge
callback alike — therefore reported `Fill = 0`, making both the gauges and stats
under-report load.

## Decision
1. **Per-filter gauges are allowed, with an explicit cardinality cap.** The
   observable gauges carrying a `filter` label — `toybloom.filter.fill_ratio`,
   `toybloom.filter.estimated_fpp`, and `toybloom.filter.items` (current element
   count) — are capped by `OBS_MAX_FILTER_GAUGES` (default 100); beyond the cap
   the service truncates and warns. A scalar `toybloom.filter.count` (number of
   live filters, **no** label) rides the same callback and is always the true
   pre-cap total. This preserves ADR 0007's *intent* (bounded cardinality) while
   admitting the one label the per-filter gauges genuinely need. The high-churn
   counters/histograms from ADR 0007 keep **no** filter label.
2. **Enumeration reuses the existing `bf:registry` set.** No new in-process
   registry: `bloom.ListFilters` wraps `SMembers(bf:registry)`, and the
   observable-gauge callback (registered by `obs.Instruments.ObserveFilters`,
   supplied by `service.FilterSamples`) loads each live filter and computes the
   two values from `Filter.Stats()`. The callback is resilient — a filter deleted
   mid-scrape is skipped, never fatal.
3. **`Load` reads the authoritative fill counter.** A `Get(key)` method was added
   to the `BitStore` interface (ADR 0002); `Load` now reads each stage's `:fill`
   counter to populate `Stage.Fill`, so stats and gauges reflect real load. The
   hash `fill_count` field remains only as a creation-time seed.
4. **Fill ratio is insertion-fill ÷ capacity**, not literal set-bits ÷ m — it
   needs no `BITCOUNT` and is computed from data `Stats()` already carries.

## Consequences
- (+) The Bloom dashboard shows real per-filter saturation and estimated FPP.
- (+) `GET /stats` is now correct across process restarts (same root-cause fix).
- (+) `filter`-label cardinality is bounded and operator-tunable.
- (−) `filter` names now appear in one metric label family (capped) — a
   deliberate, narrow reversal of ADR 0007 #2.
- (−) Each metric collection does `SMembers` + one `Load` per live filter
   (bounded by the cap); acceptable at the SDK's ~60s cadence.
- (−) `BitStore` gained a `Get` method — a small widening of the storage contract.

## Usage
`pkg/bloom/estimate.go` (`FillRatio`, `EstimatedFPP`, `ListFilters`),
`pkg/store` (`Get` on the interface, `ValkeyStore`, `MemStore`), `Load` in
`pkg/bloom/filter.go`, `internal/obs/metrics.go` (`ObserveFilters`, the two
gauges), `internal/service/service.go` (`FilterSamples` + the cap), and
`OBS_MAX_FILTER_GAUGES` in `internal/core/config`. See ADR 0005 for the stack and
ADR 0007 for the boundaries this amends.
