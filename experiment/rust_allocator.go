package experiment

import (
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"

	"github.com/wasmbench/wasmbench/protocol"
)

func ValidateRustAllocatorEvidence(b Bundle) error {
	runtimes := map[string]*protocol.Description{}
	for _, r := range b.Manifest.Lock.Runtimes {
		runtimes[r.ID] = r.Description
	}
	for _, t := range b.Trials {
		for _, o := range t.Observations {
			if strings.HasPrefix(o.Metric, "host.rust.") {
				return fmt.Errorf("Rust allocator operation evidence must be sample qualified")
			}
		}
		d := runtimes[t.Runtime]
		instrumented := d != nil && d.Capabilities["requires_memory_profile"]
		releaseEnabled := d != nil && d.Capabilities["can_rust_allocator_release_windows"]
		for _, s := range t.Samples {
			for _, o := range s.Observations {
				if strings.HasPrefix(o.Metric, "host.rust.release.") && (!instrumented || !releaseEnabled) {
					return fmt.Errorf("logical-release evidence outside locked release-enabled allocator build")
				}
				if strings.HasPrefix(o.Metric, "host.rust.") && (!instrumented || t.Profile != "memory") {
					return fmt.Errorf("Rust allocator evidence outside locked instrumented memory build")
				}
			}
		}
		if !instrumented || t.Status != "ok" {
			continue
		}
		if len(t.Samples) == 0 {
			return fmt.Errorf("successful allocator trial omitted samples")
		}
		var workload protocol.Workload
		found := false
		for _, w := range b.Manifest.Lock.Workloads {
			if w.ID == t.Workload {
				workload, found = w, true
				break
			}
		}
		if !found {
			return fmt.Errorf("allocator trial has no locked workload")
		}
		if err := protocol.ValidateRustAllocatorWorkload(workload, t.Scenario); err != nil {
			return err
		}
		if err := validateSampleSequence(trialRequest(b.Manifest.Lock.Options, workload, t.Scenario, t.Block), t.Samples); err != nil {
			return err
		}
		if t.Profile != "memory" || b.Manifest.Lock.Options.Profile != "memory" || d.Runtime != "wasmtime" || !d.Capabilities["can_measure_host_allocations"] || !strings.HasPrefix(d.Configuration["allocator_instrumentation"], protocol.RustAllocatorVersion+";") || !slices.Contains([]string{"compile", "instantiate", "first-call", "steady"}, t.Scenario) {
			return fmt.Errorf("invalid instrumented allocator runtime or profile")
		}
		if releaseEnabled && !strings.HasPrefix(d.Configuration["allocator_release_policy"], "rust-logical-release-v1;") {
			return fmt.Errorf("allocator release policy missing or unsupported")
		}
		phased := b.Manifest.Lock.Options.PhaseBarriers && t.Block >= 0
		if phased && (!d.Capabilities["can_rust_allocator_phase_boundaries"] || d.Configuration["allocator_phase_boundary_protocol"] != protocol.RustAllocatorBoundaryVersion) {
			return fmt.Errorf("allocator phase-boundary protocol missing or unsupported")
		}
		if err := validateRustAllocatorBoundaries(t, phased); err != nil {
			return err
		}
		for i, s := range t.Samples {
			if err := protocol.ValidateRustAllocatorSample(s, t.Scenario); err != nil {
				return err
			}
			if releaseEnabled {
				if err := protocol.ValidateRustAllocatorRelease(s, t.Scenario, i+1 == len(t.Samples)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateRustAllocatorBoundaries(t Trial, phased bool) error {
	if !phased {
		if len(t.PhaseEvents) != 0 {
			return fmt.Errorf("allocator boundaries recorded outside phased request")
		}
		return nil
	}
	stages := protocol.RustAllocatorPhaseStages(t.Scenario)
	if len(stages) != 4 || len(t.PhaseEvents) != len(t.Samples)*len(stages) {
		return fmt.Errorf("incomplete allocator boundary sequence")
	}
	for index, sample := range t.Samples {
		var attached []protocol.Observation
		for j, stage := range stages {
			record := t.PhaseEvents[index*len(stages)+j]
			if record.Event.SampleIndex != index || record.Event.Stage != stage {
				return fmt.Errorf("invalid allocator boundary order")
			}
			seen := map[string]bool{}
			for _, o := range record.Observations {
				if !slices.Contains([]string{"process.rss", "process.pss", "process.private", "process.virtual"}, o.Metric) {
					continue
				}
				if seen[o.Metric] || o.Phase != t.Scenario+"/"+stage || o.Scope != "adapter_process" || o.Unit != "bytes" || o.DefinitionVersion != 1 || o.Profile != "memory" || o.Quality != "boundary_snapshot_only" || o.Denominator != "process" || o.Collector != "procfs" || o.CollectorVersion != "1" {
					return fmt.Errorf("invalid allocator process boundary provenance")
				}
				if o.Status == "available" {
					if o.Value == nil || math.IsNaN(*o.Value) || math.IsInf(*o.Value, 0) || *o.Value < 0 || *o.Value > 9007199254740991 || math.Trunc(*o.Value) != *o.Value {
						return fmt.Errorf("invalid allocator process boundary value")
					}
				} else if !slices.Contains([]string{"unsupported", "unavailable", "permission_denied"}, o.Status) || o.Value != nil || o.Reason == "" {
					return fmt.Errorf("invalid allocator process boundary missing outcome")
				}
				seen[o.Metric] = true
			}
			if len(seen) != 4 {
				return fmt.Errorf("allocator boundary omitted process snapshot outcomes")
			}
			attached = append(attached, record.Observations...)
		}
		if len(sample.Observations) < len(attached) || !reflect.DeepEqual(sample.Observations[len(sample.Observations)-len(attached):], attached) {
			return fmt.Errorf("allocator boundary observations differ from attached sample evidence")
		}
	}
	return nil
}
