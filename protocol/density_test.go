package protocol

import (
	"encoding/json"
	"testing"
)

func densityPreparation() *Preparation {
	return &Preparation{Profile: "timing", Workload: Workload{ABI: "core", Export: "benchmark", Reset: "fresh_instance_per_sample", WorkUnit: "instance_group", Units: 1, Oracle: Oracle{Kind: "exact_u64"}, Density: &DensityContract{Instances: 4, Sharing: "shared_module"}}}
}

func TestDensityContract(t *testing.T) {
	r := &RunRequest{Scenario: "density", Samples: 3, Operations: 1}
	for _, mode := range []string{"shared_module", "separate_engines"} {
		p := densityPreparation()
		p.Workload.Density.Sharing = mode
		if err := ValidateDensity(p, r); err != nil {
			t.Fatal(err)
		}
		b, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		var round Preparation
		if err := json.Unmarshal(b, &round); err != nil {
			t.Fatal(err)
		}
		if *round.Workload.Density != *p.Workload.Density {
			t.Fatal("density identity lost on wire")
		}
		p.Profile = "memory"
		phased := *r
		phased.PhaseBarriers = true
		if err := ValidateDensity(p, &phased); err != nil {
			t.Fatal(err)
		}
	}
	for name, mutate := range map[string]func(*Preparation, *RunRequest){
		"nil contract":     func(p *Preparation, r *RunRequest) { p.Workload.Density = nil },
		"zero instances":   func(p *Preparation, r *RunRequest) { p.Workload.Density.Instances = 0 },
		"excess instances": func(p *Preparation, r *RunRequest) { p.Workload.Density.Instances = 129 },
		"pooled":           func(p *Preparation, r *RunRequest) { p.Workload.Density.Sharing = "pooled" },
		"reuse":            func(p *Preparation, r *RunRequest) { p.Workload.Reset = "stateless" },
		"command":          func(p *Preparation, r *RunRequest) { p.Workload.Command = &CommandContract{} },
		"host":             func(p *Preparation, r *RunRequest) { p.Workload.HostProfile = "custom" },
		"normalization":    func(p *Preparation, r *RunRequest) { p.Workload.Units = 4 },
		"wrong scenario":   func(p *Preparation, r *RunRequest) { r.Scenario = "steady" },
		"warmup":           func(p *Preparation, r *RunRequest) { r.Warmup = 1 },
		"batch":            func(p *Preparation, r *RunRequest) { r.Operations = 2 },
		"empty":            func(p *Preparation, r *RunRequest) { r.Samples = 0 },
		"profile":          func(p *Preparation, r *RunRequest) { p.Profile = "counters" },
		"timing barriers":  func(p *Preparation, r *RunRequest) { r.PhaseBarriers = true },
	} {
		t.Run(name, func(t *testing.T) {
			p := densityPreparation()
			rr := *r
			mutate(p, &rr)
			if ValidateDensity(p, &rr) == nil {
				t.Fatal("accepted invalid contract")
			}
		})
	}
	if ValidateDensity(nil, r) == nil || ValidateDensity(densityPreparation(), nil) == nil {
		t.Fatal("accepted nil")
	}
}

func TestDensityCycleIsSeparateContract(t *testing.T) {
	p := densityPreparation()
	r := &RunRequest{Scenario: "density-cycle", Samples: 4, Operations: 1}
	if ValidateDensityCycle(p, r) != nil {
		t.Fatal("valid cycle rejected")
	}
	if ValidateDensity(p, r) == nil {
		t.Fatal("ordinary density silently accepted cycles")
	}
	r.Warmup = 1
	if ValidateDensityCycle(p, r) == nil {
		t.Fatal("hidden warmup accepted")
	}
	if ValidateDensityCycle(p, nil) == nil {
		t.Fatal("nil accepted")
	}
}
