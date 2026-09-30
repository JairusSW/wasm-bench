package experiment_test

import (
	"context"
	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestBuiltAdapterInstantiatePhases(t *testing.T) {
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
	for _, runtime := range runtimes {
		for _, mode := range []string{"correct", "no-initializer", "timing", "wrong-oracle", "missing-init", "trap-init", "trap-start", "no-input"} {
			t.Run(runtime.ID+"/"+mode, func(t *testing.T) {
				suite := "lifecycle"
				if mode == "no-initializer" {
					suite = "core"
				}
				workloads, err := corpus.Generate(t.TempDir(), suite)
				if err != nil {
					t.Fatal(err)
				}
				w := workloads[0]
				profile := "memory"
				switch mode {
				case "timing":
					profile = "timing"
				case "wrong-oracle":
					w.Oracle.Expected[0]++
				case "missing-init":
					w.Initialize = "absent"
				case "trap-init":
					w.Initialize = "trap_init"
				case "trap-start":
					w.Artifact = filepath.Join(root, "corpus", "testdata", "instantiate-trap.wasm")
					data, err := os.ReadFile(w.Artifact)
					if err != nil {
						t.Fatal(err)
					}
					w.SHA256 = corpus.Hash(data)
				case "no-input":
					w.Input = nil
				}
				c, err := agent.Start(context.Background(), runtime.Command, filepath.Join(t.TempDir(), "log"), 15*time.Second)
				if err != nil {
					t.Fatal(err)
				}
				defer c.Close()
				d, err := c.Call(protocol.Request{Method: "describe"})
				if err != nil || d.Description == nil || !slices.Contains(d.Description.PhaseBarrierScenarios, "instantiate") || d.Description.Configuration["instantiate_release_policy"] == "" {
					t.Fatal("missing capability/policy", d, err)
				}
				if _, err := c.Call(protocol.Request{Method: "prepare", Prepare: &protocol.Preparation{Artifact: w.Artifact, ArtifactSHA256: w.SHA256, Workload: w, Profile: profile}}); err != nil {
					t.Fatal(err)
				}
				var events []protocol.PhaseEvent
				resp, err := c.CallPhased(protocol.Request{Method: "run", Run: &protocol.RunRequest{Scenario: "instantiate", Samples: 3, Operations: 9, Warmup: 4, PhaseBarriers: true}}, func(e protocol.PhaseEvent) error { events = append(events, e); return nil })
				if mode != "correct" && mode != "no-initializer" {
					want := 2
					if mode == "timing" {
						want = 0
					}
					if mode == "trap-start" {
						want = 1
					}
					if err == nil || len(events) != want {
						t.Fatal("invalid failure boundary", resp, events, err)
					}
					return
				}
				if err != nil || len(resp.Samples) != 3 || len(events) != 9 {
					t.Fatal(resp, events, err)
				}
				for i, e := range events {
					if e.SampleIndex != i/3 || e.Stage != protocol.PhaseStages("instantiate")[i%3] {
						t.Fatal(events)
					}
				}
				for i, s := range resp.Samples {
					if s.Index != i || s.Warmup || s.Operations != 1 || s.SampleType != "individual_operation" || !s.Verified || !slices.Equal(s.Result, w.Oracle.Expected) {
						t.Fatal(s)
					}
					for _, o := range s.Observations {
						if !strings.HasPrefix(o.Phase, "instantiate/") {
							t.Fatal(o)
						}
						if o.Scope == "adapter_process_go_heap" && (o.Phase != "instantiate/api_window" || o.Denominator != "instantiation_including_start_excluding_initialization_verification_release") {
							t.Fatal(o)
						}
					}
				}
			})
		}
	}
}
