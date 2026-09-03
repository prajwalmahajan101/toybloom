package service

import (
	"context"
	"errors"
	"testing"

	"github.com/prajwalmahajan101/toybloom/internal/obs"
	"github.com/prajwalmahajan101/toybloom/pkg/store"
)

func newService() FilterService {
	return New(store.NewMemStore(), nil, 0) // nil instruments: record helpers no-op; 0 = no gauge cap
}

func TestFilterSamples(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	for _, name := range []string{"a", "b", "c"} {
		if _, err := svc.Create(ctx, name, 1000, 0.01); err != nil {
			t.Fatalf("Create %q: %v", name, err)
		}
	}
	// Load one filter with items so its fill ratio is observably non-zero.
	if err := svc.Add(ctx, "a", []byte("x")); err != nil {
		t.Fatalf("Add: %v", err)
	}

	obsv := svc.FilterSamples(ctx)
	if obsv.Count != 3 {
		t.Fatalf("FilterSamples Count: want 3, got %d", obsv.Count)
	}
	if len(obsv.Samples) != 3 {
		t.Fatalf("FilterSamples: want 3 samples, got %d", len(obsv.Samples))
	}
	byName := map[string]obs.FilterSample{}
	for _, s := range obsv.Samples {
		if s.FillRatio < 0 || s.EstimatedFPP < 0 || s.EstimatedFPP >= 1 {
			t.Errorf("%s: out-of-range sample %+v", s.Name, s)
		}
		byName[s.Name] = s
	}
	if byName["a"].FillRatio <= 0 {
		t.Errorf("filter a should have non-zero fill ratio, got %v", byName["a"].FillRatio)
	}
	if byName["a"].Items != 1 {
		t.Errorf("filter a should have 1 item, got %d", byName["a"].Items)
	}
}

func TestFilterSamples_CapTruncates(t *testing.T) {
	svc := New(store.NewMemStore(), nil, 2) // cap of 2
	ctx := context.Background()
	for _, name := range []string{"a", "b", "c", "d"} {
		if _, err := svc.Create(ctx, name, 1000, 0.01); err != nil {
			t.Fatalf("Create %q: %v", name, err)
		}
	}
	obsv := svc.FilterSamples(ctx)
	if got := len(obsv.Samples); got != 2 {
		t.Fatalf("FilterSamples with cap 2: want 2 samples, got %d", got)
	}
	if obsv.Count != 4 {
		t.Fatalf("FilterSamples Count should be the true total 4, got %d", obsv.Count)
	}
}

func TestCreate(t *testing.T) {
	svc := newService()
	info, err := svc.Create(context.Background(), "test", 1000, 0.01)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if info.Name != "test" {
		t.Errorf("Name: want test, got %q", info.Name)
	}
	if info.Stages < 1 {
		t.Errorf("Stages: want >=1, got %d", info.Stages)
	}
	if info.M == 0 {
		t.Error("M: want >0, got 0")
	}
	if info.K == 0 {
		t.Error("K: want >0, got 0")
	}
}

func TestCreate_Duplicate(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	if _, err := svc.Create(ctx, "dup", 1000, 0.01); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	_, err := svc.Create(ctx, "dup", 1000, 0.01)
	if !errors.Is(err, ErrAlreadyExists) {
		t.Errorf("want ErrAlreadyExists, got %v", err)
	}
}

func TestCreate_Invalid(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	cases := []struct {
		name string
		n    uint64
		p    float64
	}{
		{"n=0", 0, 0.01},
		{"p=2", 1000, 2},
		{"p=0", 1000, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Create(ctx, "f", tc.n, tc.p)
			if !errors.Is(err, ErrInvalidArgument) {
				t.Errorf("want ErrInvalidArgument, got %v", err)
			}
		})
	}
}

func TestAdd_Exists(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	if _, err := svc.Create(ctx, "f", 1000, 0.01); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.Add(ctx, "f", []byte("hello")); err != nil {
		t.Fatalf("Add: %v", err)
	}
	ok, err := svc.Exists(ctx, "f", []byte("hello"))
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !ok {
		t.Error("want exists=true, got false")
	}
}

func TestExists_NotAdded(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	if _, err := svc.Create(ctx, "f", 1000, 0.01); err != nil {
		t.Fatalf("Create: %v", err)
	}
	ok, err := svc.Exists(ctx, "f", []byte("nope"))
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if ok {
		t.Error("want exists=false, got true")
	}
}

func TestAdd_NoFilter(t *testing.T) {
	svc := newService()
	err := svc.Add(context.Background(), "ghost", []byte("x"))
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestStats(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	if _, err := svc.Create(ctx, "f", 1000, 0.01); err != nil {
		t.Fatalf("Create: %v", err)
	}
	stats, err := svc.Stats(ctx, "f")
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.Name != "f" {
		t.Errorf("Name: want f, got %q", stats.Name)
	}
	if stats.N != 1000 {
		t.Errorf("N: want 1000, got %d", stats.N)
	}
	if stats.P != 0.01 {
		t.Errorf("P: want 0.01, got %v", stats.P)
	}
	if len(stats.Stages) == 0 {
		t.Error("want non-empty Stages")
	}
}

func TestDelete(t *testing.T) {
	svc := newService()
	ctx := context.Background()
	if _, err := svc.Create(ctx, "f", 1000, 0.01); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.Delete(ctx, "f"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := svc.Stats(ctx, "f")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound after delete, got %v", err)
	}
}
