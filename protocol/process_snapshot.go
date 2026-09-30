package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strconv"
)

const ProcessSnapshotMode = "linux-process-cow-clone-v1"
const ProcessSnapshotArtifactSHA256 = "7a828c361cd0790044ac23e97c5c93838605c8f202088cabe990c4c1eb6d3381"

// This contract captures a quiescent import-free instance inside an owned Linux
// process. It is not an engine serialization or an active guest-stack snapshot.
type ProcessSnapshotContract struct {
	Mode string `json:"mode"`
}

// Linux PID plus /proc/PID/stat starttime is needed for later child-inclusive
// collection. A PID alone is not a durable process identity.
type SnapshotProcess struct {
	PID            uint32 `json:"pid"`
	StartTimeTicks string `json:"start_time_ticks"`
}

type ProcessSnapshotResult struct {
	PreForkThreads              uint32          `json:"pre_fork_threads"`
	Mode                        string          `json:"mode"`
	Boundary                    string          `json:"boundary"`
	ClockProcess                SnapshotProcess `json:"clock_process"`
	Source                      SnapshotProcess `json:"source"`
	Template                    SnapshotProcess `json:"template"`
	Restored                    SnapshotProcess `json:"restored"`
	AlternateRestored           SnapshotProcess `json:"alternate_restored"`
	SourceReleased              bool            `json:"source_released_before_restore"`
	ChildrenReaped              bool            `json:"children_reaped"`
	SourceAfterMutation         uint32          `json:"source_after_mutation"`
	RestoredBeforeWrite         uint32          `json:"restored_before_write"`
	RestoredAfterWrite          uint32          `json:"restored_after_write"`
	PassiveSegmentProbe         uint32          `json:"passive_segment_probe"`
	MemoryPages                 uint32          `json:"memory_pages"`
	TableElements               uint32          `json:"table_elements"`
	IndependentRestorations     uint32          `json:"independent_restorations"`
	MemoryAtRestoreSHA256       string          `json:"memory_at_restore_sha256"`
	MemoryAfterFirstWriteSHA256 string          `json:"memory_after_first_write_sha256"`
}

func ProcessSnapshotScenarios() []string {
	return []string{"process-snapshot-capture", "process-snapshot-restore", "process-snapshot-first-write", "process-snapshot-execute"}
}
func IsProcessSnapshotScenario(s string) bool { return slices.Contains(ProcessSnapshotScenarios(), s) }

// Capture/restore are process-control round trips, not fork API-return timings.
// First write is one embedding memory write, not isolated COW-fault latency.
func ProcessSnapshotBoundary(s string) string {
	switch s {
	case "process-snapshot-capture":
		return "source_fork_to_template_ready_ack"
	case "process-snapshot-restore":
		return "restore_request_to_child_ready_ack"
	case "process-snapshot-first-write":
		return "restored_child_single_byte_memory_write"
	case "process-snapshot-execute":
		return "restored_child_typed_check_call"
	default:
		return ""
	}
}

func ValidateProcessSnapshotWorkload(w Workload) error {
	if w.SnapshotDensity != nil {
		return fmt.Errorf("sequential snapshot cannot carry restored density")
	}
	if w.ProcessSnapshot == nil || w.ProcessSnapshot.Mode != ProcessSnapshotMode || w.SHA256 != ProcessSnapshotArtifactSHA256 || w.Schema != 1 || w.ABI != "core" || w.Generator != "wasmbench-process-snapshot-v1" || w.Export != "check" || w.Initialize != "seed" || w.Reset != "fresh_process_snapshot_per_sample" || w.WorkUnit != "process_snapshot_stage" || w.Units != 1 || w.Dimension != "" || w.Size != 0 || w.HostProfile != "" || len(w.Args) != 0 || w.Input != nil || w.Command != nil || w.Vectors != nil || w.Density != nil || w.Checkpoint != nil || w.GuestDensity != nil || w.Continuation != nil || w.Oracle.Kind != "linux_process_snapshot_v1" || !slices.Equal(w.Oracle.Expected, Values{64}) || w.Oracle.Float != nil || len(w.Oracle.Memory) != 0 || w.Oracle.ExpectedTrap != "" || w.Oracle.OutputPointerExport != "" {
		return fmt.Errorf("invalid quiescent Linux process snapshot fixture contract")
	}
	return nil
}

func ValidateProcessSnapshot(p *Preparation, r *RunRequest) error {
	if p == nil || r == nil {
		return fmt.Errorf("missing process snapshot preparation/request")
	}
	if err := ValidateProcessSnapshotWorkload(p.Workload); err != nil {
		return err
	}
	// Ordinary run is timing-only. Memory requires the separate held-live
	// inspection contract; root-only RSS is not clone memory evidence.
	if p.Profile != "timing" || r.PhaseBarriers || !IsProcessSnapshotScenario(r.Scenario) || r.Samples < 1 || r.Samples > 1000 || r.Operations != 1 || r.Warmup != 0 || r.SustainedDurationNS != 0 || r.SustainedPostCollection {
		return fmt.Errorf("process snapshot run requires timing, one operation, 1 to 1000 samples and no warmup/barriers; memory requires explicit snapshot inspection")
	}
	return nil
}

