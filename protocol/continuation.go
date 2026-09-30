package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
)

const NativeContinuationMode = "wazero-native-execution-stack-v1"
const ContinuationDenominator = "single_native_continuation_stage_excluding_setup_verification_release"

// ContinuationContract never denotes heap, globals, whole-instance or COW state.
type ContinuationContract struct {
	Mode  string `json:"mode"`
	Depth int    `json:"depth"`
}

type ContinuationResult struct {
	Mode                     string `json:"mode"`
	Depth                    int    `json:"depth"`
	StackResult              uint32 `json:"stack_result"`
	GuestResumed             bool   `json:"guest_resumed"`
	MemoryAfterRestoreSHA256 string `json:"memory_after_restore_sha256"`
	MemoryAfterWriteSHA256   string `json:"memory_after_write_sha256"`
	GlobalAfterRestore       uint32 `json:"global_after_restore"`
	GlobalAfterWrite         uint32 `json:"global_after_write"`
}

func ContinuationScenarios() []string {
	return []string{"continuation-create", "continuation-resume", "continuation-first-write", "continuation-execute"}
}
func IsContinuationScenario(s string) bool { return slices.Contains(ContinuationScenarios(), s) }

func ValidateContinuationWorkload(w Workload) error {
	c := w.Continuation
	if c == nil || c.Mode != NativeContinuationMode || c.Depth < 0 || c.Depth > 128 {
		return fmt.Errorf("native continuation requires wazero-native-execution-stack-v1 and depth 0 to 128")
	}
	if w.Dimension != "recursive_ancestor_depth" || w.Size != c.Depth {
		return fmt.Errorf("native continuation scaling dimension/size must match actual recursive ancestor depth")
	}
	if w.ABI != "core" || w.Generator != "wasmbench-native-continuation-v1" || w.Export != "run" || w.Initialize != "" || w.Reset != "fresh_instance_per_sample" || w.WorkUnit != "native_continuation_stage" || w.Units != 1 || w.HostProfile != "" || len(w.Args) != 0 || w.Input != nil || w.Command != nil || w.Vectors != nil || w.Density != nil || w.Checkpoint != nil || w.GuestDensity != nil || w.Oracle.Kind != "native_continuation_v1" || w.Oracle.Float != nil || len(w.Oracle.Memory) != 0 || w.Oracle.OutputPointerExport != "" || w.Oracle.ExpectedTrap != "" || !slices.Equal(w.Oracle.Expected, Values{133}) {
		return fmt.Errorf("invalid native execution-stack continuation workload")
	}
	return nil
}

func ValidateContinuation(p *Preparation, r *RunRequest) error {
	if p == nil || r == nil {
		return fmt.Errorf("missing native continuation preparation/request")
	}
	if err := ValidateContinuationWorkload(p.Workload); err != nil {
		return err
	}
	if !IsContinuationScenario(r.Scenario) || (p.Profile != "timing" && p.Profile != "memory") || (r.PhaseBarriers && p.Profile != "memory") || r.Samples < 1 || r.Samples > 10000 || r.Operations != 1 || r.Warmup != 0 || r.SustainedDurationNS != 0 || r.SustainedPostCollection {
		return fmt.Errorf("native continuation requires timing/memory, a continuation stage, 1 to 10000 single-operation samples and no warmup; barriers require memory")
	}
	return nil
}

func ContinuationMemorySHA256(written bool) string {
	b := make([]byte, 65536)
	b[0] = 11
	if written {
		b[65535] = 22
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func VerifyContinuationSample(w Workload, scenario string, s Sample) error {
	if err := ValidateContinuationWorkload(w); err != nil {
		return err
	}
	e := s.ContinuationResult
	if !IsContinuationScenario(scenario) || e == nil || !s.Verified || s.Warmup || s.ElapsedNS < 0 || s.Operations != 1 || s.SampleType != "individual_operation" || !slices.Equal(s.Result, Values{133}) || e.Mode != NativeContinuationMode || e.Depth != w.Continuation.Depth || e.StackResult != uint32(7+e.Depth*(e.Depth+1)/2) || !e.GuestResumed || e.MemoryAfterRestoreSHA256 != ContinuationMemorySHA256(false) || e.MemoryAfterWriteSHA256 != ContinuationMemorySHA256(true) || e.GlobalAfterRestore != 99 || e.GlobalAfterWrite != 100 || s.CheckpointResult != nil || s.GuestDensityResult != nil || s.CommandResult != nil || s.TrapResult != nil || s.TierWindow != nil || s.SustainedWindow != nil || s.SustainedRelease != nil {
		return fmt.Errorf("incorrect native execution-stack continuation measurement evidence")
	}
	return nil
}
