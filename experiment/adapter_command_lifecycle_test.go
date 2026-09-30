package experiment_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

func TestBuiltAdapterCommandLifecyclePhases(t *testing.T) {
	ids := os.Getenv("WASMBENCH_COMMAND_LIFECYCLE_TEST_RUNTIMES")
	if ids == "" {
		t.Skip("build adapters and set WASMBENCH_COMMAND_LIFECYCLE_TEST_RUNTIMES")
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	runtimes, err := experiment.ResolveRuntimes(root, strings.Split(ids, ","))
	if err != nil {
		t.Fatal(err)
	}
	for _, runtime := range runtimes {
		for _, abi := range []string{"wasi-command", "emscripten"} {
			for _, scenario := range []string{"compile", "instantiate", "first-call"} {
				for _, mode := range []string{"correct", "wrong-exit", "wrong-output"} {
					t.Run(runtime.ID+"/"+abi+"/"+scenario+"/"+mode, func(t *testing.T) {
						file, export, host := "command.wasm", "_start", "wasi-preview1-readonly-v1"
						command := &protocol.CommandContract{Argv: []string{"test", "arg"}, Stdin: []byte("abc"), ExitCode: 7, StdoutSHA256: protocol.CommandDigest([]byte("abc")), StderrSHA256: protocol.CommandDigest([]byte("err")), OutputLimit: 128}
						if abi == "emscripten" {
							file, export, host = "emscripten-command.wasm", "main", protocol.EmscriptenStdioProfile
							command = &protocol.CommandContract{Argv: []string{"fixture"}, ExitCode: 0, StdoutSHA256: protocol.CommandDigest(nil), StderrSHA256: protocol.CommandDigest(nil), OutputLimit: 128}
						}
						if mode == "wrong-exit" {
							command.ExitCode++
						}
						if mode == "wrong-output" {
							command.StdoutSHA256 = protocol.CommandDigest([]byte("wrong"))
						}
						path := filepath.Join(root, "adapters/wazero/testdata", file)
						wasm, err := os.ReadFile(path)
						if err != nil {
							t.Fatal(err)
						}
						client, err := agent.Start(context.Background(), runtime.Command, filepath.Join(t.TempDir(), "log"), 15*time.Second)
						if err != nil {
							t.Fatal(err)
						}
						defer client.Close()
						description, err := client.Call(protocol.Request{Method: "describe"})
						if err != nil || description.Description == nil || !description.Description.Capabilities["can_command_"+scenario+"_phases"] || description.Description.Configuration["command_lifecycle_phases_policy"] == "" {
							t.Fatal("missing qualified command phases policy", err)
						}
						w := protocol.Workload{ABI: abi, Export: export, HostProfile: host, Reset: "fresh_instance_per_sample", Command: command, Oracle: protocol.Oracle{Kind: "exact_command"}}
						if _, err := client.Call(protocol.Request{Method: "prepare", Prepare: &protocol.Preparation{Artifact: path, ArtifactSHA256: corpus.Hash(wasm), Workload: w, Profile: "memory"}}); err != nil {
							t.Fatal(err)
						}
						var events []protocol.PhaseEvent
						response, err := client.CallPhased(protocol.Request{Method: "run", Run: &protocol.RunRequest{Scenario: scenario, Samples: 2, Operations: 9, PhaseBarriers: true}}, func(event protocol.PhaseEvent) error { events = append(events, event); return nil })
						want := 6
						if mode != "correct" {
							want = 2
							if err == nil || !strings.Contains(err.Error(), "incorrect result") || len(response.Samples) != 0 {
								t.Fatal("bad oracle returned qualified measurements", err, response)
							}
						} else if err != nil {
							t.Fatal(err)
						}
						if len(events) != want {
							t.Fatalf("unexpected boundaries: %+v", events)
						}
						stages := protocol.PhaseStages(scenario)
						for i, event := range events {
							if event.SampleIndex != i/3 || event.Stage != stages[i%3] {
								t.Fatal(events)
							}
						}
						if mode != "correct" {
							return
						}
						if len(response.Samples) != 2 {
							t.Fatal(response)
						}
						for i, sample := range response.Samples {
							if sample.Index != i || !sample.Verified || sample.Warmup || sample.Operations != 1 || sample.SampleType != "individual_operation" || sample.CommandResult == nil || command.Verify(*sample.CommandResult) != nil {
								t.Fatal("invalid command sample", sample)
							}
							if strings.HasPrefix(runtime.ID, "wazero") && len(sample.Observations) != 7 {
								t.Fatal("missing Go API-window allocation evidence", sample)
							}
							for _, observation := range sample.Observations {
								if observation.Phase != scenario+"/api_window" || observation.Denominator != "operation_excluding_verification_release_and_barriers" {
									t.Fatal("wrong API-window domain", observation)
								}
							}
						}
					})
				}
			}
		}
	}
}
