package obs

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// meterName scopes the instruments to this module in the emitted metric stream.
const meterName = "github.com/prajwalmahajan101/toybloom"

// Instruments holds the app's metric instruments. Created once from the global
// MeterProvider and passed to the middleware/service that record on them.
//
// RED latency and the aggregate counters carry no filter name. The two
// observable gauges below DO carry a bounded `filter` label (M9 / ADR 0008):
// filter names are user-supplied, so the service caps how many series it emits.
type Instruments struct {
	reqDuration metric.Float64Histogram
	filtersOps  metric.Int64Counter
	itemsAdded  metric.Int64Counter

	// Async gauges observed from a registered callback (see ObserveFilters), not
	// recorded inline. Values come from the live-filter view each collection.
	fillRatio   metric.Float64ObservableGauge
	estFPP      metric.Float64ObservableGauge
	items       metric.Int64ObservableGauge
	filterCount metric.Int64ObservableGauge
}

// NewInstruments builds the instruments from the global MeterProvider. Call
// after obs.Setup so the global meter is the real SDK meter (otherwise the
// instruments are no-ops, which is also safe).
func NewInstruments() (*Instruments, error) {
	m := otel.Meter(meterName)

	reqDur, err := m.Float64Histogram(
		"http.server.request.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Duration of inbound HTTP requests in seconds."),
	)
	if err != nil {
		return nil, err
	}

	ops, err := m.Int64Counter(
		"toybloom.filter.operations",
		metric.WithDescription("Count of filter operations by kind and outcome."),
	)
	if err != nil {
		return nil, err
	}

	items, err := m.Int64Counter(
		"toybloom.filter.items_added",
		metric.WithDescription("Total values added across all filters."),
	)
	if err != nil {
		return nil, err
	}

	fillRatio, err := m.Float64ObservableGauge(
		"toybloom.filter.fill_ratio",
		metric.WithDescription("Per-filter saturation: items added / total capacity across stages."),
	)
	if err != nil {
		return nil, err
	}

	estFPP, err := m.Float64ObservableGauge(
		"toybloom.filter.estimated_fpp",
		metric.WithDescription("Per-filter estimated false-positive probability at current load."),
	)
	if err != nil {
		return nil, err
	}

	itemsGauge, err := m.Int64ObservableGauge(
		"toybloom.filter.items",
		metric.WithDescription("Per-filter current item count (sum of stage fills)."),
	)
	if err != nil {
		return nil, err
	}

	filterCount, err := m.Int64ObservableGauge(
		"toybloom.filter.count",
		metric.WithDescription("Number of live filters registered in the store."),
	)
	if err != nil {
		return nil, err
	}

	return &Instruments{
		reqDuration: reqDur,
		filtersOps:  ops,
		itemsAdded:  items,
		fillRatio:   fillRatio,
		estFPP:      estFPP,
		items:       itemsGauge,
		filterCount: filterCount,
	}, nil
}

// FilterSample is one filter's observed gauge values, supplied by the caller's
// collect function at metric-collection time.
type FilterSample struct {
	Name         string
	FillRatio    float64
	EstimatedFPP float64
	Items        int64
}

// FilterObservation is the full per-collection view the caller hands back:
// the true count of live filters (before any cardinality cap) plus the per-filter
// samples (which may be capped).
type FilterObservation struct {
	Count   int
	Samples []FilterSample
}

// ObserveFilters registers an async callback that, on every metric collection,
// asks collect for the current filter observation and records it on the
// count/items/fill_ratio/estimated_fpp gauges. collect is owned by the caller
// (the service layer, which holds the store) so this package stays decoupled
// from the domain — obs never imports bloom/store. The `filter` attribute's
// cardinality is bounded by the caller's cap; the count gauge reports the true
// (pre-cap) number of filters. Registers once, at startup.
func (i *Instruments) ObserveFilters(collect func(context.Context) FilterObservation) error {
	if i == nil || i.fillRatio == nil || i.estFPP == nil || i.items == nil || i.filterCount == nil {
		return nil // exporter "none"/no-op mode: nothing to register
	}
	_, err := otel.Meter(meterName).RegisterCallback(
		func(ctx context.Context, o metric.Observer) error {
			obsv := collect(ctx)
			o.ObserveInt64(i.filterCount, int64(obsv.Count))
			for _, s := range obsv.Samples {
				attrs := metric.WithAttributes(attribute.String("filter", s.Name))
				o.ObserveFloat64(i.fillRatio, s.FillRatio, attrs)
				o.ObserveFloat64(i.estFPP, s.EstimatedFPP, attrs)
				o.ObserveInt64(i.items, s.Items, attrs)
			}
			return nil
		},
		i.fillRatio, i.estFPP, i.items, i.filterCount,
	)
	return err
}

// RecordRequest records one HTTP request's RED datapoint. Passing the request
// ctx lets the SDK attach a trace exemplar automatically when the span is
// sampled, linking the latency bucket back to its trace in Grafana.
func (i *Instruments) RecordRequest(ctx context.Context, route, method string, status int, seconds float64) {
	if i == nil {
		return
	}
	i.reqDuration.Record(ctx, seconds, metric.WithAttributes(
		attribute.String("http.route", route),
		attribute.String("http.request.method", method),
		attribute.Int("http.response.status_code", status),
	))
}

// RecordOp records one filter operation (create/add/exists/stats/delete) tagged
// with whether it succeeded. Low, bounded cardinality — safe as metric labels.
func (i *Instruments) RecordOp(ctx context.Context, op string, ok bool) {
	if i == nil {
		return
	}
	i.filtersOps.Add(ctx, 1, metric.WithAttributes(
		attribute.String("op", op),
		attribute.Bool("ok", ok),
	))
}

// RecordItemsAdded records values added to a filter (aggregate, no per-name
// label — see the Instruments doc comment).
func (i *Instruments) RecordItemsAdded(ctx context.Context, n int64) {
	if i == nil {
		return
	}
	i.itemsAdded.Add(ctx, n)
}
