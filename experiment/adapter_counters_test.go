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

func TestBuiltAdapterCounters(t *testing.T) {
	testBuiltCounterScenarios(t, "WASMBENCH_COUNTER_TEST_RUNTIMES", []string{"compile", "instantiate"})
}

func TestBuiltAdapterFirstCallCounters(t *testing.T) {
	testBuiltCounterScenarios(t, "WASMBENCH_FIRST_COUNTER_TEST_RUNTIMES", []string{"first-call"})
}

func testBuiltCounterScenarios(t *testing.T, variable string, scenarios []string) {
	t.Helper()
	ids := os.Getenv(variable)
	if ids == "" {
		t.Skip("set " + variable + " after building adapters")
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
		for _, scenario := range scenarios {
			modes := []string{"correct", "wrong-oracle", "no-barriers", "warmup", "negative-warmup", "batch", "zero-samples", "too-many-samples", "unsupported-scenario"}
			if scenario == "first-call" {
				modes = append(modes, "call-trap", "init-trap")
			}
			for _, mode := range modes {
				t.Run(rt.ID+"/"+scenario+"/"+mode, func(t *testing.T) {
					ws, err := corpus.Generate(t.TempDir(), "lifecycle")
					if err != nil {
						t.Fatal(err)
					}
					w := ws[0]
					if mode == "call-trap" {
						w.Export = "trap_init"
					}
					if mode == "init-trap" {
						w.Initialize = "trap_init"
					}
					if mode == "wrong-oracle" {
						w.Oracle.Expected = append(protocol.Values(nil), w.Oracle.Expected...)
						w.Oracle.Expected[0]++
					}
					c, err := agent.Start(context.Background(), rt.Command, filepath.Join(t.TempDir(), "log"), 15*time.Second)
					if err != nil {
						t.Fatal(err)
					}
					defer c.Close()
					d, err := c.Call(protocol.Request{Method: "describe"})
					if err != nil || d.Description == nil || !d.Description.Capabilities["can_counter_"+scenario] || d.Description.Configuration["counter_policy"] == "" {
						t.Fatal(d, err)
					}
					_, err = c.Call(protocol.Request{Method: "prepare", Prepare: &protocol.Preparation{Artifact: w.Artifact, ArtifactSHA256: w.SHA256, Workload: w, Profile: "counters"}})
					if err != nil {
						t.Fatal(err)
					}
					r := protocol.RunRequest{Scenario: scenario, Samples: 2, Operations: 1, PhaseBarriers: true}
					switch mode {
					case "no-barriers":
						r.PhaseBarriers = false
					case "warmup":
						r.Warmup = 1
					case "negative-warmup":
						r.Warmup = -1
					case "zero-samples":
						r.Samples = 0
					case "too-many-samples":
						r.Samples = 100001
					case "batch":
						r.Operations = 2
					case "unsupported-scenario":
						r.Scenario = "steady"
					}
					if mode == "correct" {
						resp, records, err := c.CallCounterPhases(protocol.Request{Method: "run", Run: &r})
						if err != nil || len(resp.Samples) != 2 || len(records) != 2 {
							t.Fatal(resp, records, err)
						}
						for i, s := range resp.Samples {
							if !s.Verified || s.Index != i || s.Operations != 1 || s.Warmup || s.SampleType != "individual_operation" || len(s.Observations) != 0 {
								t.Fatal("counter pass contains memory instrumentation or invalid samples", s)
							}
						}
						// This client is deliberately unisolated: the adapter is exercised,
						// but no positive perf availability is inferred from its success.
						for i, record := range records {
							if record.Sample != i || record.Status != "unavailable" || record.Reason == "" || len(record.Readings) != 0 {
								t.Fatal(record)
							}
						}
						return
					}
					var events []protocol.PhaseEvent
					resp, err := c.CallPhased(protocol.Request{Method: "run", Run: &r}, func(e protocol.PhaseEvent) error { events = append(events, e); return nil })
					want := 0
					if mode == "wrong-oracle" || mode == "call-trap" {
						want = 2
					}
					if err == nil || len(resp.Samples) != 0 || len(events) != want {
						t.Fatal("invalid request/oracle accepted or wrong boundary", resp, events, err)
					}
				})
			}
		}
	}
}
