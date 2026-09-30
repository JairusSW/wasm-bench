package agent

import (
	"github.com/wasmbench/wasmbench/protocol"
	"strings"
	"testing"
)

func TestTeardownUnavailableAccountingLabels(t *testing.T) {
	for _, stage := range []string{"before_teardown", "torn_down"} {
		out := unavailablePhaseCollection(stage, "fixture")
		peak, cpu := 0, 0
		for _, o := range out {
			if !strings.HasPrefix(o.Phase, "teardown/") || o.Status != "unavailable" || o.Value != nil {
				t.Fatal(o)
			}
			if o.Metric == "cgroup.memory.phase_peak" {
				peak++
				if o.Phase != "teardown/barrier_window" {
					t.Fatal(o)
				}
			}
			if strings.HasPrefix(o.Metric, "time.cpu.") {
				cpu++
			}
		}
		if stage == "before_teardown" && (peak != 0 || cpu != 0) {
			t.Fatal(out)
		}
		if stage == "torn_down" && (peak != 1 || cpu != 3) {
			t.Fatal(out)
		}
	}
}

func TestAppInitAccountingWindows(t *testing.T) {
	for _, scenarioName := range append(protocol.CheckpointScenarios(), "app-init", "instantiate", "guest-density") {
		stages := protocol.PhaseStages(scenarioName)
		for _, stage := range stages {
			scenario, start, end := phaseWindow(stage)
			if scenario != scenarioName || start != (stage == stages[0]) || end != (stage == stages[1]) {
				t.Fatal(stage, scenario, start, end)
			}
			peak, cpu := 0, 0
			for _, o := range unavailablePhaseCollection(stage, "fixture") {
				if !strings.HasPrefix(o.Phase, scenarioName+"/") || o.Status != "unavailable" || o.Value != nil {
					t.Fatal(o)
				}
				if o.Metric == "cgroup.memory.phase_peak" {
					peak++
				}
				if strings.HasPrefix(o.Metric, "time.cpu.") {
					cpu++
				}
			}
			if end && (peak != 1 || cpu != 3) {
				t.Fatal(stage, peak, cpu)
			}
			if !end && (peak != 0 || cpu != 0) {
				t.Fatal(stage, peak, cpu)
			}
		}
	}
}

func TestRustAllocatorBoundaryAccountingWindows(t *testing.T) {
	for _, scenario := range []string{"compile", "instantiate", "first-call", "steady"} {
		for index, stage := range protocol.RustAllocatorPhaseStages(scenario) {
			got, start, end := phaseWindow(stage)
			if got != scenario || start != (index == 0) || end != (index == 1) {
				t.Fatal("release boundary changed API peak window", scenario, stage, got, start, end)
			}
			for _, o := range unavailablePhaseCollection(stage, "fixture") {
				if !strings.HasPrefix(o.Phase, scenario+"/") || o.Value != nil {
					t.Fatal("boundary domain changed", stage, o)
				}
				if index >= 2 && (o.Metric == "cgroup.memory.phase_peak" || strings.HasPrefix(o.Metric, "time.cpu.")) {
					t.Fatal("release snapshot became API peak or CPU window", stage, o)
				}
			}
		}
	}
}
