# 0010 — Load-testing methodology: k6 with a p99<200ms gate

**Status:** Accepted · **Date:** 2026-09-03

## Context
The Definition of Done budgets p99 Add/Exists latency at < 200ms "under
representative load." That requires a load generator that can (a) drive the real
HTTP API at controlled concurrency, (b) report p99 (not just mean), and (c) turn
the SLO into a machine-checkable pass/fail so it can gate CI rather than be
eyeballed. Options considered: **k6** (JS scenarios, native percentile
thresholds, clean CI exit codes), **vegeta** (Go-native, simple, histogram
output), and a **hand-rolled Go driver** (no dep, but we'd build reporting and
thresholding ourselves).

## Decision
1. **Use k6.** Its first-class `thresholds` (`http_req_duration: ['p(99)<200']`)
   express the SLO declaratively and make k6 exit non-zero on breach — the SLO
   *is* the gate, no parsing. It runs as the `grafana/k6` container, so no
   toolchain is added to the repo; a Go-native option's saved dependency didn't
   outweigh k6's built-in percentile gating and readable summary.
2. **Representative workload.** A ramping-VUs scenario (0→20 over 15s, hold 20 for
   60s, drain) drives a ~50/50 Add/Exists mix — the two hot user-facing paths —
   against a 1M-capacity filter. Steady concurrency, not a spike test; the goal is
   a stable p99 under sustained load, matching the DoD's "representative."
3. **Correctness rides along.** Each VU re-reads the key it just added and asserts
   `data.exists === true` — a wire-level zero-false-negative check under
   concurrency — plus an `op_errors` rate threshold (<0.1%).
4. **Invocation is one command.** `make load` boots the compose stack, runs k6
   against `localhost:8080`, and tears down; the same script runs in CI.

## Consequences
- (+) The p99 SLO is enforced automatically; a regression fails the build.
- (+) No Go dependency added — k6 is a container, invoked, then gone.
- (+) The same run also catches errors and false negatives under concurrency.
  (First local run: p(99)=15.06ms, op_errors=0.00%, ~317k requests, 0 failed.)
- (−) k6 scenarios are JavaScript — a second language in the repo for tests only.
- (−) Measured p99 depends on the host; the local `make load` number is
   indicative, and a CI baseline should be established before hard-failing on it.

## Usage
`test/load/bloom_load.js`, `test/load/README.md`, the `load` target in `Makefile`.
The suite it belongs to is [0009](./0009-testing-strategy.md); the running stack
is [0005](./0005-observability-otel-lgtm.md).
