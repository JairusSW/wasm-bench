package protocol

import "testing"

func TestFloatPhaseContracts(t *testing.T) {
	p := Preparation{Profile: "memory", Workload: Workload{ABI: "core", Reset: "stateless", Export: "run", Oracle: floatOracle(7)}}
	for _, scenario := range []string{"compile", "instantiate", "teardown"} {
		r := RunRequest{Scenario: scenario, Samples: 2, Operations: 1, PhaseBarriers: true}
		if err := ValidateFloatRun(&p, &r); err != nil {
			t.Fatal(err)
		}
		p.Profile = "timing"
		if ValidateFloatRun(&p, &r) == nil {
			t.Fatal("timing barriers accepted")
		}
		p.Profile = "memory"
	}
	for _, scenario := range []string{"first-call", "steady", "trajectory", "app-init"} {
		if ValidateFloatRun(&p, &RunRequest{Scenario: scenario, Samples: 1, Operations: 1, PhaseBarriers: true}) == nil {
			t.Fatal("unsupported barriers accepted", scenario)
		}
	}
	if err := ValidateInstantiatePhases(p.Workload); err != nil {
		t.Fatal(err)
	}
	p.Workload.Oracle.Float = nil
	if ValidateInstantiatePhases(p.Workload) == nil {
		t.Fatal("malformed float accepted")
	}
}
