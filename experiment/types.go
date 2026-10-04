package experiment

import (
	"encoding/json"
	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/protocol"
	"time"
)

const Version = "0.1.0-dev"

type Runtime struct {
	ELF                    *ELFDependencies      `json:"elf_startup_dependencies,omitempty"`
	HostFiles              map[string]string     `json:"host_file_sha256,omitempty"`
	NativeDependencyPolicy string                `json:"native_dependency_policy,omitempty"`
	UnavailableReason      string                `json:"unavailable_reason,omitempty"`
	ID                     string                `json:"id"`
	Command                []string              `json:"command"`
	Files                  map[string]string     `json:"file_sha256"`
	Description            *protocol.Description `json:"description,omitempty"`
}
type Options struct {
	SustainedPostCollection bool          `json:"sustained_post_collection,omitempty"`
	SustainedDuration       time.Duration `json:"sustained_duration_ns,omitempty"`
	// Recomputed from pinned analyzer output on every run; never accepted from JSON.
	requiredValidatorFeatures map[string][]string
	monitorCPUPartition       bool
	PhaseBarriers             bool                 `json:"phase_barriers"`
	Resources                 agent.ResourcePolicy `json:"resources"`
	Suite                     string               `json:"suite"`
	Profile                   string               `json:"profile"`
	Scenarios                 []string             `json:"scenarios"`
	Launches                  int                  `json:"launches"`
	Samples                   int                  `json:"samples"`
	Workers                   int                  `json:"workers,omitempty"`
	ScenarioSamples           map[string]int       `json:"scenario_samples,omitempty"`
	Operations                int                  `json:"operations"`
	Warmup                    int                  `json:"warmup"`
	Seed                      int64                `json:"seed"`
	Timeout                   time.Duration        `json:"timeout_ns"`
	Check                     bool                 `json:"correctness_only"`
}
type Lock struct {
	RequireIRQAffinity          bool                `json:"require_irq_affinity,omitempty"`
	ArchiveTools                bool                `json:"archive_tools,omitempty"`
	RequireIsolatedCPUPartition bool                `json:"require_isolated_cpu_partition,omitempty"`
	HostPolicy                  *agent.HostPolicy   `json:"host_policy,omitempty"`
	PilotPlan                   json.RawMessage     `json:"pilot_plan,omitempty"`
	Analyzer                    *AnalyzerLock       `json:"analyzer,omitempty"`
	Schema                      int                 `json:"schema"`
	RunnerVersion               string              `json:"runner_version"`
	RunnerSHA256                string              `json:"runner_sha256"`
	Protocol                    int                 `json:"protocol"`
	Options                     Options             `json:"options"`
	Runtimes                    []Runtime           `json:"runtime_configurations"`
	Workloads                   []protocol.Workload `json:"workloads"`
}
type Manifest struct {
	IRQAffinityStart  *agent.IRQAffinityProbe  `json:"irq_affinity_start,omitempty"`
	IRQAffinityEnd    *agent.IRQAffinityProbe  `json:"irq_affinity_end,omitempty"`
	CPUPartitionStart *agent.CPUPartitionProbe `json:"cpu_partition_start,omitempty"`
	CPUPartitionEnd   *agent.CPUPartitionProbe `json:"cpu_partition_end,omitempty"`
	HostEnd           *agent.Host              `json:"host_end,omitempty"`
	HostStartCheck    *agent.HostPolicyCheck   `json:"host_start_check,omitempty"`
	HostEndCheck      *agent.HostPolicyCheck   `json:"host_end_check,omitempty"`
	Schema            int                      `json:"schema"`
	ID                string                   `json:"id"`
	Created           time.Time                `json:"created"`
	Kind              string                   `json:"kind"`
	Lock              Lock                     `json:"lock"`
	LockSHA256        string                   `json:"lock_sha256"`
	Host              agent.Host               `json:"host"`
	Order             []string                 `json:"trial_order"`
	Publication       string                   `json:"publication"`
}
type Trial struct {
	SnapshotMemory   *SnapshotMemoryEvidence       `json:"snapshot_memory,omitempty"`
	SnapshotDensity  *SnapshotDensityTrialEvidence `json:"snapshot_density,omitempty"`
	CodeLifetime     *protocol.CodeLifetime        `json:"code_lifetime,omitempty"`
	EngineTrace      *protocol.EngineTrace         `json:"engine_trace,omitempty"`
	PartitionMonitor *agent.PartitionMonitor       `json:"partition_monitor,omitempty"`
	CPUProfile       *protocol.CPUProfile          `json:"cpu_profile,omitempty"`
	CounterPhases    []agent.CounterPhase          `json:"counter_phases,omitempty"`
	CodeImage        *protocol.CodeImage           `json:"code_image,omitempty"`
	PhaseEvents      []PhaseRecord                 `json:"phase_events,omitempty"`
	Isolation        *agent.Isolation              `json:"isolation,omitempty"`
	ID               string                        `json:"id"`
	Runtime          string                        `json:"runtime_configuration"`
	Workload         string                        `json:"workload"`
	Scenario         string                        `json:"scenario"`
	Profile          string                        `json:"profile"`
	Block            int                           `json:"block"`
	Status           string                        `json:"status"`
	Reason           string                        `json:"reason,omitempty"`
	Started          time.Time                     `json:"started"`
	DurationNS       int64                         `json:"duration_ns"`
	Samples          []protocol.Sample             `json:"samples"`
	AdapterSamples   []protocol.Sample             `json:"adapter_samples,omitempty"`
	Observations     []protocol.Observation        `json:"observations,omitempty"`
	Log              string                        `json:"log"`
}

type PhaseRecord struct {
	Event        protocol.PhaseEvent    `json:"event"`
	Observations []protocol.Observation `json:"observations"`
}
type Bundle struct {
	Admission []ArtifactAdmission `json:"artifact_admission,omitempty"`
	Manifest  Manifest            `json:"manifest"`
	Trials    []Trial             `json:"trials"`
}
