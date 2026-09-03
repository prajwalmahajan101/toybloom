# 0009 — Testing strategy: layered pyramid with build-tag isolation

**Status:** Accepted · **Date:** 2026-09-03

## Context
Through M9 the suite was almost entirely fast, hermetic unit tests plus a set of
store tests that talk to a real Valkey behind a `//go:build integration` tag.
M10's Definition of Done makes three *empirical* promises — measured FPP ≤ the
configured `p`, p99 Add/Exists latency < 200ms under load, and zero false
negatives — that unit tests cannot prove. Proving them needs tests that exercise
the real `service → bloom → store` stack and the real HTTP transport, which in
turn need external infrastructure (Valkey, a running server). Those tests are
slower and Docker-dependent, so they must not run in the default `go test ./...`
loop that gates every commit.

## Decision
1. **Four layers, each with a job.** *Unit* (`*_test.go`, no tag) — pure logic,
   run always. *Integration* (`//go:build integration`) — one component against
   real Valkey via `testcontainers-go`. *E2E* (`//go:build e2e`, package
   `internal/e2e`) — the whole stack: statistical correctness in-process, plus an
   HTTP walk against a running server. *Load* (k6, ADR 0010) — latency SLO.
2. **Build tags isolate the slow layers.** `make test` stays tag-free, hermetic,
   and fast. `make integration` / `make e2e` opt into infra. This keeps the
   pre-commit gate quick while making the heavy evidence one command away.
3. **FPP is proven statistically, against the aggregate `p`.** The SBF construction
   sets per-stage error `p_i = p·(1−r)·r^i`; the geometric sum is `p`, so the
   *aggregate* false-positive probability across all stages is bounded by the
   user-configured `p`. `TestMeasuredFPP` inserts `n` known keys and probes a
   large disjoint set (200k), asserting the observed rate ≤ `p·1.5`. The 1.5×
   slack is a binomial-noise margin: at p=0.01, M=200k the true-mean std is ≈44
   hits, so the limit sits ~22σ above the mean — non-flaky, yet a broken filter
   (rate ≫ p) still trips it. (First run: measured FPP = 0.00103, target 0.01.)
4. **Zero-false-negative is asserted as an absolute.** `TestZeroFalseNegatives`
   inserts enough items to force ≥3 SBF stages, then requires *every* inserted
   key to read back present; one miss fails the run.
5. **The HTTP e2e is env-gated.** `internal/e2e/http_test.go` skips unless
   `E2E_BASE_URL` is set, so `go test -tags=e2e ./...` is safe without a server;
   `make e2e` provides the server via compose.

## Consequences
- (+) Each DoD guarantee has a named, runnable test producing fresh evidence.
- (+) Default commit gate stays fast and hermetic; infra tests are explicit.
- (+) Statistical thresholds are documented, not magic numbers.
- (−) `make e2e` / `make load` need Docker and take minutes, not seconds.
- (−) The statistical tests are probabilistic; the margin is chosen to make false
   failures astronomically unlikely, but not mathematically impossible.

## Usage
`internal/e2e/{doc.go,correctness_test.go,http_test.go}`, the `integration` /
`e2e` / `load` targets in `Makefile`, and the `//go:build` tags. Load specifics
are in [0010](./0010-load-testing-methodology.md); the stack under test is
[0005](./0005-observability-otel-lgtm.md); the SBF math is [0001](./0001-scalable-bloom-filter.md).
