package analysis

import (
	"math"

	"github.com/wasmbench/wasmbench/metrics"
	"github.com/wasmbench/wasmbench/protocol"
	"github.com/wasmbench/wasmbench/sourcebuild"
)

type SourceBuildSummary struct {
	MedianBytes         *float64       `json:"median_bytes_max_step_peak,omitempty"`
	MeanBytes           *float64       `json:"mean_bytes_max_step_peak,omitempty"`
	StdDevBytes         *float64       `json:"stddev_bytes_max_step_peak,omitempty"`
	MissingMeasurements int            `json:"missing_measurements,omitempty"`
	Variant             string         `json:"variant"`
	SuccessfulBuilds    int            `json:"successful_builds"`
	Outcomes            map[string]int `json:"outcomes"`
	WarmupOutcomes      map[string]int `json:"warmup_outcomes"`
	Median              *float64       `json:"median_ns_per_build"`
	Mean                *float64       `json:"mean_ns_per_build"`
	StdDev              *float64       `json:"stddev_ns_per_build"`
	Low                 *float64       `json:"ci95_low"`
	High                *float64       `json:"ci95_high"`
	Status              string         `json:"status"`
}
type SourceBuildReport struct {
	AnalysisVersion string                     `json:"analysis_version"`
	Measurement     metrics.Definition         `json:"metric"`
	Interpretation  string                     `json:"interpretation"`
	Evidence        sourcebuild.BuildBenchmark `json:"evidence"`
	Summaries       []SourceBuildSummary       `json:"summaries"`
	Comparisons     []Comparison               `json:"comparisons"`
}

// SummarizeSourceBuilds requires VerifyBenchmark-loaded evidence. Complete
// builds, not individual compiler steps, are the replication unit.
func SummarizeSourceBuilds(b sourcebuild.BuildBenchmark) SourceBuildReport {
	if !b.HostBaselineAllowsMeasurements() {
		b.Trials = append([]sourcebuild.BuildTrial(nil), b.Trials...)
		for i := range b.Trials {
			b.Trials[i].Status = "host_policy_mismatch"
			b.Trials[i].Reason = "locked host baseline, CPU partition or IRQ affinity requirement did not pass at both benchmark boundaries"
		}
	}
	r := SourceBuildReport{AnalysisVersion: "source-build-block-bootstrap-v1", Evidence: b, Summaries: []SourceBuildSummary{}, Comparisons: []Comparison{},
		Interpretation: "Median and mean of successful complete build recipes; warmup blocks excluded but retained. 95% percentile bootstrap over complete builds, 4000 draws, requires at least three builds. Comparisons use median candidate/baseline ratios from common successful randomized blocks; at least three pairs for intervals. First declared variant is the baseline. Missing and failed builds remain in outcome counts; results do not establish causal attribution or official performance qualification."}
	metric := "source.build.tool_wall"
	if b.Config.Profile == "cpu" {
		metric = "source.build.wait_cpu"
		r.AnalysisVersion = "source-build-cpu-block-bootstrap-v1"
		r.Interpretation += " Dedicated CPU pass: statistics use OS-reported wait CPU, not wall latency. CPU accounting coverage follows OS child-wait semantics, not a guaranteed complete process tree."
	}
	if b.Config.Profile == "memory" {
		metric = "source.build.max_step_cgroup_peak"
		r.AnalysisVersion = "source-build-step-memory-block-bootstrap-v1"
		r.Interpretation += " Dedicated memory pass: maximum fresh tool-step cgroup peak per build, not a whole-build simultaneous peak, RSS or heap. Summary and confidence interval units are bytes; timing summary fields remain null."
	}
	for _, m := range metrics.Registry {
		if m.Name == metric {
			r.Measurement = m
		}
	}
	values := make([]map[int]float64, len(b.Config.Variants))
	for i, l := range b.Config.Variants {
		s := SourceBuildSummary{Variant: l.Recipe.ID, Outcomes: map[string]int{}, WarmupOutcomes: map[string]int{}, Status: "no_successful_builds"}
		values[i] = map[int]float64{}
		var xs []float64
		for _, t := range b.Trials {
			if t.Variant != i {
				continue
			}
			if t.Warmup {
				s.WarmupOutcomes[t.Status]++
				continue
			}
			s.Outcomes[t.Status]++
			var value *float64
			if t.ToolWallNS != nil {
				value = protocol.Value(float64(*t.ToolWallNS))
			}
			if b.Config.Profile == "cpu" {
				value = nil
				if t.ToolCPUNS != nil {
					value = protocol.Value(float64(*t.ToolCPUNS))
				}
			}
			if b.Config.Profile == "memory" {
				value = t.MemoryPeakBytes
			}
			if t.Status == "ok" && value == nil {
				s.MissingMeasurements++
			}
			if t.Status == "ok" && value != nil {
				v := *value
				xs = append(xs, v)
				values[i][t.Block] = v
			}
		}
		s.SuccessfulBuilds = len(xs)
		if len(xs) > 0 {
			var mean float64
			for _, v := range xs {
				mean += v
			}
			mean /= float64(len(xs))
			s.Mean = protocol.Value(mean)
			s.Median = protocol.Value(median(xs))
			s.Status = "insufficient_builds"
			if len(xs) > 1 {
				var variance float64
				for _, v := range xs {
					variance += (v - mean) * (v - mean)
				}
				s.StdDev = protocol.Value(math.Sqrt(variance / float64(len(xs)-1)))
			}
			if len(xs) >= 3 {
				lo, hi := interval(xs)
				s.Low, s.High = protocol.Value(lo), protocol.Value(hi)
				s.Status = "estimated"
			}
		}
		if b.Config.Profile == "memory" {
			s.MedianBytes, s.MeanBytes, s.StdDevBytes = s.Median, s.Mean, s.StdDev
			s.Median, s.Mean, s.StdDev = nil, nil, nil
		}
		r.Summaries = append(r.Summaries, s)
	}
	for i := 1; i < len(values); i++ {
		p := Comparison{Workload: b.Config.Variants[0].Recipe.Workload.ID, Scenario: "source-build", Baseline: b.Config.Variants[0].Recipe.ID, Candidate: b.Config.Variants[i].Recipe.ID, Status: "no_common_successful_blocks"}
		var ratios []float64
		for block := 0; block < b.Config.Blocks; block++ {
			a, aok := values[0][block]
			v, vok := values[i][block]
			if aok && vok && a > 0 && v > 0 {
				ratios = append(ratios, v/a)
			}
		}
		p.Pairs = len(ratios)
		if len(ratios) > 0 {
			p.Ratio = protocol.Value(median(ratios))
			p.Status = "insufficient_pairs"
			if len(ratios) >= 3 {
				lo, hi := interval(ratios)
				p.Low, p.High = protocol.Value(lo), protocol.Value(hi)
				p.Status = "paired"
			}
		}
		r.Comparisons = append(r.Comparisons, p)
	}
	return r
}
