# 0004 — Derive k positions via double hashing

**Status:** Accepted · **Date:** 2026-07-16

## Context
A bloom filter needs `k` independent-ish hash positions per item, and `k` varies
per stage (derived from `m`,`n`). Running `k` genuinely independent hash
functions costs `k` full hashes per operation. `k` also changes as stages grow,
so the position generator must be parameterized by `k` and `m` at call time.

## Decision
Use **double hashing (Kirsch & Mitzenmacher, 2006)**: compute one 128-bit hash of
the item (xxhash), split into `h1`, `h2`, then derive
`g_i = (h1 + i·h2) mod m` for `i = 0..k-1`. If `h2 == 0`, set it to `1` to avoid a
degenerate sequence. This yields `k` positions from a single hash with
false-positive behavior provably indistinguishable from `k` independent hashes.

## Consequences
- (+) One hash per operation regardless of `k`; cheap and fast.
- (+) Naturally dynamic in `k` and `m` — just loop `i`.
- (−) Positions are not truly independent (proven negligible impact on FPP).

## Usage
`internal/bloom/hash.go`: `hashParts`, `positions`. See LLD §3.
