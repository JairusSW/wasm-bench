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

func TestBuiltAdapterCommandTeardown(t *testing.T) {
	ids := os.Getenv("WASMBENCH_COMMAND_TEST_RUNTIMES")
	if ids == "" {
		t.Skip("set WASMBENCH_COMMAND_TEST_RUNTIMES after building adapters")
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	runtimes, err := experiment.ResolveRuntimes(root, strings.Split(ids, ","))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "adapters", "wazero", "testdata", "command.wasm")
	wasm, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, runtime := range runtimes {
		for _, mode := range []string{"correct", "return", "memory", "file", "wrong-output", "wrong-exit", "overflow", "phase", "correct-phases", "wrong-phases"} {
			t.Run(runtime.ID+"/"+mode, func(t *testing.T) {
				path, wasm := path, wasm
				cmd := &protocol.CommandContract{Argv: []string{"test", "arg"}, Stdin: []byte("abc"), ExitCode: 7, StdoutSHA256: protocol.CommandDigest([]byte("abc")), StderrSHA256: protocol.CommandDigest([]byte("err")), OutputLimit: 16}
				profile := "timing"
				switch mode {
				case "return":
					path = filepath.Join(root, "adapters", "wazero", "testdata", "command-return.wasm")
					var err error
					wasm, err = os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					cmd.ExitCode = 0
				case "memory", "correct-phases", "wrong-phases":
					profile = "memory"
				case "file":
					cmd.Stdin = nil
					cmd.StdinFile = "input"
					cmd.Files = map[string]protocol.CommandFile{"input": {Data: []byte("abc"), SHA256: protocol.CommandDigest([]byte("abc"))}}
				case "wrong-output":
					cmd.StdoutSHA256 = protocol.CommandDigest(nil)
				case "wrong-exit":
					cmd.ExitCode = 0
				case "overflow":
					cmd.OutputLimit = 1
				}
				if mode == "wrong-phases" {
					cmd.ExitCode = 0
				}
				w := protocol.Workload{ABI: "wasi-command", Export: "_start", HostProfile: "wasi-preview1-readonly-v1", Reset: "fresh_instance_per_sample", Command: cmd, Oracle: protocol.Oracle{Kind: "exact_command"}}
				client, err := agent.Start(context.Background(), runtime.Command, filepath.Join(t.TempDir(), "log"), 15*time.Second)
				if err != nil {
					t.Fatal(err)
				}
				defer client.Close()
				if _, err = client.Call(protocol.Request{Method: "prepare", Prepare: &protocol.Preparation{Artifact: path, ArtifactSHA256: corpus.Hash(wasm), Workload: w, Profile: profile}}); err != nil {
					t.Fatal(err)
				}
				var events []protocol.PhaseEvent
				resp, err := client.CallPhased(protocol.Request{Method: "run", Run: &protocol.RunRequest{Scenario: "teardown", Samples: 3, Operations: 9, Warmup: 4, PhaseBarriers: mode == "phase" || strings.HasSuffix(mode, "phases")}}, func(e protocol.PhaseEvent) error { events = append(events, e); return nil })
				if mode != "correct" && mode != "return" && mode != "memory" && mode != "file" && mode != "correct-phases" {
					if err == nil {
						t.Fatal("accepted invalid teardown", resp)
					}
					if len(events) != 0 {
						t.Fatal("invalid command reached release boundary", events)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if len(resp.Samples) != 3 {
					t.Fatal(resp)
				}
				if mode == "correct-phases" {
					if len(events) != 6 {
						t.Fatal(events)
					}
					for i, e := range events {
						if e.SampleIndex != i/2 || e.Stage != []string{"before_teardown", "torn_down"}[i%2] {
							t.Fatal(events)
						}
					}
				}
				for i, s := range resp.Samples {
					if s.Index != i || s.Warmup || s.Operations != 1 || s.SampleType != "individual_operation" || !s.Verified || s.CommandResult == nil || cmd.Verify(*s.CommandResult) != nil {
						t.Fatal(s)
					}
					if strings.HasPrefix(runtime.ID, "wazero") && profile == "memory" && len(s.Observations) != 7 {
						t.Fatal("missing release observations", s)
					}
					for _, o := range s.Observations {
						if profile != "memory" || o.Phase != "teardown/api_window" || o.Denominator != "remaining_command_resources_excluding_execution_verification_fixture_cleanup" {
							t.Fatal(o)
						}
					}
				}
			})
		}
	}
}
