package publish

import (
	"math"
	"sort"

	"github.com/wasmbench/wasmbench/analysis"
	"github.com/wasmbench/wasmbench/experiment"
)

const ParetoVersion = "latency-rss-pareto-v1"

// A ParetoPoint joins separately measured stage medians, not jointly observed
// latency/RSS samples. Frontier membership is descriptive, without a significance
// test or a joint confidence region. Only whole-process RSS is compared here;
// allocator-specific domains cannot silently compete with process footprint.
type ParetoPoint struct {
	Runtime        string   `json:"runtime"`
	Workload       string   `json:"workload"`
	Scenario       string   `json:"scenario"`
	Status         string   `json:"status"`
	Reason         string   `json:"reason"`
	Time           *float64 `json:"time_ns"`
	TimeLow        *float64 `json:"time_ci95_low_ns"`
	TimeHigh       *float64 `json:"time_ci95_high_ns"`
	TimeLaunches   int      `json:"time_launches"`
	RSS            *float64 `json:"rss_bytes"`
	RSSLow         *float64 `json:"rss_ci95_low_bytes"`
	RSSHigh        *float64 `json:"rss_ci95_high_bytes"`
	MemoryLaunches int      `json:"memory_launches"`
	MemoryTrials   []string `json:"memory_trials"`
	DominatedBy    []string `json:"dominated_by"`
	Frontier       bool     `json:"frontier"`
}

func paretoPoints(b experiment.Bundle, summaries []analysis.Summary, memory []MemoryStage) []ParetoPoint {
	type key struct{ runtime, workload, scenario string }
	times := map[key]analysis.Summary{}
	for _, s := range summaries {
		times[key{s.Runtime, s.Workload, s.Scenario}] = s
	}
	memories := map[key]MemoryStage{}
	for _, m := range memory {
		if m.Metric == "process.peak_rss" {
			memories[key{m.Runtime, m.Workload, m.Scenario}] = m
		}
	}
	points := []ParetoPoint{}
	for _, w := range b.Manifest.Lock.Workloads {
		for _, stage := range b.Manifest.Lock.Options.Scenarios {
			for _, r := range b.Manifest.Lock.Runtimes {
				p := ParetoPoint{Runtime: r.ID, Workload: w.ID, Scenario: stage, Status: "unavailable", MemoryTrials: []string{}, DominatedBy: []string{}}
				s, hasTime := times[key{r.ID, w.ID, stage}]
				m, hasMemory := memories[key{r.ID, w.ID, stage}]
				if hasTime && s.LatencyStatus == "timing_pass" && s.Launches > 0 && nonnegativeFinite(s.Median) {
					p.Time, p.TimeLow, p.TimeHigh, p.TimeLaunches = s.Median, s.CILow, s.CIHigh, s.Launches
				}
				if hasMemory && m.Launches > 0 && nonnegativeFinite(m.Median) {
					p.RSS, p.RSSLow, p.RSSHigh, p.MemoryLaunches = m.Median, m.Low, m.High, m.Launches
					p.MemoryTrials = append(p.MemoryTrials, m.Trials...)
					sort.Strings(p.MemoryTrials)
				}
				switch {
				case p.Time == nil && p.RSS == nil:
					p.Reason = "No eligible timing and matched process-peak RSS measurements"
				case p.Time == nil:
					p.Reason = "No eligible timing measurement"
				case p.RSS == nil:
					p.Reason = "No matched process-peak RSS measurement"
				default:
					p.Status, p.Reason = "available", "Separate timing/memory launch medians; RSS includes whole-process startup and setup"
				}
				points = append(points, p)
			}
		}
	}
	groups := map[[2]string][]int{}
	for i, p := range points {
		if p.Status == "available" {
			k := [2]string{p.Workload, p.Scenario}
			groups[k] = append(groups[k], i)
		}
	}
	for i := range points {
		p := &points[i]
		if p.Status != "available" {
			continue
		}
		for _, index := range groups[[2]string{p.Workload, p.Scenario}] {
			q := points[index]
			if *q.Time <= *p.Time && *q.RSS <= *p.RSS && (*q.Time < *p.Time || *q.RSS < *p.RSS) {
				p.DominatedBy = append(p.DominatedBy, q.Runtime)
			}
		}
		sort.Strings(p.DominatedBy)
		p.Frontier = len(p.DominatedBy) == 0
	}
	return points
}

func nonnegativeFinite(value *float64) bool {
	return value != nil && !math.IsNaN(*value) && !math.IsInf(*value, 0) && *value >= 0
}
