# 0007 — OpenTelemetry instrumentation boundaries

**Status:** Accepted · **Date:** 2026-09-04

## Context
ADR 0005 chose the observability *stack* (OTel SDK → Collector → Prometheus/
Tempo/Loki + Grafana) but not the *boundaries*: where instrumentation lives, how
much of it couples the reusable `pkg/` SDK to OpenTelemetry, and how the existing
`correlation_id` relates to trace ids. M8 (app-side instrumentation) forced those
choices, and they are not obvious enough to leave implicit.

## Decision
1. **`pkg/` stays OTel-free except Valkey spans.** `pkg/bloom` (pure logic) has no
   OTel dependency. `pkg/store` opens a client span per Valkey op via the **global**
   API (`otel.Tracer(...)`), which is a no-op when no provider is registered — so
   any library consumer that hasn't installed the SDK pays nothing and imports no
   exporter. Instrumentation of I/O lives where the I/O is; instrumentation of a
   pure algorithm does not exist.
2. **Domain metrics are recorded at the service layer**, not inside `pkg/bloom`.
   `internal/service` owns the `*obs.Instruments` and records op counters +
   items-added from method boundaries. Per-filter breakdowns are deliberately
   **absent from metric labels** (filter names are user-supplied and unbounded —
   they belong in trace spans, not label cardinality).
3. **`correlation_id` is unified onto `trace_id`.** The `CorrelationID` middleware
   derives the id from the active span (otelgin runs first), falling back to an
   incoming `X-Correlation-ID` header, then a fresh UUID. One id spans logs, the
   response header, and the distributed trace.
4. **Configuration uses standard `OTEL_*` env autoconfig.** Endpoint, TLS, sampler,
   and resource attributes come from the SDK's own env support
   (`OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SERVICE_NAME`, `OTEL_TRACES_SAMPLER_ARG`).
   The app adds only `OBS_EXPORTER` (`otlp|stdout|none`) to pick the sink — `stdout`
   makes telemetry verifiable locally before the M9 stack exists.

## Consequences
- (+) The published SDK (`pkg/`) stays lightweight and dependency-honest; the app
  gets full traces including the Valkey hop.
- (+) Bounded metric cardinality; rich per-request detail still available via traces.
- (+) A single id correlates all three signals with no bespoke plumbing.
- (−) Fill-ratio / estimated-FPP are recorded at operation time, not as true
  observable gauges — deferred to M9 (needs a live-filter registry).
- (−) Metrics leave only via OTLP; there is no in-app `/metrics` scrape fallback
  (intentional, per ADR 0005).

## Usage
`internal/obs` (providers, instruments, otelslog bridge), `otelgin` +
`HTTPMetrics` + `CorrelationID` middleware in `internal/api`, service spans in
`internal/service`, and `startSpan` in `pkg/store`. Verify with
`OBS_EXPORTER=stdout`. See ADR 0005 for the backend stack and HLD §7 / LLD §10.
