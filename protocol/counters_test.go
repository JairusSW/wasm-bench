package protocol

import "testing"

func TestCounterRunContract(t *testing.T) {
	valid := func() (*Preparation, *RunRequest) {
		return &Preparation{Profile: "counters", Workload: Workload{ABI: "core", Export: "run", Reset: "stateless", Oracle: Oracle{Kind: "exact_u64"}}}, &RunRequest{Scenario: "compile", Samples: 2, Operations: 1, PhaseBarriers: true}
	}
	p, r := valid()
	if err := ValidateCounterRun(p, r); err != nil {
		t.Fatal(err)
	}
	r.Scenario = "instantiate"
	if err := ValidateCounterRun(p, r); err != nil {
		t.Fatal(err)
	}
	r.Scenario = "first-call"
	if err := ValidateCounterRun(p, r); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Preparation, *RunRequest){
		"profile":    func(p *Preparation, r *RunRequest) { p.Profile = "memory" },
		"abi":        func(p *Preparation, r *RunRequest) { p.Workload.ABI = "wasi-command" },
		"oracle":     func(p *Preparation, r *RunRequest) { p.Workload.Oracle.Kind = "float_bits_v1" },
		"reset":      func(p *Preparation, r *RunRequest) { p.Workload.Reset = "reused" },
		"export":     func(p *Preparation, r *RunRequest) { p.Workload.Export = "" },
		"scenario":   func(p *Preparation, r *RunRequest) { r.Scenario = "teardown" },
		"barriers":   func(p *Preparation, r *RunRequest) { r.PhaseBarriers = false },
		"warmup":     func(p *Preparation, r *RunRequest) { r.Warmup = 1 },
		"operations": func(p *Preparation, r *RunRequest) { r.Operations = 2 },
		"empty":      func(p *Preparation, r *RunRequest) { r.Samples = 0 },
		"oversize":   func(p *Preparation, r *RunRequest) { r.Samples = 100001 },
	} {
		t.Run(name, func(t *testing.T) {
			p, r := valid()
			mutate(p, r)
			if err := ValidateCounterRun(p, r); err == nil {
				t.Fatal("accepted")
			}
		})
	}
	if ValidateCounterRun(nil, r) == nil || ValidateCounterRun(p, nil) == nil {
		t.Fatal("nil accepted")
	}
}

func TestSteadyCounterBudget(t *testing.T) {
	p := &Preparation{Profile: "counters", Workload: Workload{ABI: "core", Export: "run", Reset: "stateless", Oracle: Oracle{Kind: "exact_u64"}}}
	r := &RunRequest{Scenario: "steady", Samples: 2, Operations: 3, Warmup: 4, PhaseBarriers: true}
	if err := ValidateCounterRun(p, r); err != nil {
		t.Fatal(err)
	}
	if n, err := CounterSampleCount(r); err != nil || n != 6 {
		t.Fatal(n, err)
	}
	for _, mutate := range []func(*RunRequest){
		func(r *RunRequest) { r.Warmup = -1 }, func(r *RunRequest) { r.Warmup = 100001 },
		func(r *RunRequest) { r.Operations = 0 }, func(r *RunRequest) { r.Operations = 1000001 },
	} {
		bad := *r
		mutate(&bad)
		if ValidateCounterRun(p, &bad) == nil {
			t.Fatal(bad)
		}
	}
	p.Workload.Reset = "fresh_instance_per_sample"
	if ValidateCounterRun(p, r) == nil {
		t.Fatal("state reset silently ignored")
	}
}
