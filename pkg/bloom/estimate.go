package bloom

import (
	"context"
	"math"

	"github.com/prajwalmahajan101/toybloom/pkg/store"
)

// FillRatio reports how full the filter is overall: the total number of items
// added across every stage divided by the total item capacity across every
// stage. It is a saturation signal — as it climbs toward 1 the newest stage is
// close to spawning the next one. Returns 0 for an empty filter (no capacity).
//
// This is insertion-fill ÷ capacity, not literal set-bits ÷ m; it needs no extra
// Valkey read because Fill and Capacity are already present on the loaded stages.
func (f *Filter) FillRatio() float64 {
	var fill, capacity uint64
	for _, st := range f.stages {
		fill += st.Fill
		capacity += st.Capacity
	}
	if capacity == 0 {
		return 0
	}
	return float64(fill) / float64(capacity)
}

// EstimatedFPP estimates the filter's current false-positive probability from
// its live fill counts. Each stage i contributes the standard bloom estimate
//
//	fpp_i = (1 - e^(-k_i * fill_i / m_i))^k_i
//
// and, because a lookup returns a false positive only when it collides in every
// stage's negative test, the stages compound as
//
//	fpp = 1 - Π(1 - fpp_i).
//
// This is the observed FPP at the current load, distinct from the configured
// target p the filter was sized against. Stages with no bits or no hashes are
// skipped so a fresh stage yields a clean 0 rather than a NaN.
func (f *Filter) EstimatedFPP() float64 {
	survive := 1.0 // running Π(1 - fpp_i)
	for _, st := range f.stages {
		if st.M == 0 || st.K <= 0 {
			continue
		}
		exponent := -float64(st.K) * float64(st.Fill) / float64(st.M)
		fppI := math.Pow(1-math.Exp(exponent), float64(st.K))
		survive *= 1 - fppI
	}
	return 1 - survive
}

// Items reports the filter's current element count: the total number of items
// added across all stages. Computed from the same loaded Stats — no extra read.
func (f *Filter) Items() uint64 {
	var n uint64
	for _, st := range f.stages {
		n += st.Fill
	}
	return n
}

// ListFilters returns the names of every registered filter. It reads the same
// bf:registry set that New writes to and Drop removes from, so callers (e.g. the
// observability layer) can enumerate live filters without the private registry
// key leaking out of this package.
func ListFilters(ctx context.Context, s store.BitStore) ([]string, error) {
	return s.SMembers(ctx, registryKey)
}
