package bloom

// ShardBits is the number of bits stored per shard key. 2^25 bits = 4 MB,
// comfortably under Valkey's 512 MB per-string limit and a power of two so the
// shard/offset split reduces to a shift and a mask.
const ShardBits uint64 = 1 << 25

// ShardOffset locates a single bit within the sharded bitmap: which shard key
// holds it (Shard) and its bit position inside that shard (Offset, < ShardBits).
type ShardOffset struct {
	Shard  uint64
	Offset uint64
}

// shardFor maps a global bit index to its (shard, offset) pair.
//
//	shard  = bitIndex / ShardBits
//	offset = bitIndex % ShardBits
//
// Because ShardBits is a power of two, these are a shift and a mask.
func shardFor(bitIndex uint64) ShardOffset {
	return ShardOffset{
		Shard:  bitIndex / ShardBits,
		Offset: bitIndex % ShardBits,
	}
}

// groupByShard buckets a set of global bit positions by shard, returning a map
// from shard number to the offsets that fall in that shard. The caller issues
// one pipelined SETBIT/GETBIT batch per shard key, so grouping first keeps each
// batch scoped to a single Valkey string.
func groupByShard(positions []uint64) map[uint64][]uint64 {
	groups := make(map[uint64][]uint64)
	for _, p := range positions {
		so := shardFor(p)
		groups[so.Shard] = append(groups[so.Shard], so.Offset)
	}
	return groups
}
