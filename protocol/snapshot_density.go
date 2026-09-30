package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strconv"
)

const SnapshotDensityBoundaryVersion = "linux-process-snapshot-density-boundary-v1"
const SnapshotDensityWorkerVersion = "linux-process-snapshot-density-worker-v1"

// This is a qualification seam, not admission for a product workload or a
// generic snapshot capability. All members must be held live simultaneously.
type SnapshotDensityBoundary struct {
	SampleIndex int               `json:"sample_index,omitempty"`
	Version     string            `json:"version"`
	Stage       string            `json:"stage"`
	Instances   int               `json:"instances"`
	Source      SnapshotProcess   `json:"source"`
	Template    SnapshotProcess   `json:"template"`
	Restored    []SnapshotProcess `json:"restored"`
}

const SnapshotDensityScenario = "process-snapshot-density"
const SnapshotDensityDiagnosticsVersion = "linux-process-snapshot-density-inspection-v1"

type SnapshotDensityContract struct {
	Mode      string `json:"mode"`
	Instances int    `json:"instances"`
}

type SnapshotDensityDiagnostics struct {
	Version         string                 `json:"version"`
	Profile         string                 `json:"profile"`
	LatencyEligible bool                   `json:"latency_eligible"`
	Samples         []SnapshotDensityProof `json:"samples"`
}

func ValidateSnapshotDensityWorkload(w Workload) error {
	d := w.SnapshotDensity
	if d == nil || w.ProcessSnapshot != nil || d.Mode != "simultaneous_linux_process_cow" || d.Instances < 1 || d.Instances > 32 || w.Dimension != "instances" || w.Size != d.Instances || w.Generator != "wasmbench-process-snapshot-density-v1" || w.WorkUnit != "restored_process_group" || w.Reset != "fresh_process_snapshot_group_per_sample" || w.Oracle.Kind != "linux_process_snapshot_density_v1" {
		return fmt.Errorf("invalid restored snapshot density contract")
	}
	base := w
	base.SnapshotDensity = nil
	base.ProcessSnapshot = &ProcessSnapshotContract{Mode: ProcessSnapshotMode}
	base.Dimension, base.Size = "", 0
	base.Generator, base.WorkUnit, base.Reset, base.Oracle.Kind = "wasmbench-process-snapshot-v1", "process_snapshot_stage", "fresh_process_snapshot_per_sample", "linux_process_snapshot_v1"
	return ValidateProcessSnapshotWorkload(base)
}

func ValidateSnapshotDensityRequest(p Preparation, r RunRequest) error {
	if err := ValidateSnapshotDensityWorkload(p.Workload); err != nil {
		return err
	}
	if p.Profile != "memory" || r.Scenario != SnapshotDensityScenario || !r.PhaseBarriers || r.Samples < 1 || r.Samples > 32 || r.Operations != 1 || r.Warmup != 0 || r.SustainedDurationNS != 0 || r.SustainedPostCollection {
		return fmt.Errorf("restored snapshot density requires memory, explicit barriers, 1-32 groups, one operation and no warmup")
	}
	return nil
}

type SnapshotDensityChild struct {
	Index                int             `json:"index"`
	Process              SnapshotProcess `json:"process"`
	Before               int             `json:"before"`
	After                int             `json:"after"`
	MemoryBeforeSHA256   string          `json:"memory_before_sha256"`
	MemoryTouchedSHA256  string          `json:"memory_touched_sha256"`
	MemoryExecutedSHA256 string          `json:"memory_executed_sha256"`
	TouchByte            int             `json:"touch_byte"`
	TouchedOffsets       []int           `json:"touched_offsets"`
	TouchElapsedNS       *int64          `json:"touch_elapsed_ns"`
	TouchClockProcess    SnapshotProcess `json:"touch_clock_process"`
	ExecuteElapsedNS     *int64          `json:"execute_elapsed_ns"`
	ExecuteClockProcess  SnapshotProcess `json:"execute_clock_process"`
	MemoryPages          int             `json:"memory_pages"`
	TableElements        int             `json:"table_elements"`
	PassiveSegmentProbe  int             `json:"passive_segment_probe"`
}

type SnapshotDensityProof struct {
	Version                     string                 `json:"version"`
	Runtime                     string                 `json:"runtime"`
	RuntimeVersion              string                 `json:"runtime_version"`
	Backend                     string                 `json:"backend"`
	Architecture                string                 `json:"architecture"`
	WasmSHA256                  string                 `json:"wasm_sha256"`
	QualificationOnly           bool                   `json:"qualification_only"`
	RegisteredAdapter           bool                   `json:"registered_adapter"`
	LatencyEligible             bool                   `json:"latency_eligible"`
	Profile                     string                 `json:"profile"`
	Mode                        string                 `json:"mode"`
	Instances                   int                    `json:"instances"`
	PreForkThreads              int                    `json:"pre_fork_threads"`
	Source                      SnapshotProcess        `json:"source"`
	Template                    SnapshotProcess        `json:"template"`
	SourceReleased              bool                   `json:"source_released_before_restore"`
	SourceAfterMutation         int                    `json:"source_after_mutation"`
	ProvisionClockProcess       SnapshotProcess        `json:"provision_clock_process"`
	ProvisionBoundary           string                 `json:"provision_boundary"`
	ProvisionElapsedNS          *int64                 `json:"provision_elapsed_ns"`
	ChildrenReaped              bool                   `json:"children_reaped"`
	TemplateReaped              bool                   `json:"template_reaped"`
	Children                    []SnapshotDensityChild `json:"children"`
	TemplateCheck               int                    `json:"template_check"`
	TemplateMemorySHA256        string                 `json:"template_memory_sha256"`
	TemplatePassiveSegmentProbe int                    `json:"template_passive_segment_probe"`
}

