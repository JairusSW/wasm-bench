package analysis

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"reflect"
	"sort"

	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

// Regression compares separately collected runs. Blocks with the same number in
// different runs are not pairs. Each bootstrap resamples the two launch sets
// independently, keeping all operations nested under their original process.
type Regression struct {
	Workload          string         `json:"workload"`
	Scenario          string         `json:"scenario"`
	BaselineRuntime   string         `json:"baseline_runtime"`
	CandidateRuntime  string         `json:"candidate_runtime"`
	BaselineLaunches  int            `json:"baseline_launches"`
	CandidateLaunches int            `json:"candidate_launches"`
	BaselineOutcomes  map[string]int `json:"baseline_outcomes"`
	CandidateOutcomes map[string]int `json:"candidate_outcomes"`
	Ratio             *float64       `json:"candidate_over_baseline"`
	Low               *float64       `json:"ci95_low"`
	High              *float64       `json:"ci95_high"`
	Status            string         `json:"status"`
	Reason            string         `json:"reason,omitempty"`
}
type RegressionReport struct {
	AnalysisVersion        string             `json:"analysis_version"`
	BaselineRun            string             `json:"baseline_run"`
	CandidateRun           string             `json:"candidate_run"`
	Method                 string             `json:"method"`
	Interpretation         string             `json:"interpretation"`
	RunnerChanged          bool               `json:"runner_changed"`
	BaselineConfiguration  experiment.Runtime `json:"baseline_configuration"`
	CandidateConfiguration experiment.Runtime `json:"candidate_configuration"`
	CommonSubset           []string           `json:"common_workload_scenario_subset"`
	Results                []Regression       `json:"results"`
}

func CompareRuns(a, b experiment.Bundle, baseline, candidate string) (RegressionReport, error) {
	return compareRuns(a, b, baseline, candidate, sameWorkload)
}

