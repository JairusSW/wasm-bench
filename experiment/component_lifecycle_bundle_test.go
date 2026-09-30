package experiment

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/protocol"
)

func TestComponentCommandLifecycleBundle(t *testing.T) {
	w, runtimes, analyzer := componentResetFixture(t)
	base := os.Getenv("WASMBENCH_COMPONENT_LIFECYCLE_EVIDENCE_DIR")
	if base == "" {
		base = filepath.Join(t.TempDir(), "lifecycle")
	}
	for _, profile := range []string{"timing", "memory"} {
		t.Run(profile, func(t *testing.T) {
			lock, err := NewLock(Options{Suite: "component-lifecycle", Profile: profile, Scenarios: []string{"compile", "instantiate", "first-call"}, Launches: 1, Samples: 2, Operations: 1, PhaseBarriers: profile == "memory", Timeout: 30 * time.Second}, runtimes, []protocol.Workload{w})
			if err != nil {
				t.Fatal(err)
			}
			lock.Analyzer, lock.ArchiveTools = analyzer, true
			out := base + "-" + profile
			original, err := Run(context.Background(), lock, filepath.Dir(w.Artifact), out, func(string) {})
			if err != nil {
				t.Fatal(err)
			}
			record, err := RestoreTools(original, out+"-restored-tools")
			if err != nil {
				t.Fatal(err)
			}
			var restored Lock
			if err := ReadJSON(record.Lock, &restored); err != nil {
				t.Fatal(err)
			}
			for i, r := range restored.Runtimes {
				if r.Command[0] == runtimes[i].Command[0] {
					t.Fatal("replay must use restored adapter")
				}
			}
			replayed, err := Run(context.Background(), restored, filepath.Dir(record.Lock), out+"-replayed", func(string) {})
			if err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{original, replayed} {
				b, err := Load(path)
				if err != nil {
					t.Fatal(err)
				}
				if len(b.Trials) != 8 {
					t.Fatalf("missing backend/scenario/check coverage: %d", len(b.Trials))
				}
				measured := map[string]bool{}
				for _, trial := range b.Trials {
					if trial.Block >= 0 {
						key := trial.Runtime + "/" + trial.Scenario
						if measured[key] {
							t.Fatal("duplicate runtime/scenario trial", key)
						}
						measured[key] = true
					}
					if trial.Status != "ok" || len(trial.Samples) != 2 {
						t.Fatalf("%s %s %s: %s %s", trial.Runtime, trial.Scenario, trial.ID, trial.Status, trial.Reason)
					}
					for _, sample := range trial.Samples {
						if !sample.Verified || sample.CommandResult == nil || sample.CommandResult.StdoutSHA256 != w.Command.StdoutSHA256 || sample.CommandResult.StderrSHA256 != w.Command.StderrSHA256 {
							t.Fatal("missing behavioral oracle outside lifecycle timer")
						}
					}
					wantPhases := 0
					if profile == "memory" && trial.Block >= 0 {
						wantPhases = 6
					}
					if len(trial.PhaseEvents) != wantPhases {
						t.Fatalf("phase coverage: got %d want %d", len(trial.PhaseEvents), wantPhases)
					}
					for i, event := range trial.PhaseEvents {
						stages := protocol.PhaseStages(trial.Scenario)
						if event.Event.SampleIndex != i/3 || event.Event.Stage != stages[i%3] || len(event.Observations) == 0 {
							t.Fatal("missing ordered external boundary observations")
						}
					}
					if profile == "timing" && len(trial.Observations) != 0 {
						t.Fatal("timing pass acquired memory instrumentation")
					}
					if profile == "memory" && trial.Block >= 0 && !slices.ContainsFunc(trial.Observations, func(o protocol.Observation) bool { return o.Metric == "process.peak_rss" }) {
						t.Fatal("missing explicit process high-water observation")
					}
				}
				for _, r := range runtimes {
					for _, scenario := range lock.Options.Scenarios {
						if !measured[r.ID+"/"+scenario] {
							t.Fatal("missing runtime/scenario cell", r.ID, scenario)
						}
					}
				}
			}
		})
	}
}

