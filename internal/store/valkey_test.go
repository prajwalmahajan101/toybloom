//go:build integration

package store

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/valkey-io/valkey-go"
)

func startValkey(t *testing.T) *ValkeyStore {
	t.Helper()
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "valkey/valkey:8",
		ExposedPorts: []string{"6379/tcp"},
		WaitingFor:   wait.ForLog("Ready to accept connections"),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("start valkey container: %v", err)
	}
	t.Cleanup(func() { container.Terminate(ctx) })

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("container host: %v", err)
	}
	port, err := container.MappedPort(ctx, "6379")
	if err != nil {
		t.Fatalf("container port: %v", err)
	}

	client, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{fmt.Sprintf("%s:%s", host, port.Port())},
	})
	if err != nil {
		t.Fatalf("valkey client: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	return NewValkeyStore(client)
}

func TestValkeyStore_SetGetBits(t *testing.T) {
	vs := startValkey(t)
	ctx := context.Background()

	offsets := []uint64{0, 7, 63, 1000, 33_554_431}
	if err := vs.SetBits(ctx, "test:bits", offsets); err != nil {
		t.Fatalf("SetBits: %v", err)
	}

	got, err := vs.GetBits(ctx, "test:bits", offsets)
	if err != nil {
		t.Fatalf("GetBits: %v", err)
	}
	for i, b := range got {
		if !b {
			t.Errorf("offset %d: want true, got false", offsets[i])
		}
	}

	unset, err := vs.GetBits(ctx, "test:bits", []uint64{1, 2, 3})
	if err != nil {
		t.Fatalf("GetBits unset: %v", err)
	}
	for i, b := range unset {
		if b {
			t.Errorf("unset offset %d: want false, got true", i)
		}
	}
}

func TestValkeyStore_EmptyOffsets(t *testing.T) {
	vs := startValkey(t)
	ctx := context.Background()

	if err := vs.SetBits(ctx, "test:empty", nil); err != nil {
		t.Fatalf("SetBits nil: %v", err)
	}
	got, err := vs.GetBits(ctx, "test:empty", nil)
	if err != nil {
		t.Fatalf("GetBits nil: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want empty slice, got len %d", len(got))
	}
}

func TestValkeyStore_HSetHGetAll(t *testing.T) {
	vs := startValkey(t)
	ctx := context.Background()

	fields := map[string]string{"n": "1000", "p": "0.01", "stage_count": "1"}
	if err := vs.HSet(ctx, "test:meta", fields); err != nil {
		t.Fatalf("HSet: %v", err)
	}

	got, err := vs.HGetAll(ctx, "test:meta")
	if err != nil {
		t.Fatalf("HGetAll: %v", err)
	}
	for k, want := range fields {
		if got[k] != want {
			t.Errorf("field %q: want %q, got %q", k, want, got[k])
		}
	}
}

func TestValkeyStore_HGetAllMissing(t *testing.T) {
	vs := startValkey(t)
	ctx := context.Background()

	got, err := vs.HGetAll(ctx, "test:nonexistent")
	if err != nil {
		t.Fatalf("HGetAll missing: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want empty map, got %v", got)
	}
}

func TestValkeyStore_Incr(t *testing.T) {
	vs := startValkey(t)
	ctx := context.Background()

	for want := int64(1); want <= 3; want++ {
		got, err := vs.Incr(ctx, "test:counter")
		if err != nil {
			t.Fatalf("Incr: %v", err)
		}
		if got != want {
			t.Errorf("Incr: want %d, got %d", want, got)
		}
	}
}

func TestValkeyStore_SetOps(t *testing.T) {
	vs := startValkey(t)
	ctx := context.Background()

	if err := vs.SAdd(ctx, "test:set", "a", "b", "c"); err != nil {
		t.Fatalf("SAdd: %v", err)
	}
	members, err := vs.SMembers(ctx, "test:set")
	if err != nil {
		t.Fatalf("SMembers: %v", err)
	}
	if len(members) != 3 {
		t.Fatalf("want 3 members, got %d", len(members))
	}

	if err := vs.SRem(ctx, "test:set", "b"); err != nil {
		t.Fatalf("SRem: %v", err)
	}
	members, err = vs.SMembers(ctx, "test:set")
	if err != nil {
		t.Fatalf("SMembers after SRem: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("want 2 members after SRem, got %d", len(members))
	}
}

func TestValkeyStore_Del(t *testing.T) {
	vs := startValkey(t)
	ctx := context.Background()

	_ = vs.HSet(ctx, "test:del1", map[string]string{"a": "1"})
	_ = vs.HSet(ctx, "test:del2", map[string]string{"b": "2"})

	if err := vs.Del(ctx, "test:del1", "test:del2"); err != nil {
		t.Fatalf("Del: %v", err)
	}
	got, _ := vs.HGetAll(ctx, "test:del1")
	if len(got) != 0 {
		t.Errorf("del1 still has data: %v", got)
	}
	got, _ = vs.HGetAll(ctx, "test:del2")
	if len(got) != 0 {
		t.Errorf("del2 still has data: %v", got)
	}
}

func TestValkeyStore_AppendStage(t *testing.T) {
	vs := startValkey(t)
	ctx := context.Background()

	_ = vs.HSet(ctx, "bf:test:meta", map[string]string{"stage_count": "1"})

	fields := map[string]string{"m": "9585059", "k": "7", "capacity": "2000000", "fill_count": "0"}
	newCount, err := vs.AppendStage(ctx, "bf:test:meta", 1, fields, "bf:test:s1:meta")
	if err != nil {
		t.Fatalf("AppendStage: %v", err)
	}
	if newCount != 2 {
		t.Errorf("want new count 2, got %d", newCount)
	}

	got, err := vs.HGetAll(ctx, "bf:test:s1:meta")
	if err != nil {
		t.Fatalf("HGetAll new stage: %v", err)
	}
	if got["m"] != "9585059" {
		t.Errorf("new stage m: want 9585059, got %q", got["m"])
	}
}

func TestValkeyStore_AppendStageRace(t *testing.T) {
	vs := startValkey(t)
	ctx := context.Background()

	_ = vs.HSet(ctx, "bf:race:meta", map[string]string{"stage_count": "0"})

	var wg sync.WaitGroup
	results := make([]int64, 2)
	errs := make([]error, 2)

	for i := range 2 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			fields := map[string]string{"m": "100", "k": "3", "capacity": "1000", "fill_count": "0"}
			key := fmt.Sprintf("bf:race:s%d:meta", idx)
			results[idx], errs[idx] = vs.AppendStage(ctx, "bf:race:meta", 0, fields, key)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
	}

	// Both goroutines return 1: the winner bumped 0→1, the loser gets the
	// current count back from the Lua CAS. The real invariant is that exactly
	// one stage meta key was written.
	written := 0
	for i := range 2 {
		got, _ := vs.HGetAll(ctx, fmt.Sprintf("bf:race:s%d:meta", i))
		if len(got) > 0 {
			written++
		}
	}
	if written != 1 {
		t.Errorf("want exactly 1 stage written, got %d", written)
	}
}
