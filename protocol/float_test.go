package protocol

import (
	"math"
	"testing"
)

func floatOracle(want float64) Oracle {
	return Oracle{Kind: "float_bits_v1", Expected: Values{math.Float64bits(want)}, Float: &FloatPolicy{Types: []string{"f64"}, AbsoluteTolerance: 1e-12, RelativeTolerance: 1e-9, NaN: "reject", SignedZero: "match"}}
}

func TestFloatOracle(t *testing.T) {
	for _, tc := range []struct {
		name      string
		got, want float64
		accept    bool
	}{
		{"exact", 1, 1, true}, {"near", 1 + 1e-10, 1, true}, {"far", 1.01, 1, false},
		{"absolute", 1e-13, 0, true}, {"relative", 1e100 + 5e90, 1e100, true},
		{"nan_actual", math.NaN(), 1, false}, {"inf_actual", math.Inf(1), 1, false},
		{"inf_exact", math.Inf(1), math.Inf(1), true}, {"inf_sign", math.Inf(-1), math.Inf(1), false},
		{"zero_sign", math.Copysign(0, -1), 0, false},
		{"overflow", math.MaxFloat64, -math.MaxFloat64, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := floatOracle(tc.want)
			err := o.VerifyFloat([]uint64{math.Float64bits(tc.got)}, []string{"f64"})
			if (err == nil) != tc.accept {
				t.Fatal(err)
			}
		})
	}
	o := floatOracle(math.NaN())
	o.Float.NaN = "any_nan"
	if err := o.VerifyFloat([]uint64{0x7ff8000000000042}, []string{"f64"}); err != nil {
		t.Fatal(err)
	}
	if err := o.VerifyFloat([]uint64{math.Float64bits(1)}, []string{"f64"}); err == nil {
		t.Fatal("finite accepted as NaN")
	}
	o = floatOracle(0)
	o.Float.SignedZero = "ignore"
	if err := o.VerifyFloat([]uint64{1 << 63}, []string{"f64"}); err != nil {
		t.Fatal(err)
	}
}

func TestFloatOracleValidation(t *testing.T) {
	for _, mode := range []string{"missing", "kind", "type", "negative", "nan_tolerance", "inf_tolerance", "nan_policy", "zero_policy", "empty", "expected_nan"} {
		t.Run(mode, func(t *testing.T) {
			o := floatOracle(1)
			switch mode {
			case "missing":
				o.Float = nil
			case "kind":
				o.Kind = "exact_u64"
			case "type":
				o.Float.Types[0] = "i64"
			case "negative":
				o.Float.AbsoluteTolerance = -1
			case "nan_tolerance":
				o.Float.RelativeTolerance = math.NaN()
			case "inf_tolerance":
				o.Float.AbsoluteTolerance = math.Inf(1)
			case "nan_policy":
				o.Float.NaN = ""
			case "zero_policy":
				o.Float.SignedZero = ""
			case "empty":
				o.Expected = nil
			case "expected_nan":
				o.Expected[0] = math.Float64bits(math.NaN())
			}
			if o.ValidateFloat() == nil {
				t.Fatal("invalid contract accepted")
			}
		})
	}
	o := floatOracle(1)
	if o.VerifyFloat(o.Expected, []string{"i64"}) == nil {
		t.Fatal("wrong signature accepted")
	}
	if o.VerifyFloat(nil, []string{"f64"}) == nil {
		t.Fatal("missing value accepted")
	}
	o.Float.Types = []string{"f32"}
	o.Expected = Values{uint64(math.Float32bits(1))}
	if err := o.VerifyFloat(o.Expected, []string{"f32"}); err != nil {
		t.Fatal(err)
	}
	if o.VerifyFloat([]uint64{1<<32 | o.Expected[0]}, []string{"f32"}) == nil {
		t.Fatal("noncanonical result accepted")
	}
}
