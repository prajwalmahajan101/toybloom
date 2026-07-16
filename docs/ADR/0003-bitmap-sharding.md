# 0003 — Shard each stage bitmap into 4MB keys

**Status:** Accepted · **Date:** 2026-07-16

## Context
Target scale is 100M–1B+ items. At `p=0.01`, `m ≈ 9.6 bits/item`, so 1B items
needs ~9.6B bits. A single Valkey string is capped at **512MB (~4.29B bits)**, so
one stage's bitmap cannot always live in a single key. We also want large bitmaps
to spread across the keyspace/cluster rather than forming one hot mega-key.

## Decision
Shard each stage's bitmap into fixed **`2^25`-bit (4MB) chunks**. For a bit
index: `shard = bitIndex / 2^25`, `offset = bitIndex % 2^25`, addressed by key
`bf:{name}:s{i}:sh{j}` with `SETBIT key offset`. Operations group their `k`
offsets by shard and pipeline per shard.

## Consequences
- (+) No single-key 512MB ceiling; supports arbitrarily large filters.
- (+) Bits spread across keys → better distribution in a Valkey cluster.
- (−) An Add/Exists may span multiple keys (still pipelined; usually 1–2 shards for small `k`).
- (−) Shard size is a fixed constant; changing it is a migration (documented).

## Usage
`internal/bloom/shard.go`: `ShardBits`, `shardFor`, `groupByShard`. See LLD §4.
