package experiment

import (
	"context"
	"github.com/wasmbench/wasmbench/protocol"
	"testing"
)

func TestAppInitWithoutInitializerIsNotApplicable(t *testing.T) {
	r := Runtime{ID: "fixture", Description: &protocol.Description{ABIs: []string{"core"}, Scenarios: []string{"app-init"}}}
	w := protocol.Workload{ABI: "core", Reset: "stateless", Oracle: protocol.Oracle{Kind: "exact_u64"}}
	trial := runTrial(context.Background(), t.TempDir(), Options{Profile: "timing"}, r, w, "app-init", 0, "no-init")
	if trial.Status != "not_applicable" || len(trial.Samples) != 0 {
		t.Fatal(trial)
	}
}

func TestAppInitSampleBoundary(t *testing.T) {
	r := protocol.RunRequest{Scenario: "app-init", Samples: 1, Operations: 99, Warmup: 5}
	s := protocol.Sample{Index: 0, Operations: 1, SampleType: "individual_operation"}
	if err := validateSampleSequence(r, []protocol.Sample{s}); err != nil {
		t.Fatal(err)
	}
	s.Operations = 99
	if validateSampleSequence(r, []protocol.Sample{s}) == nil {
		t.Fatal("accepted batched initialization")
	}
	s.Operations = 1
	s.Warmup = true
	if validateSampleSequence(r, []protocol.Sample{s}) == nil {
		t.Fatal("accepted warmup initialization")
	}
}
