package experiment

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
)

const SnapshotDensityTrialVersion = "linux-process-snapshot-density-trial-v1"

type SnapshotDensityTrialEvidence struct {
	Version string                    `json:"version"`
	Groups  []SnapshotDensityEvidence `json:"groups"`
}

func ValidateSnapshotDensityTrial(w protocol.Workload, r protocol.RunRequest, backend string, e SnapshotDensityTrialEvidence) error {
	if err := protocol.ValidateSnapshotDensityRequest(protocol.Preparation{Workload: w, Profile: "memory"}, r); err != nil {
		return err
	}
	if e.Version != SnapshotDensityTrialVersion || len(e.Groups) != r.Samples {
		return fmt.Errorf("density group count/version differs from request")
	}
	var previousEnd int64
	seen := map[protocol.SnapshotProcess]bool{}
	for index, g := range e.Groups {
		if g.Backend != backend || g.Instances != w.SnapshotDensity.Instances {
			return fmt.Errorf("density backend/count differs from lock")
		}
		if err := ValidateSnapshotDensityEvidence(g); err != nil {
			return err
		}
		members := []protocol.SnapshotProcess{g.Proof.Template}
		for _, child := range g.Proof.Children {
			members = append(members, child.Process)
		}
		for _, member := range members {
			if seen[member] {
				return fmt.Errorf("density sample reuses an earlier template/child incarnation")
			}
			seen[member] = true
		}
		for _, record := range g.Records {
			if record.Boundary.SampleIndex != index || record.Boundary.Source != e.Groups[0].Proof.Source || g.ControllerPID != e.Groups[0].ControllerPID {
				return fmt.Errorf("density sample/source/collector changed")
			}
			for _, reading := range record.Readings {
				if reading.StartNS < previousEnd {
					return fmt.Errorf("density clocks overlap across groups")
				}
				previousEnd = reading.EndNS
			}
		}
	}
	return nil
}

func collectSnapshotDensity(c *agent.Client, w protocol.Workload, r protocol.RunRequest, backend string) (*SnapshotDensityTrialEvidence, error) {
	e := &SnapshotDensityTrialEvidence{Version: SnapshotDensityTrialVersion}
	origin := time.Now()
	response, err := c.CallSnapshotDensityInspection(protocol.Request{Method: "inspect", Run: &r}, func(b protocol.SnapshotDensityBoundary) error {
		if b.Source.PID != uint32(c.PID()) || b.SampleIndex >= r.Samples || b.Instances != w.SnapshotDensity.Instances || b.SampleIndex > len(e.Groups) {
			return fmt.Errorf("density boundary differs from owned adapter/request")
		}
		if b.SampleIndex == len(e.Groups) {
			if b.Stage != "template_after_source_release" || (len(e.Groups) > 0 && len(e.Groups[len(e.Groups)-1].Records) != 4) {
				return fmt.Errorf("density sample starts before prior barriers complete")
			}
			e.Groups = append(e.Groups, SnapshotDensityEvidence{Version: SnapshotDensityEvidenceVersion, ControllerPID: uint32(os.Getpid()), Backend: backend, Instances: b.Instances})
		}
		if b.SampleIndex != len(e.Groups)-1 {
			return fmt.Errorf("density sample reordered")
		}
		g := &e.Groups[b.SampleIndex]
		record := SnapshotDensityRecord{Boundary: b}
		for index, ref := range append([]protocol.SnapshotProcess{b.Source, b.Template}, b.Restored...) {
			parent := b.Template.PID
			if index == 0 {
				parent = g.ControllerPID
			} else if index == 1 {
				parent = b.Source.PID
			}
			reading, err := collectors.CollectSnapshotProcess(ref, parent, origin)
			if err != nil {
				return err
			}
			record.Readings = append(record.Readings, reading)
		}
		g.Records = append(g.Records, record)
		return ValidateSnapshotDensityPrefix(*g)
	})
	if err != nil {
		return e, err
	}
	d := response.SnapshotDensityDiagnostics
	if d == nil || d.Version != protocol.SnapshotDensityDiagnosticsVersion || d.Profile != "memory" || d.LatencyEligible || len(d.Samples) != len(e.Groups) {
		return e, fmt.Errorf("missing/invalid density diagnostic completion")
	}
	for index, p := range d.Samples {
		e.Groups[index].Proof = p
	}
	return e, ValidateSnapshotDensityTrial(w, r, backend, *e)
}

