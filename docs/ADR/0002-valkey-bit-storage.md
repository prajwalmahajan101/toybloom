# 0002 — Store bits in Valkey via a pluggable BitStore

**Status:** Accepted · **Date:** 2026-07-16

## Context
The bit array must be shared across app instances, survive restarts, and support
high-throughput concurrent bit sets/gets. Options: in-process memory (not
shared), Memcached (no native bit ops), Redis/Valkey (native `SETBIT`/`GETBIT`),
or the `valkey-bloom` module (off-the-shelf, less control).

## Decision
Store bits in **Valkey**, accessed through a **pluggable `BitStore` interface**.
Valkey is the primary implementation (`valkey-go` client) using native
`SETBIT`/`GETBIT` (pipelined), `HSET`/`HGETALL` for metadata, `INCR` for fill
counts, and Lua for atomic stage append. Memcached is a documented, limited
single-writer secondary. The `valkey-bloom` module is not used — we keep the
algorithm in-app for control over sharding and observability.

## Consequences
- (+) Native, atomic bit ops map directly to bloom operations.
- (+) Interface keeps the core storage-agnostic and testable (Redis is drop-in compatible).
- (+) Open, license-clean default.
- (−) Network round-trips per op (mitigated by pipelining and short-circuiting).
- (−) Memcached path is intentionally limited (racy blob RMW) — not for concurrent writers.

## Usage
`internal/store`: `BitStore` interface + `ValkeyStore` (primary) and
`MemcachedStore` (limited). See LLD §6.
