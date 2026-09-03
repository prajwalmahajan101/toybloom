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
	"github.com/prajwalmahajan101/toybloom/internal/obs"
	"go.opentelemetry.io/otel/trace"
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

// CorrelationID assigns or propagates a correlation id per request. When a trace
// is active — otelgin runs first, so it usually is — the id IS the trace id, so
// logs, the X-Correlation-ID response header, and the distributed trace all
// share one identifier. Without a trace it honours an incoming header, then
// falls back to a fresh UUID.
func CorrelationID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Correlation-ID")
		if sc := trace.SpanContextFromContext(c.Request.Context()); sc.HasTraceID() {
			id = sc.TraceID().String()
		} else if id == "" {
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

		// InfoContext (not Info) so the otelslog bridge stamps the record with
		// the active trace_id/span_id, correlating this log line to its trace.
		reqLog.InfoContext(c.Request.Context(), "request",
			"method", c.Request.Method,
			"path", c.FullPath(),
			"status", c.Writer.Status(),
			"latency_ms", time.Since(start).Milliseconds(),
		)
	}
}

// HTTPMetrics records the RED latency datapoint per request on the OTel meter.
// It runs inside otelgin, so the request context already carries the server
// span — the histogram gets a trace exemplar for free.
func HTTPMetrics(inst *obs.Instruments) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		route := c.FullPath()
		if route == "" {
			route = "unmatched" // bound label cardinality from unmatched 404s
		}
		inst.RecordRequest(c.Request.Context(), route, c.Request.Method,
			c.Writer.Status(), time.Since(start).Seconds())
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
