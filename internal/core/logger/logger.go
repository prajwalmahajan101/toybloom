package logger

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

// New builds a JSON structured logger at the given level
// (debug|info|warn|error, defaulting to info). Built once at startup from
// config and injected where needed.
func New(level string) *slog.Logger {
	lvl := slog.LevelInfo
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}

type ctxKey struct{}

// IntoContext returns a copy of ctx carrying lg (the request-scoped logger).
func IntoContext(ctx context.Context, lg *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, lg)
}

// FromContext returns the logger stored in ctx, or slog.Default() if none.
// Service/store code calls this to emit logs that carry the correlation id.
func FromContext(ctx context.Context) *slog.Logger {
	if lg, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok {
		return lg
	}
	return slog.Default()
}
