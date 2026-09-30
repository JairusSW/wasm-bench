package protocol

import (
	"fmt"
	"math"
	"slices"
)

// FloatPolicy describes numeric comparison of IEEE result bits, not a change to
// the lossless uint64 wire encoding. NaN payload equality is intentionally absent.
type FloatPolicy struct {
	Types             []string `json:"types"`
	AbsoluteTolerance float64  `json:"absolute_tolerance"`
	RelativeTolerance float64  `json:"relative_tolerance"`
	NaN               string   `json:"nan"`
	SignedZero        string   `json:"signed_zero"`
}

func (o Oracle) ValidateFloat() error {
	f := o.Float
	if o.Kind != "float_bits_v1" || f == nil || len(o.Expected) == 0 || len(f.Types) != len(o.Expected) || o.ExpectedTrap != "" {
		return fmt.Errorf("invalid float_bits_v1 oracle")
	}
	if (f.NaN != "reject" && f.NaN != "any_nan") || (f.SignedZero != "match" && f.SignedZero != "ignore") {
		return fmt.Errorf("explicit float NaN and signed-zero policies required")
	}
	for _, v := range []float64{f.AbsoluteTolerance, f.RelativeTolerance} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return fmt.Errorf("float tolerances must be finite and nonnegative")
		}
	}
	for i, typ := range f.Types {
		v, err := floatValue(typ, o.Expected[i])
		if err != nil {
			return err
		}
		if math.IsNaN(v) && f.NaN == "reject" {
			return fmt.Errorf("expected NaN requires any_nan policy")
		}
	}
	return nil
}

func floatValue(typ string, bits uint64) (float64, error) {
	switch typ {
	case "f64":
		return math.Float64frombits(bits), nil
	case "f32":
		if bits > math.MaxUint32 {
			return 0, fmt.Errorf("noncanonical f32 result bits")
		}
		return float64(math.Float32frombits(uint32(bits))), nil
	default:
		return 0, fmt.Errorf("unsupported floating result type %q", typ)
	}
}

func (o Oracle) VerifyFloat(got []uint64, actualTypes []string) error {
	if err := o.ValidateFloat(); err != nil {
		return err
	}
	if !slices.Equal(actualTypes, o.Float.Types) || len(got) != len(o.Expected) {
		return fmt.Errorf("float result signature/count mismatch")
	}
	for i, bits := range got {
		value, err := floatValue(actualTypes[i], bits)
		if err != nil {
			return err
		}
		want, _ := floatValue(actualTypes[i], o.Expected[i])
		if !floatMatches(value, want, o.Float) {
			return fmt.Errorf("incorrect floating result %d: got bits %d want bits %d", i, bits, o.Expected[i])
		}
	}
	return nil
}

func floatMatches(got, want float64, p *FloatPolicy) bool {
	if math.IsNaN(want) {
		return p.NaN == "any_nan" && math.IsNaN(got)
	}
	if math.IsNaN(got) {
		return false
	}
	if math.IsInf(got, 0) || math.IsInf(want, 0) {
		return got == want
	}
	if got == 0 && want == 0 {
		return p.SignedZero == "ignore" || math.Signbit(got) == math.Signbit(want)
	}
	if got == want {
		return true
	}
	delta := math.Abs(got - want)
	if delta <= p.AbsoluteTolerance {
		return true
	}
	scale := math.Max(math.Abs(got), math.Abs(want))
	relative := delta / scale
	// Opposite-sign large finite values can overflow subtraction. Normalizing
	// first preserves their relative distance without accepting an infinite bound.
	if math.IsInf(delta, 0) {
		relative = math.Abs(got/scale - want/scale)
	}
	return relative <= p.RelativeTolerance
}

func ValidateFloatWorkload(w Workload) error {
	if err := w.Oracle.ValidateFloat(); err != nil {
		return err
	}
	if w.ABI != "core" || w.Command != nil || w.Vectors != nil {
		return fmt.Errorf("float oracle requires a core scalar-result workload")
	}
	return nil
}

func ValidateFloatRun(p *Preparation, r *RunRequest) error {
	if p == nil || r == nil {
		return fmt.Errorf("missing float preparation")
	}
	if err := ValidateFloatWorkload(p.Workload); err != nil {
		return err
	}
	if r.Scenario == "trajectory" {
		return ValidateTrajectory(p, r)
	}
	if (r.PhaseBarriers && (p.Profile != "memory" || !slices.Contains([]string{"compile", "instantiate", "teardown"}, r.Scenario))) || !slices.Contains([]string{"timing", "memory"}, p.Profile) || !slices.Contains([]string{"compile", "instantiate", "first-call", "steady", "teardown"}, r.Scenario) {
		return fmt.Errorf("unsupported float oracle scenario/profile/barriers")
	}
	return nil
}
