// Package protocol defines the versioned, language-neutral adapter contract.
// JSON lines travel on stdin/stdout; guest output must use stderr or a file.
package protocol

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// Values uses decimal strings on the wire so JavaScript cannot round i64 bits.
type Values []uint64

func (v Values) MarshalJSON() ([]byte, error) {
	out := make([]string, len(v))
	for i, x := range v {
		out[i] = strconv.FormatUint(x, 10)
	}
	return json.Marshal(out)
}
func (v *Values) UnmarshalJSON(b []byte) error {
	var raw []json.RawMessage
	if e := json.Unmarshal(b, &raw); e != nil {
		return e
	}
	out := make(Values, len(raw))
	for i, r := range raw {
		var s string
		if len(r) > 0 && r[0] == '"' {
			if e := json.Unmarshal(r, &s); e != nil {
				return e
			}
		} else {
			s = string(r)
		}
		n, e := strconv.ParseUint(s, 10, 64)
		if e != nil {
			return fmt.Errorf("invalid unsigned result %q: %w", s, e)
		}
		out[i] = n
	}
	*v = out
	return nil
}

const Version = 1

type Request struct {
	Version int          `json:"version"`
	ID      int          `json:"id"`
	Method  string       `json:"method"`
	Prepare *Preparation `json:"prepare,omitempty"`
	Run     *RunRequest  `json:"run,omitempty"`
}

type Response struct {
	SnapshotDensityBoundary    *SnapshotDensityBoundary    `json:"snapshot_density_boundary,omitempty"`
	SnapshotDensityDiagnostics *SnapshotDensityDiagnostics `json:"snapshot_density_diagnostics,omitempty"`
	SnapshotBoundary           *SnapshotBoundary           `json:"snapshot_boundary,omitempty"`
	SnapshotDiagnostics        *SnapshotDiagnostics        `json:"snapshot_diagnostics,omitempty"`
	CodeLifetime               *CodeLifetime               `json:"code_lifetime,omitempty"`
	EngineTrace                *EngineTrace                `json:"engine_trace,omitempty"`
	CPUProfile                 *CPUProfile                 `json:"cpu_profile,omitempty"`
	CodeImage                  *CodeImage                  `json:"code_image,omitempty"`
	Phase                      *PhaseEvent                 `json:"phase,omitempty"`
	Version                    int                         `json:"version"`
	ID                         int                         `json:"id"`
	Status                     string                      `json:"status"`
	Reason                     string                      `json:"reason,omitempty"`
	Description                *Description                `json:"description,omitempty"`
	Samples                    []Sample                    `json:"samples,omitempty"`
	Diagnostics                []Observation               `json:"diagnostics,omitempty"`
}

type Description struct {
	PhaseReleasePolicy    string            `json:"phase_release_policy,omitempty"`
	PhaseBarrierScenarios []string          `json:"phase_barrier_scenarios,omitempty"`
	Runtime               string            `json:"runtime"`
	Version               string            `json:"runtime_version"`
	Backend               string            `json:"backend"`
	Embedding             string            `json:"embedding"`
	Build                 string            `json:"build"`
	Configuration         map[string]string `json:"effective_configuration"`
	Capabilities          map[string]bool   `json:"capabilities"`
	Scenarios             []string          `json:"scenarios"`
	ABIs                  []string          `json:"abis"`
	Features              []string          `json:"features"`
	// ValidatorFeatures is explicit policy evidence, not an exhaustive capability
	// list. Missing flags are unknown. Keys use the named validator's vocabulary.
	ValidatorFeatures *ValidatorFeaturePolicy `json:"validator_features,omitempty"`
}

type ValidatorFeaturePolicy struct {
	Namespace string          `json:"namespace"`
	Evidence  string          `json:"evidence"`
	Supported map[string]bool `json:"supported"`
}

type Preparation struct {
	Artifact       string   `json:"artifact"`
	ArtifactSHA256 string   `json:"artifact_sha256"`
	Workload       Workload `json:"workload"`
	Profile        string   `json:"profile"`
}

type RunRequest struct {
	SustainedPostCollection bool   `json:"sustained_post_collection,omitempty"`
	SustainedDurationNS     int64  `json:"sustained_duration_ns,omitempty"`
	PhaseBarriers           bool   `json:"phase_barriers,omitempty"`
	Scenario                string `json:"scenario"`
	Samples                 int    `json:"samples"`
	Operations              int    `json:"operations"`
	Warmup                  int    `json:"warmup"`
}

// PhaseEvent is an optional diagnostic handshake within one batch request.
// It must never be enabled in headline timing passes.
type PhaseEvent struct {
	SampleIndex int    `json:"sample_index"`
	Stage       string `json:"stage"`
}

