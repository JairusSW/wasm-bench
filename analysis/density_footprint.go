package analysis

import (
	"encoding/json"
	"math"
	"sort"
	"strings"

	"github.com/wasmbench/wasmbench/protocol"
)

const DensityFootprintVersion = "density-boundary-changes-v2"

// DensityFootprint joins only snapshots within one launch and exact measurement
// domain. Differences are signed footprint changes, never allocation volume or
// proof of reclamation. The original phase-specific timelines remain evidence.
type DensityFootprint struct {
	Trial       string                  `json:"trial"`
	Measurement protocol.Observation    `json:"measurement"`
	Phases      []string                `json:"phases"`
	Points      []DensityFootprintPoint `json:"points"`
}

type DensityFootprintPoint struct {
	Sample          int      `json:"sample_index"`
	Warmup          bool     `json:"warmup"`
	ProvisionChange *float64 `json:"ready_minus_before_bytes"`
	ReleaseChange   *float64 `json:"released_minus_ready_bytes"`
	CycleChange     *float64 `json:"released_minus_before_bytes"`
	Status          string   `json:"status"`
}

func DensityFootprints(lines []MemoryTimeline) []DensityFootprint {
	type group struct {
		result    DensityFootprint
		lines     [3]*MemoryTimeline
		ambiguous bool
	}
	groups := map[string]*group{}
	for i := range lines {
		l := &lines[i]
		if l.Scenario != "density" && l.Scenario != "density-cycle" && l.Scenario != "guest-density" {
			continue
		}
		stages := protocol.PhaseStages(l.Scenario)
		stage := -1
		for j, s := range stages {
			if l.Measurement.Phase == l.Scenario+"/"+s {
				stage = j
			}
		}
		if stage < 0 {
			continue
		}
		m := l.Measurement
		m.Phase = l.Scenario
		keyBytes, _ := json.Marshal(struct {
			Trial       string
			Measurement protocol.Observation
		}{l.Trial, m})
		key := string(keyBytes)
		g := groups[key]
		if g == nil {
			phases := make([]string, len(stages))
			for j, s := range stages {
				phases[j] = l.Scenario + "/" + s
			}
			g = &group{result: DensityFootprint{Trial: l.Trial, Measurement: m, Phases: phases}}
			groups[key] = g
		}
		if g.lines[stage] != nil {
			g.ambiguous = true
		}
		g.lines[stage] = l
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]DensityFootprint, 0, len(keys))
	for _, key := range keys {
		g := groups[key]
		var base *MemoryTimeline
		for _, l := range g.lines {
			if l != nil {
				base = l
				break
			}
		}
		for i, point := range base.Points {
			p := DensityFootprintPoint{Sample: point.Sample, Warmup: point.Warmup, Status: "available"}
			var values [3]float64
			var problems []string
			for j, l := range g.lines {
				status := "available"
				switch {
				case l == nil:
					status = "not_recorded"
				case l.TrialStatus != "ok":
					status = "unsuccessful_trial"
				case l.Status == "invalid_sample_sequence":
					status = l.Status
				case len(l.Points) != len(base.Points):
					status = "invalid_sample_sequence"
				case l.Points[i].Sample != point.Sample || l.Points[i].Warmup != point.Warmup:
					status = "invalid_sample_sequence"
				case l.Points[i].Status != "available":
					status = l.Points[i].Status
				case l.Points[i].Value == nil:
					status = "invalid_snapshot"
				case math.IsNaN(*l.Points[i].Value) || math.IsInf(*l.Points[i].Value, 0) || *l.Points[i].Value < 0:
					status = "invalid_snapshot"
				default:
					values[j] = *l.Points[i].Value
				}
				if status != "available" {
					problems = append(problems, g.result.Phases[j]+": "+status)
				}
			}
			if g.ambiguous {
				problems = append(problems, "ambiguous_measurement_domain")
			}
			if len(problems) > 0 {
				p.Status = strings.Join(problems, "; ")
			} else {
				p.ProvisionChange = protocol.Value(values[1] - values[0])
				p.ReleaseChange = protocol.Value(values[2] - values[1])
				p.CycleChange = protocol.Value(values[2] - values[0])
			}
			g.result.Points = append(g.result.Points, p)
		}
		out = append(out, g.result)
	}
	return out
}
