// Package analysis treats processes as independent replicates, never inner calls.
package analysis

import (
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
	"math"
	"math/rand"
	"sort"
)

const Version = "cluster-median-bootstrap-v6"

type Summary struct {
	LatencyStatus      string             `json:"latency_status"`
	LatencyReason      string             `json:"latency_reason"`
	SuccessfulLaunches int                `json:"successful_launches"`
	RecordedSamples    int                `json:"recorded_samples"`
	WarmupDiagnostics  []WarmupDiagnostic `json:"warmup_diagnostics,omitempty"`
	Runtime            string             `json:"runtime"`
	Workload           string             `json:"workload"`
	Scenario           string             `json:"scenario"`
	Profile            string             `json:"profile"`
	Launches           int                `json:"independent_launches"`
	Attempted          int                `json:"attempted_launches"`
	Samples            int                `json:"sample_count"`
	Median             *float64           `json:"median_ns_per_operation"`
	Mean               *float64           `json:"mean_ns_per_operation"`
	StdDev             *float64           `json:"stddev_launch_medians"`
	CILow              *float64           `json:"ci95_low"`
	CIHigh             *float64           `json:"ci95_high"`
	Uncertainty        string             `json:"uncertainty"`
	Stability          string             `json:"stability"`
	Failures           map[string]int     `json:"outcomes"`
	LaunchMedians      map[int]float64    `json:"launch_medians"`
}

func median(x []float64) float64 {
	if len(x) == 0 {
		return 0
	}
	y := append([]float64(nil), x...)
	sort.Float64s(y)
	n := len(y)
	if n%2 == 1 {
		return y[n/2]
	}
	return (y[n/2-1] + y[n/2]) / 2
}
func interval(x []float64) (float64, float64) {
	if len(x) < 2 {
		return median(x), median(x)
	}
	r := rand.New(rand.NewSource(829))
	samples := make([]float64, 4000)
	draw := make([]float64, len(x))
	for i := range samples {
		for j := range draw {
			draw[j] = x[r.Intn(len(x))]
		}
		samples[i] = median(draw)
	}
	sort.Float64s(samples)
	return samples[100], samples[3899]
}

// BootstrapMedian95 returns a deterministic percentile interval over independent
// launch values. Fewer than three launches do not support a useful interval.
func BootstrapMedian95(values []float64) (low, high float64, ok bool) {
	if len(values) < 3 {
		return 0, 0, false
	}
	low, high = interval(values)
	return low, high, true
}
func Summarize(b experiment.Bundle) []Summary {
	policy := HeadlineLatencyPolicy(b.Manifest)
	// Outcome counts describe execution; latency eligibility is separate.
	groups := map[string]*Summary{}
	for _, t := range b.Trials {
		if t.Block < 0 {
			continue
		}
		key := t.Runtime + "\x00" + t.Workload + "\x00" + t.Scenario + "\x00" + t.Profile
		s := groups[key]
		if s == nil {
			s = &Summary{Runtime: t.Runtime, Workload: t.Workload, Scenario: t.Scenario, Profile: t.Profile, Failures: map[string]int{}, LaunchMedians: map[int]float64{}}
			groups[key] = s
			p := policy.ForProfile(t.Profile).ForScenario(t.Scenario)
			s.LatencyStatus, s.LatencyReason = p.Status, p.Reason
		}
		s.Attempted++
		s.RecordedSamples += len(t.Samples)
		if t.Status == "ok" {
			s.SuccessfulLaunches++
		}
		if s.LatencyStatus == "timing_pass" && (t.Scenario == "steady" || t.Scenario == "trajectory") {
			s.WarmupDiagnostics = append(s.WarmupDiagnostics, diagnoseWarmup(t))
		}
		s.Failures[t.Status]++
		// All non-timing passes retain coverage without latency estimates.
		if s.LatencyStatus != "timing_pass" {
			continue
		}
		if t.Status != "ok" {
			continue
		}
		var values []float64
		for _, v := range t.Samples {
			if !v.Warmup && v.Verified && v.Operations > 0 && v.ElapsedNS >= 0 {
				values = append(values, float64(v.ElapsedNS)/float64(v.Operations))
				s.Samples++
			}
		}
		if len(values) > 0 {
			s.LaunchMedians[t.Block] = median(values)
		}
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]Summary, 0, len(keys))
	for _, k := range keys {
		s := groups[k]
		if s.LatencyStatus != "timing_pass" {
			s.Uncertainty = s.LatencyReason
			s.Stability = "not_applicable"
			out = append(out, *s)
			continue
		}
		blocks := make([]int, 0, len(s.LaunchMedians))
		for block := range s.LaunchMedians {
			blocks = append(blocks, block)
		}
		sort.Ints(blocks)
		var values []float64
		for _, block := range blocks {
			values = append(values, s.LaunchMedians[block])
		}
		s.Launches = len(values)
		var mean, stddev float64
		for _, v := range values {
			mean += v
		}
		if len(values) > 0 {
			mean /= float64(len(values))
			s.Mean = protocol.Value(mean)
			s.Median = protocol.Value(median(values))
		}
		if len(values) > 1 {
			for _, v := range values {
				stddev += (v - mean) * (v - mean)
			}
			stddev = math.Sqrt(stddev / float64(len(values)-1))
			s.StdDev = protocol.Value(stddev)
		}
		s.Uncertainty = "95% percentile bootstrap over independent launch medians"
		if len(values) < 3 {
			s.Uncertainty = "insufficient independent launches for a useful interval"
		} else {
			lo, hi := interval(values)
			s.CILow, s.CIHigh = &lo, &hi
		}
		s.Stability = "not_established"
		if len(values) >= 6 {
			s.Stability = "low_observed_dispersion"
			if mean > 0 && stddev/mean > 0.1 {
				s.Stability = "noisy"
			}
		}
		for _, d := range s.WarmupDiagnostics {
			if d.Status == "high_variability" {
				s.Stability = "noisy_within_launch"
			}
		}
		for _, d := range s.WarmupDiagnostics {
			if d.Status == "drift_detected" {
				s.Stability = "unstable_within_launch"
			}
		}
		out = append(out, *s)
	}
	return out
}

