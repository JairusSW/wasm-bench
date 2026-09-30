package experiment

import (
	"bytes"
	"fmt"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
	"os"
	"path/filepath"
	"slices"
)

// ValidateProcessSnapshotEvidence applies the same protocol seam online/offline.
// Ordinary adapters remain unsupported; dedicated timing and live-memory
// adapters must satisfy their independently validated evidence contracts.
func ValidateProcessSnapshotEvidence(root string, b Bundle) error {
	workloads := map[string]protocol.Workload{}
	for _, w := range b.Manifest.Lock.Workloads {
		if w.ProcessSnapshot == nil && w.Oracle.Kind != "linux_process_snapshot_v1" && w.Reset != "fresh_process_snapshot_per_sample" {
			continue
		}
		if err := protocol.ValidateProcessSnapshotWorkload(w); err != nil {
			return err
		}
		canonical, err := corpus.ProcessSnapshotModule()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(filepath.Join(root, "artifacts", w.SHA256+".wasm"))
		if err != nil {
			return err
		}
		if !bytes.Equal(data, canonical) {
			return fmt.Errorf("process snapshot canonical artifact mismatch")
		}
		workloads[w.ID] = w
	}
	for _, t := range b.Trials {
		w, known := workloads[t.Workload]
		if !known {
			if t.SnapshotMemory != nil {
				return fmt.Errorf("snapshot memory outside workload contract")
			}
			for _, s := range append(append([]protocol.Sample{}, t.Samples...), t.AdapterSamples...) {
				if s.ProcessSnapshotResult != nil {
					return fmt.Errorf("process snapshot proof outside workload contract")
				}
			}
			if t.Status == "ok" && protocol.IsProcessSnapshotScenario(t.Scenario) {
				return fmt.Errorf("process snapshot scenario outside workload contract")
			}
			continue
		}
		if t.SnapshotMemory != nil && (t.Profile != "memory" || t.Block < 0) {
			return fmt.Errorf("snapshot memory outside diagnostic trial")
		}
		if t.Status != "ok" {
			continue
		}
		if b.Manifest.Host.OS != "linux" || !slices.Contains([]string{"amd64", "arm64"}, b.Manifest.Host.Arch) {
			return fmt.Errorf("process snapshot trial lacks native Linux host identity")
		}
		var description *protocol.Description
		for _, runtime := range b.Manifest.Lock.Runtimes {
			if runtime.ID == t.Runtime {
				description = runtime.Description
			}
		}
		if err := protocol.ValidateProcessSnapshotDescription(description); err != nil {
			return err
		}
		if t.Profile != b.Manifest.Lock.Options.Profile || (t.Block < 0 && t.Scenario != "process-snapshot-restore") || (t.Block >= 0 && !slices.Contains(b.Manifest.Lock.Options.Scenarios, t.Scenario)) {
			return fmt.Errorf("process snapshot scenario/profile differs from lock")
		}
		request := trialRequest(b.Manifest.Lock.Options, w, t.Scenario, t.Block)
		if len(t.PhaseEvents) != 0 || len(t.AdapterSamples) != 0 || t.CodeImage != nil || t.CPUProfile != nil || t.CodeLifetime != nil || t.EngineTrace != nil || len(t.CounterPhases) != 0 {
			return fmt.Errorf("process snapshot has foreign diagnostic payload")
		}
		if t.Profile == "memory" && t.Block >= 0 {
			if !description.Capabilities["can_inspect_linux_snapshot_lineage"] || t.SnapshotMemory == nil || len(t.Samples) != 0 {
				return fmt.Errorf("snapshot memory lacks qualified inspection or contains headline samples")
			}
			if err := ValidateSnapshotMemoryEvidence(w, request, *t.SnapshotMemory); err != nil {
				return err
			}
			if err := validateSnapshotCgroupObservations(t.Observations); err != nil {
				return err
			}
			continue
		}
		profile := t.Profile
		if t.Block < 0 && profile == "memory" {
			profile = "timing"
		}
		if err := protocol.ValidateProcessSnapshot(&protocol.Preparation{Workload: w, Profile: profile}, &request); err != nil {
			return err
		}
		if t.SnapshotMemory != nil || len(t.Observations) != 0 {
			return fmt.Errorf("process snapshot timing/preflight has memory payload")
		}
		if err := validateSampleSequence(request, t.Samples); err != nil {
			return err
		}
		if err := protocol.VerifyProcessSnapshotSequence(w, request, t.Samples); err != nil {
			return err
		}
	}
	return nil
}
