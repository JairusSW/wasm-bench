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

func TestBuiltAdapterAppInit(t *testing.T) {
	ids := os.Getenv("WASMBENCH_APP_INIT_TEST_RUNTIMES")
	if ids == "" {
		t.Skip("set WASMBENCH_APP_INIT_TEST_RUNTIMES after building adapters")
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
		for _, mode := range []string{"correct", "stateless", "memory", "missing", "trap", "params", "results", "wrong-oracle", "wrong-memory", "no-input", "phase", "correct-phases", "wrong-phases", "trap-phases"} {
			t.Run(runtime.ID+"/"+mode, func(t *testing.T) {
				workloads, err := corpus.Generate(t.TempDir(), "lifecycle")
				if err != nil {
					t.Fatal(err)
				}
				w := workloads[0]
				profile := "timing"
				switch mode {
				case "stateless":
					w.Reset = "stateless"
				case "memory", "correct-phases", "wrong-phases", "trap-phases":
					profile = "memory"
				case "missing":
					w.Initialize = "absent"
				case "trap":
					w.Initialize = "trap_init"
				case "params":
					w.Initialize = "param_init"
				case "results":
					w.Initialize = "result_init"
				case "wrong-oracle":
					w.Oracle.Expected[0] = 8
				case "wrong-memory":
					w.Oracle.Memory[0].Hex = "00000000"
				case "no-input":
					w.Input = nil
				}
				if mode == "wrong-phases" {
					w.Oracle.Expected[0] = 8
				}
				if mode == "trap-phases" {
					w.Initialize = "trap_init"
				}
				c, err := agent.Start(context.Background(), runtime.Command, filepath.Join(t.TempDir(), "log"), 15*time.Second)
				if err != nil {
					t.Fatal(err)
				}
				defer c.Close()
				if _, err = c.Call(protocol.Request{Method: "prepare", Prepare: &protocol.Preparation{Artifact: w.Artifact, ArtifactSHA256: w.SHA256, Workload: w, Profile: profile}}); err != nil {
					t.Fatal(err)
				}
				var events []protocol.PhaseEvent
				response, err := c.CallPhased(protocol.Request{Method: "run", Run: &protocol.RunRequest{Scenario: "app-init", Samples: 3, Operations: 9, Warmup: 4, PhaseBarriers: mode == "phase" || strings.HasSuffix(mode, "phases")}}, func(e protocol.PhaseEvent) error { events = append(events, e); return nil })
				if mode != "correct" && mode != "stateless" && mode != "memory" && mode != "correct-phases" {
					if err == nil {
						t.Fatal("accepted invalid initialization", response)
					}
					want := 0
					if mode == "wrong-phases" {
						want = 2
					}
					if mode == "trap-phases" {
						want = 1
					}
					if len(events) != want {
						t.Fatal("incorrect initialization failure boundary", events)
					}
					for i, e := range events {
						if e.SampleIndex != 0 || e.Stage != protocol.PhaseStages("app-init")[i] {
							t.Fatal(events)
						}
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if len(response.Samples) != 3 {
					t.Fatal(response)
				}
				if mode == "correct-phases" {
					if len(events) != 9 {
						t.Fatal(events)
					}
					for i, e := range events {
						if e.SampleIndex != i/3 || e.Stage != protocol.PhaseStages("app-init")[i%3] {
							t.Fatal(events)
						}
					}
				}
				for i, s := range response.Samples {
					if s.Index != i || s.Warmup || s.Operations != 1 || !s.Verified || len(s.Result) != 1 || s.Result[0] != 7 {
						t.Fatal(s)
					}
					if (len(s.Observations) > 0) != (profile == "memory") {
						t.Fatal("incorrect instrumentation", s)
					}
					for _, o := range s.Observations {
						if !strings.HasPrefix(o.Phase, "app-init/") {
							t.Fatal("incorrect observation phase", o)
						}
						if o.Scope == "adapter_process_go_heap" && (o.Phase != "app-init/api_window" || o.Denominator != "initialization_call_excluding_input_verification_release") {
							t.Fatal(o)
						}
					}
				}
			})
		}
	}
}
