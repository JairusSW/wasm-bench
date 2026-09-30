package protocol

import "testing"

func TestReactorContract(t *testing.T) {
	valid := Workload{ABI: "wasi-reactor", HostProfile: WASIReactorProfile, Initialize: "_initialize", Export: "run", Reset: "stateless", Oracle: Oracle{Kind: "exact_u64", Expected: Values{7}}}
	if err := ValidateReactor(valid); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Workload){func(w *Workload) { w.Initialize = "" }, func(w *Workload) { w.HostProfile = "" }, func(w *Workload) { w.ABI = "core" }, func(w *Workload) { w.Command = &CommandContract{} }, func(w *Workload) { w.Export = "_start" }, func(w *Workload) { w.Export = "_initialize" }, func(w *Workload) { w.Reset = "reused_mutable" }, func(w *Workload) { w.Oracle.Kind = "float_bits_v1" }} {
		w := valid
		mutate(&w)
		if ValidateReactor(w) == nil {
			t.Fatal("ambiguous reactor accepted", w)
		}
	}
	p := Preparation{Workload: valid, Profile: "timing"}
	r := RunRequest{Scenario: "steady", Samples: 2, Operations: 3, Warmup: 1}
	if err := ValidateReactorRun(&p, &r); err != nil {
		t.Fatal(err)
	}
	for _, profile := range []string{"profiling", "code", "counters"} {
		p.Profile = profile
		if ValidateReactorRun(&p, &r) == nil {
			t.Fatal(profile)
		}
	}
	p.Profile = "memory"
	r.PhaseBarriers = true
	if ValidateReactorRun(&p, &r) == nil {
		t.Fatal("unqualified barriers accepted")
	}
	r.PhaseBarriers = false
	r.Samples = 100001
	if ValidateReactorRun(&p, &r) == nil {
		t.Fatal("unbounded samples accepted")
	}
}
