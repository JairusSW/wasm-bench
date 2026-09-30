package experiment

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
)

func TestGuestDensityOfflineEvidence(t *testing.T) {
	root := t.TempDir()
	ws, err := corpus.Generate(root, "guest-density")
	if err != nil {
		t.Fatal(err)
	}
	w := ws[5]
	d := w.GuestDensity
	if err := os.Mkdir(filepath.Join(root, "artifacts"), 0755); err != nil {
		t.Fatal(err)
	}
	module, err := corpus.CheckpointModule(d.Pages)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "artifacts", w.SHA256+".wasm")
	if err := os.WriteFile(path, module, 0644); err != nil {
		t.Fatal(err)
	}
	e := &protocol.GuestDensityResult{Provisioning: d.Provisioning, Instances: make([]protocol.GuestDensityInstance, d.Instances)}
	g := uint32(42)
	if d.FirstWrite {
		g = 43
	}
	for i := range e.Instances {
		e.Instances[i] = protocol.GuestDensityInstance{MemorySHA256: protocol.CheckpointMemorySHA256(d.Pages, d.FirstWrite), Global: g, Checksum: w.Oracle.Expected[0]}
	}
	s := protocol.Sample{Operations: 1, SampleType: "individual_operation", Verified: true, Result: w.Oracle.Expected, GuestDensityResult: e}
	b := Bundle{Manifest: Manifest{Lock: Lock{Workloads: []protocol.Workload{w}, Options: Options{Profile: "timing", Samples: 1, Operations: 1}}}, Trials: []Trial{{ID: "t", Workload: w.ID, Scenario: "guest-density", Profile: "timing", Status: "ok", Samples: []protocol.Sample{s}}}}
	if err := ValidateGuestDensityEvidence(root, b); err != nil {
		t.Fatal(err)
	}
	e.Instances[len(e.Instances)-1].Global++
	if ValidateGuestDensityEvidence(root, b) == nil {
		t.Fatal("accepted altered final instance")
	}
	e.Instances[len(e.Instances)-1].Global--
	b.Trials[0].Scenario = "density"
	if ValidateGuestDensityEvidence(root, b) == nil {
		t.Fatal("accepted wrong scenario")
	}
	b.Trials[0].Scenario = "guest-density"
	b.Manifest.Lock.Options.Profile = "memory"
	b.Manifest.Lock.Options.PhaseBarriers = true
	b.Trials[0].Profile = "memory"
	stages := protocol.PhaseStages("guest-density")
	for _, stage := range stages {
		b.Trials[0].PhaseEvents = append(b.Trials[0].PhaseEvents, PhaseRecord{Event: protocol.PhaseEvent{Stage: stage}})
	}
	obs := collectors.GoMemoryWindow(protocol.GuestDensityPhase, protocol.GuestDensityDenominator)()
	obs = append(obs, protocol.Observation{Metric: "density.guest_memory.logical", DefinitionVersion: 1, Value: protocol.Value(float64(uint64(d.Pages) * 65536 * uint64(d.Instances))), Unit: "bytes", Scope: "instance_group_linear_memory", Phase: stages[1], Collector: "wazero.Memory.Size", CollectorVersion: "1.12.0", Quality: "exact", Profile: "memory", Status: "available", Denominator: "instance_group"})
	b.Trials[0].Samples[0].Observations = obs
	if err := ValidateGuestDensityEvidence(root, b); err != nil {
		t.Fatal(err)
	}
	obs[0].Denominator = "per_call"
	if ValidateGuestDensityEvidence(root, b) == nil {
		t.Fatal("accepted false allocator boundary")
	}
	obs[0].Denominator = protocol.GuestDensityDenominator
	b.Trials[0].Samples[0].Observations = obs[:7]
	if ValidateGuestDensityEvidence(root, b) == nil {
		t.Fatal("missing logical memory")
	}
	b.Trials[0].Samples[0].Observations = obs
	b.Trials[0].PhaseEvents[1].Event.Stage = stages[0]
	if ValidateGuestDensityEvidence(root, b) == nil {
		t.Fatal("reordered barriers")
	}
	b.Trials[0].PhaseEvents[1].Event.Stage = stages[1]
	external := protocol.Observation{Metric: "process.rss", DefinitionVersion: 1, Unit: "bytes", Scope: "adapter_process", Phase: "guest-density/" + stages[0], Collector: "procfs", CollectorVersion: "1", Quality: "boundary_snapshot_only", Profile: "memory", Status: "unsupported", Reason: "fixture", Denominator: "process"}
	b.Trials[0].PhaseEvents[0].Observations = []protocol.Observation{external}
	b.Trials[0].Samples[0].Observations = append(obs, external)
	if err := ValidateGuestDensityEvidence(root, b); err != nil {
		t.Fatal("controller-attached phase snapshots", err)
	}
	b.Trials[0].Samples[0].Observations[8].Phase = "compile/" + stages[0]
	if ValidateGuestDensityEvidence(root, b) == nil {
		t.Fatal("accepted mismatched attached snapshot")
	}
	b.Trials[0].Samples[0].Observations[8] = external
	if err := os.WriteFile(path, append(module, 0, 1, 0), 0644); err != nil {
		t.Fatal(err)
	}
	if ValidateGuestDensityEvidence(root, b) == nil {
		t.Fatal("accepted different module")
	}
}

func TestGuestDensitySacrificialBudget(t *testing.T) {
	ws, err := corpus.Generate(t.TempDir(), "guest-density")
	if err != nil {
		t.Fatal(err)
	}
	o := Options{Profile: "memory", Samples: 3, Operations: 2, Warmup: 1, PhaseBarriers: true}
	r := trialRequest(o, ws[0], "guest-density", -1)
	if r.Scenario != "guest-density" || r.Samples != 2 || r.Operations != 1 || r.Warmup != 0 || r.PhaseBarriers {
		t.Fatal("invalid sacrificial policy", r)
	}
	r = trialRequest(o, ws[0], "guest-density", 0)
	if protocol.ValidateGuestDensity(&protocol.Preparation{Workload: ws[0], Profile: "memory"}, &r) == nil {
		t.Fatal("coerced invalid measured budget")
	}
}
