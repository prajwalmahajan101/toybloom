.PHONY: build test lint vet run generate openapi-validate

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
