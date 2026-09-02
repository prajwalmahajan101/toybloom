# LLD — Dynamic (Scalable) Bloom Filter Service

**Status:** Draft · **Date:** 2026-07-16
Module: `github.com/prajwalmahajan101/toybloom`

## 1. Sizing math (`internal/bloom/sizing.go`)
Per stage, from expected items `n` and target error `p₀`:
```
m = ceil( -n · ln(p₀) / (ln2)^2 )     // bit-array size
k = max(1, round( (m/n) · ln2 ))      // number of hash functions
```
Self-check (n=1,000,000): p=0.01 → m≈9,585,059 bits, k≈7 · p=0.001 → m≈14,377,588, k≈10.

Functions:
```go
func OptimalM(n uint64, p float64) (uint64, error)
func OptimalK(n, m uint64) (int, error)
```

## 2. Scalable Bloom Filter parameters
Defaults: tightening ratio `r = 0.9`, growth ratio `s = 2`.
```
p0        = p · (1 - r)          // stage 0 error budget
p_i       = p0 · r^i             // stage i error budget
cap_i     = n · s^i              // stage i capacity (items)
```
Compounded FPP of the chain ≤ `p` (geometric series bound). A new stage `i+1` is
appended when `fill_count(i) ≥ cap_i`.

## 3. Hashing — double hashing (`internal/bloom/hash.go`)
One 128-bit hash via xxhash: `h1 = xxhash64(item, seedA)`, `h2 = xxhash64(item, seedB)`
(or split a single 128-bit digest). Positions:
```
g_i = (h1 + i*h2) mod m,   i = 0 .. k-1
```
```go
func hashParts(item []byte) (h1, h2 uint64)
func positions(h1, h2, m uint64, k int) []uint64   // len == k, each < m
```
Guard: if `h2 == 0`, set `h2 = 1` to avoid a degenerate single-position sequence.

## 4. Sharding (`internal/bloom/shard.go`)
```
const ShardBits uint64 = 1 << 25       // 33,554,432 bits ≈ 4 MB per key
shard  = bitIndex / ShardBits
offset = bitIndex % ShardBits          // 0 .. ShardBits-1, fits Valkey SETBIT
```
```go
type ShardOffset struct{ Shard uint64; Offset uint64 }
func shardFor(bitIndex uint64) ShardOffset
func groupByShard(positions []uint64) map[uint64][]uint64   // shard -> offsets
```

## 5. Keyspace
```
bf:{name}:meta          hash  { n, p, r, s, created_at, stage_count }
bf:{name}:s{i}:meta      hash  { m, k, capacity, fill_count }
bf:{name}:s{i}:sh{j}     string(bitmap)  SETBIT/GETBIT at offset
bf:registry              set   of filter names
```

## 6. Storage interface (`internal/store/store.go`)
```go
type BitStore interface {
    SetBits(ctx context.Context, key string, offsets []uint64) error
    GetBits(ctx context.Context, key string, offsets []uint64) ([]bool, error)
    HGetAll(ctx context.Context, key string) (map[string]string, error)
    HSet(ctx context.Context, key string, fields map[string]string) error
    Incr(ctx context.Context, key string) (int64, error)
    SAdd(ctx context.Context, key string, members ...string) error
    SMembers(ctx context.Context, key string) ([]string, error)
    Del(ctx context.Context, keys ...string) error
    // Atomic compare-and-append of a new stage; returns new stage_count.
    AppendStage(ctx context.Context, metaKey string, expected int64, newStageFields map[string]string, newStageMetaKey string) (int64, error)
}
```
- **ValkeyStore:** batches `SetBits`/`GetBits` via valkey-go pipelining, one
  pipeline per shard key. `AppendStage` runs a Lua script (see §8).
- **MemcachedStore:** read-modify-write of the whole bitmap blob under a CAS
  token; documented single-writer, conformance-tested against the same suite.

