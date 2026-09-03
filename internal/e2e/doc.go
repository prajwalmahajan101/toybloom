//go:build e2e

// Package e2e holds toybloom's end-to-end and statistical-correctness tests.
//
// These tests need external infrastructure (a real Valkey via testcontainers,
// or a running server addressed by E2E_BASE_URL) and are therefore guarded by
// the `e2e` build tag so the default `go test ./...` stays hermetic and fast.
// Run them with:  go test -tags=e2e ./internal/e2e/...
package e2e