type Workload struct {
	SnapshotDensity   *SnapshotDensityContract `json:"snapshot_density,omitempty"`
	ProcessSnapshot   *ProcessSnapshotContract `json:"process_snapshot,omitempty"`
	Continuation      *ContinuationContract    `json:"continuation,omitempty"`
	GuestDensity      *GuestDensityContract    `json:"guest_density,omitempty"`
	Schema            int                      `json:"schema"`
	ID                string                   `json:"id"`
	Family            string                   `json:"family"`
	Artifact          string                   `json:"artifact"`
	SHA256            string                   `json:"sha256"`
	ABI               string                   `json:"abi"`
	HostProfile       string                   `json:"host_profile,omitempty"`
	Features          []string                 `json:"features"`
	Export            string                   `json:"export"`
	Args              Values                   `json:"args"`
	Initialize        string                   `json:"initialize,omitempty"`
	Input             *MemoryInput             `json:"input,omitempty"`
	Command           *CommandContract         `json:"command,omitempty"`
	Vectors           *VectorContract          `json:"vectors,omitempty"`
	Density           *DensityContract         `json:"density,omitempty"`
	Checkpoint        *CheckpointContract      `json:"checkpoint,omitempty"`
	VectorByteBudget  uint64                   `json:"vector_byte_budget,omitempty"`
	OriginalContract  json.RawMessage          `json:"original_contract,omitempty"`
	Provenance        json.RawMessage          `json:"provenance,omitempty"`
	UnsupportedReason string                   `json:"unsupported_reason,omitempty"`
	WorkUnit          string                   `json:"work_unit"`
	Units             uint64                   `json:"units_per_invocation"`
	Reset             string                   `json:"reset"`
	Oracle            Oracle                   `json:"oracle"`
	License           string                   `json:"license"`
	Source            string                   `json:"source"`
	Generator         string                   `json:"generator"`
	Dimension         string                   `json:"dimension,omitempty"`
	Size              int                      `json:"size,omitempty"`
}

// ValidateComponentCompileWorkload defines a structural experiment whose work
// is the module compilation operation itself. It does not claim guest behavior.
func ValidateComponentCompileWorkload(w Workload) error {
	if w.ABI != "component" || w.Oracle.Kind != "component_compile_only" || w.Command != nil || w.Vectors != nil || w.Input != nil || w.Density != nil || w.HostProfile != "" || w.Initialize != "" || w.Reset != "stateless" || w.Export != "" || w.Units != 1 || w.WorkUnit != "component" {
		return fmt.Errorf("invalid component compile-only contract")
	}
	return nil
}

type Oracle struct {
	Float        *FloatPolicy `json:"float,omitempty"`
	ExpectedTrap string       `json:"expected_trap,omitempty"`
	// OutputPointerExport is called after the measured invocation, outside timing.
	// It returns one wasm32 address used as the base of all memory offsets.
	OutputPointerExport string        `json:"output_pointer_export,omitempty"`
	Kind                string        `json:"kind"`
	Expected            Values        `json:"expected"`
	Memory              []MemoryCheck `json:"memory,omitempty"`
}

type MemoryCheck struct {
	Offset uint32 `json:"offset"`
	Hex    string `json:"hex"`
}

// MemoryInput is written after initialization, before timed invocation, once
// per fresh instance. PointerExport, when set, supplies a wasm32 base address.
type MemoryInput struct {
	PointerExport string `json:"pointer_export,omitempty"`
	Offset        uint32 `json:"offset"`
	Hex           string `json:"hex"`
}

type Sample struct {
	ProcessSnapshotResult *ProcessSnapshotResult `json:"process_snapshot_result,omitempty"`
	ContinuationResult    *ContinuationResult    `json:"continuation_result,omitempty"`
	TierWindow            *TierWindow            `json:"tier_window,omitempty"`
	GuestDensityResult    *GuestDensityResult    `json:"guest_density_result,omitempty"`
	SustainedWindow       *SustainedWindow       `json:"sustained_window,omitempty"`
	SustainedRelease      *SustainedRelease      `json:"sustained_release,omitempty"`
	CheckpointResult      *CheckpointResult      `json:"checkpoint_result,omitempty"`
	TrapResult            *TrapResult            `json:"trap_result,omitempty"`
	Index                 int                    `json:"index"`
	Warmup                bool                   `json:"warmup"`
	ElapsedNS             int64                  `json:"elapsed_ns"`
	Operations            int                    `json:"operations"`
	SampleType            string                 `json:"sample_type"`
	Verified              bool                   `json:"verified"`
	Result                Values                 `json:"result,omitempty"`
	CommandResult         *CommandResult         `json:"command_result,omitempty"`
	Observations          []Observation          `json:"observations,omitempty"`
}

type Observation struct {
	Metric            string   `json:"metric"`
	DefinitionVersion int      `json:"definition_version"`
	Value             *float64 `json:"value,omitempty"`
	Unit              string   `json:"unit"`
	Scope             string   `json:"scope"`
	Phase             string   `json:"phase"`
	Collector         string   `json:"collector"`
	CollectorVersion  string   `json:"collector_version"`
	Quality           string   `json:"quality"`
	Profile           string   `json:"profile"`
	Status            string   `json:"status"`
	Reason            string   `json:"reason,omitempty"`
	Denominator       string   `json:"normalization_denominator"`
}

func Value(v float64) *float64 { return &v }

func Encode(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
