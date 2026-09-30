package analysis

import (
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
	"math"
)

const SustainedVersion = "retained-instance-session-v3"

type SustainedPoint struct {
	Sample     int                      `json:"sample"`
	Warmup     bool                     `json:"warmup"`
	Window     protocol.SustainedWindow `json:"window"`
	Operations int                      `json:"operations"`
	LatencyNS  float64                  `json:"api_ns_per_operation"`
	AllocBytes *float64                 `json:"go_allocated_bytes"`
	AllocRate  *float64                 `json:"go_allocated_bytes_per_diagnostic_second"`
	GCCycles   *float64                 `json:"go_gc_cycles"`
	HeapEnd    *float64                 `json:"go_heap_end_bytes"`
	JSHeapEnd  *float64                 `json:"v8_heap_end_bytes"`
}

type SustainedSession struct {
	Trial               string                     `json:"trial"`
	Runtime             string                     `json:"runtime"`
	Workload            string                     `json:"workload"`
	Profile             string                     `json:"profile"`
	Status              string                     `json:"status"`
	Reason              string                     `json:"reason,omitempty"`
	TargetNS            int64                      `json:"target_measured_api_ns"`
	MeasuredNS          int64                      `json:"measured_api_ns"`
	Points              []SustainedPoint           `json:"points"`
	Release             *protocol.SustainedRelease `json:"logical_release"`
	ReleaseObservations []protocol.Observation     `json:"release_observations"`
}

// Rates use the sample's observed diagnostic bracket, including snapshots and
// excluding verification/gaps. Never divide heap endpoint changes by time.
func SustainedSessions(b experiment.Bundle) []SustainedSession {
	b = hostEligibleBundle(b)
	var out []SustainedSession
	ws := map[string]protocol.Workload{}
	for _, w := range b.Manifest.Lock.Workloads {
		ws[w.ID] = w
	}
	for _, t := range b.Trials {
		if t.Block < 0 || t.Scenario != "sustained" {
			continue
		}
		session := SustainedSession{Trial: t.ID, Runtime: t.Runtime, Workload: t.Workload, Profile: t.Profile, Status: t.Status, Reason: t.Reason, TargetNS: int64(b.Manifest.Lock.Options.SustainedDuration), Points: []SustainedPoint{}}
		if t.Status != "ok" && t.Status != "duration_budget_not_met" {
			out = append(out, session)
			continue
		}
		w := ws[t.Workload]
		r := protocol.RunRequest{Scenario: "sustained", Samples: b.Manifest.Lock.Options.Samples, Operations: b.Manifest.Lock.Options.Operations, Warmup: b.Manifest.Lock.Options.Warmup, SustainedDurationNS: session.TargetNS, SustainedPostCollection: b.Manifest.Lock.Options.SustainedPostCollection}
		if err := protocol.ValidateSustained(&protocol.Preparation{Workload: w, Profile: t.Profile}, &r); err != nil {
			session.Status = "unavailable"
			session.Reason = err.Error()
			out = append(out, session)
			continue
		}
		total, err := protocol.VerifySustainedSequence(w, r, t.Samples)
		if err != nil {
			session.Status = "unavailable"
			session.Reason = err.Error()
			out = append(out, session)
			continue
		}
		session.MeasuredNS = total
		if (t.Status == "ok") != (total >= session.TargetNS) {
			session.Status = "invalid_duration_qualification"
			session.Reason = "trial status contradicts cumulative measured API duration"
		}
		for _, s := range t.Samples {
			p := SustainedPoint{Sample: s.Index, Warmup: s.Warmup, Window: *s.SustainedWindow, Operations: s.Operations, LatencyNS: float64(s.ElapsedNS) / float64(s.Operations)}
			metric := func(name string) *float64 {
				var value *float64
				count := 0
				for _, o := range s.Observations {
					unit := "bytes"
					if name == "host.gc.cycles" {
						unit = "count"
					}
					if o.Metric == name && o.DefinitionVersion == 1 && o.Unit == unit && o.Phase == "sustained" && o.Status == "available" && o.Value != nil && o.Scope == "adapter_process_go_heap" && o.Collector == "runtime.ReadMemStats" && o.Quality == "engine_reported" && o.Profile == "memory" && o.Denominator == "batch_operation_window_including_allocator_snapshots_excluding_verification" && !math.IsNaN(*o.Value) && !math.IsInf(*o.Value, 0) && *o.Value >= 0 {
						count++
						value = o.Value
					}
				}
				if count != 1 {
					return nil
				}
				return value
			}
			if t.Profile == "memory" {
				var jsCount int
				for _, o := range s.Observations {
					if o.Metric == "host.js_heap.end" && o.DefinitionVersion == 1 && o.Unit == "bytes" && o.Phase == "sustained" && o.Status == "available" && o.Value != nil && o.Scope == "adapter_process_v8_heap" && o.Collector == "node:process.memoryUsage" && o.Quality == "engine_reported" && o.Profile == "memory" && o.Denominator == "batch_operation_window_including_heap_snapshots_excluding_verification" && !math.IsNaN(*o.Value) && !math.IsInf(*o.Value, 0) && *o.Value >= 0 {
						jsCount++
						p.JSHeapEnd = o.Value
					}
				}
				if jsCount != 1 {
					p.JSHeapEnd = nil
				}
				p.AllocBytes = metric("host.alloc.bytes")
				p.GCCycles = metric("host.gc.cycles")
				p.HeapEnd = metric("host.heap.end")
				if p.AllocBytes != nil && p.Window.EndNS > p.Window.StartNS {
					p.AllocRate = protocol.Value(*p.AllocBytes * 1e9 / float64(p.Window.EndNS-p.Window.StartNS))
				}
			}
			session.Points = append(session.Points, p)
			if s.SustainedRelease != nil {
				session.Release = s.SustainedRelease
				for _, o := range s.Observations {
					if o.Phase == "sustained/release_window" {
						session.ReleaseObservations = append(session.ReleaseObservations, o)
					}
				}
			}
		}
		out = append(out, session)
	}
	return out
}
