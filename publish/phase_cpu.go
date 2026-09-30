package publish

import (
	"math"
	"sort"

	"github.com/wasmbench/wasmbench/analysis"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

const PhaseCPUVersion = "cgroup-phase-cpu-launch-median-v1"

var phaseCPUMetrics = []string{"time.cpu.total", "time.cpu.user", "time.cpu.system"}

// PhaseCPUStage summarizes diagnostic cgroup CPU work from a memory pass. It is
// not compile API CPU time: barrier transport and all cgroup tasks are included.
type PhaseCPUStage struct {
	Runtime       string           `json:"runtime"`
	Workload      string           `json:"workload"`
	Scenario      string           `json:"scenario"`
	Metric        string           `json:"metric"`
	Median        *float64         `json:"median_ns"`
	Low           *float64         `json:"ci95_low_ns"`
	High          *float64         `json:"ci95_high_ns"`
	Launches      int              `json:"independent_launches"`
	Attempted     int              `json:"attempted_launches"`
	Unavailable   int              `json:"unavailable_launches"`
	OtherOutcomes map[string]int   `json:"other_outcomes"`
	TrialIDs      []string         `json:"trial_ids"`
	LaunchValues  []PhaseCPULaunch `json:"launch_values"`
}

type PhaseCPULaunch struct {
	TrialID     string  `json:"trial_id"`
	Nanoseconds float64 `json:"nanoseconds"`
}

func phaseCPUValue(observations []protocol.Observation, metric, scenario string) (float64, bool) {
	var value float64
	count := 0
	for _, o := range observations {
		if o.Metric != metric || o.DefinitionVersion != 1 || o.Unit != "ns" || o.Scope != "adapter_cgroup_process_tree" || o.Phase != scenario+"/barrier_window" || o.Collector != "cgroup_v2_cpu.stat" || o.CollectorVersion != "1" || o.Quality != "kernel_accounted_delta" || o.Profile != "memory" || o.Denominator != "diagnostic_operation_including_barrier_transport" || o.Status != "available" || o.Value == nil || math.IsNaN(*o.Value) || math.IsInf(*o.Value, 0) || *o.Value < 0 {
			continue
		}
		value = *o.Value
		count++
	}
	return value, count == 1
}

func phaseCPUStages(b experiment.Bundle, matched map[string]bool) []PhaseCPUStage {
	type key struct{ runtime, workload, scenario, metric string }
	rows := map[key]*PhaseCPUStage{}
	hostEligible := experiment.HostBaselineAllowsMeasurements(b.Manifest)
	for _, trial := range b.Trials {
		if trial.Block < 0 || trial.Profile != "memory" || (matched != nil && !matched[trial.Runtime+"\x00"+trial.Workload]) {
			continue
		}
		for _, metric := range phaseCPUMetrics {
			k := key{trial.Runtime, trial.Workload, trial.Scenario, metric}
			row := rows[k]
			if row == nil {
				row = &PhaseCPUStage{Runtime: k.runtime, Workload: k.workload, Scenario: k.scenario, Metric: k.metric, OtherOutcomes: map[string]int{}, TrialIDs: []string{}, LaunchValues: []PhaseCPULaunch{}}
				rows[k] = row
			}
			row.Attempted++
			row.TrialIDs = append(row.TrialIDs, trial.ID)
			if !hostEligible {
				row.OtherOutcomes["host_policy_mismatch"]++
				continue
			}
			if trial.Status != "ok" {
				row.OtherOutcomes[trial.Status]++
				continue
			}
			var values []float64
			expected := 0
			for _, sample := range trial.Samples {
				if sample.Warmup {
					continue
				}
				expected++
				if value, ok := phaseCPUValue(sample.Observations, metric, trial.Scenario); ok && sample.Verified && sample.Operations == 1 && sample.SampleType == "individual_operation" {
					values = append(values, value)
				}
			}
			if expected == 0 || len(values) != expected {
				row.Unavailable++
				continue
			}
			row.LaunchValues = append(row.LaunchValues, PhaseCPULaunch{TrialID: trial.ID, Nanoseconds: *medianStage(values)})
		}
	}
	out := make([]PhaseCPUStage, 0, len(rows))
	for _, row := range rows {
		values := make([]float64, 0, len(row.LaunchValues))
		for _, launch := range row.LaunchValues {
			values = append(values, launch.Nanoseconds)
		}
		row.Launches = len(values)
		row.Median = medianStage(values)
		if low, high, ok := analysis.BootstrapMedian95(values); ok {
			row.Low, row.High = &low, &high
		}
		out = append(out, *row)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Workload != b.Workload {
			return a.Workload < b.Workload
		}
		if a.Scenario != b.Scenario {
			return a.Scenario < b.Scenario
		}
		if a.Runtime != b.Runtime {
			return a.Runtime < b.Runtime
		}
		return a.Metric < b.Metric
	})
	return out
}
