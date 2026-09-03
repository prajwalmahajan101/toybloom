# Multi-stage build: compile a static binary, then ship it on a distroless base
# so the runtime image carries no shell, package manager, or libc surface.
FROM golang:1.26 AS build
WORKDIR /src

# Cache module downloads separately from source so a code change doesn't re-fetch.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# CGO off → a fully static binary that runs on the distroless static base.
# Trim symbols/DWARF and stamp the version for the OTel resource.
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/toybloom ./cmd/server

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=build /out/toybloom /toybloom
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/toybloom"]
