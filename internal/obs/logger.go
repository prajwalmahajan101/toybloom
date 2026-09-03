package obs

import (
	"log/slog"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	otellog "go.opentelemetry.io/otel/log"
)

// newLogger bridges slog onto the given LoggerProvider. The returned logger is
// used exactly like a standard *slog.Logger, but each record becomes an OTel log
// on the OTLP stream and — when logged with a context that carries an active
// span — is automatically stamped with trace_id/span_id. That auto-stamping is
// why RequestLogger must use the *Context slog methods (InfoContext, …).
func newLogger(name string, lp otellog.LoggerProvider) *slog.Logger {
	return otelslog.NewLogger(name, otelslog.WithLoggerProvider(lp))
}
