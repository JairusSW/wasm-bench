package experiment

import (
	"github.com/wasmbench/wasmbench/protocol"
	"testing"
)

func TestScenarioSampleOverridesAreRecordedPerTimingScenario(t *testing.T) {
	o := Options{Profile: "timing", Samples: 1, ScenarioSamples: map[string]int{"*": 1, "compile": 3, "instantiate": 3, "steady": 3}}
	for scenario, want := range map[string]int{"compile": 3, "instantiate": 3, "first-call": 1, "steady": 3, "teardown": 1} {
		if got := trialRequest(o, protocol.Workload{}, scenario, 0).Samples; got != want {
			t.Errorf("%s samples = %d, want %d", scenario, got, want)
		}
	}
	o.Profile = "memory"
	if got := trialRequest(o, protocol.Workload{}, "compile", 0).Samples; got != 1 {
		t.Errorf("memory samples = %d, want 1", got)
	}
}

func TestWarmupsAreOnlyRequestedForScenariosThatRetainThem(t *testing.T) {
	o := Options{Samples: 3, Operations: 1, Warmup: 3}
	for scenario, want := range map[string]int{"compile": 0, "instantiate": 0, "first-call": 0, "steady": 3, "trajectory": 3, "sustained": 3} {
		if got := trialRequest(o, protocol.Workload{}, scenario, 0).Warmup; got != want {
			t.Errorf("%s warmup = %d, want %d", scenario, got, want)
		}
	}
}
