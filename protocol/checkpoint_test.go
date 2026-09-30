package protocol_test

import (
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
	"testing"
)

func TestCheckpointValidationAndEvidence(t *testing.T) {
	ws, err := corpus.Generate(t.TempDir(), "checkpoints")
	if err != nil {
		t.Fatal(err)
	}
	w := ws[0]
	for _, scenario := range protocol.CheckpointScenarios() {
		r := protocol.RunRequest{Scenario: scenario, Samples: 2, Operations: 1}
		p := protocol.Preparation{Workload: w, Profile: "timing"}
		if err := protocol.ValidateCheckpoint(&p, &r); err != nil {
			t.Fatal(err)
		}
		for _, bad := range []protocol.RunRequest{{Scenario: "first-call", Samples: 1, Operations: 1}, {Scenario: scenario, Samples: 1, Operations: 2}, {Scenario: scenario, Samples: 1, Operations: 1, Warmup: 1}, {Scenario: scenario, Samples: 10001, Operations: 1}, {Scenario: scenario, Samples: 1, Operations: 1, PhaseBarriers: true}} {
			if protocol.ValidateCheckpoint(&p, &bad) == nil {
				t.Fatal("accepted invalid boundary/budget")
			}
		}
		written := scenario == "checkpoint-first-write"
		state := uint32(42)
		result := w.Oracle.Expected[0]
		if written {
			state = 43
			result += 5
		}
		s := protocol.Sample{Operations: 1, SampleType: "individual_operation", Verified: true, Result: protocol.Values{result}, CheckpointResult: &protocol.CheckpointResult{Mode: protocol.GuestCheckpointMode, PayloadBytes: 65540, SavedMemorySHA256: protocol.CheckpointMemorySHA256(1, false), RestoredMemorySHA256: protocol.CheckpointMemorySHA256(1, written), SavedGlobal: 42, RestoredGlobal: state, SourceIndependent: true}}
		if err := protocol.VerifyCheckpointSample(w, scenario, s); err != nil {
			t.Fatal(err)
		}
		for _, mutate := range []func(*protocol.CheckpointResult){func(e *protocol.CheckpointResult) { e.PayloadBytes++ }, func(e *protocol.CheckpointResult) { e.SourceIndependent = false }, func(e *protocol.CheckpointResult) { e.RestoredGlobal++ }, func(e *protocol.CheckpointResult) { e.SavedMemorySHA256 = "forged" }, func(e *protocol.CheckpointResult) { e.RestoredMemorySHA256 = "forged" }} {
			copy := *s.CheckpointResult
			mutate(&copy)
			bad := s
			bad.CheckpointResult = &copy
			if protocol.VerifyCheckpointSample(w, scenario, bad) == nil {
				t.Fatal("trusted forged state evidence")
			}
		}
	}
	for _, mutate := range []func(*protocol.Workload){func(w *protocol.Workload) { w.Args = protocol.Values{1} }, func(w *protocol.Workload) { w.Initialize = "" }, func(w *protocol.Workload) { w.Reset = "stateless" }, func(w *protocol.Workload) { w.Oracle.Expected = protocol.Values{0} }, func(w *protocol.Workload) {
		w.Density = &protocol.DensityContract{Instances: 1, Sharing: "shared_module"}
	}, func(w *protocol.Workload) { w.HostProfile = "identity-v1" }} {
		bad := w
		mutate(&bad)
		if protocol.ValidateCheckpointWorkload(bad) == nil {
			t.Fatal("invalid state contract accepted")
		}
	}
	for _, pages := range []uint32{0, 65} {
		if _, err := corpus.CheckpointModule(pages); err == nil {
			t.Fatal("invalid module size")
		}
	}
}
