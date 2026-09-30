package publish

import (
	"testing"

	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

func cpuObservation(metric, scenario string, value float64) protocol.Observation {
	return protocol.Observation{Metric: metric, DefinitionVersion: 1, Unit: "ns", Scope: "adapter_cgroup_process_tree", Phase: scenario + "/barrier_window", Collector: "cgroup_v2_cpu.stat", CollectorVersion: "1", Quality: "kernel_accounted_delta", Profile: "memory", Denominator: "diagnostic_operation_including_barrier_transport", Status: "available", Value: protocol.Value(value)}
}

func TestPhaseCPUUsesCompleteVerifiedLaunches(t *testing.T) {
	b := experiment.Bundle{}
	for i, value := range []float64{30, 10, 20} {
		b.Trials = append(b.Trials, experiment.Trial{ID: []string{"third", "first", "second"}[i], Runtime: "r", Workload: "w", Scenario: "compile", Profile: "memory", Status: "ok", Block: i,
			Samples: []protocol.Sample{{Verified: true, Operations: 1, SampleType: "individual_operation", Observations: []protocol.Observation{cpuObservation("time.cpu.total", "compile", value), cpuObservation("time.cpu.user", "compile", value/2), cpuObservation("time.cpu.system", "compile", value/2)}}},
		})
	}
	b.Trials = append(b.Trials,
		experiment.Trial{ID: "wrong-collector", Runtime: "r", Workload: "w", Scenario: "compile", Profile: "memory", Status: "ok", Block: 3, Samples: []protocol.Sample{{Verified: true, Operations: 1, SampleType: "individual_operation", Observations: []protocol.Observation{{Metric: "time.cpu.total", Unit: "ns", Status: "available", Value: protocol.Value(0)}}}}},
		experiment.Trial{ID: "unverified", Runtime: "r", Workload: "w", Scenario: "compile", Profile: "memory", Status: "ok", Block: 4, Samples: []protocol.Sample{{Verified: false, Operations: 1, SampleType: "individual_operation", Observations: []protocol.Observation{cpuObservation("time.cpu.total", "compile", 0)}}}},
		experiment.Trial{ID: "wrong-version", Runtime: "r", Workload: "w", Scenario: "compile", Profile: "memory", Status: "ok", Block: 5, Samples: []protocol.Sample{{Verified: true, Operations: 1, SampleType: "individual_operation", Observations: []protocol.Observation{func() protocol.Observation {
			o := cpuObservation("time.cpu.total", "compile", 0)
			o.DefinitionVersion = 2
			return o
		}()}}}},
		experiment.Trial{ID: "failed", Runtime: "r", Workload: "w", Scenario: "compile", Profile: "memory", Status: "timeout", Block: 6},
	)
	rows := phaseCPUStages(b, nil)
	if len(rows) != 3 {
		t.Fatalf("want total/user/system rows, got %+v", rows)
	}
	for _, row := range rows {
		if row.Attempted != 7 || row.Launches != 3 || row.Unavailable != 3 || row.OtherOutcomes["timeout"] != 1 || row.Low == nil || row.High == nil {
			t.Fatalf("wrong CPU coverage or uncertainty: %+v", row)
		}
		if row.Metric == "time.cpu.total" {
			if *row.Median != 20 || row.LaunchValues[0].TrialID != "third" || row.LaunchValues[0].Nanoseconds != 30 {
				t.Fatalf("CPU launch provenance lost: %+v", row)
			}
		}
	}
}

func TestPhaseCPUZeroAndMissingAreDistinct(t *testing.T) {
	b := experiment.Bundle{Trials: []experiment.Trial{
		{ID: "zero", Runtime: "r", Workload: "w", Scenario: "instantiate", Profile: "memory", Status: "ok", Block: 0, Samples: []protocol.Sample{{Verified: true, Operations: 1, SampleType: "individual_operation", Observations: []protocol.Observation{cpuObservation("time.cpu.total", "instantiate", 0)}}}},
		{ID: "missing", Runtime: "r", Workload: "w", Scenario: "instantiate", Profile: "memory", Status: "ok", Block: 1, Samples: []protocol.Sample{{Verified: true, Operations: 1, SampleType: "individual_operation"}}},
	}}
	rows := phaseCPUStages(b, map[string]bool{"r\x00w": true})
	for _, row := range rows {
		if row.Metric == "time.cpu.total" {
			if row.Median == nil || *row.Median != 0 || row.Low != nil || row.Unavailable != 1 || row.Launches != 1 {
				t.Fatalf("zero CPU time became missing: %+v", row)
			}
			return
		}
	}
	t.Fatal("missing total CPU row")
}

func TestPhaseCPUHostMismatchWithholdsDerivedValues(t *testing.T) {
	b := experiment.Bundle{Manifest: experiment.Manifest{Lock: experiment.Lock{HostPolicy: &agent.HostPolicy{}}}, Trials: []experiment.Trial{{ID: "raw", Runtime: "r", Workload: "w", Scenario: "compile", Profile: "memory", Status: "ok", Block: 0, Samples: []protocol.Sample{{Verified: true, Operations: 1, SampleType: "individual_operation", Observations: []protocol.Observation{cpuObservation("time.cpu.total", "compile", 100)}}}}}}
	rows := phaseCPUStages(b, nil)
	for _, row := range rows {
		if row.Median != nil || row.Launches != 0 || row.Attempted != 1 || row.OtherOutcomes["host_policy_mismatch"] != 1 || len(row.TrialIDs) != 1 {
			t.Fatalf("host-invalid CPU value entered summary or raw provenance was lost: %+v", row)
		}
	}
	if got := memoryStages(b, map[string]bool{"r\x00w": true}); len(got) != 0 {
		t.Fatalf("host-invalid memory value entered summary: %+v", got)
	}
}

func TestPhaseCPURejectsBatchedDiagnosticWindow(t *testing.T) {
	b := experiment.Bundle{Trials: []experiment.Trial{{ID: "batch", Runtime: "r", Workload: "w", Scenario: "compile", Profile: "memory", Status: "ok", Block: 0, Samples: []protocol.Sample{{Verified: true, Operations: 2, SampleType: "batch_average", Observations: []protocol.Observation{cpuObservation("time.cpu.total", "compile", 100)}}}}}}
	for _, row := range phaseCPUStages(b, nil) {
		if row.Median != nil || row.Unavailable != 1 {
			t.Fatalf("batched CPU work was treated as one operation: %+v", row)
		}
	}
}
