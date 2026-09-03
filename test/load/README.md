# Load test — p99 latency SLO

`bloom_load.js` is a [k6](https://k6.io) scenario that drives the two hot paths
(Add / Exists) at steady concurrency and **fails the run** if the 99th-percentile
request latency breaches the Definition-of-Done budget (`p(99) < 200ms`) or the
error rate exceeds 0.1%.

## Run it

Bring the stack up and drive it (from repo root):

```bash
make load
```

Or manually against any running server:

```bash
docker run --rm -i --network host \
  -e BASE_URL=http://localhost:8080 \
  grafana/k6 run - < test/load/bloom_load.js
```

k6 exits non-zero when a threshold is breached, so this gates in CI.

## What "pass" means

- `http_req_duration ... p(99)<200` is green.
- `op_errors rate<0.001` is green.
- `checks` shows `add ok`, `exists ok`, and `no false negative` at ~100%.

See [ADR 0010](../../docs/ADR/0010-load-testing-methodology.md) for the workload
rationale and SLO.

## Records

Best-3-of-5 p99, most recent measurement (full table + all five runs in
[RESULTS.md](./RESULTS.md)):

| Date | Best p99 | Best-3 mean p99 | Errors | SLO |
|------|----------|-----------------|--------|-----|
| 2026-09-03 | 14.68ms | 15.95ms | 0.00% | ✅ p(99)<200 |