type Comparison struct {
	Profile   string   `json:"profile"`
	Reason    string   `json:"reason,omitempty"`
	Workload  string   `json:"workload"`
	Scenario  string   `json:"scenario"`
	Baseline  string   `json:"baseline"`
	Candidate string   `json:"candidate"`
	Pairs     int      `json:"paired_blocks"`
	Ratio     *float64 `json:"candidate_over_baseline"`
	Low       *float64 `json:"ci95_low"`
	High      *float64 `json:"ci95_high"`
	Status    string   `json:"status"`
}

func CompareWithin(b experiment.Bundle, baseline, candidate string) []Comparison {
	policy := HeadlineLatencyPolicy(b.Manifest)
	summaries := Summarize(b)
	byKey := map[string]Summary{}
	for _, s := range summaries {
		if s.LatencyStatus != "timing_pass" {
			continue
		}
		byKey[s.Runtime+"/"+s.Workload+"/"+s.Scenario] = s
	}
	var out []Comparison
	for _, w := range b.Manifest.Lock.Workloads {
		for _, scenario := range b.Manifest.Lock.Options.Scenarios {
			a := byKey[baseline+"/"+w.ID+"/"+scenario]
			c := byKey[candidate+"/"+w.ID+"/"+scenario]
			v := Comparison{Workload: w.ID, Scenario: scenario, Baseline: baseline, Candidate: candidate, Status: "no_common_successful_blocks"}
			v.Profile = policy.Profile
			if policy.Status != "timing_pass" {
				v.Status, v.Reason = policy.Status, policy.Reason
				out = append(out, v)
				continue
			}
			var ratios []float64
			for block := 0; block < b.Manifest.Lock.Options.Launches; block++ {
				av, aok := a.LaunchMedians[block]
				cv, cok := c.LaunchMedians[block]
				if aok && cok && av > 0 && cv > 0 {
					ratios = append(ratios, cv/av)
				}
			}
			v.Pairs = len(ratios)
			if len(ratios) > 0 {
				v.Ratio = protocol.Value(median(ratios))
				v.Status = "paired"
				if len(ratios) < 3 {
					v.Status = "insufficient_pairs"
				} else {
					lo, hi := interval(ratios)
					v.Low, v.High = &lo, &hi
				}
			}
			out = append(out, v)
		}
	}
	return out
}
func ValidatePublication(b experiment.Bundle) error {
	return AuditPublication(b, PilotEvidence{}).Err()
}
