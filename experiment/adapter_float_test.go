package experiment_test

import (
	"context"
	"math"
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

func TestBuiltAdapterFloat(t *testing.T) {
	ids := os.Getenv("WASMBENCH_FLOAT_TEST_RUNTIMES")
	if ids == "" {
		t.Skip("build adapters and set WASMBENCH_FLOAT_TEST_RUNTIMES")
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	runtimes, err := experiment.ResolveRuntimes(root, strings.Split(ids, ","))
	if err != nil {
		t.Fatal(err)
	}
	workloads, err := corpus.Generate(t.TempDir(), "floats")
	if err != nil {
		t.Fatal(err)
	}
	for _, rt := range runtimes {
		t.Run(rt.ID, func(t *testing.T) {
			c, err := agent.Start(context.Background(), rt.Command, filepath.Join(t.TempDir(), "log"), 15*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			d, err := c.Call(protocol.Request{Method: "describe"})
			if err != nil || d.Description == nil || !d.Description.Capabilities["can_verify_float_bits_v1"] {
				t.Fatal(d, err)
			}
			prepare := func(w protocol.Workload, profile string) {
				t.Helper()
				_, err := c.Call(protocol.Request{Method: "prepare", Prepare: &protocol.Preparation{Artifact: w.Artifact, ArtifactSHA256: w.SHA256, Workload: w, Profile: profile}})
				if err != nil {
					t.Fatal(err)
				}
			}
			for _, w := range workloads {
				for _, scenario := range []string{"compile", "instantiate", "teardown"} {
					for _, mode := range []string{"correct", "wrong-value", "wrong-type", "timing"} {
						wc := w
						profile := "memory"
						if mode == "wrong-value" {
							wc.Oracle.Expected = make(protocol.Values, len(w.Oracle.Expected))
							for i := range wc.Oracle.Expected {
								wc.Oracle.Expected[i] = 0
							}
						}
						// -0 would match a zero result under an ignore policy; this suite uses match.
						if mode == "wrong-type" {
							wc.Export = "wrong_type"
						}
						if mode == "timing" {
							profile = "timing"
						}
						_, err := c.Call(protocol.Request{Method: "prepare", Prepare: &protocol.Preparation{Artifact: wc.Artifact, ArtifactSHA256: wc.SHA256, Workload: wc, Profile: profile}})
						if err != nil {
							if mode == "wrong-type" {
								continue
							}
							t.Fatal(err)
						}
						var events []protocol.PhaseEvent
						resp, err := c.CallPhased(protocol.Request{Method: "run", Run: &protocol.RunRequest{Scenario: scenario, Samples: 2, Operations: 9, PhaseBarriers: true}}, func(e protocol.PhaseEvent) error { events = append(events, e); return nil })
						if mode != "correct" {
							if err == nil {
								t.Fatalf("%s %s accepted %s", w.Export, scenario, mode)
							}
							if (mode == "timing" || scenario == "teardown") && len(events) != 0 {
								t.Fatal("invalid request reached a measurement barrier")
							}
							continue
						}
						if err != nil {
							t.Fatalf("%s %s: %v", w.Export, scenario, err)
						}
						stages := protocol.PhaseStages(scenario)
						if scenario == "teardown" && !d.Description.Capabilities["can_float_teardown"] {
							t.Fatal("missing float teardown capability")
						}
						if !d.Description.Capabilities["can_float_phases"] || len(resp.Samples) != 2 || len(events) != 2*len(stages) {
							t.Fatal("missing float phase evidence", resp, events)
						}
						for i, e := range events {
							if e.SampleIndex != i/len(stages) || e.Stage != stages[i%len(stages)] {
								t.Fatal(events)
							}
						}
						for i, s := range resp.Samples {
							if s.Index != i || s.Operations != 1 || !s.Verified || s.Warmup {
								t.Fatal(s)
							}
						}
					}
				}
				for _, profile := range []string{"timing", "memory"} {
					for _, scenario := range []string{"compile", "instantiate", "first-call", "steady", "trajectory", "teardown"} {
						if scenario == "trajectory" && profile != "timing" {
							continue
						}
						prepare(w, profile)
						r, err := c.Call(protocol.Request{Method: "run", Run: &protocol.RunRequest{Scenario: scenario, Samples: 2, Warmup: 1, Operations: 2}})
						if err != nil {
							t.Fatalf("%s %s: %v", w.Export, scenario, err)
						}
						if len(r.Samples) < 2 {
							t.Fatal("missing samples")
						}
						if scenario == "trajectory" && len(r.Samples) != 3 {
							t.Fatal("trajectory lost warmup or measured samples", r.Samples)
						}
						for i, s := range r.Samples {
							if scenario == "teardown" && (len(r.Samples) != 2 || s.Warmup || s.Operations != 1 || s.SampleType != "individual_operation") {
								t.Fatal("invalid teardown sample", s)
							}
							if scenario == "trajectory" && (s.Index != i || s.Warmup != (i == 0) || s.Operations != 1 || s.SampleType != "individual_operation") {
								t.Fatal("invalid trajectory sample", s)
							}
							if !s.Verified {
								t.Fatal(s)
							}
							if scenario == "first-call" || scenario == "steady" || scenario == "trajectory" || scenario == "teardown" {
								if err := w.Oracle.VerifyFloat(s.Result, w.Oracle.Float.Types); err != nil {
									t.Fatal(err)
								}
							}
						}
					}
				}
			}
			for _, mode := range []string{"wrong_value", "wrong_type", "zero_sign", "finite_for_nan"} {
				w := workloads[0]
				w.Oracle.Expected = append(protocol.Values(nil), w.Oracle.Expected...)
				switch mode {
				case "wrong_value":
					w.Oracle.Expected[0] = math.Float64bits(10)
				case "wrong_type":
					w.Export = "wrong_type"
					w.Oracle.Expected[0] = 0
				case "zero_sign":
					w = workloads[2]
					w.Oracle.Expected = protocol.Values{0}
				case "finite_for_nan":
					w = workloads[3]
					w.Export = "sum_f64"
				}
				// Signature mismatches may fail at prepare, before any invocation.
				if _, err := c.Call(protocol.Request{Method: "prepare", Prepare: &protocol.Preparation{Artifact: w.Artifact, ArtifactSHA256: w.SHA256, Workload: w, Profile: "timing"}}); err != nil {
					continue
				}
				if _, err := c.Call(protocol.Request{Method: "run", Run: &protocol.RunRequest{Scenario: "first-call", Samples: 1, Operations: 1}}); err == nil {
					t.Fatal("accepted", mode)
				}
			}
		})
	}
}
