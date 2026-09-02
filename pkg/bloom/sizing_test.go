package bloom

import (
	"errors"
	"math"
	"testing"
)

const eps = 1e-12

var (
	nan    = math.NaN()
	posInf = math.Inf(1)
	negInf = math.Inf(-1)
	maxU64 = uint64(math.MaxUint64)
	tinyP  = 1e-300 // small enough to force m past uint64 range
)

// ---------- OptimalM ----------

func TestOptimalM_Valid(t *testing.T) {
	cases := []struct {
		name string
		n    uint64
		p    float64
		want uint64
	}{
		{"1M @ 0.01", 1_000_000, 0.01, 9_585_059},
		{"1M @ 0.001", 1_000_000, 0.001, 14_377_588},
		{"1M @ 0.0001", 1_000_000, 0.0001, 19_170_117},
		{"n=1 p=0.5", 1, 0.5, 2},
		{"n=1 p=0.99", 1, 0.99, 1},
		{"n=10 p=0.5", 10, 0.5, 15},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := OptimalM(c.n, c.p)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Errorf("OptimalM(%d, %g) = %d, want %d", c.n, c.p, got, c.want)
			}
		})
	}
}

func TestOptimalM_Errors(t *testing.T) {
	cases := []struct {
		name string
		n    uint64
		p    float64
		want error
	}{
		{"n=0", 0, 0.01, ErrInvalidN},
		{"p=0", 100, 0, ErrInvalidP},
		{"p=1", 100, 1, ErrInvalidP},
		{"p<0", 100, -0.5, ErrInvalidP},
		{"p>1", 100, 1.5, ErrInvalidP},
		{"p=NaN", 100, nan, ErrInvalidP},
		{"p=+Inf", 100, posInf, ErrInvalidP},
		{"p=-Inf", 100, negInf, ErrInvalidP},
		{"overflow", maxU64, tinyP, ErrSizeOverflow},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := OptimalM(c.n, c.p)
			if !errors.Is(err, c.want) {
				t.Fatalf("OptimalM(%d, %g) err = %v, want %v", c.n, c.p, err, c.want)
			}
			if got != 0 {
				t.Errorf("on error, value = %d, want 0", got)
			}
		})
	}
}

// ---------- OptimalK ----------

func TestOptimalK_Valid(t *testing.T) {
	cases := []struct {
		name string
		n, m uint64
		want int
	}{
		{"1M / 9.585M", 1_000_000, 9_585_059, 7},
		{"1M / 14.377M", 1_000_000, 14_377_588, 10},
		{"1M / 19.170M", 1_000_000, 19_170_117, 13},
		{"m=0 floors to 1", 1_000_000, 0, 1},
		{"m<n floors to 1", 1_000, 100, 1},
		{"n=1 m=20", 1, 20, 14},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := OptimalK(c.n, c.m)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Errorf("OptimalK(%d, %d) = %d, want %d", c.n, c.m, got, c.want)
			}
		})
	}
}

func TestOptimalK_Errors(t *testing.T) {
	got, err := OptimalK(0, 100)
	if !errors.Is(err, ErrInvalidN) {
		t.Fatalf("OptimalK(0, 100) err = %v, want ErrInvalidN", err)
	}
	if got != 0 {
		t.Errorf("on error, value = %d, want 0", got)
	}
}

// ---------- StageError ----------

func TestStageError_Valid(t *testing.T) {
	cases := []struct {
		name string
		p    float64
		i    int
		r    []float64
		want float64
	}{
		{"default i=0", 0.01, 0, nil, 0.001},
		{"default i=1", 0.01, 1, nil, 0.0009},
		{"default i=2", 0.01, 2, nil, 0.00081},
		{"custom r=0.5 i=1", 0.01, 1, []float64{0.5}, 0.0025},
		{"explicit DefaultR i=2", 0.01, 2, []float64{DefaultR}, 0.01 * (1 - DefaultR) * DefaultR * DefaultR},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := StageError(c.p, c.i, c.r...)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if math.Abs(got-c.want) > eps {
				t.Errorf("StageError(%g, %d, %v) = %g, want %g", c.p, c.i, c.r, got, c.want)
			}
		})
	}
	// Explicit default must equal implicit default.
	exp, _ := StageError(0.01, 3, DefaultR)
	imp, _ := StageError(0.01, 3)
	if exp != imp {
		t.Errorf("explicit DefaultR %g != implicit %g", exp, imp)
	}
}

