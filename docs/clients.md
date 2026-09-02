# Clients

toybloom is consumed at two layers. See [ADR-0006](ADR/0006-api-contract-and-client-architecture.md)
for the decision behind this split.

```
                 pkg/bloom + pkg/store   (Go engine library)
                          │
        ┌─────────────────┴──────────────────┐
        │ IN-PROCESS (import Go)              │ OVER-THE-WIRE (HTTP)
   cmd/cli, cmd/tui, cmd/worker         cmd/server (gin/REST)
   embed the engine directly                │
                                            │  api/openapi.yaml
                          ┌──────────┬───────┴────┬──────────┐
                       Go SDK    Python SDK   JS/TS SDK   Java SDK
```

## Client matrix

| Client | Kind | Connects via | Built how | Status |
|---|---|---|---|---|
| `cmd/cli` | in-process | `import pkg/bloom` | Go binary in this repo | planned |
| `cmd/tui` | in-process | `import pkg/bloom` | Go binary in this repo | planned |
| `cmd/worker` (queue) | in-process | `import pkg/bloom` (+ broker) | Go binary in this repo | planned |
| Go HTTP client | wire | REST | generated from spec (or thin hand-written) | planned |
| Python SDK | wire | REST | `openapi-generator` → separate repo | planned |
| JS/TS SDK | wire | REST | `openapi-generator` → separate repo | planned |
| Java SDK | wire | REST | `openapi-generator` → separate repo | planned |

- **In-process embedders** run the algorithm locally against Valkey; no server
  needed. They must be Go (that is what "import the engine" means).
- **Wire clients** are network clients of `cmd/server`. They are **generated
  from `api/openapi.yaml`** so all languages stay in lockstep with the contract.

> The **server itself** is also generated from the same spec (via `oapi-codegen`
> into `internal/api/gen`), so client and server share one source of truth. See
> [ADR-0006](ADR/0006-api-contract-and-client-architecture.md).

## Generating a wire SDK

The contract is `api/openapi.yaml` (OpenAPI 3.1). From the repo root:

```sh
# Python
openapi-generator generate -i api/openapi.yaml -g python \
  -o ../toybloom-python --additional-properties=packageName=toybloom

# TypeScript (fetch)
openapi-generator generate -i api/openapi.yaml -g typescript-fetch \
  -o ../toybloom-js

# Java
openapi-generator generate -i api/openapi.yaml -g java -o ../toybloom-java

# Go client
openapi-generator generate -i api/openapi.yaml -g go -o ../toybloom-go-client
```

Generated SDK repos live **outside** this module (their own repos, published to
PyPI / npm / Maven), regenerated in CI whenever `api/openapi.yaml` changes.

## Viewing / validating the contract

- Interactive: run the server and open `http://localhost:8080/docs`.
- Raw spec: `http://localhost:8080/openapi.yaml` or the file `api/openapi.yaml`.
- Lint: `npx @redocly/cli lint api/openapi.yaml`
  (or `openapi-generator validate -i api/openapi.yaml`).
