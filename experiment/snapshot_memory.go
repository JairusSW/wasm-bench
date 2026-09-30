package experiment

import (
	"fmt"
	"math"
	"os"
	"time"

	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/protocol"
)

const SnapshotMemoryVersion = "linux-process-snapshot-memory-v1"

// Raw, bracketed readings remain distinct by live process and boundary. They
// are neither a tree peak nor a physical attribution of shared COW pages.
type SnapshotMemoryRecord struct {
	Boundary protocol.SnapshotBoundary           `json:"boundary"`
	Readings []collectors.SnapshotProcessReading `json:"readings"`
}

type SnapshotMemoryEvidence struct {
	Version       string                       `json:"version"`
	ControllerPID uint32                       `json:"controller_pid"`
	Diagnostics   protocol.SnapshotDiagnostics `json:"diagnostics"`
	Records       []SnapshotMemoryRecord       `json:"records"`
}

type snapshotMemoryProcess struct {
	ref    protocol.SnapshotProcess
	parent uint32
}

func snapshotMemoryProcesses(b protocol.SnapshotBoundary, controller uint32) []snapshotMemoryProcess {
	items := []snapshotMemoryProcess{{b.Source, controller}, {b.Template, b.Source.PID}}
	if b.Restored != nil {
		items = append(items, snapshotMemoryProcess{*b.Restored, b.Template.PID})
	}
	return items
}

// ValidateSnapshotMemoryEvidence is the collection and sealed-loader seam. It
// independently derives footprints and lineage from retained procfs evidence.
func ValidateSnapshotMemoryEvidence(w protocol.Workload, r protocol.RunRequest, e SnapshotMemoryEvidence) error {
	if e.Version != SnapshotMemoryVersion || e.ControllerPID == 0 || e.ControllerPID > math.MaxInt32 {
		return fmt.Errorf("invalid snapshot memory envelope")
	}
	events := make([]protocol.SnapshotBoundary, len(e.Records))
	for i, record := range e.Records {
		events[i] = record.Boundary
	}
	if err := protocol.VerifySnapshotInspection(w, r, events, e.Diagnostics); err != nil {
		return err
	}
	var previousEnd int64
	for _, record := range e.Records {
		b := record.Boundary
		if b.Source.PID == e.ControllerPID || b.Template.PID == e.ControllerPID || (b.Restored != nil && b.Restored.PID == e.ControllerPID) {
			return fmt.Errorf("snapshot lineage aliases collector process")
		}
		items := snapshotMemoryProcesses(b, e.ControllerPID)
		if len(record.Readings) != len(items) {
			return fmt.Errorf("missing/extra snapshot process reading")
		}
		for i, item := range items {
			reading := record.Readings[i]
			footprint, err := collectors.ValidateSnapshotProcessReading(reading, item.ref, item.parent)
			if err != nil {
				return err
			}
			if footprint.Threads != 1 || reading.StartNS < previousEnd {
				return fmt.Errorf("invalid snapshot threads or collector ordering")
			}
			previousEnd = reading.EndNS
		}
	}
	return validateSampleSequence(protocol.RunRequest{Scenario: r.Scenario, Samples: r.Samples, Operations: r.Operations}, e.Diagnostics.Samples)
}

func collectSnapshotMemory(c *agent.Client, w protocol.Workload, r protocol.RunRequest) (*SnapshotMemoryEvidence, error) {
	e := &SnapshotMemoryEvidence{Version: SnapshotMemoryVersion, ControllerPID: uint32(os.Getpid())}
	origin := time.Now()
	response, err := c.CallSnapshotInspection(protocol.Request{Method: "inspect", Run: &r}, func(b protocol.SnapshotBoundary) error {
		if b.Source.PID != uint32(c.PID()) || b.SampleIndex >= r.Samples || len(e.Records) >= 7*r.Samples {
			return fmt.Errorf("snapshot boundary differs from owned adapter/request")
		}
		record := SnapshotMemoryRecord{Boundary: b}
		for _, item := range snapshotMemoryProcesses(b, e.ControllerPID) {
			reading, err := collectors.CollectSnapshotProcess(item.ref, item.parent, origin)
			if err != nil {
				return err
			}
			record.Readings = append(record.Readings, reading)
		}
		e.Records = append(e.Records, record)
		return nil
	})
	if err != nil {
		return e, err
	}
	if response.SnapshotDiagnostics == nil || len(response.Samples) != 0 || len(response.Diagnostics) != 0 {
		return e, fmt.Errorf("missing snapshot memory diagnostics or foreign payload")
	}
	e.Diagnostics = *response.SnapshotDiagnostics
	return e, ValidateSnapshotMemoryEvidence(w, r, *e)
}

// Cgroup readings account for the whole owned tree, but are not RSS or phase
// peaks. The generic root-only process sampler is forbidden in this profile.
func validateSnapshotCgroupObservations(observations []protocol.Observation) error {
	seen := map[string]bool{}
	for _, o := range observations {
		quality := ""
		switch o.Metric {
		case "cgroup.memory.current":
			quality = "boundary_snapshot_only"
		case "cgroup.memory.peak":
			quality = "kernel_accounted_peak"
		default:
			return fmt.Errorf("foreign snapshot memory observation")
		}
		if seen[o.Metric] || o.DefinitionVersion != 1 || o.Unit != "bytes" || o.Scope != "adapter_cgroup" || o.Phase != "process_lifetime/response_end" || o.Collector != "cgroup_v2" || o.CollectorVersion != "1" || o.Quality != quality || o.Profile != "memory" || o.Denominator != "cgroup" {
			return fmt.Errorf("invalid snapshot cgroup observation scope")
		}
		seen[o.Metric] = true
		if o.Status == "available" {
			if o.Value == nil || math.IsNaN(*o.Value) || math.IsInf(*o.Value, 0) || *o.Value < 0 || math.Trunc(*o.Value) != *o.Value || o.Reason != "" {
				return fmt.Errorf("invalid snapshot cgroup value")
			}
		} else if o.Status != "unavailable" || o.Value != nil || o.Reason == "" {
			return fmt.Errorf("invalid snapshot cgroup availability")
		}
	}
	return nil
}
