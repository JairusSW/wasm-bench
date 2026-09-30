package analysis

import (
	"encoding/json"
	"math"
	"sort"

	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

const MemoryTimelineVersion = "memory-snapshot-sequence-v1"

type MemoryTimelinePoint struct {
	Sample int      `json:"sample_index"`
	Warmup bool     `json:"warmup"`
	Value  *float64 `json:"value"`
	Status string   `json:"status"`
	Reason string   `json:"reason,omitempty"`
}

type MemoryTimeline struct {
	Trial           string                `json:"trial"`
	Runtime         string                `json:"runtime"`
	Workload        string                `json:"workload"`
	Scenario        string                `json:"scenario"`
	Block           int                   `json:"block"`
	TrialStatus     string                `json:"trial_status"`
	Measurement     protocol.Observation  `json:"measurement"`
	Points          []MemoryTimelinePoint `json:"points"`
	FirstLastChange *float64              `json:"first_last_snapshot_change_bytes"`
	Status          string                `json:"status"`
}

// snapshotMetric deliberately excludes allocation-volume counters, sampled
// peaks and GC activity. An endpoint difference is meaningful only within one
// snapshot domain; it is not allocation volume, causal retention, or a leak test.
func snapshotMetric(metric string) bool {
	switch metric {
	case "process.rss", "process.pss", "process.private", "process.virtual",
		"host.heap.start", "host.heap.end", "host.js_heap.start", "host.js_heap.end",
		"guest.memory.logical", "density.guest_memory.logical", "cgroup.memory.current",
		"cgroup.memory.anon", "cgroup.memory.file", "cgroup.memory.kernel", "cgroup.memory.sock", "cgroup.memory.pagetables", "cgroup.memory.slab":
		return true
	}
	return false
}

// MemoryTimelines preserves sample order (not elapsed wall-time spacing) and
// warmup labels. It never joins independent processes or silently skips a hole.
func MemoryTimelines(b experiment.Bundle) []MemoryTimeline {
	b = hostEligibleBundle(b)
	var out []MemoryTimeline
	for _, t := range b.Trials {
		if t.Block < 0 || t.Profile != "memory" {
			continue
		}
		type group struct {
			measurement protocol.Observation
			values      map[int][]protocol.Observation
		}
		groups := map[string]*group{}
		sequenceValid := true
		for i, s := range t.Samples {
			if s.Index != i || s.Operations <= 0 || s.SampleType == "" || (i > 0 && (s.Operations != t.Samples[0].Operations || s.SampleType != t.Samples[0].SampleType)) {
				sequenceValid = false
			}
			for _, o := range s.Observations {
				if !snapshotMetric(o.Metric) || o.Unit != "bytes" || (o.Quality != "boundary_snapshot_only" && o.Quality != "engine_reported") {
					continue
				}
				identity := o
				identity.Value = nil
				identity.Status = ""
				identity.Reason = ""
				keyBytes, _ := json.Marshal(identity)
				key := string(keyBytes)
				g := groups[key]
				if g == nil {
					g = &group{measurement: identity, values: map[int][]protocol.Observation{}}
					groups[key] = g
				}
				g.values[i] = append(g.values[i], o)
			}
		}
		keys := make([]string, 0, len(groups))
		for k := range groups {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, key := range keys {
			g := groups[key]
			line := MemoryTimeline{Trial: t.ID, Runtime: t.Runtime, Workload: t.Workload, Scenario: t.Scenario, Block: t.Block, TrialStatus: t.Status, Measurement: g.measurement, Status: "incomplete_snapshots"}
			complete := true
			for i, s := range t.Samples {
				p := MemoryTimelinePoint{Sample: s.Index, Warmup: s.Warmup, Status: "not_recorded"}
				values := g.values[i]
				if len(values) > 1 {
					p.Status = "ambiguous_snapshot"
				} else if len(values) == 1 {
					o := values[0]
					p.Status = o.Status
					p.Reason = o.Reason
					if o.Status == "available" {
						if o.Value == nil || math.IsNaN(*o.Value) || math.IsInf(*o.Value, 0) || *o.Value < 0 {
							p.Status = "invalid_snapshot"
						} else {
							p.Value = protocol.Value(*o.Value)
						}
					}
				}
				if !s.Verified {
					p.Value = nil
					p.Status = "unverified_sample"
				}
				if p.Value == nil {
					complete = false
				}
				line.Points = append(line.Points, p)
			}
			switch {
			case !sequenceValid:
				line.Status = "invalid_sample_sequence"
			case t.Status != "ok":
				line.Status = "unsuccessful_trial"
			case len(line.Points) < 2:
				line.Status = "insufficient_samples"
			case complete:
				delta := *line.Points[len(line.Points)-1].Value - *line.Points[0].Value
				line.FirstLastChange = protocol.Value(delta)
				line.Status = "available"
			}
			out = append(out, line)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Trial < out[j].Trial })
	return out
}
