package protocol

import (
	"fmt"
	"math"
	"slices"
)

// Times are monotonic offsets from a session epoch after instance setup.
// Window includes diagnostic snapshots, not verification or inter-batch gaps.
type SustainedWindow struct {
	StartNS          int64 `json:"start_ns"`
	OperationStartNS int64 `json:"operation_start_ns"`
	OperationEndNS   int64 `json:"operation_end_ns"`
	EndNS            int64 `json:"end_ns"`
}

// Release distinguishes runtime close from JS reference dropping. Neither is
// evidence of allocator or physical-page reclamation.
type SustainedRelease struct {
	Policy         string               `json:"policy,omitempty"`
	StartNS        int64                `json:"start_ns"`
	EndNS          int64                `json:"end_ns"`
	Closed         bool                 `json:"closed"`
	PostCollection *SustainedCollection `json:"post_collection,omitempty"`
}

const SustainedCollectionDenominator = "one_forced_go_gc_after_logical_release_with_sample_evidence_retained"

type SustainedCollection struct {
	StartNS      int64         `json:"start_ns"`
	EndNS        int64         `json:"end_ns"`
	Policy       string        `json:"policy"`
	Observations []Observation `json:"observations"`
}

func ValidateSustained(p *Preparation, r *RunRequest) error {
	if p == nil || r == nil {
		return fmt.Errorf("missing sustained preparation/request")
	}
	w := p.Workload
	if w.GuestDensity != nil {
		return fmt.Errorf("sustained cannot include guest density")
	}
	if r.SustainedPostCollection && p.Profile != "memory" {
		return fmt.Errorf("sustained post-collection requires a dedicated memory pass")
	}
	if r.Scenario != "sustained" || !slices.Contains([]string{"timing", "memory"}, p.Profile) || r.PhaseBarriers || w.ABI != "core" || w.Reset != "stateless" || w.Oracle.Kind != "exact_u64" || w.Oracle.Float != nil || w.Command != nil || w.Vectors != nil || w.Density != nil || w.Checkpoint != nil || w.Export == "" || len(w.Oracle.Expected) == 0 {
		return fmt.Errorf("sustained requires stateless core exact scalar timing/memory without phase barriers")
	}
	if r.Samples < 2 || r.Samples > 100000 || r.Operations < 1 || r.Operations > 1000000 || r.Warmup < 0 || r.Warmup > 10000 || r.SustainedDurationNS < 1000000 || r.SustainedDurationNS > 3600000000000 {
		return fmt.Errorf("sustained requires 2 to 100000 fixed samples, bounded batches/warmup and a 1 ms to 1 h measured-operation duration target")
	}
	return nil
}

// VerifySustainedSequence rechecks clocks, operation counts, warmup, scalar
// oracle and logical release. It returns measured API time, NOT wall/service
// throughput time; warmup, diagnostics, verification and gaps are excluded.
func VerifySustainedSequence(w Workload, r RunRequest, samples []Sample) (int64, error) {
	if len(samples) != r.Samples+r.Warmup {
		return 0, fmt.Errorf("sustained fixed sample budget mismatch")
	}
	var total, last int64
	for i, s := range samples {
		p := s.SustainedWindow
		typeName := "batch_average"
		if r.Operations == 1 {
			typeName = "individual_operation"
		}
		if p == nil || s.Index != i || s.Warmup != (i < r.Warmup) || !s.Verified || s.Operations != r.Operations || s.SampleType != typeName || s.ElapsedNS < 0 || !slices.Equal([]uint64(s.Result), []uint64(w.Oracle.Expected)) || s.CommandResult != nil || s.TrapResult != nil || s.CheckpointResult != nil || p.StartNS < last || p.OperationStartNS < p.StartNS || p.OperationEndNS < p.OperationStartNS || p.EndNS < p.OperationEndNS || p.OperationEndNS-p.OperationStartNS != s.ElapsedNS {
			return 0, fmt.Errorf("invalid sustained state/clock evidence at sample %d", i)
		}
		last = p.EndNS
		if !s.Warmup {
			if total > math.MaxInt64-s.ElapsedNS {
				return 0, fmt.Errorf("sustained duration overflow")
			}
			total += s.ElapsedNS
		}
		if i < len(samples)-1 && s.SustainedRelease != nil {
			return 0, fmt.Errorf("sustained released before final sample")
		}
	}
	if len(samples) == 0 {
		return 0, fmt.Errorf("missing sustained samples")
	}
	release := samples[len(samples)-1].SustainedRelease
	if release == nil || release.StartNS < last || release.EndNS < release.StartNS {
		return 0, fmt.Errorf("missing or invalid logical sustained release")
	}
	if (release.Policy == "" || release.Policy == "runtime_closed") && !release.Closed || release.Policy == "js_references_dropped" && release.Closed || release.Policy != "" && release.Policy != "runtime_closed" && release.Policy != "js_references_dropped" {
		return 0, fmt.Errorf("sustained release policy contradicts close evidence")
	}
	if release.Policy == "js_references_dropped" && r.SustainedPostCollection {
		return 0, fmt.Errorf("Go post-collection unsupported for JS reference release")
	}
	c := release.PostCollection
	if !r.SustainedPostCollection && c != nil {
		return 0, fmt.Errorf("unrequested sustained post-collection")
	}
	if r.SustainedPostCollection {
		if c == nil || c.Policy != "one_forced_go_gc" || c.StartNS < release.EndNS || c.EndNS < c.StartNS {
			return 0, fmt.Errorf("missing or invalid sustained post-collection clocks/policy")
		}
		seen := map[string]bool{}
		for _, o := range c.Observations {
			unit := "bytes"
			switch o.Metric {
			case "host.alloc.bytes", "host.heap.start", "host.heap.end":
			case "host.alloc.count", "host.gc.cycles", "host.gc.forced_cycles":
				unit = "count"
			case "host.gc.pause_time":
				unit = "ns"
			default:
				return 0, fmt.Errorf("unknown post-collection allocator metric")
			}
			if seen[o.Metric] || o.DefinitionVersion != 1 || o.Unit != unit || o.Phase != "sustained/post_collection" || o.Scope != "adapter_process_go_heap" || o.Collector != "runtime.ReadMemStats" || o.CollectorVersion == "" || o.Profile != "memory" || o.Quality != "engine_reported" || o.Denominator != SustainedCollectionDenominator || o.Status != "available" || o.Value == nil || math.IsNaN(*o.Value) || math.IsInf(*o.Value, 0) || *o.Value < 0 {
				return 0, fmt.Errorf("invalid post-collection allocator evidence")
			}
			if o.Metric == "host.gc.forced_cycles" && *o.Value < 1 {
				return 0, fmt.Errorf("post-collection has no observed forced GC")
			}
			if unit == "count" && math.Trunc(*o.Value) != *o.Value {
				return 0, fmt.Errorf("fractional post-collection counter")
			}
			seen[o.Metric] = true
		}
		if len(seen) != 7 {
			return 0, fmt.Errorf("incomplete post-collection allocator evidence")
		}
	}
	return total, nil
}
