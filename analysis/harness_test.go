package analysis

import (
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
	"testing"
)

func TestHarnessNeverScoresWorkloadPerformance(t *testing.T) {
	b := breakEvenFixture()
	for i := range b.Trials {
		b.Trials[i].Scenario = protocol.HarnessCalibrationScenario
	}
	b.Manifest.Lock.Options.Scenarios = []string{protocol.HarnessCalibrationScenario}
	for _, s := range Summarize(b) {
		if s.LatencyStatus != "calibration_only" || s.Median != nil || s.Launches != 0 || s.RecordedSamples == 0 {
			t.Fatal("harness acquired workload latency", s)
		}
	}
	for _, s := range Throughput(b) {
		if s.Status == "timing_pass" {
			t.Fatal("harness acquired useful-work throughput", s)
		}
	}
	if _, err := NewAggregateSet(b, "calibration", protocol.HarnessCalibrationScenario); err == nil {
		t.Fatal("harness acquired a performance aggregate")
	}
	if _, err := BreakEven(b, nil, "a", "b"); err == nil {
		t.Fatal("harness acquired a setup/execution model")
	}
	for _, scenarios := range [][]string{{protocol.HarnessCalibrationScenario}, {"compile", protocol.HarnessCalibrationScenario}} {
		b.Manifest.Lock.Options.Scenarios = scenarios
		audit := AuditPublication(b, PilotEvidence{})
		found := false
		for _, requirement := range audit.Requirements {
			if requirement.ID == "workload_performance_scope" {
				found = true
				if requirement.Status == "passed" {
					t.Fatal("calibration admitted to official workload publication", scenarios)
				}
			}
		}
		if !found || audit.Err() == nil {
			t.Fatal("missing calibration publication gate", audit)
		}
	}
	if HeadlineLatencyPolicy(experiment.Manifest{Kind: "measurement", Lock: experiment.Lock{Options: experiment.Options{Profile: "timing"}}}).ForScenario(protocol.HarnessCalibrationScenario).Status != "calibration_only" {
		t.Fatal("missing calibration eligibility rule")
	}
}
