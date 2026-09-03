# Architecture Decision Records

Numbered, immutable records of significant architectural decisions. Each file is
`NNNN-<slug>.md` and follows the template: **Context / Decision / Consequences /
Usage**. Superseding a decision adds a new ADR that references the old one; the
old one is not deleted.

## Index
| # | Decision | Status |
|---|----------|--------|
| [0001](./0001-scalable-bloom-filter.md) | Use a Scalable Bloom Filter (not classic/counting/cuckoo) | Accepted |
| [0002](./0002-valkey-bit-storage.md) | Store bits in Valkey via a pluggable BitStore | Accepted |
| [0003](./0003-bitmap-sharding.md) | Shard each stage bitmap into 4MB keys | Accepted |
| [0004](./0004-double-hashing.md) | Derive k positions via double hashing (Kirsch–Mitzenmacher) | Accepted |
| [0005](./0005-observability-otel-lgtm.md) | OpenTelemetry → Collector → Prometheus/Tempo/Loki + Grafana | Accepted |
| [0006](./0006-api-contract-and-client-architecture.md) | API contract and client architecture | Accepted |
| [0007](./0007-otel-instrumentation-boundaries.md) | OTel instrumentation boundaries (pkg purity, cardinality, id unification) | Accepted |
| [0008](./0008-per-filter-gauge-cardinality.md) | Per-filter gauges with a bounded cardinality cap (amends 0007) | Accepted |