## 7. Core types & ops (`internal/bloom/filter.go`)
```go
type Stage struct { Index int; M uint64; K int; Capacity uint64; Fill uint64 }
type Filter struct { Name string; N uint64; P, R, S float64; Stages []Stage; store store.BitStore }

func New(ctx, store, name string, n uint64, p float64) (*Filter, error) // creates meta + stage 0
func Load(ctx, store, name string) (*Filter, error)
func (f *Filter) Add(ctx, item []byte) error
func (f *Filter) Exists(ctx, item []byte) (bool, error)
func (f *Filter) Stats() Stats
func (f *Filter) Drop(ctx) error
```
**Add:** current stage `c` → `pos = positions(item, c.M, c.K)` → `groupByShard` →
per shard `SetBits(bf:{name}:s{c}:sh{j}, offsets)` → `Incr(fill_count)` → if
`fill ≥ cap`, `AppendStage`.
**Exists:** iterate stages newest→oldest; for each, `GetBits` per shard; if all
true → return true; else continue. Return false after all stages.

## 8. Lua stage-append guard
```lua
-- KEYS[1]=filter meta, KEYS[2]=new stage meta
-- ARGV[1]=expected stage_count, ARGV[2..]=new stage meta field/value pairs
if redis.call('HGET', KEYS[1], 'stage_count') ~= ARGV[1] then
  return redis.call('HGET', KEYS[1], 'stage_count')   -- lost the race; caller reloads
end
-- set new stage meta, bump stage_count atomically
for i=2,#ARGV,2 do redis.call('HSET', KEYS[2], ARGV[i], ARGV[i+1]) end
return redis.call('HINCRBY', KEYS[1], 'stage_count', 1)
```

## 9. REST contract (`internal/api`)
| Method | Path | Body | Success | Errors |
|--------|------|------|---------|--------|
| POST | `/v1/filters` | `{name, n, p}` | 201 `{name, stages, m, k}` | 400 invalid, 409 exists |
| POST | `/v1/filters/:name/items` | `{value}` | 200 `{added:true}` | 404 no filter, 400 |
| GET | `/v1/filters/:name/items/:value` | — | 200 `{exists:bool}` | 404 no filter |
| GET | `/v1/filters/:name` | — | 200 stats | 404 |
| DELETE | `/v1/filters/:name` | — | 204 | 404 |

**Error envelope:**
```json
{ "correlation_id": "...", "error": { "code": "INVALID_ARGUMENT", "message": "p must be in (0,1)" } }
```
`correlation_id` is a response-level field (top level) shared by success and
error envelopes; success responses use `{ "correlation_id": "...", "data": {...} }`.
Validation at the edge: `n ≥ 1`, `0 < p < 1`, `name` matches `^[a-zA-Z0-9_-]{1,64}$`.

## 10. Observability instruments (`internal/obs`)
```
http_server_request_duration_seconds  histogram (route, method, status)  → p50/p95/p99
http_server_requests_total            counter   (route, method, status)
bloom_items_added_total               counter   (filter)
bloom_stage_count                     gauge     (filter)
bloom_fill_ratio                      gauge     (filter, stage)
bloom_estimated_fpp                   gauge     (filter)
bloom_valkey_op_duration_seconds      histogram (op)
bloom_false_positive_checks_total     counter   (filter)   // best-effort, if truth known
```
Histogram buckets tuned around the 200ms budget (e.g. 1,2,5,10,25,50,100,200,500,1000 ms).
Spans: `http.request` (otelgin) → `bloom.add`/`bloom.exists` → `valkey.pipeline` / `bloom.stage_append`.

## 11. Testing (`internal/bloom/*_test.go`, `internal/store/*_test.go`)
- Unit: `OptimalM/K` against known values; position distribution; `shardFor`
  round-trip; SBF tightening keeps ∑ error ≤ p.
- Integration (testcontainers Valkey): insert N > n items, assert measured FPP ≤
  p across stage growth; concurrent-writer `AppendStage` creates exactly one
  stage; membership has zero false negatives.
