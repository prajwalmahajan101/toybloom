package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

// pinger is the minimal store capability the readiness probe needs. Declared at
// the consumer (Go idiom: accept the narrow interface you actually use), so the
// engine's BitStore contract stays free of operational concerns.
type pinger interface {
	Ping(ctx context.Context) error
}

type HealthHandler struct {
	store pinger
}

func NewHealthHandler(s pinger) *HealthHandler {
	return &HealthHandler{store: s}
}

// Healthz is a liveness probe — the process is up. Never touches dependencies.
func (h *HealthHandler) Healthz(c *gin.Context) {
	writeSuccess(c, http.StatusOK, "ok", gin.H{"status": "alive"})
}

// Readyz is a readiness probe — Valkey is reachable, so traffic can be routed.
func (h *HealthHandler) Readyz(c *gin.Context) {
	if err := h.store.Ping(c.Request.Context()); err != nil {
		writeError(c, http.StatusServiceUnavailable, "NOT_READY", "store unreachable")
		return
	}
	writeSuccess(c, http.StatusOK, "ok", gin.H{"status": "ready"})
}
