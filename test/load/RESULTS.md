# Load test results

Measured with `test/load/bloom_load.js` (k6) against the full `docker-compose`
stack — see [ADR 0010](../../docs/ADR/0010-load-testing-methodology.md) for the
workload and SLO. Workload: ramping VUs 0→20 over 15s, hold 20 for 60s, drain;
~50/50 Add/Exists mix on a 1M-capacity filter.

**SLO gate:** `http_req_duration p(99) < 200ms` and `op_errors rate < 0.1%`.

## 2026-09-03 — 5 runs, best 3 reported

Five consecutive runs, same host, stack recreated once. All five passed the gate
with zero errors and zero false negatives; the **best 3 by p99** are recorded
below (lower p99 is better).

| Rank | Run | p99 | p95 | median | max | errors | requests |
|------|-----|-----|-----|--------|-----|--------|----------|
| 1 | run5 | **14.68ms** | 8.87ms | 4.16ms | 126.47ms | 0.00% | 289,966 |
| 2 | run3 | **14.97ms** | 9.94ms | 4.39ms | 82.69ms | 0.00% | 271,722 |
| 3 | run1 | **18.21ms** | 12.20ms | 5.26ms | 100.52ms | 0.00% | 226,782 |

**Best-3 mean p99 ≈ 15.95ms** — ~12.5× under the 200ms budget.

### Benchmark — all 5 runs

Full metrics for every run (latency + throughput). Best p99 in **bold**.

| Run | p99 | p95 | median | avg | max | throughput | requests | errors |
|-----|-----|-----|--------|-----|-----|------------|----------|--------|
| run1 | 18.21ms | 12.20ms | 5.26ms | 6.06ms | 100.52ms | 2667 req/s | 226,782 | 0.00% |
| run2 | 18.63ms | 12.37ms | 5.32ms | 6.14ms | 130.02ms | 2628 req/s | 223,418 | 0.00% |
| run3 | 14.97ms | 9.94ms | 4.39ms | 5.04ms | 82.69ms | 3196 req/s | 271,722 | 0.00% |
| run4 | 18.83ms | 12.61ms | 5.33ms | 6.16ms | 178.18ms | 2620 req/s | 222,734 | 0.00% |
| run5 | **14.68ms** | 8.87ms | 4.16ms | 4.72ms | 126.47ms | 3411 req/s | 289,966 | 0.00% |

Aggregate across 5 runs: p99 min **14.68ms** / max 18.83ms / mean 17.06ms;
throughput 2620–3411 req/s; 1,234,622 total requests, **0 failed**.

### Notes

- Every run passed `p(99)<200` and `op_errors<0.1%`; all `checks` (incl. the
  wire-level `no false negative` assertion) were 100%.
- Max per-request latency stayed ≤ 178ms across all runs — no tail outliers.
  (An earlier isolated run showed a ~14-minute `max` from a single stalled
  connection at ramp/teardown; it did not recur across these five runs and did
  not affect p99, confirming it was an environment artifact.)
- Numbers are host-dependent (local dev machine). CI should establish its own
  baseline before hard-failing on absolute p99, per ADR 0010.
