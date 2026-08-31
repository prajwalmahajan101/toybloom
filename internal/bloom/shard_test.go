package bloom

import (
	"math"
	"reflect"
	"testing"
)

// ---------- ShardBits ----------

func TestShardBits_PowerOfTwo(t *testing.T) {
	// The shard/offset split relies on ShardBits being a power of two so that
	// division/modulo lower to a shift and a mask. Guard that invariant.
	if ShardBits == 0 || ShardBits&(ShardBits-1) != 0 {
		t.Fatalf("ShardBits = %d is not a power of two", ShardBits)
	}
	if ShardBits != 1<<25 {
		t.Errorf("ShardBits = %d, want %d (2^25)", ShardBits, uint64(1<<25))
	}
}

// ---------- shardFor ----------

func TestShardFor_KnownValues(t *testing.T) {
	cases := []struct {
		name     string
		bitIndex uint64
		want     ShardOffset
	}{
		{"bit zero", 0, ShardOffset{0, 0}},
		{"within first shard", 100, ShardOffset{0, 100}},
		{"last bit of first shard", ShardBits - 1, ShardOffset{0, ShardBits - 1}},
		{"first bit of second shard", ShardBits, ShardOffset{1, 0}},
		{"second bit of second shard", ShardBits + 1, ShardOffset{1, 1}},
		{"start of third shard", 2 * ShardBits, ShardOffset{2, 0}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := shardFor(c.bitIndex); got != c.want {
				t.Errorf("shardFor(%d) = %+v, want %+v", c.bitIndex, got, c.want)
			}
		})
	}
}

func TestShardFor_OffsetInRange(t *testing.T) {
	// Offset must always be < ShardBits, including at boundaries and near max.
	for _, b := range []uint64{0, 1, ShardBits - 1, ShardBits, ShardBits + 1, 5*ShardBits + 7, math.MaxUint64} {
		if so := shardFor(b); so.Offset >= ShardBits {
			t.Errorf("shardFor(%d).Offset = %d, out of range [0, %d)", b, so.Offset, ShardBits)
		}
	}
}

func TestShardFor_RoundTrip(t *testing.T) {
	// index -> (shard, offset) -> index must be the identity. The reverse is the
	// trivial shard*ShardBits + offset.
	for _, b := range []uint64{0, 1, 42, ShardBits - 1, ShardBits, ShardBits + 1, 3*ShardBits + 999, math.MaxUint64} {
		so := shardFor(b)
		if got := so.Shard*ShardBits + so.Offset; got != b {
			t.Errorf("round-trip of %d = %d (via %+v)", b, got, so)
		}
	}
}

// ---------- groupByShard ----------

func TestGroupByShard_KnownGrouping(t *testing.T) {
	// Two positions in shard 0, one in shard 1, one in shard 2. Offsets are local.
	positions := []uint64{5, ShardBits + 10, 7, 2*ShardBits + 3}
	want := map[uint64][]uint64{
		0: {5, 7},
		1: {10},
		2: {3},
	}
	if got := groupByShard(positions); !reflect.DeepEqual(got, want) {
		t.Errorf("groupByShard(%v) = %v, want %v", positions, got, want)
	}
}

func TestGroupByShard_Empty(t *testing.T) {
	if got := groupByShard(nil); len(got) != 0 {
		t.Errorf("groupByShard(nil) = %v, want empty", got)
	}
}

func TestGroupByShard_AllSameShard(t *testing.T) {
	positions := []uint64{0, 1, 2, ShardBits - 1}
	got := groupByShard(positions)
	if len(got) != 1 {
		t.Fatalf("expected 1 shard, got %d", len(got))
	}
	if !reflect.DeepEqual(got[0], []uint64{0, 1, 2, ShardBits - 1}) {
		t.Errorf("shard 0 offsets = %v, want %v", got[0], positions)
	}
}

func TestGroupByShard_PreservesDuplicates(t *testing.T) {
	// Grouping must not deduplicate; repeated global positions repeat as offsets.
	got := groupByShard([]uint64{5, 5, ShardBits + 5, 5})
	if !reflect.DeepEqual(got[0], []uint64{5, 5, 5}) {
		t.Errorf("shard 0 offsets = %v, want [5 5 5]", got[0])
	}
	if !reflect.DeepEqual(got[1], []uint64{5}) {
		t.Errorf("shard 1 offsets = %v, want [5]", got[1])
	}
}

func TestGroupByShard_OffsetsReconstruct(t *testing.T) {
	// Every (shard, offset) emitted by grouping must reconstruct to a position
	// that was in the input — closing the loop with shardFor.
	positions := []uint64{0, ShardBits, 2*ShardBits + 1, 999, ShardBits + 999}
	in := make(map[uint64]int)
	for _, p := range positions {
		in[p]++
	}
	for shard, offsets := range groupByShard(positions) {
		for _, off := range offsets {
			global := shard*ShardBits + off
			if in[global] == 0 {
				t.Errorf("reconstructed %d (shard %d, offset %d) not in input", global, shard, off)
				continue
			}
			in[global]--
		}
	}
	for p, remaining := range in {
		if remaining != 0 {
			t.Errorf("position %d unaccounted for (%d left)", p, remaining)
		}
	}
}

func TestGroupByShard_Deterministic(t *testing.T) {
	positions := []uint64{ShardBits + 1, 3, ShardBits + 2, 3, 2 * ShardBits}
	a := groupByShard(positions)
	b := groupByShard(positions)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("groupByShard not deterministic: %v vs %v", a, b)
	}
}

func TestGroupByShard_Distribution(t *testing.T) {
	// Positions spread across many shards should produce many groups, not collapse.
	positions := make([]uint64, 0, 100)
	for i := range 100 {
		positions = append(positions, uint64(i)*ShardBits+uint64(i))
	}
	if got := groupByShard(positions); len(got) != 100 {
		t.Errorf("expected 100 distinct shards, got %d", len(got))
	}
}
