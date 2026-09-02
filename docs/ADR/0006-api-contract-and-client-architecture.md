# 0006 — Contract-first API + two-tier client architecture

**Status:** Accepted · **Date:** 2026-09-02

## Context
The bloom engine now lives in an importable library (`pkg/bloom` + `pkg/store`),
and the gin REST server is only one consumer of it. We want to add many clients
over time — in-process Go tools (CLI, TUI, queue worker) and cross-language
SDKs (Python, JS/TS, Java, and a Go HTTP client). Without an agreed model these
would drift: someone would try to "wrap `pkg/bloom` from Python" (impossible —
Go can't be imported in-process by other languages) or hand-write four
divergent client libraries against an undocumented HTTP surface.

## Decision
Adopt a **two-tier client model** and a **contract-first API**.

**Two tiers of client:**
- **In-process embedders** (Go CLI/TUI/worker): `import pkg/bloom` and talk to
  Valkey directly — no network hop. Each is a `cmd/<name>` binary in this repo
  (or an external Go module). Enabled by the `pkg/` engine split.
- **Wire clients** (Python/JS/Java/Go SDKs): cannot import Go; they call
  `cmd/server` over HTTP. They are **generated from the API contract**, not
  hand-written.

**Contract-first:** `api/openapi.yaml` (OpenAPI **3.1**) is the single source of
truth for the wire shape. The server serves it at `/openapi.yaml` and a Swagger
UI at `/docs`. All wire-client SDKs are generated from this one file.

Rejected alternatives:
- **swaggo/swag (code-first)** — generates OpenAPI **2.0** from Go annotations,
  making Go code the source of truth. Worse for polyglot SDK generation and a
  dated spec version.
- **gRPC/protobuf now** — excellent for polyglot codegen, but a second server
  transport we don't need yet. Deferred until a streaming or high-throughput
  worker path justifies it; the REST contract stays primary.

## Consequences
- (+) One language-neutral contract; SDKs are generated, not hand-maintained.
- (+) Embedders and wire clients are cleanly separated — no impossible designs.
- (+) Swagger UI gives interactive docs for free (`/docs`).
- (−) The hand-written spec must be kept in sync with handlers (mitigation
  below: `oapi-codegen` can later generate/validate server types from it).
- (−) Contract-first discipline is on the author until server-side codegen lands.

## Usage
- Contract: `api/openapi.yaml`; embedded + served by `internal/api/docs.go`.
- Client matrix and SDK generation commands: `docs/clients.md`.
- Supersedes the storage-location note in
  [ADR-0002](0002-valkey-bit-storage.md): `BitStore` and its implementations now
  live in `pkg/store` (was `internal/store`) so embedders can import them.
- Named follow-ups (not in this change): `oapi-codegen` to generate/validate the
  gin server's request/response types from the spec; `openapi-generator` for the
  Python/JS/Java/Go client SDKs (separate repos, CI-published); `cmd/cli`,
  `cmd/tui`, `cmd/worker` embedder binaries.

## Update (2026-09-02) — server is now generated from the contract
The `oapi-codegen` follow-up above is **done**. The gin server's request types,
route registration, and payload models are generated from `api/openapi.yaml`
into `internal/api/gen` (`go generate ./...`), and `internal/api/handler.go`
implements the generated `gen.ServerInterface` (a `var _ gen.ServerInterface`
assertion makes drift a compile error). Request validation is spec-driven via
`oapi-codegen/gin-middleware` (`OapiRequestValidator`), replacing the hand-rolled
`validName`/param checks. CI regenerates and runs `git diff --exit-code` so the
committed code can never diverge from the contract. The response **envelope**
(`{success,message,data,errors,correlation_id}`) is still applied by
`internal/core/response`, wrapping the generated payload models — non-strict mode
keeps the envelope ours while the payload schema is generated.
