package experiment

import (
	"github.com/wasmbench/wasmbench/protocol"
	"testing"
)

func TestColdVectorRequestIsNotSacrificialBudget(t *testing.T) {
	o := Options{Samples: 7, Operations: 99, Warmup: 3, PhaseBarriers: true}
	w := protocol.Workload{Oracle: protocol.Oracle{Kind: "exact_vectors"}}
	for _, tc := range []struct {
		block    int
		scenario string
		samples  int
	}{{-1, "first-call", 2}, {0, "cold-process", 1}, {1, "cold-process", 1}} {
		r := trialRequest(o, w, tc.scenario, tc.block)
		if r.Samples != tc.samples || r.Operations != 1 || r.Warmup != 0 || r.PhaseBarriers || r.Scenario != "first-call" {
			t.Fatal(r)
		}
	}
	w.Oracle.Kind = "exact_u64"
	if r := trialRequest(o, w, "first-call", -1); r.Samples != 1 {
		t.Fatal(r)
	}
	w.Oracle.Kind = "exact_command"
	if r := trialRequest(o, w, "first-call", -1); r.Samples != 2 {
		t.Fatal(r)
	}
	if r := trialRequest(o, w, "cold-process", 0); r.Samples != 1 {
		t.Fatal(r)
	}
}

func TestSampleSequenceValidation(t *testing.T) {
	r := protocol.RunRequest{Scenario: "steady", Samples: 2, Warmup: 1}
	valid := []protocol.Sample{{Index: 0, Warmup: true}, {Index: 1}, {Index: 2}}
	if err := validateSampleSequence(r, valid); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]protocol.Sample{nil, valid[:2], append(append([]protocol.Sample{}, valid...), protocol.Sample{Index: 3}), {{Index: 0}, {Index: 1}, {Index: 2}}, {{Index: 0, Warmup: true}, {Index: 2}, {Index: 1}}} {
		if err := validateSampleSequence(r, bad); err == nil {
			t.Fatal("accepted invalid sequence", bad)
		}
	}
	r = protocol.RunRequest{Scenario: "first-call", Samples: 1}
	if err := validateSampleSequence(r, []protocol.Sample{{Index: 0}, {Index: 1}}); err == nil {
		t.Fatal("cold process admitted extra invocation")
	}
}
