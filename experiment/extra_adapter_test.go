package experiment_test

import (
	"context"
	"encoding/hex"
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

// No adapter silently skips: opt in explicitly after building the selected SDKs.
func TestBuiltExtraAdapter(t *testing.T) {
	ids := os.Getenv("WASMBENCH_EXTRA_TEST_RUNTIMES")
	if ids == "" {
		t.Skip("set WASMBENCH_EXTRA_TEST_RUNTIMES after building")
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	runtimes, err := experiment.ResolveRuntimes(root, strings.Split(ids, ","))
	if err != nil {
		t.Fatal(err)
	}
	for _, rt := range runtimes {
		t.Run(rt.ID, func(t *testing.T) {
			start := func(t *testing.T) *agent.Client {
				t.Helper()
				c, err := agent.Start(context.Background(), rt.Command, filepath.Join(t.TempDir(), "log"), 20*time.Second)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { c.Close() })
				return c
			}
			ws, err := corpus.Generate(t.TempDir(), "core")
			if err != nil {
				t.Fatal(err)
			}
			t.Run("integer-boundaries", func(t *testing.T) {
				for _, bits := range []int{32, 64} {
					data, err := hex.DecodeString("0061736d0100000001060160017e017e030201000707010372756e00000a0601040020000b")
					if err != nil {
						t.Fatal(err)
					}
					value := uint64(^uint64(0))
					if bits == 32 {
						data[13] = 0x7f
						data[15] = 0x7f
						value = uint64(^uint32(0))
					}
					artifact := filepath.Join(t.TempDir(), "integer.wasm")
					if err = os.WriteFile(artifact, data, 0644); err != nil {
						t.Fatal(err)
					}
					w := protocol.Workload{ABI: "core", Export: "run", Reset: "stateless", Args: protocol.Values{value}, Oracle: protocol.Oracle{Kind: "exact_u64", Expected: protocol.Values{value}}}
					for _, scenario := range []string{"compile", "instantiate", "first-call", "steady"} {
						c := start(t)
						if _, err = c.Call(protocol.Request{Method: "prepare", Prepare: &protocol.Preparation{Artifact: artifact, ArtifactSHA256: corpus.Hash(data), Workload: w, Profile: "timing"}}); err != nil {
							t.Fatal(err)
						}
						r := protocol.RunRequest{Scenario: scenario, Samples: 1, Operations: 1}
						response, err := c.Call(protocol.Request{Method: "run", Run: &r})
						if rt.ID == "wasm3" && scenario == "instantiate" {
							if response.Status != "unsupported" {
								t.Fatal(response, err)
							}
							continue
						}
						if err != nil || len(response.Samples) != 1 || len(response.Samples[0].Result) != 1 || response.Samples[0].Result[0] != value {
							t.Fatal(bits, scenario, response, err)
						}
						c.Close()
					}
				}
			})
			for _, scenario := range []string{"compile", "instantiate", "first-call", "steady"} {
				for _, profile := range []string{"timing", "memory"} {
					t.Run(scenario+"/"+profile, func(t *testing.T) {
						c := start(t)
						d, err := c.Call(protocol.Request{Method: "describe"})
						if err != nil || d.Description == nil {
							t.Fatal(d, err)
						}
						if d.Description.Version == "" || d.Description.Configuration["release_policy"] == "" {
							t.Fatal("missing configuration", d)
						}
						for _, w := range ws {
							if _, err = c.Call(protocol.Request{Method: "prepare", Prepare: &protocol.Preparation{Artifact: w.Artifact, ArtifactSHA256: w.SHA256, Workload: w, Profile: profile}}); err != nil {
								t.Fatal(err)
							}
							r := protocol.RunRequest{Scenario: scenario, Samples: 2, Operations: 1, PhaseBarriers: profile == "memory"}
							if scenario == "steady" {
								r.Operations = 3
								r.Warmup = 1
							}
							var events []protocol.PhaseEvent
							response, err := c.CallPhased(protocol.Request{Method: "run", Run: &r}, func(e protocol.PhaseEvent) error { events = append(events, e); return nil })
							if rt.ID == "wasm3" && scenario == "instantiate" {
								if response.Status != "unsupported" {
									t.Fatal("wasm3 must reject separate instantiate", response, err)
								}
								continue
							}
							if err != nil || response.Status != "ok" {
								t.Fatal(response, err)
							}
							if len(response.Samples) != r.Samples+r.Warmup {
								t.Fatal(response)
							}
							for i, s := range response.Samples {
								if s.Index != i || s.Warmup != (i < r.Warmup) || !s.Verified || s.ElapsedNS < 0 || s.Operations != r.Operations || len(s.Result) != 1 || s.Result[0] != w.Oracle.Expected[0] {
									t.Fatal(s)
								}
							}
							if r.PhaseBarriers {
								want := protocol.PhaseStages(scenario)
								if len(events) != len(want)*len(response.Samples) {
									t.Fatal(events)
								}
								for i, e := range events {
									if e.SampleIndex != i/len(want) || e.Stage != want[i%len(want)] {
										t.Fatal(events)
									}
								}
							}
						}
					})
				}
			}
			for _, mode := range []string{"wrong-result", "digest", "profile", "abi", "vectors", "unprepared", "bounds", "steady-reset"} {
				t.Run(mode, func(t *testing.T) {
					c := start(t)
					w := ws[0]
					p := protocol.Preparation{Artifact: w.Artifact, ArtifactSHA256: w.SHA256, Workload: w, Profile: "timing"}
					r := protocol.RunRequest{Scenario: "first-call", Samples: 1, Operations: 1}
					switch mode {
					case "wrong-result":
						p.Workload.Oracle.Expected = protocol.Values{99}
					case "digest":
						p.ArtifactSHA256 = strings.Repeat("0", 64)
					case "profile":
						p.Profile = "profiling"
					case "abi":
						p.Workload.ABI = "wasi-command"
					case "vectors":
						p.Workload.Vectors = &protocol.VectorContract{}
					case "bounds":
						r.Samples = 0
					case "steady-reset":
						p.Workload.Reset = "fresh_instance_per_sample"
						r.Scenario = "steady"
					}
					if mode != "unprepared" {
						resp, err := c.Call(protocol.Request{Method: "prepare", Prepare: &p})
						if mode == "digest" || mode == "profile" || mode == "abi" || mode == "vectors" {
							if err == nil && resp.Status == "ok" {
								t.Fatal("invalid preparation accepted", resp)
							}
							return
						}
						if err != nil {
							t.Fatal(err)
						}
					}
					resp, err := c.Call(protocol.Request{Method: "run", Run: &r})
					if err == nil && resp.Status == "ok" {
						t.Fatal("invalid run accepted", resp)
					}
				})
			}
			for _, scenario := range []string{"compile", "instantiate", "first-call"} {
				if rt.ID == "wasm3" && scenario == "instantiate" {
					continue // Unsupported-stage rejection is tested above, not a positive case.
				}
				for _, wrong := range []bool{false, true} {
					t.Run("initialization/"+scenario+"/"+map[bool]string{false: "correct", true: "wrong-memory"}[wrong], func(t *testing.T) {
						data, err := os.ReadFile(filepath.Join(root, "corpus", "testdata", "app-init.wasm"))
						if err != nil {
							t.Fatal(err)
						}
						w := protocol.Workload{ABI: "core", Export: "benchmark", Reset: "fresh_instance_per_sample", Initialize: "initialize", Args: protocol.Values{}, Input: &protocol.MemoryInput{PointerExport: "input_ptr", Offset: 0, Hex: "07000000"}, Oracle: protocol.Oracle{Kind: "exact_u64", Expected: protocol.Values{7}, Memory: []protocol.MemoryCheck{{Offset: 64, Hex: "2a000000"}}}}
						if wrong {
							w.Oracle.Memory[0].Hex = "00000000"
						}
						c := start(t)
						if _, err = c.Call(protocol.Request{Method: "prepare", Prepare: &protocol.Preparation{Artifact: filepath.Join(root, "corpus", "testdata", "app-init.wasm"), ArtifactSHA256: corpus.Hash(data), Workload: w, Profile: "memory"}}); err != nil {
							t.Fatal(err)
						}
						resp, err := c.CallPhased(protocol.Request{Method: "run", Run: &protocol.RunRequest{Scenario: scenario, Samples: 2, Operations: 1, PhaseBarriers: true}}, func(protocol.PhaseEvent) error { return nil })
						if wrong {
							if err == nil && resp.Status == "ok" {
								t.Fatal("wrong memory accepted")
							}
							return
						}
						if err != nil || resp.Status != "ok" || len(resp.Samples) != 2 {
							t.Fatal(resp, err)
						}
						for _, s := range resp.Samples {
							if !s.Verified || len(s.Observations) == 0 {
								t.Fatal(s)
							}
						}
					})
				}
			}
		})
	}
}
