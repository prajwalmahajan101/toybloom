package bloom

import (
	"math"
	"testing"
)

const fppTol = 1e-6

func TestFillRatio(t *testing.T) {
	tests := []struct {
		name   string
		stages []Stage
		want   float64
	}{
		{"empty filter", nil, 0},
		{"zero capacity guard", []Stage{{Capacity: 0, Fill: 0}}, 0},
		{"half full single stage", []Stage{{Capacity: 100, Fill: 50}}, 0.5},
		{"full single stage", []Stage{{Capacity: 100, Fill: 100}}, 1.0},
		{"two stages aggregate", []Stage{{Capacity: 100, Fill: 100}, {Capacity: 200, Fill: 50}}, 150.0 / 300.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Filter{stages: tt.stages}
			if got := f.FillRatio(); math.Abs(got-tt.want) > fppTol {
				t.Fatalf("FillRatio() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEstimatedFPP(t *testing.T) {
	// fpp for one stage with k=1, m=2, fill=2: 1 - e^(-1) = 0.63212055...
	single := 1 - math.Exp(-1)

	tests := []struct {
		name   string
		stages []Stage
		want   float64
	}{
		{"empty filter", nil, 0},
		{"unfilled stage is zero", []Stage{{M: 100, K: 7, Fill: 0}}, 0},
		{"degenerate stage skipped", []Stage{{M: 0, K: 0, Fill: 5}}, 0},
		{"single stage", []Stage{{M: 2, K: 1, Fill: 2}}, single},
		// Two identical stages compound: 1 - (1 - fpp)^2.
		{"two stages compound", []Stage{{M: 2, K: 1, Fill: 2}, {M: 2, K: 1, Fill: 2}}, 1 - (1-single)*(1-single)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Filter{stages: tt.stages}
			got := f.EstimatedFPP()
			if math.Abs(got-tt.want) > fppTol {
				t.Fatalf("EstimatedFPP() = %v, want %v", got, tt.want)
			}
			if got < 0 || got >= 1 {
				t.Fatalf("EstimatedFPP() = %v, out of [0,1) range", got)
			}
		})
	}
}
