package experiment

import (
	"context"
	"github.com/wasmbench/wasmbench/protocol"
	"testing"
)

func TestHarnessSequenceRejectsForgedCountsAndInstrumentation(t *testing.T) {
	r := protocol.RunRequest{Scenario: protocol.HarnessCalibrationScenario, Samples: 1, Operations: 5}
	s := protocol.Sample{Index: 0, Operations: 5, Verified: true, SampleType: "batch_average", Result: protocol.Values{5}}
	if err := validateSampleSequence(r, []protocol.Sample{s}); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"result", "operations", "type", "verified", "observations", "index", "warmup"} {
		bad := s
		bad.Result = append(protocol.Values(nil), s.Result...)
		switch mode {
		case "result":
			bad.Result[0] = 9
		case "operations":
			bad.Operations = 9
		case "type":
			bad.SampleType = "individual_operation"
		case "verified":
			bad.Verified = false
		case "observations":
			bad.Observations = []protocol.Observation{{Metric: "host.heap.end"}}
		case "index":
			bad.Index = 2
		case "warmup":
			bad.Warmup = true
		}
		if validateSampleSequence(r, []protocol.Sample{bad}) == nil {
			t.Fatal("forged calibration sample accepted", mode)
		}
	}
}

func TestHarnessControllerRejectsMemoryBeforeLaunching(t *testing.T) {
	r := Runtime{ID: "fixture", Command: []string{"must-not-launch"}, Description: &protocol.Description{ABIs: []string{"core"}, Scenarios: []string{protocol.HarnessCalibrationScenario}}}
	w := protocol.Workload{ABI: "core", Export: "benchmark", Reset: "stateless", Oracle: protocol.Oracle{Kind: "exact_u64"}}
	trial := runTrial(context.Background(), t.TempDir(), Options{Profile: "memory", Samples: 1, Operations: 1}, r, w, protocol.HarnessCalibrationScenario, 0, "bad-memory")
	if trial.Status != "unsupported" {
		t.Fatal(trial)
	}
}
