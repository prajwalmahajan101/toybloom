package bloom

import (
	"encoding/binary"

	"github.com/cespare/xxhash/v2"
)

// hashParts hashes item once and returns two 64-bit values for double hashing.
// h2 is a rehash of h1's bytes, forced non-zero so the k positions don't all
// collapse onto h1.
func hashParts(item []byte) (h1, h2 uint64) {
	h1 = xxhash.Sum64(item)
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], h1)
	h2 = ensureNonZero(xxhash.Sum64(buf[:]))
	return h1, h2
}

// ensureNonZero maps 0 to 1 and leaves every other value unchanged. It keeps the
// double-hashing step size h2 non-zero; a zero step would make g_i = h1 for all
// i, collapsing all k positions onto a single bit.
func ensureNonZero(x uint64) uint64 {
	if x == 0 {
		return 1
	}
	return x
}

// positions returns k bit indices in [0, m) via g_i = (h1 + i*h2) mod m.
// A non-positive k yields an empty slice. Precondition (guaranteed by the
// caller): m >= 1. uint64 multiplication may wrap; that is fine — mod m still
// yields a valid, well-distributed index (modular arithmetic).
func positions(h1, h2, m uint64, k int) []uint64 {
	if k <= 0 {
		return []uint64{}
	}
	pos := make([]uint64, k)
	for i := range k {
		pos[i] = (h1 + uint64(i)*h2) % m
	}
	return pos
}