func compareRuns(a, b experiment.Bundle, baseline, candidate string, comparable func(protocol.Workload, protocol.Workload) bool) (RegressionReport, error) {
	out := RegressionReport{AnalysisVersion: "independent-launch-ratio-bootstrap-v1", BaselineRun: a.Manifest.ID, CandidateRun: b.Manifest.ID, Method: "ratio of launch medians; independent percentile bootstrap, 4000 resamples; no cross-run pairing", RunnerChanged: a.Manifest.Lock.RunnerSHA256 != b.Manifest.Lock.RunnerSHA256, CommonSubset: []string{}, Results: []Regression{}}
	out.Interpretation = "Intervals describe observed run differences, not causal attribution to a compiler change; environmental drift and multiple comparisons are not corrected."
	if a.Manifest.Kind != "measurement" || b.Manifest.Kind != "measurement" {
		return out, fmt.Errorf("cross-run comparison requires measurement bundles")
	}
	ao, bo := a.Manifest.Lock.Options, b.Manifest.Lock.Options
	if err := experiment.MatchHostMeasurementPolicy(a.Manifest.Lock, b.Manifest.Lock); err != nil {
		return out, err
	}
	if ao.Profile != "timing" || bo.Profile != "timing" {
		return out, fmt.Errorf("latency regression comparison requires timing passes")
	}
	if a.Manifest.Lock.Protocol != b.Manifest.Lock.Protocol || ao.Samples != bo.Samples || ao.Operations != bo.Operations || ao.Warmup != bo.Warmup || ao.Timeout != bo.Timeout || ao.SustainedDuration != bo.SustainedDuration || ao.SustainedPostCollection != bo.SustainedPostCollection {
		return out, fmt.Errorf("measurement protocols differ (protocol, samples, operations, warmup or timeout)")
	}
	if !reflect.DeepEqual(a.Manifest.Host, b.Manifest.Host) {
		return out, fmt.Errorf("host fingerprint or environment differs; collect on a matched host before claiming a regression")
	}
	find := func(bundle experiment.Bundle, id string) (experiment.Runtime, error) {
		for _, r := range bundle.Manifest.Lock.Runtimes {
			if r.ID == id {
				return r, nil
			}
		}
		return experiment.Runtime{}, fmt.Errorf("runtime configuration %q absent from %s", id, bundle.Manifest.ID)
	}
	var err error
	out.BaselineConfiguration, err = find(a, baseline)
	if err != nil {
		return out, err
	}
	out.CandidateConfiguration, err = find(b, candidate)
	if err != nil {
		return out, err
	}
	workloadsA, workloadsB := map[string]protocol.Workload{}, map[string]protocol.Workload{}
	names := map[string]bool{}
	scenarios := map[string]bool{}
	for _, w := range a.Manifest.Lock.Workloads {
		workloadsA[w.ID] = w
		names[w.ID] = true
	}
	for _, w := range b.Manifest.Lock.Workloads {
		workloadsB[w.ID] = w
		names[w.ID] = true
	}
	for _, s := range ao.Scenarios {
		scenarios[s] = true
	}
	for _, s := range bo.Scenarios {
		scenarios[s] = true
	}
	type key struct{ workload, scenario string }
	as, bs := map[key]Summary{}, map[key]Summary{}
	for _, s := range Summarize(a) {
		if s.Runtime == baseline {
			as[key{s.Workload, s.Scenario}] = s
		}
	}
	for _, s := range Summarize(b) {
		if s.Runtime == candidate {
			bs[key{s.Workload, s.Scenario}] = s
		}
	}
	for _, name := range sortedKeys(names) {
		for _, scenario := range sortedKeys(scenarios) {
			x, y := as[key{name, scenario}], bs[key{name, scenario}]
			r := Regression{Workload: name, Scenario: scenario, BaselineRuntime: baseline, CandidateRuntime: candidate, BaselineLaunches: x.Launches, CandidateLaunches: y.Launches, BaselineOutcomes: x.Failures, CandidateOutcomes: y.Failures, Status: "unavailable"}
			wa, aok := workloadsA[name]
			wb, bok := workloadsB[name]
			switch {
			case !aok || !bok:
				r.Reason = "workload absent from one run"
			case !comparable(wa, wb):
				r.Status = "incomparable"
				r.Reason = "artifact or workload contract changed"
			case x.Launches == 0 || y.Launches == 0:
				r.Reason = "no successful measured launches on one side"
			default:
				av, bv := launchValues(x), launchValues(y)
				positive := true
				for _, v := range av {
					if v <= 0 {
						positive = false
					}
				}
				for _, v := range bv {
					if v <= 0 {
						positive = false
					}
				}
				if !positive {
					r.Reason = "nonpositive launch latency cannot support a ratio"
					break
				}
				ratio := median(bv) / median(av)
				r.Ratio = &ratio
				r.Status = "insufficient_launches"
				out.CommonSubset = append(out.CommonSubset, name+" / "+scenario)
				if len(av) >= 3 && len(bv) >= 3 {
					lo, hi := independentRatioInterval(av, bv)
					r.Low = &lo
					r.High = &hi
					r.Status = "uncertain"
					if lo > 1 {
						r.Status = "observed_slower"
					} else if hi < 1 {
						r.Status = "observed_faster"
					}
				} else {
					r.Reason = "at least three independent launches per side required for an interval"
				}
			}
			out.Results = append(out.Results, r)
		}
	}
	return out, nil
}
func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
func launchValues(s Summary) []float64 {
	blocks := make([]int, 0, len(s.LaunchMedians))
	for block := range s.LaunchMedians {
		blocks = append(blocks, block)
	}
	sort.Ints(blocks)
	v := make([]float64, 0, len(blocks))
	for _, block := range blocks {
		v = append(v, s.LaunchMedians[block])
	}
	return v
}
func sameWorkload(a, b protocol.Workload) bool {
	// Paths and source provenance may change between source builds, but every
	// executable contract field and the exact Wasm bytes must remain identical.
	a.Artifact = ""
	b.Artifact = ""
	a.Source = ""
	b.Source = ""
	a.Provenance = nil
	b.Provenance = nil
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
func independentRatioInterval(a, b []float64) (float64, float64) {
	rng := rand.New(rand.NewSource(541))
	drawA, drawB := make([]float64, len(a)), make([]float64, len(b))
	ratios := make([]float64, 4000)
	for i := range ratios {
		for j := range drawA {
			drawA[j] = a[rng.Intn(len(a))]
		}
		for j := range drawB {
			drawB[j] = b[rng.Intn(len(b))]
		}
		ratios[i] = median(drawB) / median(drawA)
	}
	sort.Float64s(ratios)
	return ratios[100], ratios[3899]
}
