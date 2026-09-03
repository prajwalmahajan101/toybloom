package bloom

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/prajwalmahajan101/toybloom/pkg/store"
)

var (
	ErrEmptyName    = errors.New("bloom: filter name must not be empty")
	ErrFilterExists = errors.New("bloom: filter already exists")
	ErrNotFound     = errors.New("bloom: filter not found")
)

// Stage holds the parameters and fill count for one stage of a scalable bloom filter.
type Stage struct {
	Index    int
	M        uint64
	K        int
	Capacity uint64
	Fill     uint64
}

// Stats is a read-only snapshot of the filter's current state.
type Stats struct {
	Name       string
	N          uint64
	P          float64
	R          float64
	S          float64
	StageCount int
	Stages     []Stage
}

// Filter is a scalable bloom filter backed by a BitStore.
type Filter struct {
	name    string
	n       uint64
	p, r, s float64
	stages  []Stage
	store   store.BitStore
}

const registryKey = "bf:registry"

func metaKey(name string) string {
	return fmt.Sprintf("bf:%s:meta", name)
}

func stageMetaKey(name string, i int) string {
	return fmt.Sprintf("bf:%s:s%d:meta", name, i)
}

func shardKey(name string, stage int, shard uint64) string {
	return fmt.Sprintf("bf:%s:s%d:sh%d", name, stage, shard)
}

func stageToMap(s Stage) map[string]string {
	return map[string]string{
		"m":          strconv.FormatUint(s.M, 10),
		"k":          strconv.Itoa(s.K),
		"capacity":   strconv.FormatUint(s.Capacity, 10),
		"fill_count": strconv.FormatUint(s.Fill, 10),
	}
}

func mapToStage(fields map[string]string, index int) (Stage, error) {
	m, err := strconv.ParseUint(fields["m"], 10, 64)
	if err != nil {
		return Stage{}, fmt.Errorf("bloom: parse m: %w", err)
	}
	k, err := strconv.Atoi(fields["k"])
	if err != nil {
		return Stage{}, fmt.Errorf("bloom: parse k: %w", err)
	}
	cap, err := strconv.ParseUint(fields["capacity"], 10, 64)
	if err != nil {
		return Stage{}, fmt.Errorf("bloom: parse capacity: %w", err)
	}
	fill, err := strconv.ParseUint(fields["fill_count"], 10, 64)
	if err != nil {
		return Stage{}, fmt.Errorf("bloom: parse fill_count: %w", err)
	}
	return Stage{Index: index, M: m, K: k, Capacity: cap, Fill: fill}, nil
}

func computeStage(n uint64, p, r, s float64, index int) (Stage, error) {
	pi, err := StageError(p, index, r)
	if err != nil {
		return Stage{}, err
	}
	cap, err := StageCapacity(n, index, s)
	if err != nil {
		return Stage{}, err
	}
	m, err := OptimalM(cap, pi)
	if err != nil {
		return Stage{}, err
	}
	k, err := OptimalK(cap, m)
	if err != nil {
		return Stage{}, err
	}
	return Stage{Index: index, M: m, K: k, Capacity: cap, Fill: 0}, nil
}

// New creates a new scalable bloom filter with the given parameters and persists
// its metadata and stage-0 to the store.
func New(ctx context.Context, s store.BitStore, name string, n uint64, p float64) (*Filter, error) {
	if name == "" {
		return nil, ErrEmptyName
	}
	if n == 0 {
		return nil, ErrInvalidN
	}
	if p <= 0 || p >= 1 {
		return nil, ErrInvalidP
	}

	existing, err := s.HGetAll(ctx, metaKey(name))
	if err != nil {
		return nil, fmt.Errorf("bloom: check existing: %w", err)
	}
	if len(existing) > 0 {
		return nil, ErrFilterExists
	}

	stage0, err := computeStage(n, p, DefaultR, DefaultS, 0)
	if err != nil {
		return nil, err
	}

	filterMeta := map[string]string{
		"n":           strconv.FormatUint(n, 10),
		"p":           strconv.FormatFloat(p, 'g', -1, 64),
		"r":           strconv.FormatFloat(DefaultR, 'g', -1, 64),
		"s":           strconv.FormatFloat(DefaultS, 'g', -1, 64),
		"stage_count": "1",
		"created_at":  time.Now().UTC().Format(time.RFC3339),
	}

	if err := s.HSet(ctx, metaKey(name), filterMeta); err != nil {
		return nil, fmt.Errorf("bloom: write filter meta: %w", err)
	}
	if err := s.HSet(ctx, stageMetaKey(name, 0), stageToMap(stage0)); err != nil {
		return nil, fmt.Errorf("bloom: write stage-0 meta: %w", err)
	}
	if err := s.SAdd(ctx, registryKey, name); err != nil {
		return nil, fmt.Errorf("bloom: register filter: %w", err)
	}

	return &Filter{
		name:   name,
		n:      n,
		p:      p,
		r:      DefaultR,
		s:      DefaultS,
		stages: []Stage{stage0},
		store:  s,
	}, nil
}

