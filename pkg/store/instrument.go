package store

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// tracer is fetched from the global provider. When no provider is registered
// (any library user who hasn't installed the OTel SDK), it is a no-op — so this
// package never forces OTel on its consumers, per ADR 0007.
var tracer = otel.Tracer("github.com/prajwalmahajan101/toybloom/pkg/store")

// startSpan opens a client span for one Valkey operation and returns a finisher
// that records the error (if any) and closes the span. Each instrumented method
// declares a named error return, opens a span, and defers the finisher with a
// pointer to that return so the deferred call observes the final error:
//
//	ctx, end := startSpan(ctx, "op")
//	defer end(&err)
func startSpan(ctx context.Context, op string) (context.Context, func(*error)) {
	ctx, span := tracer.Start(ctx, "valkey."+op,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("db.system", "valkey"),
			attribute.String("db.operation", op),
		),
	)
	return ctx, func(errp *error) {
		if errp != nil && *errp != nil {
			span.RecordError(*errp)
			span.SetStatus(codes.Error, (*errp).Error())
		}
		span.End()
	}
}
