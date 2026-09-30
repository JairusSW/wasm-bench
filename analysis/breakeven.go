package analysis

import (
	"fmt"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
	"slices"
	"sort"
)

const BreakEvenVersion = "observed-prefix-block-bootstrap-v2"

type BreakEvenPoint struct {
	Invocations int      `json:"invocations"`
	Median      *float64 `json:"median_total_ns"`
	Low         *float64 `json:"ci95_low"`
	High        *float64 `json:"ci95_high"`
}
type BreakEvenCurve struct {
	Runtime   string                    `json:"runtime"`
	Workload  string                    `json:"workload"`
	Setup     []string                  `json:"setup_scenarios"`
	Status    string                    `json:"status"`
	Reason    string                    `json:"reason,omitempty"`
	Attempted int                       `json:"planned_blocks"`
	Blocks    []int                     `json:"complete_blocks"`
	Outcomes  map[string]map[string]int `json:"scenario_outcomes"`
	Trials    []string                  `json:"source_trials"`
	Points    []BreakEvenPoint          `json:"points"`
}
type BreakEvenDelta struct {
	Invocations int      `json:"invocations"`
	Median      float64  `json:"median_candidate_minus_baseline_ns"`
	Low         *float64 `json:"ci95_low"`
	High        *float64 `json:"ci95_high"`
	Conclusion  string   `json:"pointwise_conclusion"`
}
type BreakEvenComparison struct {
	Workload  string           `json:"workload"`
	Baseline  string           `json:"baseline"`
	Candidate string           `json:"candidate"`
	Blocks    []int            `json:"paired_complete_blocks"`
	Status    string           `json:"status"`
	Points    []BreakEvenDelta `json:"points"`
}
type BreakEvenReport struct {
	Version        string                `json:"analysis_version"`
	Run            string                `json:"run"`
	LockSHA256     string                `json:"lock_sha256"`
	Method         string                `json:"method"`
	Interpretation string                `json:"interpretation"`
	Curves         []BreakEvenCurve      `json:"curves"`
	Comparisons    []BreakEvenComparison `json:"comparisons"`
}

