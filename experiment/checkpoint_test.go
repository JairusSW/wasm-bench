package experiment

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
)

func TestCheckpointOfflineEvidence(t *testing.T) {
	root := t.TempDir()
	ws, err := corpus.Generate(root, "checkpoints")
	if err != nil {
		t.Fatal(err)
	}
	w := ws[0]
	if err := os.Mkdir(filepath.Join(root, "artifacts"), 0755); err != nil {
		t.Fatal(err)
	}
	module, err := corpus.CheckpointModule(w.Checkpoint.Pages)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "artifacts", w.SHA256+".wasm")
	if err := os.WriteFile(path, module, 0644); err != nil {
		t.Fatal(err)
	}
	e := protocol.CheckpointResult{Mode: protocol.GuestCheckpointMode, PayloadBytes: 65540, SavedMemorySHA256: protocol.CheckpointMemorySHA256(1, false), RestoredMemorySHA256: protocol.CheckpointMemorySHA256(1, false), SavedGlobal: 42, RestoredGlobal: 42, SourceIndependent: true}
	s := protocol.Sample{Operations: 1, SampleType: "individual_operation", Verified: true, Result: w.Oracle.Expected, CheckpointResult: &e}
	b := Bundle{Manifest: Manifest{Lock: Lock{Workloads: []protocol.Workload{w}, Options: Options{Profile: "timing", Samples: 1, Operations: 1}}}, Trials: []Trial{{ID: "t", Workload: w.ID, Scenario: "checkpoint-restore", Profile: "timing", Status: "ok", Block: 0, Samples: []protocol.Sample{s}}}}
	if err := ValidateCheckpointEvidence(root, b); err != nil {
		t.Fatal(err)
	}
	e.RestoredGlobal = 43
	if ValidateCheckpointEvidence(root, b) == nil {
		t.Fatal("accepted forged saved-run global")
	}
	e.RestoredGlobal = 42
	b.Trials[0].Samples[0].Operations = 2
	if ValidateCheckpointEvidence(root, b) == nil {
		t.Fatal("accepted batch checkpoint")
	}
	b.Trials[0].Samples[0].Operations = 1
	b.Trials[0].Scenario = "first-call"
	if ValidateCheckpointEvidence(root, b) == nil {
		t.Fatal("accepted incorrect boundary")
	}
	b.Trials[0].Scenario = "checkpoint-restore"
	b.Trials[0].Block = -1
	b.Trials[0].Samples = append(b.Trials[0].Samples, s)
	b.Trials[0].Samples[1].Index = 1
	if err := ValidateCheckpointEvidence(root, b); err != nil {
		t.Fatal("sacrificial fresh-state repetition", err)
	}
	b.Trials[0].Block = 0
	b.Trials[0].Samples = b.Trials[0].Samples[:1]
	b.Manifest.Lock.Options.Profile = "memory"
	b.Manifest.Lock.Options.PhaseBarriers = true
	b.Trials[0].Profile = "memory"
	stages := protocol.PhaseStages("checkpoint-restore")
	for _, stage := range stages {
		b.Trials[0].PhaseEvents = append(b.Trials[0].PhaseEvents, PhaseRecord{Event: protocol.PhaseEvent{SampleIndex: 0, Stage: stage}})
	}
	b.Trials[0].Samples[0].Observations = []protocol.Observation{{Metric: "checkpoint.payload_bytes", DefinitionVersion: 1, Value: protocol.Value(65540), Unit: "bytes", Scope: "guest_state_checkpoint", Phase: stages[1], Collector: "wasmbench.eager_guest_checkpoint", CollectorVersion: "1", Quality: "exact", Profile: "memory", Status: "available", Denominator: "one_checkpoint"}}
	if err := ValidateCheckpointEvidence(root, b); err != nil {
		t.Fatal("valid memory evidence", err)
	}
	b.Trials[0].Samples[0].Observations[0].Value = protocol.Value(0)
	if ValidateCheckpointEvidence(root, b) == nil {
		t.Fatal("accepted forged payload metric")
	}
	b.Trials[0].Samples[0].Observations[0].Value = protocol.Value(65540)
	b.Trials[0].PhaseEvents[1].Event.Stage = stages[0]
	if ValidateCheckpointEvidence(root, b) == nil {
		t.Fatal("accepted reordered phase barriers")
	}
	b.Trials[0].PhaseEvents[1].Event.Stage = stages[1]
	if err := os.WriteFile(path, append(module, 0, 1, 0), 0644); err != nil {
		t.Fatal(err)
	}
	if ValidateCheckpointEvidence(root, b) == nil {
		t.Fatal("accepted changed checkpoint module shape")
	}
}

func TestCheckpointSacrificialRequestAndInvalidMeasuredBudget(t *testing.T) {
	ws, err := corpus.Generate(t.TempDir(), "checkpoints")
	if err != nil {
		t.Fatal(err)
	}
	o := Options{Profile: "memory", Samples: 3, Operations: 2, Warmup: 1, PhaseBarriers: true}
	r := trialRequest(o, ws[0], "checkpoint-create", -1)
	if r.Scenario != "checkpoint-restore" || r.Samples != 2 || r.Operations != 1 || r.Warmup != 0 || r.PhaseBarriers {
		t.Fatal("invalid fresh-process admission policy", r)
	}
	r = trialRequest(o, ws[0], "checkpoint-create", 0)
	if protocol.ValidateCheckpoint(&protocol.Preparation{Workload: ws[0], Profile: "memory"}, &r) == nil {
		t.Fatal("silently coerced invalid measured budget")
	}
}
