package experiment

import (
	"context"
	"strings"
	"testing"

	"github.com/wasmbench/wasmbench/protocol"
)

func TestDensityCannotSilentlyRunAsOneInstance(t *testing.T) {
	w := protocol.Workload{ABI: "core", Export: "benchmark", Reset: "fresh_instance_per_sample", WorkUnit: "instance_group", Units: 1, Oracle: protocol.Oracle{Kind: "exact_u64"}, Density: &protocol.DensityContract{Instances: 16, Sharing: "shared_module"}}
	r := Runtime{ID: "unsupported-density", Description: &protocol.Description{Capabilities: map[string]bool{}}}
	for _, scenario := range []string{"first-call", "steady", "density"} {
		for _, block := range []int{-1, 0} {
			got := runTrial(context.Background(), t.TempDir(), Options{Profile: "timing"}, r, w, scenario, block, "test")
			if got.Status != "unsupported" || !strings.Contains(got.Reason, "instance-group support") {
				t.Fatalf("%s/%d: %+v", scenario, block, got)
			}
		}
	}
	r.Description.Capabilities["can_density"] = true
	r.Description.Capabilities["can_density_separate_engines"] = false
	w.Density.Sharing = "separate_engines"
	for _, block := range []int{-1, 0} {
		got := runTrial(context.Background(), t.TempDir(), Options{Profile: "timing"}, r, w, "density", block, "test")
		if got.Status != "unsupported" || !strings.Contains(got.Reason, "sharing policy") {
			t.Fatalf("unsupported sharing executed: %+v", got)
		}
	}
	w.Density.Sharing = "shared_module"
	got := runTrial(context.Background(), t.TempDir(), Options{Profile: "timing"}, r, w, "steady", 0, "test")
	if got.Status != "unsupported" || !strings.Contains(got.Reason, "density scenario") {
		t.Fatalf("wrong scenario accepted: %+v", got)
	}
}

func TestDensityCycleRequestBudgets(t *testing.T) {
	w := protocol.Workload{Density: &protocol.DensityContract{Instances: 4, Sharing: "shared_module"}}
	o := Options{Samples: 20, Operations: 9, Warmup: 5, PhaseBarriers: true}
	for _, block := range []int{-1, 0} {
		r := trialRequest(o, w, "density-cycle", block)
		want := 20
		if block < 0 {
			want = 2
		}
		if r.Scenario != "density-cycle" || r.Samples != want || r.Operations != 1 || r.Warmup != 0 || r.PhaseBarriers != (block >= 0) {
			t.Fatalf("wrong cycle budget: %+v", r)
		}
	}
	if err := validateSampleSequence(protocol.RunRequest{Scenario: "density-cycle", Samples: 1}, []protocol.Sample{{Index: 0, Operations: 9, SampleType: "batch_average"}}); err == nil {
		t.Fatal("batched cycles accepted")
	}
}
