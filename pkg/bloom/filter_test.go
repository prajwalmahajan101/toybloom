package bloom

import (
	"context"
	"fmt"
	"testing"

	"github.com/prajwalmahajan101/toybloom/pkg/store"
)

func newTestFilter(t *testing.T) (*Filter, *store.MemStore) {
	t.Helper()
	ms := store.NewMemStore()
	ctx := context.Background()
	f, err := New(ctx, ms, "test", 1000, 0.01)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return f, ms
}

func TestNew_CreatesStage0(t *testing.T) {
	f, _ := newTestFilter(t)
	if len(f.stages) != 1 {
		t.Fatalf("expected 1 stage, got %d", len(f.stages))
	}
	s := f.stages[0]
	if s.Index != 0 {
		t.Errorf("stage index = %d, want 0", s.Index)
	}
	if s.M == 0 {
		t.Error("stage M = 0")
	}
	if s.K == 0 {
		t.Error("stage K = 0")
	}
	if s.Capacity == 0 {
		t.Error("stage Capacity = 0")
	}
	if s.Fill != 0 {
		t.Errorf("stage Fill = %d, want 0", s.Fill)
	}
}

func TestNew_InRegistry(t *testing.T) {
	_, ms := newTestFilter(t)
	members, err := ms.SMembers(context.Background(), registryKey)
	if err != nil {
		t.Fatalf("SMembers: %v", err)
	}
	found := false
	for _, m := range members {
		if m == "test" {
			found = true
		}
	}
	if !found {
		t.Error("filter not in registry")
	}
}

func TestNew_DuplicateErrors(t *testing.T) {
	ms := store.NewMemStore()
	ctx := context.Background()
	if _, err := New(ctx, ms, "dup", 100, 0.01); err != nil {
		t.Fatalf("first New: %v", err)
	}
	if _, err := New(ctx, ms, "dup", 100, 0.01); err != ErrFilterExists {
		t.Errorf("second New err = %v, want ErrFilterExists", err)
	}
}

func TestNew_ValidationErrors(t *testing.T) {
	ms := store.NewMemStore()
	ctx := context.Background()
	cases := []struct {
		label      string
		filterName string
		n          uint64
		p          float64
		want       error
	}{
		{"empty name", "", 100, 0.01, ErrEmptyName},
		{"n=0", "ok", 0, 0.01, ErrInvalidN},
		{"p=0", "ok", 100, 0, ErrInvalidP},
		{"p=1", "ok", 100, 1, ErrInvalidP},
		{"p<0", "ok", 100, -0.1, ErrInvalidP},
	}
	for _, c := range cases {
		t.Run(c.label, func(t *testing.T) {
			_, err := New(ctx, ms, c.filterName, c.n, c.p)
			if err != c.want {
				t.Errorf("err = %v, want %v", err, c.want)
			}
		})
	}
}

func TestLoad_RoundTrip(t *testing.T) {
	f, ms := newTestFilter(t)
	ctx := context.Background()
	loaded, err := Load(ctx, ms, "test")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.name != f.name {
		t.Errorf("name = %q, want %q", loaded.name, f.name)
	}
	if loaded.n != f.n {
		t.Errorf("n = %d, want %d", loaded.n, f.n)
	}
	if loaded.p != f.p {
		t.Errorf("p = %f, want %f", loaded.p, f.p)
	}
	if len(loaded.stages) != len(f.stages) {
		t.Fatalf("stages = %d, want %d", len(loaded.stages), len(f.stages))
	}
	if loaded.stages[0].M != f.stages[0].M {
		t.Errorf("stage-0 M = %d, want %d", loaded.stages[0].M, f.stages[0].M)
	}
}

