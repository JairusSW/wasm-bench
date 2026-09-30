package experiment_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

func TestBuiltAdapterSteadyCounters(t *testing.T) {
	ids := os.Getenv("WASMBENCH_STEADY_COUNTER_TEST_RUNTIMES")
	if ids == "" {
		t.Skip("set WASMBENCH_STEADY_COUNTER_TEST_RUNTIMES after building adapters")
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	rts, err := experiment.ResolveRuntimes(root, strings.Split(ids, ","))
	if err != nil {
		t.Fatal(err)
	}
	for _, rt := range rts {
		for _, mode := range []string{"batch", "warmup", "sixth-call-trap", "middle-wrong", "warmup-wrong", "timing-middle-wrong"} {
			t.Run(rt.ID+"/"+mode, func(t *testing.T) {
				fixture := "trajectory-limit.wasm"
				r := protocol.RunRequest{Scenario: "steady", Samples: 1, Operations: 5, PhaseBarriers: true}
				wantErr := false
				switch mode {
				case "warmup":
					r.Samples, r.Operations, r.Warmup = 3, 1, 2
				case "sixth-call-trap":
					r.Samples, r.Operations, r.Warmup, wantErr = 2, 2, 1, true
				case "middle-wrong":
					fixture, wantErr = "counter-batch-wrong.wasm", true
				case "timing-middle-wrong":
					fixture, wantErr, r.PhaseBarriers = "counter-batch-wrong.wasm", true, false
				case "warmup-wrong":
					fixture, r.Warmup, wantErr = "counter-batch-wrong.wasm", 1, true
				}
				path := filepath.Join(root, "corpus/testdata", fixture)
				digest, err := experiment.DigestFile(path)
				if err != nil {
					t.Fatal(err)
				}
				c, err := agent.Start(context.Background(), rt.Command, filepath.Join(t.TempDir(), "log"), 15*time.Second)
				if err != nil {
					t.Fatal(err)
				}
				defer c.Close()
				d, err := c.Call(protocol.Request{Method: "describe"})
				if err != nil || d.Description == nil || !d.Description.Capabilities["can_counter_steady"] || d.Description.Configuration["counter_steady_policy"] == "" {
					t.Fatal(d, err)
				}
				w := protocol.Workload{ABI: "core", Reset: "stateless", Export: "run", Args: protocol.Values{}, Oracle: protocol.Oracle{Kind: "exact_u64", Expected: protocol.Values{7}}}
				profile := "counters"
				if mode == "timing-middle-wrong" {
					profile = "timing"
				}
				_, err = c.Call(protocol.Request{Method: "prepare", Prepare: &protocol.Preparation{Profile: profile, Artifact: path, ArtifactSHA256: digest, Workload: w}})
				if err != nil {
					t.Fatal(err)
				}
				if mode == "timing-middle-wrong" {
					if resp, err := c.Call(protocol.Request{Method: "run", Run: &r}); err == nil {
						t.Fatal("incorrect middle result accepted", resp)
					}
					return
				}
				for repeat := 0; repeat < 2; repeat++ {
					resp, records, err := c.CallCounterPhases(protocol.Request{Method: "run", Run: &r})
					if wantErr {
						if err == nil || len(records) == 0 {
							t.Fatal(resp, records, err)
						}
						if mode == "warmup-wrong" && len(records) != 1 {
							t.Fatal("continued after bad warmup", records)
						}
						break
					}
					if err != nil || len(resp.Samples) != r.Samples+r.Warmup || len(records) != len(resp.Samples) {
						t.Fatal(resp, records, err)
					}
					for i, s := range resp.Samples {
						if s.Index != i || s.Warmup != (i < r.Warmup) || s.Operations != r.Operations || !s.Verified || len(s.Observations) != 0 || len(s.Result) != 1 || s.Result[0] != 7 {
							t.Fatal(s)
						}
						if records[i].Status != "unavailable" || records[i].Sample != i {
							t.Fatal(records[i])
						}
					}
				}
			})
		}
	}
}
