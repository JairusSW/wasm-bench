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

func TestBuiltAdapterTraps(t *testing.T) {
	ids := os.Getenv("WASMBENCH_TRAP_TEST_RUNTIMES")
	if ids == "" {
		t.Skip("build adapters and set WASMBENCH_TRAP_TEST_RUNTIMES")
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	runtimes, err := experiment.ResolveRuntimes(root, strings.Split(ids, ","))
	if err != nil {
		t.Fatal(err)
	}
	workloads, err := corpus.Generate(t.TempDir(), "traps")
	if err != nil {
		t.Fatal(err)
	}
	for _, runtime := range runtimes {
		for _, scenario := range []string{"first-call", "steady"} {
			for _, original := range workloads {
				for _, mode := range []string{"correct", "returns", "wrong-trap", "missing-export", "start-trap", "memory-profile", "code-profile", "memory-returns", "memory-wrong-trap"} {
					t.Run(runtime.ID+"/"+scenario+"/"+original.Export+"/"+mode, func(t *testing.T) {
						w := original
						profile := "timing"
						if strings.HasPrefix(mode, "memory-") {
							profile = "memory"
						}
						switch strings.TrimPrefix(mode, "memory-") {
						case "returns":
							w.Export = "no_trap"
						case "wrong-trap":
							if w.Oracle.ExpectedTrap == "unreachable" {
								w.Export = "integer_overflow"
							} else {
								w.Export = "unreachable"
							}
						case "missing-export":
							w.Export = "absent"
						case "start-trap":
							w.Artifact = filepath.Join(root, "corpus/testdata/instantiate-trap.wasm")
							w.SHA256, err = experiment.DigestFile(w.Artifact)
							if err != nil {
								t.Fatal(err)
							}
						case "code-profile":
							profile = "code"
						}
						c, err := agent.Start(context.Background(), runtime.Command, filepath.Join(t.TempDir(), "log"), 15*time.Second)
						if err != nil {
							t.Fatal(err)
						}
						defer c.Close()
						d, err := c.Call(protocol.Request{Method: "describe"})
						if err != nil || d.Description == nil || !d.Description.Capabilities["can_verify_invocation_traps"] {
							t.Fatal(d, err)
						}
						if profile == "memory" && !d.Description.Capabilities["can_measure_invocation_traps"] {
							t.Fatal("missing memory capability")
						}
						_, err = c.Call(protocol.Request{Method: "prepare", Prepare: &protocol.Preparation{Artifact: w.Artifact, ArtifactSHA256: w.SHA256, Workload: w, Profile: profile}})
						if err != nil {
							t.Fatal(err)
						}
						r, err := c.Call(protocol.Request{Method: "run", Run: &protocol.RunRequest{Scenario: scenario, Samples: 3, Warmup: 2, Operations: 9}})
						if mode != "correct" && mode != "memory-profile" {
							if err == nil || len(r.Samples) != 0 {
								t.Fatal("invalid trap accepted", r, err)
							}
							return
						}
						want := 3
						if scenario == "steady" {
							want = 5
						}
						if err != nil || len(r.Samples) != want {
							t.Fatal(r, err)
						}
						for i, s := range r.Samples {
							if profile == "timing" && len(s.Observations) != 0 {
								t.Fatal("timing collected memory", s)
							}
							if profile == "memory" {
								seen := map[string]bool{}
								for _, obs := range s.Observations {
									seen[obs.Metric] = true
									if obs.Profile != "memory" || obs.Status != "available" || obs.Value == nil {
										t.Fatal(obs)
									}
									if obs.Metric == "guest.memory.logical" && (*obs.Value != 65536 || obs.Phase != scenario+"/after_trap" || obs.Denominator != "instance") {
										t.Fatal(obs)
									}
									if strings.HasPrefix(obs.Metric, "host.") && obs.Phase != scenario+"/trap_api_window" {
										t.Fatal(obs)
									}
								}
								if !seen["guest.memory.logical"] {
									t.Fatal("missing guest snapshot", s)
								}
								if strings.HasPrefix(runtime.ID, "wago") || strings.HasPrefix(runtime.ID, "wazero") {
									for _, metric := range []string{"host.alloc.bytes", "host.alloc.count", "host.heap.start", "host.heap.end", "host.gc.cycles", "host.gc.forced_cycles", "host.gc.pause_time"} {
										if !seen[metric] {
											t.Fatal("missing", metric)
										}
									}
								} else if runtime.ID == "v8" && (!seen["host.js_heap.start"] || !seen["host.js_heap.end"] || seen["host.alloc.bytes"]) {
									t.Fatal("invalid V8 heap evidence", s)
								}
							}
							if s.Index != i || s.Warmup != (scenario == "steady" && i < 2) || s.Operations != 1 || s.SampleType != "individual_operation" || !s.Verified || protocol.VerifyTrap(w, s.TrapResult) != nil {
								t.Fatal(s)
							}
						}
					})
				}
			}
		}
	}
}
