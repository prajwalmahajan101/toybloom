# 0001 — Use a Scalable Bloom Filter

**Status:** Accepted · **Date:** 2026-07-16

## Context
Callers supply `n` (expected items) and `p` (target false-positive rate). A
classic fixed-size bloom filter meets `p` only up to `n`; once inserts exceed
`n`, its false-positive rate degrades without bound. We need the target `p` to
hold even when the dataset grows past the initial estimate. Deletes are out of
scope (see PRD).

## Decision
Implement a **Scalable Bloom Filter** (Almeida et al., 2007): a chain of
fixed-size classic bloom "stages". Each stage `i` is sized from `n` and a
tightened error budget `p_i = p·(1−r)·r^i` with capacity `n·s^i`
(defaults `r=0.9`, `s=2`). A new, larger stage is appended when the current one
fills. Membership = present in **any** stage; the compounded FPP is bounded ≤ `p`.

## Consequences
- (+) Target `p` preserved as data grows; caller need not know the exact size upfront.
- (+) Each stage is a plain bloom filter — simple to size and store.
- (−) Membership checks may touch multiple stages (mitigated by newest→oldest short-circuit).
- (−) No deletion (reset = drop & rebuild). Revisit with a counting/cuckoo variant if needed.

## Usage
Core lives in `internal/bloom`. `r`/`s` are configurable with the stated
defaults. See LLD §2 for the parameter derivation and §7 for Add/Exists.
