package service

import (
	"context"
	"errors"
	"testing"

	"github.com/prajwalmahajan101/toybloom/pkg/store"
)

func newService() FilterService {
	return New(store.NewMemStore())
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
