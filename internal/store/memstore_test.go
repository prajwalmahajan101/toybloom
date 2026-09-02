package store

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

func TestMemStore_SetGetBits(t *testing.T) {
	ms := NewMemStore()
	ctx := context.Background()

	offsets := []uint64{0, 7, 63, 1000, 33_554_431}
	if err := ms.SetBits(ctx, "test:bits", offsets); err != nil {
		t.Fatalf("SetBits: %v", err)
	}

	got, err := ms.GetBits(ctx, "test:bits", offsets)
	if err != nil {
		t.Fatalf("GetBits: %v", err)
	}
	for i, b := range got {
		if !b {
			t.Errorf("offset %d: want true, got false", offsets[i])
		}
	}

	unset, err := ms.GetBits(ctx, "test:bits", []uint64{1, 2, 3})
	if err != nil {
		t.Fatalf("GetBits unset: %v", err)
	}
	for i, b := range unset {
		if b {
			t.Errorf("unset offset %d: want false, got true", i)
		}
	}
}

func TestMemStore_EmptyOffsets(t *testing.T) {
	ms := NewMemStore()
	ctx := context.Background()

	if err := ms.SetBits(ctx, "test:empty", nil); err != nil {
		t.Fatalf("SetBits nil: %v", err)
	}
	got, err := ms.GetBits(ctx, "test:empty", nil)
	if err != nil {
		t.Fatalf("GetBits nil: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want empty slice, got len %d", len(got))
	}
}

func TestMemStore_HSetHGetAll(t *testing.T) {
	ms := NewMemStore()
	ctx := context.Background()

	fields := map[string]string{"n": "1000", "p": "0.01", "stage_count": "1"}
	if err := ms.HSet(ctx, "test:meta", fields); err != nil {
		t.Fatalf("HSet: %v", err)
	}

	got, err := ms.HGetAll(ctx, "test:meta")
	if err != nil {
		t.Fatalf("HGetAll: %v", err)
	}
	for k, want := range fields {
		if got[k] != want {
			t.Errorf("field %q: want %q, got %q", k, want, got[k])
		}
	}
}

func TestMemStore_HGetAllMissing(t *testing.T) {
	ms := NewMemStore()
	ctx := context.Background()

	got, err := ms.HGetAll(ctx, "test:nonexistent")
	if err != nil {
		t.Fatalf("HGetAll missing: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want empty map, got %v", got)
	}
}

func TestMemStore_Incr(t *testing.T) {
	ms := NewMemStore()
	ctx := context.Background()

	for want := int64(1); want <= 3; want++ {
		got, err := ms.Incr(ctx, "test:counter")
		if err != nil {
			t.Fatalf("Incr: %v", err)
		}
		if got != want {
			t.Errorf("Incr: want %d, got %d", want, got)
		}
	}
}

func TestMemStore_SetOps(t *testing.T) {
	ms := NewMemStore()
	ctx := context.Background()

	if err := ms.SAdd(ctx, "test:set", "a", "b", "c"); err != nil {
		t.Fatalf("SAdd: %v", err)
	}
	members, err := ms.SMembers(ctx, "test:set")
	if err != nil {
		t.Fatalf("SMembers: %v", err)
	}
	if len(members) != 3 {
		t.Fatalf("want 3 members, got %d", len(members))
	}

	if err := ms.SRem(ctx, "test:set", "b"); err != nil {
		t.Fatalf("SRem: %v", err)
	}
	members, err = ms.SMembers(ctx, "test:set")
	if err != nil {
		t.Fatalf("SMembers after SRem: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("want 2 members after SRem, got %d", len(members))
	}
}

func TestMemStore_Del(t *testing.T) {
	ms := NewMemStore()
	ctx := context.Background()

	_ = ms.HSet(ctx, "test:del1", map[string]string{"a": "1"})
	_ = ms.HSet(ctx, "test:del2", map[string]string{"b": "2"})

	if err := ms.Del(ctx, "test:del1", "test:del2"); err != nil {
		t.Fatalf("Del: %v", err)
	}
	got, _ := ms.HGetAll(ctx, "test:del1")
	if len(got) != 0 {
		t.Errorf("del1 still has data: %v", got)
	}
	got, _ = ms.HGetAll(ctx, "test:del2")
	if len(got) != 0 {
		t.Errorf("del2 still has data: %v", got)
	}
}

func TestMemStore_AppendStage(t *testing.T) {
	ms := NewMemStore()
	ctx := context.Background()

	_ = ms.HSet(ctx, "bf:test:meta", map[string]string{"stage_count": "1"})

	fields := map[string]string{"m": "9585059", "k": "7", "capacity": "2000000", "fill_count": "0"}
	newCount, err := ms.AppendStage(ctx, "bf:test:meta", 1, fields, "bf:test:s1:meta")
	if err != nil {
		t.Fatalf("AppendStage: %v", err)
	}
	if newCount != 2 {
		t.Errorf("want new count 2, got %d", newCount)
	}

	got, err := ms.HGetAll(ctx, "bf:test:s1:meta")
	if err != nil {
		t.Fatalf("HGetAll new stage: %v", err)
	}
	if got["m"] != "9585059" {
		t.Errorf("new stage m: want 9585059, got %q", got["m"])
	}
}

func TestMemStore_AppendStageRace(t *testing.T) {
	ms := NewMemStore()
	ctx := context.Background()

	_ = ms.HSet(ctx, "bf:race:meta", map[string]string{"stage_count": "0"})

	var wg sync.WaitGroup
	results := make([]int64, 2)
	errs := make([]error, 2)

	for i := range 2 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			fields := map[string]string{"m": "100", "k": "3", "capacity": "1000", "fill_count": "0"}
			key := fmt.Sprintf("bf:race:s%d:meta", idx)
			results[idx], errs[idx] = ms.AppendStage(ctx, "bf:race:meta", 0, fields, key)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
	}

	// Both goroutines return 1: the winner because it bumped 0→1, the loser
	// because the script returns the current count when the CAS fails. The real
	// invariant is that exactly one stage meta key was written.
	written := 0
	for i := range 2 {
		got, _ := ms.HGetAll(ctx, fmt.Sprintf("bf:race:s%d:meta", i))
		if len(got) > 0 {
			written++
		}
	}
	if written != 1 {
		t.Errorf("want exactly 1 stage written, got %d", written)
	}
}