// BreakEven uses only observed trajectory prefixes. Each synthetic block total
// combines separately measured setup-phase medians with that block's cumulative
// call times. It is not a measured end-to-end latency or a steady-state forecast.
// A nil setup chooses compile + instantiate (+ app-init when declared).
func BreakEven(b experiment.Bundle, setup []string, baseline, candidate string) (BreakEvenReport, error) {
	b = hostEligibleBundle(b)
	out := BreakEvenReport{Version: BreakEvenVersion, Run: b.Manifest.ID, LockSHA256: b.Manifest.LockSHA256, Curves: []BreakEvenCurve{}, Comparisons: []BreakEvenComparison{}, Method: "median synthetic block totals; 4000-resample percentile bootstrap over complete blocks; warmup included; no extrapolation", Interpretation: "Setup phases and execution are collected in separate processes, not one end-to-end measurement. Excludes unselected setup, process launch, verification, input preparation and inter-call gaps. Tier/background activity is unobserved. Intervals are pointwise, not simultaneous; crossing at one observed count does not establish a permanent winner."}
	o := b.Manifest.Lock.Options
	if policy := HeadlineLatencyPolicy(b.Manifest); policy.Status != "timing_pass" {
		return out, fmt.Errorf("break-even requires an eligible timing measurement bundle: %s", policy.Reason)
	}
	if !slices.Contains(o.Scenarios, "trajectory") {
		return out, fmt.Errorf("break-even requires a trajectory scenario; steady samples may omit invocation 1")
	}
	if len(setup) == 0 && setup != nil {
		return out, fmt.Errorf("at least one setup phase required")
	}
	seen := map[string]bool{}
	for _, s := range setup {
		if seen[s] || !slices.Contains([]string{"engine-init", "compile", "instantiate", "app-init"}, s) {
			return out, fmt.Errorf("invalid or duplicate setup phase %q", s)
		}
		seen[s] = true
	}
	if (baseline == "") != (candidate == "") || baseline != "" && baseline == candidate {
		return out, fmt.Errorf("select two distinct runtime configurations")
	}
	names := map[string]bool{}
	for _, r := range b.Manifest.Lock.Runtimes {
		names[r.ID] = true
	}
	if baseline != "" && (!names[baseline] || !names[candidate]) {
		return out, fmt.Errorf("comparison runtime absent from lock")
	}
	n := o.Samples + o.Warmup
	if n < 1 || n > 200000 || o.Launches < 1 {
		return out, fmt.Errorf("invalid trajectory budget")
	}
	counts := []int{0}
	for step := 1; step <= n; step *= 10 {
		for _, mult := range []int{1, 2, 5} {
			if v := step * mult; v <= n {
				counts = append(counts, v)
			}
		}
	}
	if !slices.Contains(counts, n) {
		counts = append(counts, n)
	}
	sort.Ints(counts)
	type key struct {
		runtime, workload, scenario string
		block                       int
	}
	indexed := map[key][]experiment.Trial{}
	for _, t := range b.Trials {
		if t.Block >= 0 {
			k := key{t.Runtime, t.Workload, t.Scenario, t.Block}
			indexed[k] = append(indexed[k], t)
		}
	}
	type cell struct{ runtime, workload string }
	totals := map[cell]map[int][]float64{}
	for _, w := range b.Manifest.Lock.Workloads {
		for _, rt := range b.Manifest.Lock.Runtimes {
			phases := append([]string{}, setup...)
			if setup == nil {
				phases = []string{"compile", "instantiate"}
				if w.Initialize != "" {
					phases = append(phases, "app-init")
				}
			}
			c := BreakEvenCurve{Runtime: rt.ID, Workload: w.ID, Setup: phases, Status: "unavailable", Attempted: o.Launches, Blocks: []int{}, Trials: []string{}, Points: []BreakEvenPoint{}, Outcomes: map[string]map[string]int{}}
			all := append(append([]string{}, phases...), "trajectory")
			for _, s := range all {
				c.Outcomes[s] = map[string]int{}
			}
			values := map[int][]float64{}
			totals[cell{rt.ID, w.ID}] = values
			contractOK := protocol.ValidateTrajectory(&protocol.Preparation{Workload: w, Profile: "timing"}, &protocol.RunRequest{Scenario: "trajectory", Samples: o.Samples, Warmup: o.Warmup, Operations: o.Operations}) == nil
			for block := 0; block < o.Launches; block++ {
				valid := contractOK
				sum := 0.0
				var prefix []float64
				var ids []string
				for _, s := range all {
					ts := indexed[key{rt.ID, w.ID, s, block}]
					if len(ts) != 1 {
						status := "missing"
						if len(ts) > 1 {
							status = "duplicate"
						}
						c.Outcomes[s][status]++
						valid = false
						continue
					}
					t := ts[0]
					c.Outcomes[s][t.Status]++
					ids = append(ids, t.ID)
					if t.Status != "ok" || t.Profile != "timing" {
						valid = false
						continue
					}
					if s == "trajectory" {
						if len(t.Samples) != n {
							valid = false
							continue
						}
						prefix = make([]float64, n+1)
						for i, v := range t.Samples {
							if !v.Verified || v.ElapsedNS < 0 || v.Operations != 1 || v.SampleType != "individual_operation" || v.Index != i || v.Warmup != (i < o.Warmup) {
								valid = false
							}
							prefix[i+1] = prefix[i] + float64(v.ElapsedNS)
						}
					} else {
						var durations []float64
						if len(t.Samples) != o.Samples {
							valid = false
						}
						for i, v := range t.Samples {
							if v.Warmup || !v.Verified || v.ElapsedNS < 0 || v.Operations < 1 || v.Index != i || !slices.Contains([]string{"individual_operation", "batch_average"}, v.SampleType) {
								valid = false
								continue
							}
							durations = append(durations, float64(v.ElapsedNS)/float64(v.Operations))
						}
						if len(durations) == 0 {
							valid = false
						} else {
							sum += median(durations)
						}
					}
				}
				if valid {
					v := make([]float64, len(counts))
					for i, count := range counts {
						v[i] = sum + prefix[count]
					}
					values[block] = v
					c.Blocks = append(c.Blocks, block)
					c.Trials = append(c.Trials, ids...)
				}
			}
			if !contractOK {
				c.Reason = "trajectory requires a stateless core scalar workload"
			} else if len(c.Blocks) == 0 {
				c.Reason = "no complete valid blocks across every selected setup phase and trajectory"
			} else {
				c.Status = "insufficient_blocks"
				if len(c.Blocks) >= 3 {
					c.Status = "available"
				}
				if len(c.Blocks) < o.Launches {
					c.Reason = "conditional on complete successful blocks; inspect retained coverage and outcomes"
				}
				for i, count := range counts {
					var v []float64
					for _, block := range c.Blocks {
						v = append(v, values[block][i])
					}
					p := BreakEvenPoint{Invocations: count, Median: protocol.Value(median(v))}
					if len(v) >= 3 {
						lo, hi := interval(v)
						p.Low, p.High = &lo, &hi
					}
					c.Points = append(c.Points, p)
				}
			}
			out.Curves = append(out.Curves, c)
		}
	}
	if baseline != "" {
		for _, w := range b.Manifest.Lock.Workloads {
			a, c := totals[cell{baseline, w.ID}], totals[cell{candidate, w.ID}]
			comparison := BreakEvenComparison{Workload: w.ID, Baseline: baseline, Candidate: candidate, Status: "no_common_complete_blocks", Blocks: []int{}, Points: []BreakEvenDelta{}}
			for block := 0; block < o.Launches; block++ {
				if a[block] != nil && c[block] != nil {
					comparison.Blocks = append(comparison.Blocks, block)
				}
			}
			if len(comparison.Blocks) > 0 {
				comparison.Status = "insufficient_pairs"
				if len(comparison.Blocks) >= 3 {
					comparison.Status = "paired"
				}
				for i, count := range counts {
					var deltas []float64
					for _, block := range comparison.Blocks {
						deltas = append(deltas, c[block][i]-a[block][i])
					}
					p := BreakEvenDelta{Invocations: count, Median: median(deltas), Conclusion: "insufficient_pairs"}
					if len(deltas) >= 3 {
						lo, hi := interval(deltas)
						p.Low, p.High = &lo, &hi
						p.Conclusion = "inconclusive"
						if hi < 0 {
							p.Conclusion = "candidate_faster"
						} else if lo > 0 {
							p.Conclusion = "baseline_faster"
						}
					}
					comparison.Points = append(comparison.Points, p)
				}
			}
			out.Comparisons = append(out.Comparisons, comparison)
		}
	}
	return out, nil
}
