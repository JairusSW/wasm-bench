package experiment

import (
	"context"
	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
	"os"
	"path/filepath"
	"testing"
)

func snapshotEvidence(t *testing.T) (string, Bundle) {
	t.Helper()
	root := t.TempDir()
	ws, err := corpus.Generate(root, "process-snapshots")
	if err != nil {
		t.Fatal(err)
	}
	w := ws[0]
	b, err := corpus.ProcessSnapshotModule()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "artifacts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "artifacts", w.SHA256+".wasm"), b, 0644); err != nil {
		t.Fatal(err)
	}
	e := &protocol.ProcessSnapshotResult{Mode: protocol.ProcessSnapshotMode, PreForkThreads: 1, Boundary: protocol.ProcessSnapshotBoundary("process-snapshot-restore"), Source: protocol.SnapshotProcess{PID: 101, StartTimeTicks: "100"}, Template: protocol.SnapshotProcess{PID: 102, StartTimeTicks: "101"}, Restored: protocol.SnapshotProcess{PID: 103, StartTimeTicks: "102"}, AlternateRestored: protocol.SnapshotProcess{PID: 104, StartTimeTicks: "103"}, SourceReleased: true, ChildrenReaped: true, SourceAfterMutation: 125, RestoredBeforeWrite: 64, RestoredAfterWrite: 64, PassiveSegmentProbe: 127, MemoryPages: 3, TableElements: 3, IndependentRestorations: 2, MemoryAtRestoreSHA256: protocol.ProcessSnapshotMemorySHA256(false), MemoryAfterFirstWriteSHA256: protocol.ProcessSnapshotMemorySHA256(true)}
	e.ClockProcess = e.Source
	s := protocol.Sample{Operations: 1, SampleType: "individual_operation", Verified: true, Result: protocol.Values{64}, ProcessSnapshotResult: e}
	d := &protocol.Description{Runtime: "wasmtime", Version: "46.0.1", Backend: "cranelift", Embedding: "Rust API", Scenarios: protocol.ProcessSnapshotScenarios(), Capabilities: map[string]bool{"can_linux_process_snapshot": true, "can_snapshot": false}, Configuration: map[string]string{"process_snapshot_protocol": protocol.ProcessSnapshotMode, "process_snapshot_os": "linux", "process_snapshot_spawn": "single_threaded_owned_process", "parallel_compilation": "false", "cache": "disabled"}}
	return root, Bundle{Manifest: Manifest{Host: agent.Host{OS: "linux", Arch: "arm64"}, Lock: Lock{Workloads: ws, Runtimes: []Runtime{{ID: "r", Description: d}}, Options: Options{Profile: "timing", Samples: 1, Operations: 1, Scenarios: protocol.ProcessSnapshotScenarios()}}}, Trials: []Trial{{Runtime: "r", Workload: w.ID, Scenario: "process-snapshot-restore", Profile: "timing", Status: "ok", Samples: []protocol.Sample{s}}}}
}
func TestProcessSnapshotOfflineContract(t *testing.T) {
	root, b := snapshotEvidence(t)
	if err := ValidateProcessSnapshotEvidence(root, b); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*Bundle){
		"foreign_workload": func(b *Bundle) { b.Trials[0].Workload = "foreign" }, "foreign_stage": func(b *Bundle) { b.Trials[0].Scenario = "checkpoint-restore" }, "unlocked_stage": func(b *Bundle) { b.Manifest.Lock.Options.Scenarios = []string{"compile"} }, "memory_pass": func(b *Bundle) { b.Manifest.Lock.Options.Profile = "memory"; b.Trials[0].Profile = "memory" }, "wrong_host": func(b *Bundle) { b.Manifest.Host.OS = "darwin" }, "wrong_arch": func(b *Bundle) { b.Manifest.Host.Arch = "386" }, "capability": func(b *Bundle) {
			b.Manifest.Lock.Runtimes[0].Description.Capabilities["can_linux_process_snapshot"] = false
		}, "generic_snapshot": func(b *Bundle) { b.Manifest.Lock.Runtimes[0].Description.Capabilities["can_snapshot"] = true }, "wrong_backend": func(b *Bundle) { b.Manifest.Lock.Runtimes[0].Description.Backend = "interpreter" }, "wrong_version": func(b *Bundle) { b.Manifest.Lock.Runtimes[0].Description.Version = "46.0.2" }, "parallel": func(b *Bundle) {
			b.Manifest.Lock.Runtimes[0].Description.Configuration["parallel_compilation"] = "true"
		}, "multithreaded": func(b *Bundle) { b.Trials[0].Samples[0].ProcessSnapshotResult.PreForkThreads = 2 }, "source_alive": func(b *Bundle) { b.Trials[0].Samples[0].ProcessSnapshotResult.SourceReleased = false }, "wrong_clock": func(b *Bundle) { e := b.Trials[0].Samples[0].ProcessSnapshotResult; e.ClockProcess = e.Restored }, "phase_payload": func(b *Bundle) { b.Trials[0].PhaseEvents = []PhaseRecord{{}} }, "trial_memory": func(b *Bundle) { b.Trials[0].Observations = []protocol.Observation{{Metric: "process.peak_rss"}} }, "sample_count": func(b *Bundle) { b.Trials[0].Samples = nil }, "adapter_payload": func(b *Bundle) { b.Trials[0].AdapterSamples = b.Trials[0].Samples }, "unrelated_reset": func(b *Bundle) { b.Manifest.Lock.Workloads[0].Reset = "stateless" },
	} {
		t.Run(name, func(t *testing.T) {
			root, b := snapshotEvidence(t)
			change(&b)
			if ValidateProcessSnapshotEvidence(root, b) == nil {
				t.Fatal("forged process snapshot trial admitted")
			}
		})
	}
	for _, stage := range protocol.ProcessSnapshotScenarios() {
		root, b := snapshotEvidence(t)
		s := &b.Trials[0].Samples[0]
		e := s.ProcessSnapshotResult
		b.Trials[0].Scenario = stage
		e.Boundary = protocol.ProcessSnapshotBoundary(stage)
		if stage == "process-snapshot-first-write" || stage == "process-snapshot-execute" {
			e.ClockProcess = e.Restored
		}
		if err := ValidateProcessSnapshotEvidence(root, b); err != nil {
			t.Fatal(err)
		}
	}
	root, b = snapshotEvidence(t)
	b.Trials[0].Block = -1
	second := b.Trials[0].Samples[0]
	second.Index = 1
	proof := *second.ProcessSnapshotResult
	proof.Template = protocol.SnapshotProcess{PID: 105, StartTimeTicks: "104"}
	proof.Restored = protocol.SnapshotProcess{PID: 106, StartTimeTicks: "105"}
	proof.AlternateRestored = protocol.SnapshotProcess{PID: 107, StartTimeTicks: "106"}
	second.ProcessSnapshotResult = &proof
	b.Trials[0].Samples = append(b.Trials[0].Samples, second)
	if err := ValidateProcessSnapshotEvidence(root, b); err != nil {
		t.Fatal(err)
	}
	root, b = snapshotEvidence(t)
	b.Trials[0].Status = "unsupported"
	b.Trials[0].Samples = nil
	b.Manifest.Lock.Runtimes[0].Description.Capabilities["can_linux_process_snapshot"] = false
	if err := ValidateProcessSnapshotEvidence(root, b); err != nil {
		t.Fatal(err)
	}
	root, b = snapshotEvidence(t)
	if err := os.WriteFile(filepath.Join(root, "artifacts", protocol.ProcessSnapshotArtifactSHA256+".wasm"), []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}
	if ValidateProcessSnapshotEvidence(root, b) == nil {
		t.Fatal("changed input admitted")
	}
}
func TestProcessSnapshotUnqualifiedAdapterNeverStarts(t *testing.T) {
	root, b := snapshotEvidence(t)
	r := b.Manifest.Lock.Runtimes[0]
	r.Description.Capabilities["can_linux_process_snapshot"] = false
	r.Command = []string{"/does/not/exist"}
	for _, stage := range protocol.ProcessSnapshotScenarios() {
		trial := runTrial(context.Background(), root, b.Manifest.Lock.Options, r, b.Manifest.Lock.Workloads[0], stage, 0, "test")
		if trial.Status != "unsupported" || len(trial.Samples) != 0 {
			t.Fatalf("unqualified process snapshot: %+v", trial)
		}
	}
	if request := trialRequest(b.Manifest.Lock.Options, b.Manifest.Lock.Workloads[0], "compile", -1); request.Scenario != "process-snapshot-restore" || request.Samples != 2 || request.Operations != 1 {
		t.Fatal("sacrificial snapshot preflight must restore twice")
	}
}