func ValidateSnapshotDensityBundle(root string, b Bundle) error {
	workloads := map[string]protocol.Workload{}
	for _, w := range b.Manifest.Lock.Workloads {
		if w.SnapshotDensity == nil && w.Oracle.Kind != "linux_process_snapshot_density_v1" {
			continue
		}
		if err := protocol.ValidateSnapshotDensityWorkload(w); err != nil {
			return err
		}
		canonical, err := corpus.ProcessSnapshotModule()
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(filepath.Join(root, "artifacts", w.SHA256+".wasm"))
		if err != nil {
			return err
		}
		if !bytes.Equal(raw, canonical) {
			return fmt.Errorf("density fixture differs from canonical bytes")
		}
		workloads[w.ID] = w
	}
	for _, t := range b.Trials {
		w, known := workloads[t.Workload]
		if !known {
			if t.SnapshotDensity != nil || (t.Status == "ok" && t.Scenario == protocol.SnapshotDensityScenario) {
				return fmt.Errorf("density payload/scenario outside workload")
			}
			continue
		}
		if t.SnapshotDensity != nil && (t.Profile != "memory" || t.Scenario != protocol.SnapshotDensityScenario) {
			return fmt.Errorf("density evidence outside memory inspection")
		}
		if t.Status != "ok" {
			continue
		}
		if b.Manifest.Host.OS != "linux" || !slices.Contains([]string{"arm64", "amd64"}, b.Manifest.Host.Arch) || t.Profile != "memory" || b.Manifest.Lock.Options.Profile != "memory" || t.Scenario != protocol.SnapshotDensityScenario || (t.Block >= 0 && !slices.Contains(b.Manifest.Lock.Options.Scenarios, t.Scenario)) {
			return fmt.Errorf("density trial lacks locked native Linux memory identity")
		}
		var d *protocol.Description
		for _, runtime := range b.Manifest.Lock.Runtimes {
			if runtime.ID == t.Runtime {
				d = runtime.Description
			}
		}
		if err := protocol.ValidateProcessSnapshotDescription(d); err != nil {
			return err
		}
		if !d.Capabilities["can_inspect_linux_snapshot_density"] || !slices.Contains(d.Scenarios, protocol.SnapshotDensityScenario) || t.SnapshotDensity == nil || len(t.Samples) != 0 || len(t.AdapterSamples) != 0 || t.SnapshotMemory != nil || len(t.PhaseEvents) != 0 || t.CodeImage != nil || t.CodeLifetime != nil || t.CPUProfile != nil || t.EngineTrace != nil || len(t.CounterPhases) != 0 {
			return fmt.Errorf("density completion lacks capability/proof or mixes other payloads")
		}
		if err := ValidateSnapshotDensityTrial(w, trialRequest(b.Manifest.Lock.Options, w, t.Scenario, t.Block), d.Backend, *t.SnapshotDensity); err != nil {
			return err
		}
		wantArch := map[string]string{"arm64": "aarch64", "amd64": "x86_64"}[b.Manifest.Host.Arch]
		for _, group := range t.SnapshotDensity.Groups {
			if group.Proof.Architecture != wantArch {
				return fmt.Errorf("density architecture differs from host")
			}
		}
		if err := validateSnapshotCgroupObservations(t.Observations); err != nil {
			return err
		}
	}
	return nil
}
