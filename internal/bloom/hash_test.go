package bloom

import (
	"fmt"
	"math"
	"reflect"
	"testing"
)

// ---------- ensureNonZero ----------

func TestEnsureNonZero(t *testing.T) {
	cases := []struct {
		name     string
		in, want uint64
	}{
		{"zero maps to one", 0, 1},
		{"one stays one", 1, 1},
		{"arbitrary unchanged", 5, 5},
		{"max unchanged", math.MaxUint64, math.MaxUint64},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ensureNonZero(c.in); got != c.want {
				t.Errorf("ensureNonZero(%d) = %d, want %d", c.in, got, c.want)
			}
		})
	}
}

// ---------- hashParts ----------

func TestHashParts_KnownValues(t *testing.T) {
	// Regression anchors: exact xxhash outputs for fixed inputs. If these change,
	// the hashing algorithm changed (which would silently invalidate any persisted
	// filter), so they are pinned deliberately.
	cases := []struct {
		in     string
		wantH1 uint64
		wantH2 uint64
	}{
		{"", 17241709254077376921, 10409512625188310609},
		{"user-42", 4142921581652311169, 18280868780128622206},
		{"alpha", 14364478406410262600, 14104052351429218332},
		{"beta", 17721147283167156420, 1516948496052056262},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			h1, h2 := hashParts([]byte(c.in))
			if h1 != c.wantH1 || h2 != c.wantH2 {
				t.Errorf("hashParts(%q) = (%d, %d), want (%d, %d)", c.in, h1, h2, c.wantH1, c.wantH2)
			}
		})
	}
}

func TestHashParts_Deterministic(t *testing.T) {
	item := []byte("user-42")
	h1a, h2a := hashParts(item)
	h1b, h2b := hashParts(item)
	if h1a != h1b || h2a != h2b {
		t.Errorf("hashParts not deterministic: (%d,%d) vs (%d,%d)", h1a, h2a, h1b, h2b)
	}
}

func TestHashParts_DistinctItemsDiffer(t *testing.T) {
	h1a, _ := hashParts([]byte("alpha"))
	h1b, _ := hashParts([]byte("beta"))
	if h1a == h1b {
		t.Errorf("distinct items produced equal h1 = %d", h1a)
	}
}

func TestHashParts_EmptyEqualsNil(t *testing.T) {
	h1, h2 := hashParts([]byte{})
	h1b, h2b := hashParts(nil)
	if h1 != h1b || h2 != h2b {
		t.Errorf("empty vs nil differ: (%d,%d) vs (%d,%d)", h1, h2, h1b, h2b)
	}
}

func TestHashParts_H2NeverZero(t *testing.T) {
	for i := range 10_000 {
		if _, h2 := hashParts(fmt.Appendf(nil, "item-%d", i)); h2 == 0 {
			t.Fatalf("h2 == 0 for item-%d (guard failed)", i)
		}
	}
}

// ---------- positions ----------

func TestPositions_Valid(t *testing.T) {
	cases := []struct {
		name      string
		h1, h2, m uint64
		k         int
		want      []uint64
	}{
		{"basic mod 7", 10, 3, 7, 5, []uint64{3, 6, 2, 5, 1}},
		{"step 1 is sequential", 100, 1, 1000, 4, []uint64{100, 101, 102, 103}},
		{"from zero", 0, 1, 5, 5, []uint64{0, 1, 2, 3, 4}},
		{"h2=0 collapses (positions does not guard h2)", 5, 0, 10, 3, []uint64{5, 5, 5}},
		{"m=1 all zero", 999, 777, 1, 3, []uint64{0, 0, 0}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := positions(c.h1, c.h2, c.m, c.k)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("positions(%d,%d,%d,%d) = %v, want %v", c.h1, c.h2, c.m, c.k, got, c.want)
			}
		})
	}
}

func TestPositions_NonPositiveK(t *testing.T) {
	for _, k := range []int{0, -1, -100} {
		t.Run(fmt.Sprintf("k=%d", k), func(t *testing.T) {
			if got := positions(1, 2, 10, k); len(got) != 0 {
				t.Errorf("positions(k=%d) = %v, want empty", k, got)
			}
		})
	}
}

func TestPositions_CountAndBounds(t *testing.T) {
	h1, h2 := hashParts([]byte("some-key"))
	const m, k = 9_585_059, 7
	pos := positions(h1, h2, m, k)
	if len(pos) != k {
		t.Fatalf("len = %d, want %d", len(pos), k)
	}
	for i, p := range pos {
		if p >= m {
			t.Errorf("positions[%d] = %d, out of range [0, %d)", i, p, m)
		}
	}
}

func TestPositions_Deterministic(t *testing.T) {
	a := positions(123, 456, 1000, 8)
	b := positions(123, 456, 1000, 8)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("positions not deterministic: %v vs %v", a, b)
	}
}

func TestPositions_OverflowStaysInRange(t *testing.T) {
	// Max h1/h2 force uint64 wraparound in (h1 + i*h2); results must still be < m.
	const m, k = 97, 20
	pos := positions(math.MaxUint64, math.MaxUint64, m, k)
	if len(pos) != k {
		t.Fatalf("len = %d, want %d", len(pos), k)
	}
	for i, p := range pos {
		if p >= m {
			t.Errorf("positions[%d] = %d, out of range after overflow", i, p)
		}
	}
}

func TestPositions_Distribution(t *testing.T) {
	// With k=1 over many distinct items, positions should spread across [0, m),
	// not collapse to a single bucket.
	const m = 1000
	seen := make(map[uint64]struct{})
	for i := range 1000 {
		h1, h2 := hashParts(fmt.Appendf(nil, "k-%d", i))
		seen[positions(h1, h2, m, 1)[0]] = struct{}{}
	}
	if len(seen) < 100 {
		t.Errorf("only %d distinct buckets for 1000 items; distribution looks broken", len(seen))
	}
}
