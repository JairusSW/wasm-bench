package protocol

import (
	"fmt"
	"math"
	"strings"
)

const RustAllocatorCollector = "wasmbench Rust GlobalAlloc/System"
const RustAllocatorVersion = "rust-global-alloc-v1"
const RustAllocatorScope = "allocations_routed_through_rust_global_allocator"
const RustAllocatorBoundaryVersion = "rust-allocator-boundaries-v1"

// These handshakes surround API and logical-release windows, not inferred
// compiler subphases. Steady's final boundary is a release decision: non-final
// samples retain their Store. They must not be labeled physically reclaimed.
func RustAllocatorPhaseStages(scenario string) []string {
	switch scenario {
	case "compile":
		return []string{"before_compile", "compiled", "compile_release_entry", "compile_release_completed"}
	case "instantiate":
		return []string{"before_instantiate", "instantiated", "instantiate_release_entry", "instantiate_release_completed"}
	case "first-call":
		return []string{"before_first_call", "first_call_returned", "first_call_release_entry", "first_call_release_completed"}
	case "steady":
		return []string{"before_steady_batch", "steady_batch_returned", "steady_release_entry", "steady_release_decision_completed"}
	default:
		return nil
	}
}

// The allocator build uses the scalar core embedding path, not command,
// vector, checkpoint or instance-group execution. Fresh-state workloads are
// valid only where each sample already constructs a fresh measured resource.
func ValidateRustAllocatorWorkload(w Workload, scenario string) error {
	if w.ABI != "core" || (w.Oracle.Kind != "exact_u64" && w.Oracle.Kind != "float_bits_v1") || w.Command != nil || w.Vectors != nil || w.Density != nil || w.Checkpoint != nil || w.GuestDensity != nil || (w.Reset != "stateless" && (w.Reset != "fresh_instance_per_sample" || scenario == "steady")) {
		return fmt.Errorf("Rust allocator diagnostics require scalar exact/float core workload; steady requires stateless reset")
	}
	return nil
}

func ValidateRustAllocatorSample(s Sample, scenario string) error {
	if s.Operations != 1 || s.Warmup || !s.Verified || s.SampleType != "individual_operation" || s.ElapsedNS < 0 {
		return fmt.Errorf("invalid Rust allocator sample boundary")
	}
	units := map[string]string{"host.rust.alloc.bytes": "bytes", "host.rust.alloc.count": "count", "host.rust.freed.bytes": "bytes", "host.rust.outstanding.start": "bytes", "host.rust.outstanding.end": "bytes", "host.rust.outstanding.observed_peak": "bytes"}
	values := map[string]uint64{}
	for _, o := range s.Observations {
		if strings.HasPrefix(o.Metric, "host.rust.release.") {
			continue
		}
		if !strings.HasPrefix(o.Metric, "host.rust.") {
			continue
		}
		unit, known := units[o.Metric]
		_, duplicate := values[o.Metric]
		if !known || duplicate || o.Value == nil || math.IsNaN(*o.Value) || math.IsInf(*o.Value, 0) || *o.Value < 0 || *o.Value > 9007199254740991 || math.Trunc(*o.Value) != *o.Value || o.Unit != unit || o.DefinitionVersion != 1 || o.Scope != RustAllocatorScope || o.Phase != scenario || o.Collector != RustAllocatorCollector || o.CollectorVersion != RustAllocatorVersion || o.Quality != "instrumented" || o.Profile != "memory" || o.Status != "available" || o.Denominator != "single_embedding_api_operation" {
			return fmt.Errorf("invalid Rust allocator observation contract")
		}
		values[o.Metric] = uint64(*o.Value)
	}
	if len(values) != len(units) {
		return fmt.Errorf("incomplete Rust allocator observations")
	}
	start, end, allocated, freed, peak := values["host.rust.outstanding.start"], values["host.rust.outstanding.end"], values["host.rust.alloc.bytes"], values["host.rust.freed.bytes"], values["host.rust.outstanding.observed_peak"]
	if start+allocated < freed || start+allocated-freed != end || peak < start || peak < end || peak > start+allocated {
		return fmt.Errorf("inconsistent Rust allocator request accounting")
	}
	return nil
}

// Release is a distinct diagnostic window after correctness verification.
// Non-final steady samples retain their Store and have no numerical release
// observations, not zero-valued resource reclamation.
func ValidateRustAllocatorRelease(s Sample, scenario string, final bool) error {
	units := map[string]string{"host.rust.release.alloc.bytes": "bytes", "host.rust.release.alloc.count": "count", "host.rust.release.freed.bytes": "bytes", "host.rust.release.outstanding.start": "bytes", "host.rust.release.outstanding.end": "bytes", "host.rust.release.outstanding.observed_peak": "bytes", "host.rust.release.elapsed": "ns"}
	performed := scenario != "steady" || final
	seen := map[string]bool{}
	values := map[string]uint64{}
	for _, o := range s.Observations {
		if !strings.HasPrefix(o.Metric, "host.rust.release.") {
			continue
		}
		unit, known := units[o.Metric]
		if !known || seen[o.Metric] || o.Unit != unit || o.DefinitionVersion != 1 || o.Scope != RustAllocatorScope || o.Phase != scenario+"/logical_release_window" || o.Collector != RustAllocatorCollector || o.CollectorVersion != RustAllocatorVersion || o.Quality != "instrumented" || o.Profile != "memory" || o.Denominator != "single_logical_release_operation" {
			return fmt.Errorf("invalid Rust logical-release observation identity")
		}
		seen[o.Metric] = true
		if !performed {
			if o.Value != nil || o.Status != "not_applicable" || o.Reason != "steady Store retained across samples; release on final sample" {
				return fmt.Errorf("retained steady instance cannot claim release values")
			}
			continue
		}
		if o.Status != "available" || o.Value == nil || math.IsNaN(*o.Value) || math.IsInf(*o.Value, 0) || *o.Value < 0 || *o.Value > 9007199254740991 || math.Trunc(*o.Value) != *o.Value {
			return fmt.Errorf("invalid Rust logical-release value")
		}
		values[o.Metric] = uint64(*o.Value)
	}
	if len(seen) != len(units) {
		return fmt.Errorf("incomplete Rust logical-release observations")
	}
	if performed {
		start, end, allocated, freed, peak := values["host.rust.release.outstanding.start"], values["host.rust.release.outstanding.end"], values["host.rust.release.alloc.bytes"], values["host.rust.release.freed.bytes"], values["host.rust.release.outstanding.observed_peak"]
		if start+allocated < freed || start+allocated-freed != end || peak < start || peak < end || peak > start+allocated {
			return fmt.Errorf("inconsistent Rust logical-release accounting")
		}
	}
	return nil
}
