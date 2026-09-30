package experiment_test

import (
	"context"
	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuiltAdapterAssemblyScriptAbort(t *testing.T) {
	ids := os.Getenv("WASMBENCH_HOST_TEST_RUNTIMES")
	if ids == "" {
		t.Skip("set WASMBENCH_HOST_TEST_RUNTIMES after building adapters")
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	runtimes, err := experiment.ResolveRuntimes(root, strings.Split(ids, ","))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "corpus", "testdata", "assemblyscript-abort.wasm")
	wasm, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range runtimes {
		for _, scenario := range []string{"compile", "instantiate", "first-call", "steady", "app-init", "teardown", "phased-compile", "phased-app-init", "phased-instantiate"} {
			for _, mode := range []string{"correct", "abort-call", "abort-init", "missing-host", "wrong-oracle", "unknown-import"} {
				t.Run(r.ID+"/"+scenario+"/"+mode, func(t *testing.T) {
					c, err := agent.Start(context.Background(), r.Command, filepath.Join(t.TempDir(), "log"), 15*time.Second)
					if err != nil {
						t.Fatal(err)
					}
					defer c.Close()
					w := protocol.Workload{ABI: "core", Export: "benchmark", Initialize: "initialize", Args: protocol.Values{}, Reset: "fresh_instance_per_sample", HostProfile: protocol.AssemblyScriptAbortProfile, Oracle: protocol.Oracle{Kind: "exact_u64", Expected: protocol.Values{7}}}
					switch mode {
					case "abort-call":
						w.Export = "abort_benchmark"
					case "abort-init":
						w.Initialize = "abort_init"
					case "missing-host":
						w.HostProfile = ""
					case "wrong-oracle":
						w.Oracle.Expected[0] = 8
					}
					profile := "timing"
					artifact, data := path, wasm
					if mode == "unknown-import" {
						artifact = filepath.Join(root, "corpus", "testdata", "assemblyscript-unknown-import.wasm")
						data, err = os.ReadFile(artifact)
						if err != nil {
							t.Fatal(err)
						}
					}
					req := protocol.RunRequest{Scenario: scenario, Samples: 2, Operations: 1, Warmup: 2}
					if strings.HasPrefix(scenario, "phased-") {
						profile = "memory"
						req.Scenario = strings.TrimPrefix(scenario, "phased-")
						req.PhaseBarriers = true
					}
					if _, err = c.Call(protocol.Request{Method: "prepare", Prepare: &protocol.Preparation{Artifact: artifact, ArtifactSHA256: corpus.Hash(data), Workload: w, Profile: profile}}); err != nil {
						t.Fatal(err)
					}
					var events []protocol.PhaseEvent
					response, err := c.CallPhased(protocol.Request{Method: "run", Run: &req}, func(e protocol.PhaseEvent) error { events = append(events, e); return nil })
					if mode != "correct" {
						if err == nil {
							t.Fatal("invalid host workload accepted", response)
						}
						if strings.HasPrefix(mode, "abort-") && (!strings.Contains(err.Error(), "AssemblyScript abort") || !strings.Contains(err.Error(), "message_ptr=4294967295")) {
							t.Fatal("lost abort evidence", err)
						}
						for _, e := range events {
							if e.Stage == "released" || e.Stage == "app_released" || e.Stage == "instance_released" {
								t.Fatal("failed workload has successful release", events)
							}
						}
						if scenario == "phased-app-init" {
							want := 0
							if mode == "abort-init" {
								want = 1
							}
							if mode == "abort-call" || mode == "wrong-oracle" {
								want = 2
							}
							if len(events) != want {
								t.Fatal("incorrect abort boundary", events)
							}
						}
						if scenario == "phased-instantiate" && (mode == "abort-init" || mode == "abort-call" || mode == "wrong-oracle") && len(events) != 2 {
							t.Fatal("incorrect instantiation failure boundary", events)
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					want := 2
					if scenario == "steady" {
						want = 4
					}
					if len(response.Samples) != want {
						t.Fatal(response)
					}
					if req.PhaseBarriers && len(events) != 6 {
						t.Fatal(events)
					}
					for _, s := range response.Samples {
						if !s.Verified || ((scenario == "first-call" || scenario == "steady" || scenario == "app-init") && (len(s.Result) != 1 || s.Result[0] != 7)) {
							t.Fatal(s)
						}
					}
				})
			}
		}
	}
}
