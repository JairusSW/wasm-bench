package analysis

import (
	"encoding/json"
	"math"
	"sort"

	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

const ScalingVersion = "scaling-launch-medians-v3"

// ScalingCurve never merges different collector domains or measurement windows.
// Observations retain their original denominator; only wall time is normalized
// by the recorded operation count.
type ScalingCurve struct {
	Runtime         string               `json:"runtime"`
	Generator       string               `json:"generator"`
	Dimension       string               `json:"dimension"`
	Scenario        string               `json:"scenario"`
	Profile         string               `json:"profile"`
	Measurement     protocol.Observation `json:"measurement"`
	BatchOperations int                  `json:"batch_operations"`
	Points          []ScalingPoint       `json:"points"`
	LogLogSlope     *float64             `json:"diagnostic_log_log_slope"`
	FitStatus       string               `json:"fit_status"`
	Marginal        []ScalingMarginal    `json:"marginal_costs"`
}

type ScalingPoint struct {
	Workload  string         `json:"workload"`
	Size      int            `json:"size"`
	Attempted int            `json:"attempted_launches"`
	Launches  int            `json:"independent_launches"`
	Median    *float64       `json:"median"`
	Low       *float64       `json:"ci95_low"`
	High      *float64       `json:"ci95_high"`
	Status    string         `json:"status"`
	Outcomes  map[string]int `json:"outcomes"`
}

func Scaling(b experiment.Bundle) []ScalingCurve {
	latency := HeadlineLatencyPolicy(b.Manifest)
	b = hostEligibleBundle(b)
	workloads := map[string]protocol.Workload{}
	for _, w := range b.Manifest.Lock.Workloads {
		if w.Dimension != "" && w.Size > 0 {
			workloads[w.ID] = w
		}
	}
	type accumulated struct {
		curve  ScalingCurve
		values map[string]map[string][]float64
	}
	groups := map[string]*accumulated{}
	add := func(t experiment.Trial, o protocol.Observation, operations int, eligible bool) {
		w, ok := workloads[t.Workload]
		if !ok {
			return
		}
		value, available := o.Value, o.Status == "available"
		o.Value, o.Status, o.Reason = nil, "", ""
		c := ScalingCurve{Runtime: t.Runtime, Generator: w.Generator, Dimension: w.Dimension, Scenario: t.Scenario, Profile: t.Profile, Measurement: o, BatchOperations: operations}
		keyBytes, _ := json.Marshal(c)
		key := string(keyBytes)
		g := groups[key]
		if g == nil {
			g = &accumulated{curve: c, values: map[string]map[string][]float64{}}
			groups[key] = g
		}
		if !eligible || !available || value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) {
			return
		}
		if g.values[w.ID] == nil {
			g.values[w.ID] = map[string][]float64{}
		}
		g.values[w.ID][t.ID] = append(g.values[w.ID][t.ID], *value)
	}
	for _, t := range b.Trials {
		if t.Block < 0 {
			continue
		}
		scope := "embedding_api"
		if t.Scenario == "continuation-resume" {
			scope = "native_continuation_resumption"
		}
		if t.Scenario == "cold-process" {
			scope = "process_end_to_end"
		}
		wall := protocol.Observation{Metric: "time.wall", DefinitionVersion: 1, Unit: "ns", Scope: scope, Phase: t.Scenario, Collector: "adapter_monotonic_clock", Quality: "measured", Profile: t.Profile, Denominator: "operation", Status: "available"}
		// Even a failed cell contributes an unavailable timing point.
		timingEligible := latency.ForProfile(t.Profile).ForScenario(t.Scenario).Status == "timing_pass"
		if timingEligible {
			add(t, wall, 0, false)
		}
		for _, s := range t.Samples {
			if s.Warmup {
				continue
			}
			eligible := t.Status == "ok" && s.Verified
			if timingEligible && s.Operations > 0 && s.ElapsedNS >= 0 {
				wall.Value = protocol.Value(float64(s.ElapsedNS) / float64(s.Operations))
				add(t, wall, 0, eligible)
			}
			for _, o := range s.Observations {
				add(t, o, s.Operations, eligible)
			}
		}
		for _, o := range t.Observations {
			add(t, o, 0, t.Status == "ok")
		}
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]ScalingCurve, 0, len(keys))
	for _, key := range keys {
		g := groups[key]
		c := g.curve
		for _, w := range workloads {
			if w.Generator != c.Generator || w.Dimension != c.Dimension {
				continue
			}
			p := ScalingPoint{Workload: w.ID, Size: w.Size, Status: "unavailable", Outcomes: map[string]int{}}
			for _, t := range b.Trials {
				if t.Block >= 0 && t.Runtime == c.Runtime && t.Scenario == c.Scenario && t.Profile == c.Profile && t.Workload == w.ID {
					p.Attempted++
					p.Outcomes[t.Status]++
				}
			}
			var values []float64
			ids := make([]string, 0, len(g.values[w.ID]))
			for id := range g.values[w.ID] {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			for _, id := range ids {
				values = append(values, median(g.values[w.ID][id]))
			}
			p.Launches = len(values)
			if len(values) > 0 {
				p.Median = protocol.Value(median(values))
				p.Status = "insufficient_launches"
			}
			if len(values) >= 3 {
				lo, hi := interval(values)
				p.Low, p.High = &lo, &hi
				p.Status = "available"
			}
			c.Points = append(c.Points, p)
		}
		sort.Slice(c.Points, func(i, j int) bool {
			if c.Points[i].Size == c.Points[j].Size {
				return c.Points[i].Workload < c.Points[j].Workload
			}
			return c.Points[i].Size < c.Points[j].Size
		})
		c.FitStatus = "requires_complete_positive_points_and_three_launches_each"
		c.Marginal = scalingMarginals(c, g.values, b.Trials)
		valid := len(c.Points) >= 3
		seen := map[int]bool{}
		var xs, ys []float64
		for _, p := range c.Points {
			if seen[p.Size] || p.Status != "available" || p.Median == nil || *p.Median <= 0 {
				valid = false
				break
			}
			seen[p.Size] = true
			xs = append(xs, math.Log(float64(p.Size)))
			ys = append(ys, math.Log(*p.Median))
		}
		if valid {
			var mx, my float64
			for i := range xs {
				mx += xs[i]
				my += ys[i]
			}
			mx /= float64(len(xs))
			my /= float64(len(ys))
			var cov, variance float64
			for i := range xs {
				cov += (xs[i] - mx) * (ys[i] - my)
				variance += (xs[i] - mx) * (xs[i] - mx)
			}
			if variance > 0 {
				c.LogLogSlope = protocol.Value(cov / variance)
				c.FitStatus = "descriptive_fit_not_asymptotic_proof"
			}
		}
		out = append(out, c)
	}
	return out
}
