package experiment

import (
	"fmt"
	"github.com/wasmbench/wasmbench/protocol"
)

func ValidateSustainedEvidence(b Bundle) error {
	ws := map[string]protocol.Workload{}
	for _, w := range b.Manifest.Lock.Workloads {
		ws[w.ID] = w
	}
	for _, t := range b.Trials {
		if t.Scenario != "sustained" {
			for _, s := range t.Samples {
				if s.SustainedWindow != nil || s.SustainedRelease != nil {
					return fmt.Errorf("sustained evidence outside sustained scenario")
				}
			}
			continue
		}
		if t.Status != "ok" && t.Status != "duration_budget_not_met" {
			continue
		}
		w, ok := ws[t.Workload]
		if !ok || t.Block < 0 || t.Profile != b.Manifest.Lock.Options.Profile {
			return fmt.Errorf("sustained trial identity/profile mismatch")
		}
		r := trialRequest(b.Manifest.Lock.Options, w, t.Scenario, t.Block)
		if err := protocol.ValidateSustained(&protocol.Preparation{Workload: w, Profile: t.Profile}, &r); err != nil {
			return err
		}
		total, err := protocol.VerifySustainedSequence(w, r, t.Samples)
		if err != nil {
			return err
		}
		if t.Samples[len(t.Samples)-1].SustainedRelease.EndNS > t.DurationNS {
			return fmt.Errorf("sustained clocks exceed observed trial lifetime")
		}
		if c := t.Samples[len(t.Samples)-1].SustainedRelease.PostCollection; c != nil && c.EndNS > t.DurationNS {
			return fmt.Errorf("post-collection clocks exceed observed trial lifetime")
		}
		qualified := total >= r.SustainedDurationNS
		if (t.Status == "ok") != qualified {
			return fmt.Errorf("sustained duration qualification contradicts raw operation times")
		}
		if len(t.PhaseEvents) > 0 {
			return fmt.Errorf("sustained experiment does not support phase barriers")
		}
		for _, s := range t.Samples {
			for _, o := range s.Observations {
				if o.Collector == "node:process.memoryUsage" {
					if t.Profile != "memory" || o.DefinitionVersion != 1 || o.Scope != "adapter_process_v8_heap" || o.Quality != "engine_reported" || o.Profile != "memory" || o.Unit != "bytes" || (o.Metric != "host.js_heap.start" && o.Metric != "host.js_heap.end") {
						return fmt.Errorf("invalid sustained V8 heap domain")
					}
					if o.Phase == "sustained" {
						if o.Denominator != "batch_operation_window_including_heap_snapshots_excluding_verification" {
							return fmt.Errorf("invalid sustained V8 heap bracket")
						}
					} else if o.Phase == "sustained/release_window" {
						if s.SustainedRelease == nil || s.SustainedRelease.Policy != "js_references_dropped" || o.Denominator != "js_module_instance_export_reference_release_without_forced_gc" {
							return fmt.Errorf("invalid V8 reference-release heap window")
						}
					} else {
						return fmt.Errorf("unknown sustained V8 heap phase")
					}
				}
				if o.Collector == "runtime.ReadMemStats" {
					if o.Phase != "sustained" && o.Phase != "sustained/release_window" {
						return fmt.Errorf("unknown sustained allocator window")
					}
					if t.Profile != "memory" || o.DefinitionVersion != 1 || o.Scope != "adapter_process_go_heap" || o.Quality != "engine_reported" || o.Profile != "memory" {
						return fmt.Errorf("invalid sustained allocator domain")
					}
					if o.Phase == "sustained" && o.Denominator != "batch_operation_window_including_allocator_snapshots_excluding_verification" {
						return fmt.Errorf("invalid sustained operation allocator window")
					}
					if o.Phase == "sustained/release_window" && (s.SustainedRelease == nil || o.Denominator != "logical_engine_module_instance_release_without_forced_gc") {
						return fmt.Errorf("invalid sustained release allocator window")
					}
				}
			}
		}
	}
	return nil
}
