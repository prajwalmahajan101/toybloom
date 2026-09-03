.PHONY: build test lint vet run generate openapi-validate integration e2e load compose-up compose-down

# Regenerate the server types/routes from api/openapi.yaml (oapi-codegen).
generate:
	go generate ./...

build: generate
	go build ./...

test:
	go test ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

run:
	go run ./cmd/server

# Validate the OpenAPI contract (source of truth for client SDKs).
openapi-validate:
	npx --yes @redocly/cli lint api/openapi.yaml

# ── M10: integration, e2e & load ────────────────────────────────────────────

# Fast in-process store integration tests (real Valkey via testcontainers).
integration:
	go test -tags=integration ./...

# Statistical correctness + HTTP e2e. The correctness tests self-provision Valkey
# via testcontainers; the HTTP walk needs a running server, so we boot the full
# compose stack, point E2E_BASE_URL at it, run, and always tear down.
e2e: compose-up
	E2E_BASE_URL=http://localhost:8080 go test -tags=e2e -count=1 ./internal/e2e/... ; \
	  status=$$? ; $(MAKE) compose-down ; exit $$status

# Load test: bring the stack up, drive it with dockerized k6, gate on p99<200ms.
load: compose-up
	docker run --rm -i --network host -e BASE_URL=http://localhost:8080 \
	  grafana/k6 run - < test/load/bloom_load.js ; \
	  status=$$? ; $(MAKE) compose-down ; exit $$status

# Bring up the compose stack and wait until the app is ready.
compose-up:
	docker compose up -d --build
	@echo "waiting for /readyz ..."
	@for i in $$(seq 1 30); do \
	  if curl -sf http://localhost:8080/readyz >/dev/null 2>&1; then echo "ready"; exit 0; fi; \
	  sleep 2; \
	done; echo "server did not become ready" >&2; docker compose logs app >&2; exit 1

compose-down:
	docker compose down -v
