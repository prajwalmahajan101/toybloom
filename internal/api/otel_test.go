package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prajwalmahajan101/toybloom/internal/core/config"
	"github.com/prajwalmahajan101/toybloom/internal/obs"
	"github.com/prajwalmahajan101/toybloom/internal/service"
	"github.com/prajwalmahajan101/toybloom/pkg/store"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// logRecorder is an in-memory sdklog.Exporter that captures emitted records so
// the test can assert trace correlation on logs.
type logRecorder struct {
	mu      sync.Mutex
	records []sdklog.Record
}

func (r *logRecorder) Export(_ context.Context, recs []sdklog.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, recs...)
	return nil
}
func (r *logRecorder) Shutdown(context.Context) error   { return nil }
func (r *logRecorder) ForceFlush(context.Context) error { return nil }

func (r *logRecorder) all() []sdklog.Record {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]sdklog.Record(nil), r.records...)
}

// TestOTelInstrumentation is the M8 acceptance test: one request through the real
// router must produce (1) an otelgin server span with a child FilterService span
// in the same trace, (2) the RED latency histogram tagged with http.route, and
// (3) a structured log record stamped with that request's trace_id.
func TestOTelInstrumentation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// In-memory tracer, meter, and log pipeline registered as the OTel globals
	// our production code reads (otelgin, the service/store tracers, the meter
	// behind obs.NewInstruments).
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	otel.SetTracerProvider(tp)

	reader := sdkmetric.NewManualReader()
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))

	logExp := &logRecorder{}
	lp := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(logExp)))
	lg := otelslog.NewLogger("test", otelslog.WithLoggerProvider(lp))

	inst, err := obs.NewInstruments()
	if err != nil {
		t.Fatalf("instruments: %v", err)
	}

	ms := store.NewMemStore()
	svc := service.New(ms, inst, 0)
	r := NewRouter(NewHandler(svc), NewHealthHandler(ms), config.Load(), lg, inst)

	// One create request drives the whole span tree.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/filters",
		strings.NewReader(`{"name":"otel","n":1000,"p":0.01}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d: %s", w.Code, w.Body.String())
	}

	// (1) Spans: a server span and a FilterService.Create span, same trace, with
	// the service span descending from the server span.
	var server, svcSpan sdktrace.ReadOnlySpan
	for _, s := range sr.Ended() {
		switch {
		case s.SpanKind() == trace.SpanKindServer:
			server = s
		case s.Name() == "FilterService.Create":
			svcSpan = s
		}
	}
	if server == nil {
		t.Fatal("no server span recorded (otelgin not wired?)")
	}
	if svcSpan == nil {
		t.Fatal("no FilterService.Create span recorded (service not instrumented?)")
	}
	if server.SpanContext().TraceID() != svcSpan.SpanContext().TraceID() {
		t.Error("server and service spans are in different traces")
	}
	if svcSpan.Parent().SpanID() != server.SpanContext().SpanID() {
		t.Error("service span is not a child of the server span")
	}

	// (2) RED histogram recorded with an http.route attribute.
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	if !hasRouteHistogram(rm, "http.server.request.duration") {
		t.Error("http.server.request.duration histogram with http.route attribute not found")
	}

	// (3) A log record carrying the request's trace_id.
	traceID := server.SpanContext().TraceID()
	var correlated bool
	for _, rec := range logExp.all() {
		if rec.TraceID() == traceID {
			correlated = true
			break
		}
	}
	if !correlated {
		t.Errorf("no log record stamped with trace_id %s", traceID)
	}
}

// hasRouteHistogram reports whether rm contains a histogram named metric with at
// least one data point carrying an http.route attribute.
func hasRouteHistogram(rm metricdata.ResourceMetrics, metric string) bool {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != metric {
				continue
			}
			h, ok := m.Data.(metricdata.Histogram[float64])
			if !ok {
				continue
			}
			for _, dp := range h.DataPoints {
				if _, ok := dp.Attributes.Value("http.route"); ok {
					return true
				}
			}
		}
	}
	return false
}