func TestLoad_NotFound(t *testing.T) {
	ms := store.NewMemStore()
	_, err := Load(context.Background(), ms, "nope")
	if err != ErrNotFound {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestAddExists_Basic(t *testing.T) {
	f, _ := newTestFilter(t)
	ctx := context.Background()

	item := []byte("hello")
	if err := f.Add(ctx, item); err != nil {
		t.Fatalf("Add: %v", err)
	}
	exists, err := f.Exists(ctx, item)
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !exists {
		t.Error("Exists returned false for added item")
	}
}

func TestExists_MissingItem(t *testing.T) {
	f, _ := newTestFilter(t)
	ctx := context.Background()

	exists, err := f.Exists(ctx, []byte("never-added"))
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if exists {
		t.Error("Exists returned true for never-added item")
	}
}

func TestNoFalseNegatives(t *testing.T) {
	f, _ := newTestFilter(t)
	ctx := context.Background()

	items := make([][]byte, 500)
	for i := range items {
		items[i] = fmt.Appendf(nil, "item-%d", i)
		if err := f.Add(ctx, items[i]); err != nil {
			t.Fatalf("Add item-%d: %v", i, err)
		}
	}

	for i, item := range items {
		exists, err := f.Exists(ctx, item)
		if err != nil {
			t.Fatalf("Exists item-%d: %v", i, err)
		}
		if !exists {
			t.Errorf("false negative for item-%d", i)
		}
	}
}

func TestFalsePositiveRate(t *testing.T) {
	ms := store.NewMemStore()
	ctx := context.Background()
	f, err := New(ctx, ms, "fpp", 10_000, 0.01)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	for i := range 10_000 {
		if err := f.Add(ctx, fmt.Appendf(nil, "member-%d", i)); err != nil {
			t.Fatalf("Add %d: %v", i, err)
		}
	}

	fp := 0
	probes := 10_000
	for i := range probes {
		exists, err := f.Exists(ctx, fmt.Appendf(nil, "nonmember-%d", i))
		if err != nil {
			t.Fatalf("Exists probe %d: %v", i, err)
		}
		if exists {
			fp++
		}
	}

	rate := float64(fp) / float64(probes)
	// Allow 2× the target FPP as margin for statistical noise.
	if rate > 0.02 {
		t.Errorf("false positive rate = %.4f, want ≤ 0.02", rate)
	}
	t.Logf("FPP: %d/%d = %.4f (target ≤ 0.01)", fp, probes, rate)
}

func TestStageGrowth(t *testing.T) {
	ms := store.NewMemStore()
	ctx := context.Background()
	f, err := New(ctx, ms, "grow", 100, 0.01)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	initialStages := len(f.stages)
	for i := range 250 {
		if err := f.Add(ctx, fmt.Appendf(nil, "item-%d", i)); err != nil {
			t.Fatalf("Add %d: %v", i, err)
		}
	}

	if len(f.stages) <= initialStages {
		t.Errorf("no stage growth after inserting 250 items (capacity=%d); stages=%d",
			f.stages[0].Capacity, len(f.stages))
	}
	t.Logf("stages grew from %d to %d", initialStages, len(f.stages))
}

func TestStats(t *testing.T) {
	f, _ := newTestFilter(t)
	s := f.Stats()
	if s.Name != "test" {
		t.Errorf("Name = %q, want %q", s.Name, "test")
	}
	if s.N != 1000 {
		t.Errorf("N = %d, want 1000", s.N)
	}
	if s.StageCount != 1 {
		t.Errorf("StageCount = %d, want 1", s.StageCount)
	}
	if len(s.Stages) != 1 {
		t.Errorf("len(Stages) = %d, want 1", len(s.Stages))
	}
}

func TestDrop(t *testing.T) {
	f, ms := newTestFilter(t)
	ctx := context.Background()

	if err := f.Add(ctx, []byte("data")); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := f.Drop(ctx); err != nil {
		t.Fatalf("Drop: %v", err)
	}

	_, err := Load(ctx, ms, "test")
	if err != ErrNotFound {
		t.Errorf("Load after Drop: err = %v, want ErrNotFound", err)
	}

	members, _ := ms.SMembers(ctx, registryKey)
	for _, m := range members {
		if m == "test" {
			t.Error("filter still in registry after Drop")
		}
	}
}