// The adapter must fail closed before emitting measured samples or barriers for
// profiles/scenarios that have not been qualified, and retain output checking.
func TestComponentCommandLifecycleRejections(t *testing.T) {
	w, runtimes, _ := componentResetFixture(t)
	for _, runtime := range runtimes {
		for _, mode := range []string{"timing-barriers", "counters", "steady", "operations", "wrong-output"} {
			t.Run(runtime.ID+"/"+mode, func(t *testing.T) {
				c, err := agent.Start(context.Background(), runtime.Command, filepath.Join(t.TempDir(), "log"), 30*time.Second)
				if err != nil {
					t.Fatal(err)
				}
				defer c.Close()
				workload, profile := w, "timing"
				command := *w.Command
				workload.Command = &command
				req := protocol.RunRequest{Scenario: "compile", Samples: 1, Operations: 1}
				switch mode {
				case "timing-barriers":
					req.PhaseBarriers = true
				case "counters":
					profile = "counters"
				case "steady":
					req.Scenario = "steady"
				case "operations":
					req.Operations = 2
				case "wrong-output":
					command.StdoutSHA256 = protocol.CommandDigest(nil)
				}
				if _, err := c.Call(protocol.Request{Method: "prepare", Prepare: &protocol.Preparation{Artifact: w.Artifact, ArtifactSHA256: w.SHA256, Workload: workload, Profile: profile}}); err != nil {
					t.Fatal(err)
				}
				events := 0
				response, err := c.CallPhased(protocol.Request{Method: "run", Run: &req}, func(protocol.PhaseEvent) error { events++; return nil })
				if len(response.Samples) != 0 || events != 0 {
					t.Fatal("unsupported/incorrect command emitted measurements")
				}
				if mode == "wrong-output" {
					if err == nil {
						t.Fatal("compile-only success bypassed command oracle")
					}
				} else if response.Status != "unsupported" {
					t.Fatalf("unsupported request not rejected explicitly: %s %v", response.Status, err)
				}
			})
		}
	}
}

func TestComponentCommandControllerCapabilityGate(t *testing.T) {
	w := protocol.Workload{ID: "component/command", ABI: "component", Export: "_start", HostProfile: "wasi-preview2-readonly-v1", Reset: "fresh_instance_per_sample", Oracle: protocol.Oracle{Kind: "exact_command"}, Command: &protocol.CommandContract{Argv: []string{"test"}, OutputLimit: 4096, StdoutSHA256: protocol.CommandDigest(nil)}}
	for _, mode := range []string{"legacy-compile", "legacy-instantiate", "legacy-memory", "timing-barriers", "counters", "steady", "operations", "missing-barrier-capability"} {
		t.Run(mode, func(t *testing.T) {
			options := Options{Profile: "timing", Samples: 1, Operations: 1, Timeout: time.Second}
			scenario := "first-call"
			capabilities := map[string]bool{"can_run_component_commands": true, "can_component_command_lifecycle": true, "can_component_command_phases": true}
			switch mode {
			case "legacy-compile", "legacy-instantiate", "legacy-memory":
				delete(capabilities, "can_component_command_lifecycle")
				if mode == "legacy-memory" {
					options.Profile = "memory"
				} else if mode == "legacy-compile" {
					scenario = "compile"
				} else {
					scenario = "instantiate"
				}
			case "timing-barriers":
				options.PhaseBarriers = true
			case "counters":
				options.Profile = "counters"
			case "steady":
				scenario = "steady"
			case "operations":
				options.Operations = 2
			case "missing-barrier-capability":
				options.Profile, options.PhaseBarriers = "memory", true
				delete(capabilities, "can_component_command_phases")
			}
			// No executable exists: refusal must happen before process launch.
			r := Runtime{ID: "unsupported", Description: &protocol.Description{ABIs: []string{"component"}, Scenarios: []string{scenario}, Capabilities: capabilities, PhaseBarrierScenarios: []string{scenario}}}
			trial := runTrial(context.Background(), t.TempDir(), options, r, w, scenario, 0, "refused")
			if trial.Status != "unsupported" || len(trial.Samples) != 0 || len(trial.PhaseEvents) != 0 {
				t.Fatalf("capability gate reached execution: %+v", trial)
			}
		})
	}
}