// Load reconstructs a Filter from the store.
func Load(ctx context.Context, s store.BitStore, name string) (*Filter, error) {
	if name == "" {
		return nil, ErrEmptyName
	}

	meta, err := s.HGetAll(ctx, metaKey(name))
	if err != nil {
		return nil, fmt.Errorf("bloom: read filter meta: %w", err)
	}
	if len(meta) == 0 {
		return nil, ErrNotFound
	}

	n, err := strconv.ParseUint(meta["n"], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("bloom: parse n: %w", err)
	}
	p, err := strconv.ParseFloat(meta["p"], 64)
	if err != nil {
		return nil, fmt.Errorf("bloom: parse p: %w", err)
	}
	r, err := strconv.ParseFloat(meta["r"], 64)
	if err != nil {
		return nil, fmt.Errorf("bloom: parse r: %w", err)
	}
	growth, err := strconv.ParseFloat(meta["s"], 64)
	if err != nil {
		return nil, fmt.Errorf("bloom: parse s: %w", err)
	}
	sc, err := strconv.Atoi(meta["stage_count"])
	if err != nil {
		return nil, fmt.Errorf("bloom: parse stage_count: %w", err)
	}

	stages := make([]Stage, sc)
	for i := range sc {
		fields, err := s.HGetAll(ctx, stageMetaKey(name, i))
		if err != nil {
			return nil, fmt.Errorf("bloom: read stage %d meta: %w", i, err)
		}
		stages[i], err = mapToStage(fields, i)
		if err != nil {
			return nil, err
		}

		// The hash's fill_count is only the creation-time seed; the live count is
		// the atomic :fill counter that Add increments. Read it so Stats and the
		// observability gauges reflect real load, not zero, on a fresh Load.
		fillStr, ok, err := s.Get(ctx, stageMetaKey(name, i)+":fill")
		if err != nil {
			return nil, fmt.Errorf("bloom: read stage %d fill: %w", i, err)
		}
		if ok {
			fill, err := strconv.ParseUint(fillStr, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("bloom: parse stage %d fill: %w", i, err)
			}
			stages[i].Fill = fill
		}
	}

	return &Filter{
		name:   name,
		n:      n,
		p:      p,
		r:      r,
		s:      growth,
		stages: stages,
		store:  s,
	}, nil
}

// Add inserts an item into the filter's current (latest) stage.
func (f *Filter) Add(ctx context.Context, item []byte) error {
	cur := &f.stages[len(f.stages)-1]

	h1, h2 := hashParts(item)
	pos := positions(h1, h2, cur.M, cur.K)
	groups := groupByShard(pos)

	for shard, offsets := range groups {
		key := shardKey(f.name, cur.Index, shard)
		if err := f.store.SetBits(ctx, key, offsets); err != nil {
			return fmt.Errorf("bloom: set bits shard %d: %w", shard, err)
		}
	}

	fillKey := stageMetaKey(f.name, cur.Index)
	newFill, err := f.store.Incr(ctx, fillKey+":fill")
	if err != nil {
		return fmt.Errorf("bloom: incr fill: %w", err)
	}
	cur.Fill = uint64(newFill)

	if cur.Fill >= cur.Capacity {
		if err := f.appendNewStage(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Exists checks whether item may be in the filter. It iterates stages from
// newest to oldest; a hit in any stage returns true. False means the item was
// definitely never added.
func (f *Filter) Exists(ctx context.Context, item []byte) (bool, error) {
	h1, h2 := hashParts(item)

	for i := len(f.stages) - 1; i >= 0; i-- {
		st := &f.stages[i]
		pos := positions(h1, h2, st.M, st.K)
		groups := groupByShard(pos)

		allSet := true
		for shard, offsets := range groups {
			key := shardKey(f.name, st.Index, shard)
			bits, err := f.store.GetBits(ctx, key, offsets)
			if err != nil {
				return false, fmt.Errorf("bloom: get bits shard %d: %w", shard, err)
			}
			for _, b := range bits {
				if !b {
					allSet = false
					break
				}
			}
			if !allSet {
				break
			}
		}
		if allSet {
			return true, nil
		}
	}
	return false, nil
}

// Stats returns a read-only snapshot of the filter's current state.
func (f *Filter) Stats() Stats {
	stages := make([]Stage, len(f.stages))
	copy(stages, f.stages)
	return Stats{
		Name:       f.name,
		N:          f.n,
		P:          f.p,
		R:          f.r,
		S:          f.s,
		StageCount: len(f.stages),
		Stages:     stages,
	}
}

// Drop removes the filter and all its data from the store.
func (f *Filter) Drop(ctx context.Context) error {
	var keys []string
	keys = append(keys, metaKey(f.name))
	for _, st := range f.stages {
		keys = append(keys, stageMetaKey(f.name, st.Index))
		keys = append(keys, stageMetaKey(f.name, st.Index)+":fill")
		numShards := st.M / ShardBits
		if st.M%ShardBits != 0 {
			numShards++
		}
		for j := uint64(0); j < numShards; j++ {
			keys = append(keys, shardKey(f.name, st.Index, j))
		}
	}

	if err := f.store.Del(ctx, keys...); err != nil {
		return fmt.Errorf("bloom: delete keys: %w", err)
	}
	if err := f.store.SRem(ctx, registryKey, f.name); err != nil {
		return fmt.Errorf("bloom: remove from registry: %w", err)
	}
	return nil
}

func (f *Filter) appendNewStage(ctx context.Context) error {
	idx := len(f.stages)
	newStage, err := computeStage(f.n, f.p, f.r, f.s, idx)
	if err != nil {
		return fmt.Errorf("bloom: compute stage %d: %w", idx, err)
	}

	newCount, err := f.store.AppendStage(
		ctx,
		metaKey(f.name),
		int64(idx),
		stageToMap(newStage),
		stageMetaKey(f.name, idx),
	)
	if err != nil {
		return fmt.Errorf("bloom: append stage: %w", err)
	}

	if newCount > int64(idx)+1 {
		return f.reload(ctx)
	}

	f.stages = append(f.stages, newStage)
	return nil
}

func (f *Filter) reload(ctx context.Context) error {
	reloaded, err := Load(ctx, f.store, f.name)
	if err != nil {
		return fmt.Errorf("bloom: reload after race: %w", err)
	}
	f.stages = reloaded.stages
	return nil
}
