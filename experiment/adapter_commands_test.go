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

func TestBuiltAdapterCommandPhases(t *testing.T) {
	ids := os.Getenv("WASMBENCH_PHASE_TEST_RUNTIMES")
	if ids == "" {
		t.Skip("set WASMBENCH_PHASE_TEST_RUNTIMES after building adapters")
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
	for _, r := range runtimes {
		if r.ID != "wazero" && r.ID != "wazero-interpreter" && r.ID != "wasmtime" && r.ID != "wasmtime-winch" {
			continue
		}
		for _, mode := range []string{"correct", "wrong-exit", "wrong-output", "overflow", "bad-file"} {
			t.Run(r.ID+"/"+mode, func(t *testing.T) {
				c, err := agent.Start(context.Background(), r.Command, filepath.Join(t.TempDir(), "log"), 10*time.Second)
				if err != nil {
					t.Fatal(err)
				}
				defer c.Close()
				fixture := filepath.Join(t.TempDir(), "input")
				if err := os.WriteFile(fixture, []byte("abc"), 0600); err != nil {
					t.Fatal(err)
				}
				command := &protocol.CommandContract{Argv: []string{"test", "arg"}, StdinFile: "input", Files: map[string]protocol.CommandFile{"input": {Path: fixture, Size: 3, SHA256: protocol.CommandDigest([]byte("abc"))}}, ExitCode: 7, StdoutSHA256: protocol.CommandDigest([]byte("abc")), StderrSHA256: protocol.CommandDigest([]byte("err")), OutputLimit: 16}
				switch mode {
				case "wrong-exit":
					command.ExitCode = 0
				case "wrong-output":
					command.StdoutSHA256 = protocol.CommandDigest(nil)
				case "overflow":
					command.OutputLimit = 1
				case "bad-file":
					if err := os.WriteFile(fixture, []byte("bad"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				w := protocol.Workload{ABI: "wasi-command", Export: "_start", HostProfile: "wasi-preview1-readonly-v1", Reset: "fresh_instance_per_sample", Command: command, Oracle: protocol.Oracle{Kind: "exact_command"}}
				if _, err = c.Call(protocol.Request{Method: "prepare", Prepare: &protocol.Preparation{Artifact: path, ArtifactSHA256: corpus.Hash(wasm), Workload: w, Profile: "memory"}}); err != nil {
					t.Fatal(err)
				}
				var events []protocol.PhaseEvent
				response, err := c.CallPhased(protocol.Request{Method: "run", Run: &protocol.RunRequest{Scenario: "compile", Samples: 2, Operations: 9, PhaseBarriers: true}}, func(e protocol.PhaseEvent) error { events = append(events, e); return nil })
				want := 6
				if mode != "correct" {
					want = 2
					if mode == "bad-file" {
						want = 0
					}
					if err == nil {
						t.Fatal("accepted invalid command")
					}
				} else if err != nil {
					t.Fatal(err)
				}
				if len(events) != want {
					t.Fatalf("events: %+v", events)
				}
				for i, e := range events {
					if e.SampleIndex != i/3 || e.Stage != []string{"before_compile", "compiled", "released"}[i%3] {
						t.Fatal(events)
					}
				}
				if mode != "correct" {
					return
				}
				if len(response.Samples) != 2 {
					t.Fatal(response)
				}
				for _, s := range response.Samples {
					if !s.Verified || s.CommandResult == nil || command.Verify(*s.CommandResult) != nil || s.Operations != 1 {
						t.Fatal(s)
					}
					if strings.HasPrefix(r.ID, "wazero") && len(s.Observations) != 7 {
						t.Fatal("missing Go allocation observations", s)
					}
					for _, o := range s.Observations {
						if o.Phase != "compile/api_window" || o.Denominator != "operation_excluding_verification_release_and_barriers" {
							t.Fatal(o)
						}
					}
				}
			})
		}
	}
}
