package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/prajwalmahajan101/toybloom/internal/core/logger"
	"github.com/prajwalmahajan101/toybloom/internal/core/response"
)

// gin stores per-request values under string keys; centralize the key + access
// so it isn't a magic string scattered across handlers.
const correlationIDKey = "correlation_id"

func setCorrelationID(c *gin.Context, id string) { c.Set(correlationIDKey, id) }

func getCorrelationID(c *gin.Context) string {
	if v, ok := c.Get(correlationIDKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// CorrelationID assigns or propagates a correlation id per request.
func CorrelationID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Correlation-ID")
		if id == "" {
			id = uuid.New().String()
		}
		setCorrelationID(c, id)
		c.Header("X-Correlation-ID", id)
		c.Next()
	}
}

// RequestLogger attaches a correlation-scoped logger to the request context and
// logs one structured line per completed request (the RED-metrics foundation).
func RequestLogger(base *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		reqLog := base.With("correlation_id", getCorrelationID(c))
		c.Request = c.Request.WithContext(logger.IntoContext(c.Request.Context(), reqLog))

		c.Next()

		reqLog.Info("request",
			"method", c.Request.Method,
			"path", c.FullPath(),
			"status", c.Writer.Status(),
			"latency_ms", time.Since(start).Milliseconds(),
		)
	}
}

// BodyLimit caps the request body so an oversized payload can't exhaust memory.
func BodyLimit(limit int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		c.Next()
	}
}

// Timeout bounds each request's context so a slow Valkey can't hang a handler.
// Runs after RequestLogger so the timeout ctx inherits the scoped logger.
func Timeout(d time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), d)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// recoveryHandler renders panics through the standard envelope instead of gin's
// bare plaintext 500.
func recoveryHandler() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		logger.FromContext(c.Request.Context()).Error("panic recovered", "err", recovered)
		c.AbortWithStatusJSON(
			http.StatusInternalServerError,
			response.NewError(getCorrelationID(c), "internal server error",
				response.ErrorDetail{Code: "INTERNAL", Message: "internal server error"}),
		)
	})
}
