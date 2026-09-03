package config

import (
	"log/slog"
	"os"
	"strconv"
	"time"
)

// Config holds all runtime configuration, loaded once at startup from the
// environment. Everything downstream receives it by injection rather than
// reading os.Getenv directly — one source of truth, trivially testable.
type Config struct {
	Port            string
	ValkeyAddr      string
	LogLevel        string
	RequestTimeout  time.Duration
	MaxBodyBytes    int64
	ShutdownTimeout time.Duration

	// Observability. ServiceName labels every span/metric/log; ObsExporter picks
	// the telemetry sink: "otlp" (Collector, ADR 0005), "stdout" (local verify),
	// or "none" (SDK off). OTLP endpoint/sampler come from standard OTEL_* env
	// vars, read by the SDK directly — not duplicated here.
	ServiceName string
	ObsExporter string
	// ObsMaxFilterGauges caps how many per-filter fill_ratio/estimated_fpp series
	// are emitted, bounding the unbounded `filter` label's cardinality (ADR 0008).
	ObsMaxFilterGauges int
}

// Load reads configuration from the environment, applying a safe default for
// any unset variable, so Load never fails.
func Load() Config {
	return Config{
		Port:               envStr("PORT", "8080"),
		ValkeyAddr:         envStr("VALKEY_ADDR", "localhost:6379"),
		LogLevel:           envStr("LOG_LEVEL", "info"),
		RequestTimeout:     envDuration("REQUEST_TIMEOUT", 5*time.Second),
		MaxBodyBytes:       envInt64("MAX_BODY_BYTES", 1<<20), // 1 MiB
		ShutdownTimeout:    envDuration("SHUTDOWN_TIMEOUT", 10*time.Second),
		ServiceName:        envStr("OTEL_SERVICE_NAME", "toybloom"),
		ObsExporter:        envStr("OBS_EXPORTER", "stdout"),
		ObsMaxFilterGauges: int(envInt64("OBS_MAX_FILTER_GAUGES", 100)),
	}
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt64(key string, def int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		// Don't silently hide a misconfiguration — surface it and use the default.
		slog.Default().Warn("invalid config value, using default", "key", key, "value", v, "default", def)
		return def
	}
	return n
}

func envDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v) // e.g. "5s", "200ms"
	if err != nil {
		slog.Default().Warn("invalid config value, using default", "key", key, "value", v, "default", def)
		return def
	}
	return d
}
