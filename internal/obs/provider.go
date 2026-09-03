// Package obs owns all OpenTelemetry setup: it builds the tracer/meter/logger
// providers, wires them to exporters, registers the OTel globals other packages
// read, and exposes a single Shutdown that flushes everything on exit.
//
// Per ADR 0005 the app emits one OTLP stream (traces + metrics + logs) to a
// Collector. For local verification without the full stack (M9), the exporter
// mode can be switched to "stdout" so telemetry prints as JSON, or "none" to
// disable the SDK entirely (plain logs, no-op tracer/meter).
package obs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	otellog "go.opentelemetry.io/otel/log"
	logglobal "go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
)

// Exporter modes.
const (
	ExporterOTLP   = "otlp"   // OTLP/gRPC to a Collector (production, ADR 0005)
	ExporterStdout = "stdout" // print telemetry as JSON (local verification, M8)
	ExporterNone   = "none"   // SDK disabled: plain logs, no-op tracer/meter
)

// Config is the narrow slice of runtime configuration obs needs. main builds it
// from core/config so this package stays decoupled from the full app Config.
// Endpoint, sampler, and resource attributes come from the standard OTEL_* env
// vars, which the SDK reads on its own — obs only picks the exporter mode.
type Config struct {
	ServiceName    string
	ServiceVersion string
	Exporter       string // one of the Exporter* constants
}

// Providers bundles the three SDK providers and a slog.Logger bridged onto the
// log provider. Built once at startup by Setup, torn down once by Shutdown.
type Providers struct {
	tracer *sdktrace.TracerProvider
	meter  *sdkmetric.MeterProvider
	logger *sdklog.LoggerProvider

	// Log rides the same OTLP stream as traces/metrics and is auto-stamped with
	// the active trace_id/span_id whenever it is used with a context. The app
	// uses it exactly like the old core logger; only the handler changed.
	Log *slog.Logger
}

// Setup constructs the OTel SDK from cfg and registers the globals that otelgin,
// otelslog, and pkg/store (via otel.Tracer) rely on. The returned Providers.
// Shutdown MUST be called before the process exits or batched telemetry is lost.
//
// When cfg.Exporter is "none", no SDK is installed: the OTel globals stay their
// default no-ops and Log is a plain stdout JSON logger.
func Setup(ctx context.Context, cfg Config) (*Providers, error) {
	if cfg.Exporter == ExporterNone {
		return &Providers{Log: slog.Default()}, nil
	}

	res, err := newResource(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("obs: build resource: %w", err)
	}

	traceExp, metricExp, logExp, err := newExporters(ctx, cfg.Exporter)
	if err != nil {
		return nil, fmt.Errorf("obs: build exporters: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(traceExp),
	)
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp)),
	)
	lp := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExp)),
	)

	// Register globals so downstream code needs no explicit provider wiring.
	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	logglobal.SetLoggerProvider(lp)
	// W3C trace-context + baggage so trace ids propagate across service hops.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &Providers{
		tracer: tp,
		meter:  mp,
		logger: lp,
		Log:    newLogger(cfg.ServiceName, lp),
	}, nil
}

// Shutdown flushes and closes every provider. It is safe to call when a provider
// is nil (ExporterNone). Call it AFTER the HTTP server has drained, so in-flight
// requests' spans and metrics are recorded before the exporters close.
func (p *Providers) Shutdown(ctx context.Context) error {
	if p == nil {
		return nil
	}
	var errs []error
	if p.tracer != nil {
		errs = append(errs, p.tracer.Shutdown(ctx))
	}
	if p.meter != nil {
		errs = append(errs, p.meter.Shutdown(ctx))
	}
	if p.logger != nil {
		errs = append(errs, p.logger.Shutdown(ctx))
	}
	return errors.Join(errs...)
}

// newResource describes this service on every emitted span/metric/log. Explicit
// service.name/version are set first; WithFromEnv lets OTEL_RESOURCE_ATTRIBUTES
// and OTEL_SERVICE_NAME override them, matching standard OTel autoconfig.
func newResource(ctx context.Context, cfg Config) (*resource.Resource, error) {
	return resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
		),
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
	)
}

// newExporters builds one exporter per signal for the chosen mode. OTLP targets
// (endpoint, TLS, headers) come from the standard OTEL_EXPORTER_OTLP_* env vars.
func newExporters(ctx context.Context, mode string) (
	sdktrace.SpanExporter, sdkmetric.Exporter, sdklog.Exporter, error,
) {
	switch mode {
	case ExporterOTLP:
		te, err := otlptracegrpc.New(ctx)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("otlp trace exporter: %w", err)
		}
		me, err := otlpmetricgrpc.New(ctx)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("otlp metric exporter: %w", err)
		}
		le, err := otlploggrpc.New(ctx)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("otlp log exporter: %w", err)
		}
		return te, me, le, nil

	case ExporterStdout:
		te, err := stdouttrace.New()
		if err != nil {
			return nil, nil, nil, fmt.Errorf("stdout trace exporter: %w", err)
		}
		me, err := stdoutmetric.New()
		if err != nil {
			return nil, nil, nil, fmt.Errorf("stdout metric exporter: %w", err)
		}
		le, err := stdoutlog.New()
		if err != nil {
			return nil, nil, nil, fmt.Errorf("stdout log exporter: %w", err)
		}
		return te, me, le, nil

	default:
		return nil, nil, nil, fmt.Errorf("unknown exporter mode %q (want %s|%s|%s)",
			mode, ExporterOTLP, ExporterStdout, ExporterNone)
	}
}

// ensure the sdk log provider satisfies the interface newLogger expects.
var _ otellog.LoggerProvider = (*sdklog.LoggerProvider)(nil)
