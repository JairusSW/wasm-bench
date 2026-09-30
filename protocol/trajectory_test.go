package protocol

import (
	"math"
	"testing"
)

func TestFloatTrajectoryContract(t *testing.T) {
	p := Preparation{Profile: "timing", Workload: Workload{ABI: "core", Reset: "stateless", Export: "run", Oracle: Oracle{Kind: "float_bits_v1", Expected: Values{math.Float64bits(7)}, Float: &FloatPolicy{Types: []string{"f64"}, NaN: "reject", SignedZero: "match"}}}}
	r := RunRequest{Scenario: "trajectory", Samples: 3, Warmup: 2, Operations: 99}
	for _, validate := range []func(*Preparation, *RunRequest) error{ValidateTrajectory, ValidateFloatRun} {
		if err := validate(&p, &r); err != nil {
			t.Fatal(err)
		}
		if validate(nil, &r) == nil || validate(&p, nil) == nil {
			t.Fatal("nil contract accepted")
		}
		for _, change := range []func(*Preparation, *RunRequest){
			func(p *Preparation, r *RunRequest) { p.Profile = "memory" },
			func(p *Preparation, r *RunRequest) { p.Workload.Reset = "fresh_instance_per_sample" },
			func(p *Preparation, r *RunRequest) { r.PhaseBarriers = true },
			func(p *Preparation, r *RunRequest) { p.Workload.Oracle.Float = nil },
			func(p *Preparation, r *RunRequest) { p.Workload.Oracle.Expected = nil },
		} {
			pc, rc := p, r
			change(&pc, &rc)
			if validate(&pc, &rc) == nil {
				t.Fatal("invalid float trajectory accepted", pc, rc)
			}
		}
	}
}

func TestTrajectoryContract(t *testing.T) {
	p := Preparation{Profile: "timing", Workload: Workload{ABI: "core", Reset: "stateless", Export: "run", Oracle: Oracle{Kind: "exact_u64"}}}
	r := RunRequest{Scenario: "trajectory", Samples: 1, Warmup: 2, Operations: 99}
	if err := ValidateTrajectory(&p, &r); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*Preparation, *RunRequest){
		func(p *Preparation, r *RunRequest) { p.Profile = "memory" }, func(p *Preparation, r *RunRequest) { p.Workload.Reset = "fresh_instance_per_sample" }, func(p *Preparation, r *RunRequest) { p.Workload.Oracle.Kind = "expected_trap" }, func(p *Preparation, r *RunRequest) { p.Workload.Vectors = &VectorContract{} }, func(p *Preparation, r *RunRequest) { r.PhaseBarriers = true }, func(p *Preparation, r *RunRequest) { r.Samples = 0 }, func(p *Preparation, r *RunRequest) { r.Warmup = -1 }, func(p *Preparation, r *RunRequest) { r.Operations = 0 },
	} {
		pc, rc := p, r
		change(&pc, &rc)
		if ValidateTrajectory(&pc, &rc) == nil {
			t.Fatal(pc, rc)
		}
	}
}