func ValidateProcessSnapshotDescription(d *Description) error {
	if d == nil || d.Runtime != "wasmtime" || d.Version != "46.0.1" || !slices.Contains([]string{"cranelift", "winch"}, d.Backend) || d.Embedding != "Rust API" || !d.Capabilities["can_linux_process_snapshot"] || d.Capabilities["can_snapshot"] || d.Configuration["process_snapshot_protocol"] != ProcessSnapshotMode || d.Configuration["process_snapshot_os"] != "linux" || d.Configuration["process_snapshot_spawn"] != "single_threaded_owned_process" || d.Configuration["parallel_compilation"] != "false" {
		return fmt.Errorf("adapter is not qualified for pinned Linux process snapshots")
	}
	if d.Configuration["cache"] != "disabled" {
		return fmt.Errorf("process snapshot adapter must disable code cache")
	}
	for _, scenario := range ProcessSnapshotScenarios() {
		if !slices.Contains(d.Scenarios, scenario) {
			return fmt.Errorf("process snapshot adapter omits stage %s", scenario)
		}
	}
	return nil
}

func ProcessSnapshotMemorySHA256(written bool) string {
	b := make([]byte, 3*65536)
	b[0] = 7
	if written {
		b[65535] = 22
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func VerifyProcessSnapshotSample(w Workload, scenario string, s Sample) error {
	if err := ValidateProcessSnapshotWorkload(w); err != nil {
		return err
	}
	e := s.ProcessSnapshotResult
	if e != nil && e.PreForkThreads != 1 {
		return fmt.Errorf("process snapshot fork must retain single-threaded evidence")
	}
	if e == nil || !IsProcessSnapshotScenario(scenario) || !s.Verified || s.Warmup || s.ElapsedNS < 0 || s.Index < 0 || s.Operations != 1 || s.SampleType != "individual_operation" || !slices.Equal(s.Result, Values{64}) || len(s.Observations) != 0 || e.Mode != ProcessSnapshotMode || e.Boundary != ProcessSnapshotBoundary(scenario) || !e.SourceReleased || !e.ChildrenReaped || e.SourceAfterMutation != 125 || e.RestoredBeforeWrite != 64 || e.RestoredAfterWrite != 64 || e.PassiveSegmentProbe != 127 || e.MemoryPages != 3 || e.TableElements != 3 || e.IndependentRestorations != 2 || e.MemoryAtRestoreSHA256 != ProcessSnapshotMemorySHA256(false) || e.MemoryAfterFirstWriteSHA256 != ProcessSnapshotMemorySHA256(true) || s.CheckpointResult != nil || s.GuestDensityResult != nil || s.ContinuationResult != nil || s.CommandResult != nil || s.TrapResult != nil || s.TierWindow != nil || s.SustainedWindow != nil || s.SustainedRelease != nil {
		return fmt.Errorf("invalid process snapshot stage/state evidence")
	}
	seen := map[SnapshotProcess]bool{}
	births := make([]uint64, 0, 4)
	for _, p := range []SnapshotProcess{e.Source, e.Template, e.Restored, e.AlternateRestored} {
		ticks, err := strconv.ParseUint(p.StartTimeTicks, 10, 64)
		if p.PID == 0 || p.PID > 2147483647 || seen[p] || err != nil || ticks == 0 || strconv.FormatUint(ticks, 10) != p.StartTimeTicks {
			return fmt.Errorf("invalid process snapshot lineage identity")
		}
		seen[p] = true
		births = append(births, ticks)
	}
	if e.Source.PID == e.Template.PID || e.Restored.PID == e.Source.PID || e.Restored.PID == e.Template.PID || e.AlternateRestored.PID == e.Source.PID || e.AlternateRestored.PID == e.Template.PID || births[1] < births[0] || births[2] < births[1] || births[3] < births[1] {
		return fmt.Errorf("impossible process snapshot ancestry")
	}
	wantClock := e.Source
	if scenario == "process-snapshot-first-write" || scenario == "process-snapshot-execute" {
		wantClock = e.Restored
	}
	if e.ClockProcess != wantClock {
		return fmt.Errorf("process snapshot elapsed clock belongs to wrong process")
	}
	return nil
}

func VerifyProcessSnapshotSequence(w Workload, r RunRequest, samples []Sample) error {
	if err := ValidateProcessSnapshot(&Preparation{Workload: w, Profile: "timing"}, &r); err != nil {
		return err
	}
	if len(samples) != r.Samples {
		return fmt.Errorf("process snapshot sample count differs from request")
	}
	seen := map[SnapshotProcess]bool{}
	var source SnapshotProcess
	for i, s := range samples {
		if err := VerifyProcessSnapshotSample(w, r.Scenario, s); err != nil {
			return err
		}
		if s.Index != i {
			return fmt.Errorf("process snapshot sample sequence is unordered")
		}
		e := s.ProcessSnapshotResult
		if i == 0 {
			source = e.Source
		} else if e.Source != source {
			return fmt.Errorf("process snapshot adapter root changed during batch")
		}
		for _, p := range []SnapshotProcess{e.Template, e.Restored, e.AlternateRestored} {
			if seen[p] {
				return fmt.Errorf("process snapshot reuses captured/restored process across samples")
			}
			seen[p] = true
		}
	}
	return nil
}
