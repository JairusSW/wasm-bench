package corpus

import (
	_ "embed"
	"math"
	"os"
	"path/filepath"

	"github.com/wasmbench/wasmbench/protocol"
)

//go:embed testdata/floats.wasm
var floatModule []byte

func generateFloats(root string) ([]protocol.Workload, error) {
	path := filepath.Join(root, "mechanisms", "floats.wasm")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, floatModule, 0644); err != nil {
		return nil, err
	}
	var out []protocol.Workload
	for _, f := range []struct {
		name, typ, nan     string
		bits               uint64
		absolute, relative float64
	}{
		{"sum_f64", "f64", "reject", math.Float64bits(0.3), 1e-15, 1e-12},
		{"sqrt_f32", "f32", "reject", uint64(math.Float32bits(float32(math.Sqrt(2)))), 1e-6, 1e-6},
		{"negative_zero", "f64", "reject", 1 << 63, 0, 0},
		{"nan_f64", "f64", "any_nan", 0x7ff8000000000000, 0, 0},
		{"infinity", "f64", "reject", math.Float64bits(math.Inf(1)), 0, 0},
	} {
		out = append(out, protocol.Workload{Schema: 1, ID: "mechanisms/float/" + f.name, Family: "mechanisms", Artifact: path, SHA256: Hash(floatModule), ABI: "core", Features: []string{"mvp"}, Export: f.name, Args: protocol.Values{}, WorkUnit: "invocation", Units: 1, Reset: "stateless", Oracle: protocol.Oracle{Kind: "float_bits_v1", Expected: protocol.Values{f.bits}, Float: &protocol.FloatPolicy{Types: []string{f.typ}, AbsoluteTolerance: f.absolute, RelativeTolerance: f.relative, NaN: f.nan, SignedZero: "match"}}, License: "MIT", Source: "corpus/testdata/floats.wat", Generator: "wasmbench-floats-v1/WABT-1.0.41"})
	}
	for _, name := range []string{"with_args", "mixed_results"} {
		w := out[0]
		w.ID = "mechanisms/float/" + name
		w.Export = name
		p := *w.Oracle.Float
		w.Oracle.Float = &p
		if name == "with_args" {
			w.Args = protocol.Values{uint64(math.Float32bits(1.5)), math.Float64bits(2.25), 3, 4}
			w.Oracle.Expected = protocol.Values{math.Float64bits(10.75)}
		} else {
			w.Features = []string{"mvp", "multi-value"}
			p.Types = []string{"f32", "f64"}
			w.Oracle.Expected = protocol.Values{uint64(math.Float32bits(1.5)), math.Float64bits(2.25)}
		}
		out = append(out, w)
	}
	return out, nil
}
