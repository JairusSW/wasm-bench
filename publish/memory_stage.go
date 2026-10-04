package publish

import (
	"fmt"
	"math"
	"reflect"
	"sort"

	"github.com/wasmbench/wasmbench/analysis"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

// MemoryStage is a median of independent launch measurements. Its
// measurement domain is explicit; unlike timing segments these values are never
// added into a lifecycle total.
type MemoryStage struct {
	SourceRun     string         `json:"source_run,omitempty"`
	SourceProfile string         `json:"source_profile,omitempty"`
	Runtime       string         `json:"runtime"`
	Workload      string         `json:"workload"`
	Scenario      string         `json:"scenario"`
	Metric        string         `json:"metric"`
	Median        *float64       `json:"median_bytes"`
	Low           *float64       `json:"ci95_low_bytes"`
	High          *float64       `json:"ci95_high_bytes"`
	Launches      int            `json:"independent_launches"`
	Trials        []string       `json:"trial_ids"`
	LaunchValues  []MemoryLaunch `json:"launch_values"`
}

type MemoryLaunch struct {
	TrialID string  `json:"trial_id"`
	Bytes   float64 `json:"bytes"`
}

type MemorySource struct {
	TimingID string `json:"timing_id,omitempty"`
	ID       string `json:"id"`
	Profile  string `json:"profile"`
	Note     string `json:"note"`
}

var memoryStageMetrics = []string{"host.alloc.bytes", "host.rust.alloc.bytes", "host.rust.outstanding.end", "host.rust.outstanding.observed_peak", "cgroup.memory.phase_peak", "process.peak_rss", "process.rss", "host.heap.end", "host.js_heap.end", "checkpoint.payload_bytes"}

func matchingMemoryCells(timing, memory experiment.Bundle) (map[string]bool, error) {
	return matchingPassCells(timing, memory, "memory")
}

func matchingPassCells(timing, other experiment.Bundle, profile string) (map[string]bool, error) {
	if timing.Manifest.Kind != "measurement" || timing.Manifest.Lock.Options.Profile != "timing" {
		return nil, fmt.Errorf("paired report requires a timing-profile measurement bundle")
	}
	if other.Manifest.Kind != "measurement" || other.Manifest.Lock.Options.Profile != profile {
		return nil, fmt.Errorf("--%s-run must be a %s-profile measurement bundle", profile, profile)
	}
	if !reflect.DeepEqual(timing.Manifest.Host, other.Manifest.Host) {
		return nil, fmt.Errorf("--%s-run host identity or observed policy differs from timing run", profile)
	}
	if timing.Manifest.Lock.Protocol != other.Manifest.Lock.Protocol {
		return nil, fmt.Errorf("--%s-run protocol, resource budget, or host policy differs from timing run", profile)
	}
	if err := experiment.MatchHostMeasurementPolicy(timing.Manifest.Lock, other.Manifest.Lock); err != nil {
		return nil, fmt.Errorf("--%s-run host measurement policy differs from timing run: %w", profile, err)
	}
	if timing.Manifest.Lock.Options.SustainedDuration != other.Manifest.Lock.Options.SustainedDuration {
		return nil, fmt.Errorf("--%s-run sustained duration targets differ", profile)
	}
	if timing.Manifest.Lock.Options.SustainedDuration != 0 {
		a, b := timing.Manifest.Lock.Options, other.Manifest.Lock.Options
		if a.Samples != b.Samples || a.Operations != b.Operations || a.Warmup != b.Warmup {
			return nil, fmt.Errorf("--%s-run sustained sample, operation, or warmup budgets differ", profile)
		}
	}
	runtimes := map[string]experiment.Runtime{}
	for _, r := range timing.Manifest.Lock.Runtimes {
		runtimes[r.ID] = r
	}
	workloads := map[string]protocol.Workload{}
	for _, w := range timing.Manifest.Lock.Workloads {
		workloads[w.ID] = w
	}
	matched := map[string]bool{}
	for _, r := range other.Manifest.Lock.Runtimes {
		base, ok := runtimes[r.ID]
		if !ok || !reflect.DeepEqual(base.Files, r.Files) || !reflect.DeepEqual(base.Description, r.Description) || !reflect.DeepEqual(base.HostFiles, r.HostFiles) || !reflect.DeepEqual(base.ELF, r.ELF) || base.NativeDependencyPolicy != r.NativeDependencyPolicy {
			continue
		}
		for _, w := range other.Manifest.Lock.Workloads {
			baseWorkload, ok := workloads[w.ID]
			// Artifact paths may differ after exact-byte restoration; the
			// workload contract, including input and oracle, must not.
			baseWorkload.Artifact, w.Artifact = "", ""
			if ok && reflect.DeepEqual(baseWorkload, w) {
				matched[r.ID+"\x00"+w.ID] = true
			}
		}
	}
	if len(matched) == 0 {
		return nil, fmt.Errorf("--%s-run has no matching runtime binary and workload artifact pairs", profile)
	}
	return matched, nil
}

func medianStage(values []float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	ordered := append([]float64(nil), values...)
	sort.Float64s(ordered)
	n := len(ordered)
	v := ordered[n/2]
	if n%2 == 0 {
		v = (ordered[n/2-1] + v) / 2
	}
	return &v
}

func memoryObservationValue(o protocol.Observation, metric, scenario string, operations int) (float64, bool) {
	if o.Metric != metric || o.Unit != "bytes" || o.Status != "available" || o.Value == nil || math.IsNaN(*o.Value) || math.IsInf(*o.Value, 0) || *o.Value < 0 {
		return 0, false
	}
	switch metric {
	case "host.rust.alloc.bytes", "host.rust.outstanding.end", "host.rust.outstanding.observed_peak":
		if o.DefinitionVersion != 1 || o.Phase != scenario || o.Quality != "instrumented" || o.Profile != "memory" || o.Scope != "allocations_routed_through_rust_global_allocator" || o.Collector != "wasmbench Rust GlobalAlloc/System" || o.CollectorVersion != "rust-global-alloc-v1" || o.Denominator != "single_embedding_api_operation" || operations != 1 {
			return 0, false
		}
	case "host.alloc.bytes", "host.heap.end", "host.js_heap.end":
		checkpointWindow := protocol.IsCheckpointScenario(scenario) && metric != "host.js_heap.end" && o.Phase == scenario+"/operation_window" && o.Collector == "runtime.ReadMemStats" && o.Scope == "adapter_process_go_heap" && o.Denominator == "single_guest_checkpoint_operation_excluding_setup_verification_release"
		continuationWindow := protocol.IsContinuationScenario(scenario) && metric != "host.js_heap.end" && o.Phase == scenario+"/operation_window" && o.Collector == "runtime.ReadMemStats" && o.Scope == "adapter_process_go_heap" && o.Denominator == protocol.ContinuationDenominator && operations == 1
		guestDensityWindow := scenario == "guest-density" && metric != "host.js_heap.end" && o.Phase == protocol.GuestDensityPhase && o.Collector == "runtime.ReadMemStats" && o.Scope == "adapter_process_go_heap" && o.Denominator == protocol.GuestDensityDenominator
		if (o.Phase != scenario && !checkpointWindow && !guestDensityWindow && !continuationWindow) || o.Quality != "engine_reported" {
			return 0, false
		}
	case "checkpoint.payload_bytes":
		if !protocol.IsCheckpointScenario(scenario) || o.Phase != protocol.PhaseStages(scenario)[1] || o.DefinitionVersion != 1 || o.Scope != "guest_state_checkpoint" || o.Quality != "exact" || o.Collector != "wasmbench.eager_guest_checkpoint" || o.CollectorVersion != "1" || o.Denominator != "one_checkpoint" || operations != 1 {
			return 0, false
		}
	case "cgroup.memory.phase_peak":
		if o.Phase != scenario+"/barrier_window" || o.Quality != "kernel_accounted_peak" {
			return 0, false
		}
	case "process.rss":
		if o.Phase != scenario+"/after_batch" || o.Quality != "boundary_snapshot_only" {
			return 0, false
		}
	case "process.peak_rss":
		if o.Phase != scenario+"/process_lifetime" || o.Quality != "kernel_accounted_peak" || o.Collector != "wait4_rusage" {
			return 0, false
		}
	}
	if metric == "host.alloc.bytes" {
		if operations <= 0 {
			return 0, false
		}
		return *o.Value / float64(operations), true
	}
	return *o.Value, true
}

func singleMemoryObservation(observations []protocol.Observation, metric, scenario string, operations int) (float64, bool) {
	var value float64
	count := 0
	for _, o := range observations {
		if v, ok := memoryObservationValue(o, metric, scenario, operations); ok {
			value = v
			count++
		}
	}
	return value, count == 1
}

func memoryStages(memory experiment.Bundle, matched map[string]bool) []MemoryStage {
	if !experiment.HostBaselineAllowsMeasurements(memory.Manifest) {
		return nil
	}
	type key struct{ runtime, workload, scenario, metric string }
	launches := map[key][]float64{}
	trialIDs := map[key][]string{}
	launchValues := map[key][]MemoryLaunch{}
	for _, trial := range memory.Trials {
		sameTimingTrial := memory.Manifest.Lock.Options.TimingPeakRSS && trial.Profile == "timing"
		if trial.Block < 0 || (trial.Profile != "memory" && !sameTimingTrial) || trial.Status != "ok" || !matched[trial.Runtime+"\x00"+trial.Workload] {
			continue
		}
		for _, metric := range memoryStageMetrics {
			if sameTimingTrial && metric != "process.peak_rss" {
				continue
			}
			if sameTimingTrial && !validTimingPeak(trial.Observations) {
				continue
			}
			var values []float64
			expected := 1
			// RSS boundaries and wait4 lifetime peaks are trial-scoped.
			if metric == "process.rss" || metric == "process.peak_rss" {
				if v, ok := singleMemoryObservation(trial.Observations, metric, trial.Scenario, 1); ok {
					values = append(values, v)
				}
			} else {
				expected = 0
				for _, sample := range trial.Samples {
					if !sample.Verified || sample.Warmup {
						continue
					}
					expected++
					if v, ok := singleMemoryObservation(sample.Observations, metric, trial.Scenario, sample.Operations); ok {
						values = append(values, v)
					}
				}
			}
			if expected == 0 || len(values) != expected {
				continue
			}
			if m := medianStage(values); m != nil {
				k := key{trial.Runtime, trial.Workload, trial.Scenario, metric}
				launches[k] = append(launches[k], *m)
				trialIDs[k] = append(trialIDs[k], trial.ID)
				launchValues[k] = append(launchValues[k], MemoryLaunch{TrialID: trial.ID, Bytes: *m})
			}
		}
	}
	var out []MemoryStage
	for k, values := range launches {
		row := MemoryStage{Runtime: k.runtime, Workload: k.workload, Scenario: k.scenario, Metric: k.metric, Median: medianStage(values), Launches: len(values), Trials: trialIDs[k], LaunchValues: launchValues[k]}
		if memory.Manifest.Lock.Options.TimingPeakRSS {
			row.SourceRun = memory.Manifest.ID
			row.SourceProfile = "timing"
		}
		if low, high, ok := analysis.BootstrapMedian95(values); ok {
			row.Low, row.High = &low, &high
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Workload != b.Workload {
			return a.Workload < b.Workload
		}
		if a.Runtime != b.Runtime {
			return a.Runtime < b.Runtime
		}
		if a.Scenario != b.Scenario {
			return a.Scenario < b.Scenario
		}
		return a.Metric < b.Metric
	})
	return out
}

func validTimingPeak(observations []protocol.Observation) bool {
	count := 0
	for _, o := range observations {
		if o.Metric != "process.peak_rss" {
			continue
		}
		count++
		if o.Profile != "timing" || o.DefinitionVersion != 1 || o.Scope != "adapter_process" || o.CollectorVersion != "1" || o.Denominator != "process" {
			return false
		}
	}
	return count == 1
}