func TestStageError_Errors(t *testing.T) {
	cases := []struct {
		name string
		p    float64
		i    int
		r    []float64
		want error
	}{
		{"p=0", 0, 0, nil, ErrInvalidP},
		{"p=1", 1, 0, nil, ErrInvalidP},
		{"p<0", -0.5, 0, nil, ErrInvalidP},
		{"p=NaN", nan, 0, nil, ErrInvalidP},
		{"p=+Inf", posInf, 0, nil, ErrInvalidP},
		{"stage<0", 0.01, -1, nil, ErrInvalidStage},
		{"r=0", 0.01, 0, []float64{0}, ErrInvalidRatio},
		{"r=1", 0.01, 0, []float64{1}, ErrInvalidRatio},
		{"r<0", 0.01, 0, []float64{-0.1}, ErrInvalidRatio},
		{"r>1", 0.01, 0, []float64{1.5}, ErrInvalidRatio},
		{"r=NaN", 0.01, 0, []float64{nan}, ErrInvalidRatio},
		{"too many r", 0.01, 0, []float64{0.9, 0.8}, ErrTooManyArgs},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := StageError(c.p, c.i, c.r...)
			if !errors.Is(err, c.want) {
				t.Fatalf("StageError(%g, %d, %v) err = %v, want %v", c.p, c.i, c.r, err, c.want)
			}
			if got != 0 {
				t.Errorf("on error, value = %g, want 0", got)
			}
		})
	}
}

// ---------- StageCapacity ----------

func TestStageCapacity_Valid(t *testing.T) {
	cases := []struct {
		name string
		n    uint64
		i    int
		s    []float64
		want uint64
	}{
		{"default i=0", 1_000_000, 0, nil, 1_000_000},
		{"default i=1", 1_000_000, 1, nil, 2_000_000},
		{"default i=2", 1_000_000, 2, nil, 4_000_000},
		{"custom s=3 i=2", 1_000_000, 2, []float64{3.0}, 9_000_000},
		{"i=0 ignores s", 1_000_000, 0, []float64{5.0}, 1_000_000},
		{"explicit DefaultS i=3", 1_000_000, 3, []float64{DefaultS}, 8_000_000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := StageCapacity(c.n, c.i, c.s...)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Errorf("StageCapacity(%d, %d, %v) = %d, want %d", c.n, c.i, c.s, got, c.want)
			}
		})
	}
	// Explicit default must equal implicit default.
	exp, _ := StageCapacity(1_000_000, 4, DefaultS)
	imp, _ := StageCapacity(1_000_000, 4)
	if exp != imp {
		t.Errorf("explicit DefaultS %d != implicit %d", exp, imp)
	}
}

func TestStageCapacity_Errors(t *testing.T) {
	cases := []struct {
		name string
		n    uint64
		i    int
		s    []float64
		want error
	}{
		{"n=0", 0, 0, nil, ErrInvalidN},
		{"stage<0", 1_000_000, -1, nil, ErrInvalidStage},
		{"s=1", 1_000_000, 0, []float64{1}, ErrInvalidRatio},
		{"s<1", 1_000_000, 0, []float64{0.5}, ErrInvalidRatio},
		{"s=0", 1_000_000, 0, []float64{0}, ErrInvalidRatio},
		{"s=NaN", 1_000_000, 0, []float64{nan}, ErrInvalidRatio},
		{"too many s", 1_000_000, 0, []float64{2.0, 3.0}, ErrTooManyArgs},
		{"overflow default s", maxU64, 1, nil, ErrSizeOverflow},
		{"overflow custom s", maxU64, 1, []float64{2.0}, ErrSizeOverflow},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := StageCapacity(c.n, c.i, c.s...)
			if !errors.Is(err, c.want) {
				t.Fatalf("StageCapacity(%d, %d, %v) err = %v, want %v", c.n, c.i, c.s, err, c.want)
			}
			if got != 0 {
				t.Errorf("on error, value = %d, want 0", got)
			}
		})
	}
}