func snapshotDensityBirth(p SnapshotProcess) (uint64, error) {
	n, err := strconv.ParseUint(p.StartTimeTicks, 10, 64)
	if err != nil || n == 0 || strconv.FormatUint(n, 10) != p.StartTimeTicks || p.PID == 0 || p.PID > 2147483647 {
		return 0, fmt.Errorf("invalid density process identity")
	}
	return n, nil
}

func ValidateSnapshotDensityBoundary(b SnapshotDensityBoundary) error {
	if b.Version != SnapshotDensityBoundaryVersion || b.Instances < 1 || b.Instances > 32 || b.SampleIndex < 0 {
		return fmt.Errorf("invalid density boundary scope/count")
	}
	if !slices.Contains([]string{"template_after_source_release", "idle", "touched", "executed"}, b.Stage) {
		return fmt.Errorf("invalid density boundary stage")
	}
	want := b.Instances
	if b.Stage == "template_after_source_release" {
		want = 0
	}
	if len(b.Restored) != want {
		return fmt.Errorf("density group is not completely declared")
	}
	sourceBirth, err := snapshotDensityBirth(b.Source)
	if err != nil {
		return err
	}
	templateBirth, err := snapshotDensityBirth(b.Template)
	if err != nil {
		return err
	}
	if b.Source.PID == b.Template.PID || templateBirth < sourceBirth {
		return fmt.Errorf("invalid density template ancestry")
	}
	seen := map[uint32]bool{b.Source.PID: true, b.Template.PID: true}
	for _, p := range b.Restored {
		birth, err := snapshotDensityBirth(p)
		if err != nil {
			return err
		}
		if seen[p.PID] || birth < templateBirth {
			return fmt.Errorf("density members cannot alias live PIDs or precede template")
		}
		seen[p.PID] = true
	}
	return nil
}

func SnapshotDensityMemorySHA256(index int, executed bool) string {
	b := make([]byte, 3*65536)
	b[0] = 7
	if index >= 0 {
		for _, offset := range []int{65535, 131071, 196607} {
			b[offset] = byte(22 + index)
		}
	}
	if executed {
		copy(b[128:131], "xyz")
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func VerifySnapshotDensityQualification(backend string, instances int, events []SnapshotDensityBoundary, p SnapshotDensityProof) error {
	if !slices.Contains([]string{"cranelift", "winch"}, backend) || instances < 1 || instances > 32 || p.Version != SnapshotDensityWorkerVersion || p.Runtime != "wasmtime" || p.RuntimeVersion != "46.0.1" || p.Backend != backend || !slices.Contains([]string{"aarch64", "x86_64"}, p.Architecture) || p.WasmSHA256 != ProcessSnapshotArtifactSHA256 || !p.QualificationOnly || p.RegisteredAdapter || p.LatencyEligible || p.Profile != "memory" || p.Mode != "simultaneous_linux_process_cow" || p.Instances != instances || p.PreForkThreads != 1 || !p.SourceReleased || p.SourceAfterMutation != 125 || !p.ChildrenReaped || !p.TemplateReaped || p.ProvisionClockProcess != p.Source || p.ProvisionBoundary != "restore_request_to_all_live_ready_identity_frames" || p.ProvisionElapsedNS == nil || *p.ProvisionElapsedNS < 0 || p.TemplateCheck != 64 || p.TemplateMemorySHA256 != SnapshotDensityMemorySHA256(-1, false) || p.TemplatePassiveSegmentProbe != 127 || len(p.Children) != instances || len(events) != 4 {
		return fmt.Errorf("invalid simultaneous snapshot density qualification")
	}
	refs := make([]SnapshotProcess, instances)
	for i, c := range p.Children {
		if c.Index != i || c.Before != 64 || c.After != 64 || c.MemoryPages != 3 || c.TableElements != 3 || c.PassiveSegmentProbe != 127 || c.TouchByte != 22+i || !slices.Equal(c.TouchedOffsets, []int{65535, 131071, 196607}) || c.TouchElapsedNS == nil || *c.TouchElapsedNS < 0 || c.ExecuteElapsedNS == nil || *c.ExecuteElapsedNS < 0 || c.TouchClockProcess != c.Process || c.ExecuteClockProcess != c.Process || c.MemoryBeforeSHA256 != SnapshotDensityMemorySHA256(-1, false) || c.MemoryTouchedSHA256 != SnapshotDensityMemorySHA256(i, false) || c.MemoryExecutedSHA256 != SnapshotDensityMemorySHA256(i, true) {
			return fmt.Errorf("invalid independent density child state/clock")
		}
		refs[i] = c.Process
	}
	for i, stage := range []string{"template_after_source_release", "idle", "touched", "executed"} {
		b := events[i]
		if err := ValidateSnapshotDensityBoundary(b); err != nil {
			return err
		}
		if b.Stage != stage || b.Instances != instances || b.Source != p.Source || b.Template != p.Template || (i > 0 && !slices.Equal(b.Restored, refs)) {
			return fmt.Errorf("density live membership differs from completed proof")
		}
	}
	return nil
}
