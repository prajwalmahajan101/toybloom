package service

import (
	"context"
	"errors"
	"log/slog"

	"github.com/prajwalmahajan101/toybloom/internal/obs"
	"github.com/prajwalmahajan101/toybloom/pkg/bloom"
	"github.com/prajwalmahajan101/toybloom/pkg/store"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// tracer for service-level spans; no-op until obs.Setup installs a provider.
var tracer = otel.Tracer("github.com/prajwalmahajan101/toybloom/internal/service")

var (
	ErrInvalidArgument = errors.New("service: invalid argument")
	ErrNotFound        = errors.New("service: filter not found")
	ErrAlreadyExists   = errors.New("service: filter already exists")
)

// FilterInfo is the result of creating a filter.
type FilterInfo struct {
	Name   string `json:"name"`
	Stages int    `json:"stages"`
	M      uint64 `json:"m"`
	K      int    `json:"k"`
}

// StageInfo is a read-only view of one stage.
type StageInfo struct {
	Index    int    `json:"index"`
	M        uint64 `json:"m"`
	K        int    `json:"k"`
	Capacity uint64 `json:"capacity"`
	Fill     uint64 `json:"fill"`
}

// FilterStats is the full stats view of a filter.
type FilterStats struct {
	Name       string      `json:"name"`
	N          uint64      `json:"n"`
	P          float64     `json:"p"`
	R          float64     `json:"r"`
	S          float64     `json:"s"`
	StageCount int         `json:"stage_count"`
	Stages     []StageInfo `json:"stages"`
}

// FilterService coordinates filter operations, hiding the bloom/store layers
// behind transport-agnostic DTOs and service-level error sentinels.
type FilterService interface {
	Create(ctx context.Context, name string, n uint64, p float64) (FilterInfo, error)
	Add(ctx context.Context, name string, value []byte) error
	Exists(ctx context.Context, name string, value []byte) (bool, error)
	Stats(ctx context.Context, name string) (FilterStats, error)
	Delete(ctx context.Context, name string) error
	// FilterSamples reports the current filter observation (live count + per-filter
	// gauge samples) for the observability layer's async callback. The per-filter
	// samples are bounded by the cardinality cap; the count is the true total.
	FilterSamples(ctx context.Context) obs.FilterObservation
}

type filterService struct {
	store store.BitStore
	inst  *obs.Instruments
	// maxGauges caps how many per-filter gauge series FilterSamples emits, keeping
	// the unbounded `filter` label from exploding metric cardinality. <= 0 = no cap.
	maxGauges int
}

func New(s store.BitStore, inst *obs.Instruments, maxGauges int) FilterService {
	return &filterService{store: s, inst: inst, maxGauges: maxGauges}
}

// finishSpan closes a service span (recording an error status) and increments
// the operation counter tagged with success/failure. One helper keeps every
// method's telemetry identical.
func (s *filterService) finishSpan(ctx context.Context, span trace.Span, op string, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
	s.inst.RecordOp(ctx, op, err == nil)
}

func (s *filterService) Create(ctx context.Context, name string, n uint64, p float64) (_ FilterInfo, err error) {
	ctx, span := tracer.Start(ctx, "FilterService.Create")
	defer func() { s.finishSpan(ctx, span, "create", err) }()

	f, err := bloom.New(ctx, s.store, name, n, p)
	if err != nil {
		return FilterInfo{}, mapErr(err)
	}
	st := f.Stats()
	return FilterInfo{Name: st.Name, Stages: st.StageCount, M: st.Stages[0].M, K: st.Stages[0].K}, nil
}

func (s *filterService) Add(ctx context.Context, name string, value []byte) (err error) {
	ctx, span := tracer.Start(ctx, "FilterService.Add")
	defer func() { s.finishSpan(ctx, span, "add", err) }()

	f, err := bloom.Load(ctx, s.store, name)
	if err != nil {
		return mapErr(err)
	}
	if err = f.Add(ctx, value); err != nil {
		return mapErr(err)
	}
	s.inst.RecordItemsAdded(ctx, 1)
	return nil
}

func (s *filterService) Exists(ctx context.Context, name string, value []byte) (_ bool, err error) {
	ctx, span := tracer.Start(ctx, "FilterService.Exists")
	defer func() { s.finishSpan(ctx, span, "exists", err) }()

	f, err := bloom.Load(ctx, s.store, name)
	if err != nil {
		return false, mapErr(err)
	}
	ok, err := f.Exists(ctx, value)
	if err != nil {
		return false, mapErr(err)
	}
	return ok, nil
}

func (s *filterService) Stats(ctx context.Context, name string) (_ FilterStats, err error) {
	ctx, span := tracer.Start(ctx, "FilterService.Stats")
	defer func() { s.finishSpan(ctx, span, "stats", err) }()

	f, err := bloom.Load(ctx, s.store, name)
	if err != nil {
		return FilterStats{}, mapErr(err)
	}
	return toStats(f.Stats()), nil
}

func (s *filterService) Delete(ctx context.Context, name string) (err error) {
	ctx, span := tracer.Start(ctx, "FilterService.Delete")
	defer func() { s.finishSpan(ctx, span, "delete", err) }()

	f, err := bloom.Load(ctx, s.store, name)
	if err != nil {
		return mapErr(err)
	}
	if err = f.Drop(ctx); err != nil {
		return mapErr(err)
	}
	return nil
}

// FilterSamples enumerates live filters and computes each one's fill ratio and
// estimated FPP for the fill_ratio / estimated_fpp observable gauges. It runs on
// the metric-collection goroutine, so it stays cheap and never fails the whole
// collection: a filter that vanished mid-scrape (Load miss) is skipped, not
// fatal. The cardinality cap bounds how many filter series we emit.
func (s *filterService) FilterSamples(ctx context.Context) obs.FilterObservation {
	names, err := bloom.ListFilters(ctx, s.store)
	if err != nil {
		slog.Default().WarnContext(ctx, "filter gauge collection: list filters failed", "err", err)
		return obs.FilterObservation{}
	}
	total := len(names) // true live count, reported before any cap truncation
	if s.maxGauges > 0 && len(names) > s.maxGauges {
		slog.Default().WarnContext(ctx, "filter gauge cardinality cap hit; truncating",
			"live", len(names), "cap", s.maxGauges)
		names = names[:s.maxGauges]
	}

	samples := make([]obs.FilterSample, 0, len(names))
	for _, name := range names {
		f, err := bloom.Load(ctx, s.store, name)
		if err != nil {
			slog.Default().DebugContext(ctx, "filter gauge collection: skipping filter",
				"filter", name, "err", err)
			continue
		}
		samples = append(samples, obs.FilterSample{
			Name:         name,
			FillRatio:    f.FillRatio(),
			EstimatedFPP: f.EstimatedFPP(),
			Items:        int64(f.Items()),
		})
	}
	return obs.FilterObservation{Count: total, Samples: samples}
}

func toStats(st bloom.Stats) FilterStats {
	stages := make([]StageInfo, len(st.Stages))
	for i, s := range st.Stages {
		stages[i] = StageInfo{Index: s.Index, M: s.M, K: s.K, Capacity: s.Capacity, Fill: s.Fill}
	}
	return FilterStats{
		Name:       st.Name,
		N:          st.N,
		P:          st.P,
		R:          st.R,
		S:          st.S,
		StageCount: st.StageCount,
		Stages:     stages,
	}
}

// mapErr translates bloom sentinels into service sentinels; anything else
// passes through as-is (treated as internal by the transport layer).
func mapErr(err error) error {
	switch {
	case errors.Is(err, bloom.ErrFilterExists):
		return ErrAlreadyExists
	case errors.Is(err, bloom.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, bloom.ErrEmptyName),
		errors.Is(err, bloom.ErrInvalidN),
		errors.Is(err, bloom.ErrInvalidP):
		return ErrInvalidArgument
	default:
		return err
	}
}
