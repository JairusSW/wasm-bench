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

func TestBuiltAdapterVectors(t *testing.T) {
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
	// Requires the ordered sequence exactly once on each fresh instance.
	wasm, err := os.ReadFile(filepath.Join(root, "corpus", "testdata", "vector-lifecycle.wasm"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "vectors.wasm")
	if err := os.WriteFile(path, wasm, 0600); err != nil {
		t.Fatal(err)
	}
	for _, runtime := range runtimes {
		for _, scenario := range []string{"compile", "instantiate", "first-call", "steady", "teardown"} {
			for _, mode := range []string{"correct", "correct-memory", "correct-phases", "correct-single-phases", "wrong-phases", "wrong-first-phases", "correct-init-phases", "wrong-init-phases", "wrong-digest", "bad-hex", "empty-cases", "bad-length", "budget", "missing-pointer", "input-bounds", "output-bounds", "mixed-input"} {
				phased := strings.HasSuffix(mode, "phases")
				if phased && scenario != "compile" && scenario != "teardown" && scenario != "instantiate" && scenario != "first-call" {
					continue
				}
				t.Run(runtime.ID+"/"+scenario+"/"+mode, func(t *testing.T) {
					c, err := agent.Start(context.Background(), runtime.Command, filepath.Join(t.TempDir(), "log"), 10*time.Second)
					if err != nil {
						t.Fatal(err)
					}
					defer c.Close()
					if phased && (scenario == "instantiate" || scenario == "first-call") {
						policy := "vector_instantiate_phases_policy"
						if scenario == "first-call" {
							policy = "vector_first_call_phases_policy"
						}
						description, err := c.Call(protocol.Request{Method: "describe"})
						if err != nil || description.Description == nil || !description.Description.Capabilities["can_vector_"+scenario+"_phases"] || description.Description.Configuration[policy] == "" {
							t.Fatal("missing vector lifecycle capability/policy", err)
						}
						if scenario == "first-call" && description.Description.Configuration[policy] != protocol.VectorFirstCallPhasesPolicy {
							t.Fatal("inconsistent vector memory window policy", description.Description.Configuration[policy])
						}
					}
					v := &protocol.VectorContract{InputOffset: 32, OutputOffset: 16, OutputLen: 1, Mod: 3, Cases: []protocol.VectorCase{{Out: "ab"}, {Len: 7, Out: "ab"}}}
					if mode == "correct-single-phases" {
						v.Cases = v.Cases[:1]
					}
					if mode == "wrong-digest" || mode == "wrong-phases" || mode == "wrong-init-phases" {
						v.Cases[1].Out = "ac"
					}
					if mode == "wrong-first-phases" {
						v.Cases[0].Out = "ac"
					}
					w := protocol.Workload{Schema: 1, ID: "vectors", ABI: "core", Export: "benchmark", Reset: "fresh_instance_per_sample", WorkUnit: "vector_sequence", Units: 1, Vectors: v, VectorByteBudget: 1024, Oracle: protocol.Oracle{Kind: "exact_vectors"}}
					artifact, artifactBytes := path, wasm
					if strings.Contains(mode, "init-phases") {
						artifact = filepath.Join(root, "corpus", "testdata", "vector-initialization.wasm")
						artifactBytes, err = os.ReadFile(artifact)
						if err != nil {
							t.Fatal(err)
						}
						w.Initialize = "initialize"
					}
					switch mode {
					case "bad-hex":
						v.Cases[1].Out = "zz"
					case "empty-cases":
						v.Cases = nil
					case "bad-length":
						v.OutputLen = 2
					case "budget":
						w.VectorByteBudget = 1
					case "missing-pointer":
						v.InputPointerExport = "absent"
					case "input-bounds":
						v.InputOffset = 65535
					case "output-bounds":
						v.OutputOffset = 65536
					case "mixed-input":
						w.Input = &protocol.MemoryInput{Hex: "ab"}
					}
					profile := "timing"
					if mode == "correct-memory" || phased {
						profile = "memory"
					}
					_, err = c.Call(protocol.Request{Method: "prepare", Prepare: &protocol.Preparation{Artifact: artifact, ArtifactSHA256: corpus.Hash(artifactBytes), Profile: profile, Workload: w}})
					if err != nil {
						t.Fatal(err)
					}
					var events []protocol.PhaseEvent
					var callback func(protocol.PhaseEvent) error
					if phased {
						callback = func(e protocol.PhaseEvent) error { events = append(events, e); return nil }
					}
					response, err := c.CallPhased(protocol.Request{Method: "run", Run: &protocol.RunRequest{Scenario: scenario, Samples: 2, Operations: 99, PhaseBarriers: phased}}, callback)
					if !strings.HasPrefix(mode, "correct") {
						if len(response.Samples) != 0 {
							t.Fatal("invalid vector returned qualified samples", response.Samples)
						}
						if phased && scenario == "teardown" && len(events) != 0 {
							t.Fatal("unverified vector reached release barrier", events)
						}
						if phased && (scenario == "compile" || scenario == "instantiate") && (len(events) != 2 || events[0].Stage != protocol.PhaseStages(scenario)[0] || events[1].Stage != protocol.PhaseStages(scenario)[1]) {
							t.Fatal("incorrect phase failure boundary", events)
						}
						if phased && scenario == "first-call" {
							want := 2
							if mode == "wrong-first-phases" {
								want = 1
							}
							if len(events) != want {
								t.Fatal("incorrect failed sequence boundaries", events)
							}
							for i, e := range events {
								if e.SampleIndex != 0 || e.Stage != protocol.PhaseStages(scenario)[i] {
									t.Fatal(events)
								}
							}
						}
						if err == nil || ((mode == "wrong-digest" || mode == "wrong-phases" || mode == "wrong-init-phases" || mode == "wrong-first-phases") && !strings.Contains(err.Error(), "incorrect result")) {
							t.Fatalf("invalid vector contract accepted: %v", err)
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					if len(response.Samples) != 2 {
						t.Fatal(response)
					}
					if phased {
						stages := protocol.PhaseStages(scenario)
						if len(events) != 2*len(stages) {
							t.Fatal(events)
						}
						for i, e := range events {
							if e.SampleIndex != i/len(stages) || e.Stage != stages[i%len(stages)] {
								t.Fatal(events)
							}
						}
					}
					for _, s := range response.Samples {
						if scenario == "first-call" && s.SampleType != "sequence_call_sum" {
							t.Fatal("sequence mislabeled as individual call", s)
						}
						if scenario == "instantiate" && s.SampleType != "individual_operation" {
							t.Fatal("instantiation mislabeled as a batch/sequence timer", s)
						}
						if phased && scenario == "instantiate" {
							count := 0
							for _, o := range s.Observations {
								if o.Metric == "guest.memory.logical" {
									continue
								}
								count++
								if o.Phase != "instantiate/api_window" || o.Status != "available" || o.Value == nil {
									t.Fatal("wrong instantiation diagnostic window", o)
								}
								if (runtime.ID == "wago" || strings.HasPrefix(runtime.ID, "wazero")) && o.Denominator != "instantiation_including_start_excluding_initialization_verification_release" {
									t.Fatal(o)
								}
							}
							if (runtime.ID == "wago" || strings.HasPrefix(runtime.ID, "wazero")) && count != 7 {
								t.Fatal("missing Go allocator observations", s)
							}
							if strings.HasPrefix(runtime.ID, "v8") && count != 2 {
								t.Fatal("missing V8 heap snapshots", s)
							}
						}
						if phased && scenario == "first-call" {
							count := 0
							for _, o := range s.Observations {
								if o.Metric == "guest.memory.logical" {
									continue
								}
								count++
								if o.Phase != "first-call/vector_sequence_call_window" || o.Denominator != protocol.VectorFirstCallWindowDenominator || o.Status != "available" || o.Value == nil {
									t.Fatal("wrong vector sequence memory domain", o)
								}
							}
							if (runtime.ID == "wago" || strings.HasPrefix(runtime.ID, "wazero")) && count != 7 {
								t.Fatal("missing Go sequence memory evidence", s)
							}
							if strings.HasPrefix(runtime.ID, "v8") && count != 2 {
								t.Fatal("missing V8 sequence heap snapshots", s)
							}
						}
						if profile == "memory" && !phased {
							found := false
							for _, o := range s.Observations {
								if o.Metric == "guest.memory.logical" && o.Status == "available" && o.Value != nil && *o.Value == 65536 {
									found = true
								}
							}
							if !found {
								t.Fatal("missing logical memory snapshot", s)
							}
						} else if profile == "timing" && len(s.Observations) != 0 {
							t.Fatal("diagnostics contaminated timing", s)
						}
						if !s.Verified || s.Operations != 1 {
							t.Fatal(s)
						}
					}
				})
			}
		}
	}
}
