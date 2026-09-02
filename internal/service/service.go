package service

import (
	"context"
	"errors"

	"github.com/prajwalmahajan101/toybloom/pkg/bloom"
	"github.com/prajwalmahajan101/toybloom/pkg/store"
)

var (
	ErrInvalidArgument = errors.New("service: invalid argument")
	ErrNotFound        = errors.New("service: filter not found")
	ErrAlreadyExists   = errors.New("service: filter already exists")
)

// FilterInfo is the result of creating a filter.
type FilterInfo struct {
	Name   string `json:"name"`
	Stages int    `json:"stages"`
	M      uint64 `json:"m"`
	K      int    `json:"k"`
}

// StageInfo is a read-only view of one stage.
type StageInfo struct {
	Index    int    `json:"index"`
	M        uint64 `json:"m"`
	K        int    `json:"k"`
	Capacity uint64 `json:"capacity"`
	Fill     uint64 `json:"fill"`
}

// FilterStats is the full stats view of a filter.
type FilterStats struct {
	Name       string      `json:"name"`
	N          uint64      `json:"n"`
	P          float64     `json:"p"`
	R          float64     `json:"r"`
	S          float64     `json:"s"`
	StageCount int         `json:"stage_count"`
	Stages     []StageInfo `json:"stages"`
}

// FilterService coordinates filter operations, hiding the bloom/store layers
// behind transport-agnostic DTOs and service-level error sentinels.
type FilterService interface {
	Create(ctx context.Context, name string, n uint64, p float64) (FilterInfo, error)
	Add(ctx context.Context, name string, value []byte) error
	Exists(ctx context.Context, name string, value []byte) (bool, error)
	Stats(ctx context.Context, name string) (FilterStats, error)
	Delete(ctx context.Context, name string) error
}

type filterService struct {
	store store.BitStore
}

func New(s store.BitStore) FilterService {
	return &filterService{store: s}
}

func (s *filterService) Create(ctx context.Context, name string, n uint64, p float64) (FilterInfo, error) {
	f, err := bloom.New(ctx, s.store, name, n, p)
	if err != nil {
		return FilterInfo{}, mapErr(err)
	}
	st := f.Stats()
	return FilterInfo{Name: st.Name, Stages: st.StageCount, M: st.Stages[0].M, K: st.Stages[0].K}, nil
}

func (s *filterService) Add(ctx context.Context, name string, value []byte) error {
	f, err := bloom.Load(ctx, s.store, name)
	if err != nil {
		return mapErr(err)
	}
	if err := f.Add(ctx, value); err != nil {
		return mapErr(err)
	}
	return nil
}

func (s *filterService) Exists(ctx context.Context, name string, value []byte) (bool, error) {
	f, err := bloom.Load(ctx, s.store, name)
	if err != nil {
		return false, mapErr(err)
	}
	ok, err := f.Exists(ctx, value)
	if err != nil {
		return false, mapErr(err)
	}
	return ok, nil
}

func (s *filterService) Stats(ctx context.Context, name string) (FilterStats, error) {
	f, err := bloom.Load(ctx, s.store, name)
	if err != nil {
		return FilterStats{}, mapErr(err)
	}
	return toStats(f.Stats()), nil
}

func (s *filterService) Delete(ctx context.Context, name string) error {
	f, err := bloom.Load(ctx, s.store, name)
	if err != nil {
		return mapErr(err)
	}
	if err := f.Drop(ctx); err != nil {
		return mapErr(err)
	}
	return nil
}

func toStats(st bloom.Stats) FilterStats {
	stages := make([]StageInfo, len(st.Stages))
	for i, s := range st.Stages {
		stages[i] = StageInfo{Index: s.Index, M: s.M, K: s.K, Capacity: s.Capacity, Fill: s.Fill}
	}
	return FilterStats{
		Name:       st.Name,
		N:          st.N,
		P:          st.P,
		R:          st.R,
		S:          st.S,
		StageCount: st.StageCount,
		Stages:     stages,
	}
}

// mapErr translates bloom sentinels into service sentinels; anything else
// passes through as-is (treated as internal by the transport layer).
func mapErr(err error) error {
	switch {
	case errors.Is(err, bloom.ErrFilterExists):
		return ErrAlreadyExists
	case errors.Is(err, bloom.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, bloom.ErrEmptyName),
		errors.Is(err, bloom.ErrInvalidN),
		errors.Is(err, bloom.ErrInvalidP):
		return ErrInvalidArgument
	default:
		return err
	}
}
