package experiment_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

// Opt-in adapter conformance runs against exact independently built binaries.
// CI selects the adapters built in each job; missing requested builds fail.
func TestBuiltAdapterCompilePhases(t *testing.T) {
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
	workloads, err := corpus.Generate(t.TempDir(), "scaling")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range runtimes {
		t.Run(r.ID, func(t *testing.T) {
			for _, w := range workloads {
				if w.Size != 16 || !slices.Contains([]string{"imports", "data-segments"}, w.Dimension) {
					continue
				}
				for _, mode := range []string{"correct", "wrong-oracle", "wrong-memory", "timing-rejected"} {
					t.Run(w.Dimension+"/"+mode, func(t *testing.T) {
						c, err := agent.Start(context.Background(), r.Command, filepath.Join(t.TempDir(), "log"), 10*time.Second)
						if err != nil {
							t.Fatal(err)
						}
						defer c.Close()
						description, err := c.Call(protocol.Request{Method: "describe"})
						if err != nil {
							t.Fatal(err)
						}
						if !slices.Contains(description.Description.PhaseBarrierScenarios, "compile") || description.Description.PhaseReleasePolicy == "" {
							t.Fatal("phase contract not advertised")
						}
						workload := w
						profile := "memory"
						if mode == "wrong-oracle" {
							workload.Oracle.Expected = protocol.Values{987654321}
						}
						if mode == "wrong-memory" {
							workload.Oracle.Memory = []protocol.MemoryCheck{{Offset: 0, Hex: "fe"}}
						}
						if mode == "timing-rejected" {
							profile = "timing"
						}
						if _, err = c.Call(protocol.Request{Method: "prepare", Prepare: &protocol.Preparation{Artifact: w.Artifact, ArtifactSHA256: w.SHA256, Workload: workload, Profile: profile}}); err != nil {
							t.Fatal(err)
						}
						var events []protocol.PhaseEvent
						response, err := c.CallPhased(protocol.Request{Method: "run", Run: &protocol.RunRequest{Scenario: "compile", Samples: 2, Operations: 9, PhaseBarriers: true}}, func(event protocol.PhaseEvent) error { events = append(events, event); return nil })
						if mode != "correct" {
							if err == nil {
								t.Fatal("invalid request succeeded")
							}
							if (mode == "wrong-oracle" || mode == "wrong-memory") && !strings.Contains(err.Error(), "incorrect result") {
								t.Fatal(err)
							}
							if mode == "timing-rejected" && len(events) != 0 {
								t.Fatal("timing emitted barriers")
							}
							return
						}
						if err != nil {
							t.Fatal(err)
						}
						if len(events) != 6 || len(response.Samples) != 2 {
							t.Fatal(events, response)
						}
						for i, e := range events {
							if e.SampleIndex != i/3 || e.Stage != []string{"before_compile", "compiled", "released"}[i%3] {
								t.Fatal(events)
							}
						}
						for i, s := range response.Samples {
							if s.Index != i || s.Operations != 1 || !s.Verified || s.Warmup {
								t.Fatal(s)
							}
						}
					})
				}
			}
		})
	}
}
