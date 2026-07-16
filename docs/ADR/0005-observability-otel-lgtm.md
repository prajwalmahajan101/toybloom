# 0005 — OpenTelemetry → Collector → Prometheus/Tempo/Loki + Grafana

**Status:** Accepted · **Date:** 2026-07-16

## Context
We need SLO-grade observability: RED metrics with p99 latency, distributed
traces across the request → bloom → Valkey path, and correlated structured logs.
The app should not be tightly coupled to each telemetry backend, and the whole
stack must run locally with one command.

## Decision
Instrument the app with the **OpenTelemetry SDK** and emit a **single OTLP/gRPC
stream** (metrics + logs + traces) to an **OpenTelemetry Collector**, which fans
out to **Prometheus** (metrics), **Tempo** (traces), and **Loki** (logs).
**Grafana** is the single UI (Grafana LGTM stack) with auto-provisioned
datasources and two dashboards: RED (rate/errors/p50-p95-p99 latency with trace
exemplars) and Bloom internals (items added, stage count, fill ratio, estimated
FPP, Valkey op latency). The full stack ships in `docker-compose`.

Rejected: direct per-backend export / `/metrics` scrape — simpler but couples the
app to each backend and splits the pipeline. Jaeger for traces — an extra UI vs.
one Grafana pane.

## Consequences
- (+) One instrumentation path; backends reroutable at the Collector without app changes.
- (+) Metrics/logs/traces cross-linked in a single Grafana UI; p99 SLO panel + alert.
- (−) More containers to run locally (scoped to the dev `docker-compose` stack).
- (−) Collector is an extra hop (buffers/drops telemetry on outage without affecting requests).

## Usage
`internal/obs` (providers, instruments), `otelgin` middleware, and `deploy/`
(collector, prometheus, tempo, loki, grafana configs). See HLD §7 and LLD §10.
