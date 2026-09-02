package bloom

import (
	"errors"
	"math"
)

// Precomputed constants for the sizing formulas.
const (
	ln2        = math.Ln2
	ln2Squared = ln2 * ln2
)

// Default Scalable Bloom Filter tuning. Each stage's error budget shrinks by
// DefaultR, and each stage's capacity grows by DefaultS.
const (
	DefaultR = 0.9 // tightening ratio
	DefaultS = 2.0 // growth ratio
)

// Sentinel errors returned by the sizing functions.
var (
	ErrInvalidN     = errors.New("bloom: n must be greater than 0")
	ErrInvalidP     = errors.New("bloom: p must be in the open interval (0, 1)")
	ErrInvalidStage = errors.New("bloom: stage index must be non-negative")
	ErrSizeOverflow = errors.New("bloom: computed size exceeds uint64 range")
	ErrInvalidRatio = errors.New("bloom: invalid tightening/growth ratio")
	ErrTooManyArgs  = errors.New("bloom: at most one optional ratio may be provided")
)

// optionalRatio resolves a variadic override against a default. It allows zero
// or one value; more than one is a programmer misuse.
func optionalRatio(def float64, override []float64) (float64, error) {
	switch len(override) {
	case 0:
		return def, nil
	case 1:
		return override[0], nil
	default:
		return 0, ErrTooManyArgs
	}
}

// OptimalM returns the number of bits m for n expected items at target
// false-positive probability p.
//
//	m = ceil( -n * ln(p) / (ln2)^2 )
func OptimalM(n uint64, p float64) (uint64, error) {
	if n == 0 {
		return 0, ErrInvalidN
	}
	// Reject NaN explicitly: NaN is unordered, so the ordered checks below
	// would otherwise let it slip through and produce a garbage size.
	if math.IsNaN(p) || p <= 0 || p >= 1 {
		return 0, ErrInvalidP
	}
	m := math.Ceil(-float64(n) * math.Log(p) / ln2Squared)
	if m > float64(math.MaxUint64) {
		return 0, ErrSizeOverflow
	}
	return uint64(m), nil
}

// OptimalK returns the number of hash functions k for m bits and n items.
//
//	k = round( (m/n) * ln2 ),  floored at 1.
func OptimalK(n, m uint64) (int, error) {
	if n == 0 {
		return 0, ErrInvalidN
	}
	k := math.Round(float64(m) / float64(n) * ln2)
	if k < 1 {
		k = 1
	}
	return int(k), nil
}

// StageError returns the error budget p_i for stage i of a Scalable Bloom
// Filter targeting overall probability p. The tightening ratio r is optional
// and defaults to DefaultR; when provided it must lie in (0, 1).
//
//	p0 = p * (1 - r);  p_i = p0 * r^i
func StageError(p float64, i int, r ...float64) (float64, error) {
	// Reject NaN explicitly (NaN is unordered).
	if math.IsNaN(p) || p <= 0 || p >= 1 {
		return 0, ErrInvalidP
	}
	if i < 0 {
		return 0, ErrInvalidStage
	}
	ratio, err := optionalRatio(DefaultR, r)
	if err != nil {
		return 0, err
	}
	// Reject NaN explicitly (NaN is unordered).
	if math.IsNaN(ratio) || ratio <= 0 || ratio >= 1 {
		return 0, ErrInvalidRatio
	}
	p0 := p * (1 - ratio)
	return p0 * math.Pow(ratio, float64(i)), nil
}

// StageCapacity returns the item capacity of stage i. The growth ratio s is
// optional and defaults to DefaultS; when provided it must be greater than 1.
//
//	cap_i = n * s^i
func StageCapacity(n uint64, i int, s ...float64) (uint64, error) {
	if n == 0 {
		return 0, ErrInvalidN
	}
	if i < 0 {
		return 0, ErrInvalidStage
	}
	ratio, err := optionalRatio(DefaultS, s)
	if err != nil {
		return 0, err
	}
	// Reject NaN explicitly (NaN is unordered).
	if math.IsNaN(ratio) || ratio <= 1 {
		return 0, ErrInvalidRatio
	}
	c := float64(n) * math.Pow(ratio, float64(i))
	if c > float64(math.MaxUint64) {
		return 0, ErrSizeOverflow
	}
	return uint64(c), nil
}
