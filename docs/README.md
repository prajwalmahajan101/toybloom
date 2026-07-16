# Documentation Index

Design and decision records for the dynamic (scalable) bloom filter service.

## Design docs
| Doc | Purpose |
|-----|---------|
| [PRD.md](./PRD.md) | Product requirements — problem, goal, functional & non-functional requirements, success metrics, scope. |
| [HLD.md](./HLD.md) | High-level design — architecture, components, data flow, sharding, observability, failure modes. |
| [LLD.md](./LLD.md) | Low-level design — sizing math, hashing, sharding constants, keyspace, interfaces, algorithms, API contract, OTel instruments. |
| [RFC.md](./RFC.md) | Proposal — motivation, alternatives considered, drawbacks, rollout, open questions. |

## Architecture Decision Records
[ADR/](./ADR/README.md) — numbered, immutable records of the key decisions
(SBF, Valkey, sharding, double-hashing, OTel/LGTM).

## Phase Journal
[Journal/](./Journal/README.md) — per-milestone build log (what was done, why,
verification), following [Journal/TEMPLATE.md](./Journal/TEMPLATE.md).

## Milestone map
M0 init · M1 sizing math · M2 double hashing · M3 sharding · M4 BitStore iface ·
M5 SBF core · M6 ValkeyStore · M7 Gin REST · M8 OTel instrumentation ·
M9 docker-compose + Grafana/Prometheus/Tempo/Loki · M10 integration tests + ADRs
