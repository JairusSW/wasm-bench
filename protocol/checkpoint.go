package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
)

const GuestCheckpointMode = "eager-memory-scalar-v1"

// Guest checkpoints are NOT general runtime/whole-instance snapshots. This
// version permits only the exact generated module: fixed memory, one exported
// mutable i32, no imports, tables, passive segments, GC objects or active stack.
type CheckpointContract struct {
	Mode  string `json:"mode"`
	Pages uint32 `json:"pages"`
}

type CheckpointResult struct {
	Mode                 string `json:"mode"`
	PayloadBytes         uint64 `json:"payload_bytes"`
	SavedMemorySHA256    string `json:"saved_memory_sha256"`
	RestoredMemorySHA256 string `json:"restored_memory_sha256"`
	SavedGlobal          uint32 `json:"saved_global"`
	RestoredGlobal       uint32 `json:"restored_global"`
	SourceIndependent    bool   `json:"source_independent"`
}

func CheckpointScenarios() []string {
	return []string{"checkpoint-create", "checkpoint-restore", "checkpoint-first-write", "checkpoint-execute"}
}

func IsCheckpointScenario(s string) bool { return slices.Contains(CheckpointScenarios(), s) }

func ValidateCheckpointWorkload(w Workload) error {
	if w.GuestDensity != nil {
		return fmt.Errorf("checkpoint contract cannot include guest density")
	}
	c := w.Checkpoint
	if c == nil || c.Mode != GuestCheckpointMode || c.Pages < 1 || c.Pages > 64 {
		return fmt.Errorf("guest checkpoint requires eager-memory-scalar-v1 and 1 to 64 fixed memory pages")
	}
	if w.ABI != "core" || w.HostProfile != "" || w.Input != nil || w.Command != nil || w.Vectors != nil || w.Density != nil || len(w.Args) != 0 || w.Export != "benchmark" || w.Initialize != "initialize" || w.Reset != "fresh_instance_per_sample" || w.WorkUnit != "guest_checkpoint" || w.Units != 1 || w.Generator != "wasmbench-guest-checkpoint-v1" || w.Oracle.Kind != "guest_checkpoint_v1" || w.Oracle.Float != nil || len(w.Oracle.Memory) != 0 || w.Oracle.OutputPointerExport != "" || w.Oracle.ExpectedTrap != "" || len(w.Oracle.Expected) != 1 || w.Oracle.Expected[0] != uint64(c.Pages)*65536*7+42 {
		return fmt.Errorf("invalid fixed-memory scalar guest checkpoint execution contract")
	}
	return nil
}

func ValidateCheckpoint(p *Preparation, r *RunRequest) error {
	if p == nil || r == nil {
		return fmt.Errorf("missing guest checkpoint preparation/request")
	}
	if err := ValidateCheckpointWorkload(p.Workload); err != nil {
		return err
	}
	if !IsCheckpointScenario(r.Scenario) || (p.Profile != "timing" && p.Profile != "memory") || (r.PhaseBarriers && p.Profile != "memory") || r.Samples < 1 || r.Samples > 10000 || r.Operations != 1 || r.Warmup != 0 {
		return fmt.Errorf("guest checkpoint requires a checkpoint scenario, timing/memory, 1 to 10000 single-operation samples, no warmup; barriers require memory")
	}
	return nil
}

// CheckpointMemory is the canonical full state oracle, not a sampled checksum.
func CheckpointMemory(pages uint32, written bool) []byte {
	if pages < 1 || pages > 64 {
		return nil
	}
	b := make([]byte, int(pages)*65536)
	for i := range b {
		b[i] = 7
	}
	if written {
		b[len(b)-1] = 11
	}
	return b
}

func CheckpointMemorySHA256(pages uint32, written bool) string {
	s := sha256.Sum256(CheckpointMemory(pages, written))
	return hex.EncodeToString(s[:])
}

func VerifyCheckpointSample(w Workload, scenario string, s Sample) error {
	if s.GuestDensityResult != nil {
		return fmt.Errorf("checkpoint sample cannot include guest density")
	}
	if err := ValidateCheckpointWorkload(w); err != nil {
		return err
	}
	e := s.CheckpointResult
	if !IsCheckpointScenario(scenario) || e == nil {
		return fmt.Errorf("missing guest checkpoint scenario/evidence")
	}
	written := scenario == "checkpoint-first-write"
	state, result := uint32(42), w.Oracle.Expected[0]
	if written {
		state = 43
		result += 5
	}
	if !s.Verified || s.Warmup || s.ElapsedNS < 0 || s.Operations != 1 || s.SampleType != "individual_operation" || len(s.Result) != 1 || s.Result[0] != result || e.Mode != GuestCheckpointMode || e.PayloadBytes != uint64(w.Checkpoint.Pages)*65536+4 || e.SavedGlobal != 42 || e.RestoredGlobal != state || !e.SourceIndependent || e.SavedMemorySHA256 != CheckpointMemorySHA256(w.Checkpoint.Pages, false) || e.RestoredMemorySHA256 != CheckpointMemorySHA256(w.Checkpoint.Pages, written) || s.CommandResult != nil || s.TrapResult != nil {
		return fmt.Errorf("incorrect guest checkpoint state or measurement evidence")
	}
	return nil
}
