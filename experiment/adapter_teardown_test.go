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

func TestBuiltAdapterTeardown(t *testing.T) {
	ids := os.Getenv("WASMBENCH_TEARDOWN_TEST_RUNTIMES")
	if ids == "" {
		t.Skip("set WASMBENCH_TEARDOWN_TEST_RUNTIMES after building adapters")
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
		for _, mode := range []string{"correct", "memory", "wrong-result", "wrong-memory", "no-input", "trap-init", "phase", "correct-phases", "wrong-phases"} {
			t.Run(runtime.ID+"/"+mode, func(t *testing.T) {
				ws, err := corpus.Generate(t.TempDir(), "lifecycle")
				if err != nil {
					t.Fatal(err)
				}
				w, profile := ws[0], "timing"
				switch mode {
				case "memory", "correct-phases", "wrong-phases":
					profile = "memory"
				case "wrong-result":
					w.Oracle.Expected[0] = 8
				case "wrong-memory":
					w.Oracle.Memory[0].Hex = "00000000"
				case "no-input":
					w.Input = nil
				case "trap-init":
					w.Initialize = "trap_init"
				}
				if mode == "wrong-phases" {
					w.Oracle.Expected[0] = 8
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
				resp, err := c.CallPhased(protocol.Request{Method: "run", Run: &protocol.RunRequest{Scenario: "teardown", Samples: 3, Operations: 9, Warmup: 4, PhaseBarriers: mode == "phase" || strings.HasSuffix(mode, "phases")}}, func(e protocol.PhaseEvent) error { events = append(events, e); return nil })
				if mode != "correct" && mode != "memory" && mode != "correct-phases" {
					if err == nil {
						t.Fatal("accepted invalid teardown", resp)
					}
					if len(events) != 0 {
						t.Fatal("unverified workload reached release boundary", events)
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
					if s.Index != i || s.Warmup || s.Operations != 1 || s.SampleType != "individual_operation" || !s.Verified || len(s.Result) != 1 || s.Result[0] != 7 {
						t.Fatal(s)
					}
					if (len(s.Observations) > 0) != (profile == "memory") {
						t.Fatal("instrumentation mismatch", s)
					}
				}
			})
		}
	}
}
