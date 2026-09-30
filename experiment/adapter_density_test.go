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

func TestBuiltAdapterDensity(t *testing.T) {
	ids := os.Getenv("WASMBENCH_DENSITY_TEST_RUNTIMES")
	if ids == "" {
		t.Skip("set WASMBENCH_DENSITY_TEST_RUNTIMES after building adapters")
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	runtimes, err := experiment.ResolveRuntimes(root, strings.Split(ids, ","))
	if err != nil {
		t.Fatal(err)
	}
	ws, err := corpus.Generate(t.TempDir(), "density")
	if err != nil {
		t.Fatal(err)
	}
	for _, rt := range runtimes {
		for _, w := range ws {
			for _, scenario := range []string{"density", "density-cycle"} {
				for _, profile := range []string{"timing", "memory"} {
					t.Run(rt.ID+"/"+w.ID+"/"+profile+"/"+scenario, func(t *testing.T) {
						c, err := agent.Start(context.Background(), rt.Command, filepath.Join(t.TempDir(), "log"), 20*time.Second)
						if err != nil {
							t.Fatal(err)
						}
						defer c.Close()
						description, err := c.Call(protocol.Request{Method: "describe"})
						if err != nil || description.Description == nil {
							t.Fatalf("description unavailable: %v", err)
						}
						prep := &protocol.Preparation{Artifact: w.Artifact, ArtifactSHA256: w.SHA256, Workload: w, Profile: profile}
						if _, err := c.Call(protocol.Request{Method: "prepare", Prepare: prep}); err != nil {
							t.Fatal(err)
						}
						var events []protocol.PhaseEvent
						r := &protocol.RunRequest{Scenario: scenario, Samples: 2, Operations: 1, PhaseBarriers: profile == "memory"}
						response, err := c.CallPhased(protocol.Request{Method: "run", Run: r}, func(e protocol.PhaseEvent) error { events = append(events, e); return nil })
						if scenario == "density-cycle" && !description.Description.Capabilities["can_density_cycle"] {
							if err == nil || len(events) != 0 {
								t.Fatal("unsupported cycle executed")
							}
							return
						}
						if supported, declared := description.Description.Capabilities["can_density_"+w.Density.Sharing]; declared && !supported {
							if err == nil || len(events) != 0 {
								t.Fatal("unsupported sharing policy was executed")
							}
							return
						}
						if err != nil {
							t.Fatal(err)
						}
						if len(response.Samples) != 2 {
							t.Fatal("missing groups")
						}
						wantEvents := 0
						if profile == "memory" {
							wantEvents = 6
						}
						if len(events) != wantEvents {
							t.Fatal("wrong phase count")
						}
						for i, e := range events {
							if e.SampleIndex != i/3 || e.Stage != protocol.PhaseStages(scenario)[i%3] {
								t.Fatal("wrong phase ordering")
							}
						}
						for _, s := range response.Samples {
							if !s.Verified || s.Operations != 1 || s.Warmup || s.SampleType != "individual_operation" {
								t.Fatal("wrong sample semantics")
							}
							if profile == "memory" {
								found := false
								for _, o := range s.Observations {
									if o.Metric == "density.guest_memory.logical" {
										found = true
										if o.Value == nil || *o.Value != float64(w.Density.Instances*65536) {
											t.Fatal("wrong logical memory")
										}
									}
								}
								if !found {
									t.Fatal("missing logical memory")
								}
							}
						}
						// A group failure cannot emit a ready/released boundary or verified sample.
						prep.Workload.Oracle.Expected = protocol.Values{999999}
						if _, err := c.Call(protocol.Request{Method: "prepare", Prepare: prep}); err != nil {
							t.Fatal(err)
						}
						events = nil
						if _, err := c.CallPhased(protocol.Request{Method: "run", Run: r}, func(e protocol.PhaseEvent) error { events = append(events, e); return nil }); err == nil {
							t.Fatal("wrong oracle accepted")
						}
						wantEvents = 0
						if profile == "memory" {
							wantEvents = 1
						}
						if len(events) != wantEvents {
							t.Fatal("failed group reported ready")
						}
					})
				}
			}
		}
	}
}
