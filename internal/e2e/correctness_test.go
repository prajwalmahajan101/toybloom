//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/prajwalmahajan101/toybloom/internal/service"
	"github.com/prajwalmahajan101/toybloom/pkg/store"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/valkey-io/valkey-go"
)

// startService spins a throwaway Valkey and returns the real service stack wired
// to it (nil instruments — record helpers no-op). Mirrors pkg/store/valkey_test.go.
func startService(t *testing.T) (service.FilterService, context.Context) {
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
	t.Cleanup(func() { _ = container.Terminate(ctx) })

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

	return service.New(store.NewValkeyStore(client), nil, 0), ctx
}

// TestZeroFalseNegatives is the hard guarantee: every item ever added MUST be
// reported present, across multiple SBF stages. n is deliberately small so that
// inserting far more than n forces growth (cap_0 = n, cap_1 = 2n, cap_2 = 4n …).
func TestZeroFalseNegatives(t *testing.T) {
	svc, ctx := startService(t)

	const (
		name = "zfn"
		n    = 1_000 // stage-0 capacity
		adds = 7_000 // > n+2n = forces at least 3 stages
	)
	if _, err := svc.Create(ctx, name, n, 0.01); err != nil {
		t.Fatalf("Create: %v", err)
	}
	for i := 0; i < adds; i++ {
		if err := svc.Add(ctx, name, key(i)); err != nil {
			t.Fatalf("Add %d: %v", i, err)
		}
	}
	// Every inserted key must be present — a single miss is a correctness bug.
	for i := 0; i < adds; i++ {
		ok, err := svc.Exists(ctx, name, key(i))
		if err != nil {
			t.Fatalf("Exists %d: %v", i, err)
		}
		if !ok {
			t.Fatalf("FALSE NEGATIVE at item %d — inserted key reported absent", i)
		}
	}

	// Confirm growth actually engaged (guards the test itself).
	st, err := svc.Stats(ctx, name)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if st.StageCount < 3 {
		t.Fatalf("expected >=3 stages to exercise growth, got %d", st.StageCount)
	}
}

// TestMeasuredFPP proves the aggregate false-positive rate stays at or below the
// configured p. The SBF construction (p_i = p·(1-r)·r^i, Σ = p) bounds the total
// FPP by p across all stages. We insert n known keys, then probe a large disjoint
// set of never-inserted keys and count hits.
//
// Sample sizing: with M probes the observed FP count X ~ Binomial(M, fpr) with
// fpr <= p. For p=0.01, M=200_000: mean <= 2000, std = sqrt(M·p·(1-p)) ≈ 44. A
// threshold of p·1.5 (=0.015, i.e. 3000 hits) sits ~22σ above the true mean —
// effectively never flaky — while a broken filter (fpr well above p) trips it.
func TestMeasuredFPP(t *testing.T) {
	svc, ctx := startService(t)

	const (
		name      = "fpp"
		n         = 50_000
		p         = 0.01
		probes    = 200_000
		tolerance = 1.5 // allowed slack over p to absorb binomial noise
	)
	if _, err := svc.Create(ctx, name, n, p); err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Insert the known set: keys 0..n-1.
	for i := 0; i < n; i++ {
		if err := svc.Add(ctx, name, key(i)); err != nil {
			t.Fatalf("Add %d: %v", i, err)
		}
	}
	// Probe a disjoint never-inserted set: keys n..n+probes-1.
	falsePositives := 0
	for i := n; i < n+probes; i++ {
		ok, err := svc.Exists(ctx, name, key(i))
		if err != nil {
			t.Fatalf("Exists %d: %v", i, err)
		}
		if ok {
			falsePositives++
		}
	}
	observed := float64(falsePositives) / float64(probes)
	limit := p * tolerance
	t.Logf("measured FPP = %.5f over %d probes (target p=%.3f, limit=%.5f)", observed, probes, p, limit)
	if observed > limit {
		t.Fatalf("measured FPP %.5f exceeds limit %.5f (target p=%.3f)", observed, limit, p)
	}
	if math.IsNaN(observed) {
		t.Fatalf("observed FPP is NaN")
	}
}

// key produces a stable, unique byte key for index i.
func key(i int) []byte {
	return []byte(fmt.Sprintf("item-%09d", i))
}
