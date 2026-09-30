package main

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
)

func TestGuestCheckpointLifecycle(t *testing.T) {
	workloads, err := corpus.Generate(t.TempDir(), "checkpoints")
	if err != nil {
		t.Fatal(err)
	}
	for _, interpreter := range []bool{false, true} {
		for _, w := range workloads {
			t.Run(fmt.Sprintf("%t/%d", interpreter, w.Checkpoint.Pages), func(t *testing.T) {
				a := &adapter{interpreter: interpreter}
				defer a.close()
				if err := a.prepare(&protocol.Preparation{Artifact: w.Artifact, ArtifactSHA256: w.SHA256, Workload: w, Profile: "memory"}); err != nil {
					t.Fatal(err)
				}
				for _, scenario := range protocol.CheckpointScenarios() {
					var events []protocol.PhaseEvent
					a.barrier = func(e protocol.PhaseEvent) error { events = append(events, e); return nil }
					samples, err := a.run(&protocol.RunRequest{Scenario: scenario, Samples: 2, Operations: 1, PhaseBarriers: true})
					if err != nil {
						t.Fatal(scenario, err)
					}
					if len(samples) != 2 || len(events) != 6 {
						t.Fatal("missing fresh samples/barriers")
					}
					for i, s := range samples {
						if s.Index != i {
							t.Fatal("sample index")
						}
						if err := protocol.VerifyCheckpointSample(w, scenario, s); err != nil {
							t.Fatal(err)
						}
						if len(s.Observations) != 8 {
							t.Fatal("missing precise allocation/payload evidence")
						}
						for j, stage := range protocol.PhaseStages(scenario) {
							if !reflect.DeepEqual(events[i*3+j], protocol.PhaseEvent{SampleIndex: i, Stage: stage}) {
								t.Fatal("phase order")
							}
						}
					}
				}
				// A valid custom section still violates the exact module-state scope.
				a.wasm = append(a.wasm, 0, 1, 0)
				if _, err := a.run(&protocol.RunRequest{Scenario: "checkpoint-create", Samples: 1, Operations: 1}); err == nil {
					t.Fatal("accepted arbitrary modified module")
				}
			})
		}
	}
}

func TestCheckpointCopyIsolation(t *testing.T) {
	ws, err := corpus.Generate(t.TempDir(), "checkpoints")
	if err != nil {
		t.Fatal(err)
	}
	w := ws[0]
	a := &adapter{}
	defer a.close()
	if err := a.prepare(&protocol.Preparation{Artifact: w.Artifact, ArtifactSHA256: w.SHA256, Workload: w, Profile: "timing"}); err != nil {
		t.Fatal(err)
	}
	if err := a.setupCompiled(); err != nil {
		t.Fatal(err)
	}
	m, err := a.instantiate()
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close(ctx)
	if err := a.initialize(m); err != nil {
		t.Fatal(err)
	}
	data, g, err := checkpointState(m, 65536)
	if err != nil {
		t.Fatal(err)
	}
	saved := saveGuestCheckpoint(data, g)
	data[100] = 99
	g.Set(99)
	if saved.memory[100] != 7 || saved.state != 42 {
		t.Fatal("saved state aliases source")
	}
	if err := restoreGuestCheckpoint(m, g, saved); err != nil {
		t.Fatal(err)
	}
	if data[100] != 7 || g.Get() != 42 {
		t.Fatal("did not restore all state")
	}
	saved.memory[100] = 88
	if data[100] != 7 {
		t.Fatal("restored instance aliases checkpoint")
	}
}
