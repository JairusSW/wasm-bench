package experiment_test

import (
	"context"
	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestBuiltAdapterTrajectory(t *testing.T) {
	ids := os.Getenv("WASMBENCH_TRAJECTORY_TEST_RUNTIMES")
	if ids == "" {
		t.Skip("build adapters and set WASMBENCH_TRAJECTORY_TEST_RUNTIMES")
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	runtimes, err := experiment.ResolveRuntimes(root, strings.Split(ids, ","))
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"exact_u64", "float_bits_v1"} {
		fixture := "trajectory-limit.wasm"
		want := uint64(7)
		if kind == "float_bits_v1" {
			fixture = "float-trajectory-limit.wasm"
			want = math.Float64bits(7)
		}
		artifact := filepath.Join(root, "corpus/testdata", fixture)
		digest, err := experiment.DigestFile(artifact)
		if err != nil {
			t.Fatal(err)
		}
		for _, rt := range runtimes {
			t.Run(rt.ID+"/"+kind, func(t *testing.T) {
				c, err := agent.Start(context.Background(), rt.Command, filepath.Join(t.TempDir(), "log"), 15*time.Second)
				if err != nil {
					t.Fatal(err)
				}
				defer c.Close()
				d, err := c.Call(protocol.Request{Method: "describe"})
				if err != nil || d.Description == nil || !slices.Contains(d.Description.Scenarios, "trajectory") {
					t.Fatal(d, err)
				}
				w := protocol.Workload{ABI: "core", Reset: "stateless", Export: "run", Args: protocol.Values{}, Oracle: protocol.Oracle{Kind: kind, Expected: protocol.Values{want}}}
				if kind == "float_bits_v1" {
					w.Oracle.Float = &protocol.FloatPolicy{Types: []string{"f64"}, NaN: "reject", SignedZero: "match"}
					if !d.Description.Capabilities["can_float_trajectory"] {
						t.Fatal("missing float trajectory capability")
					}
				}
				prepare := func(profile string) {
					_, err := c.Call(protocol.Request{Method: "prepare", Prepare: &protocol.Preparation{Workload: w, Artifact: artifact, ArtifactSHA256: digest, Profile: profile}})
					if err != nil {
						t.Fatal(err)
					}
				}
				prepare("timing")
				if _, err := c.Call(protocol.Request{Method: "run", Run: &protocol.RunRequest{Scenario: "trajectory", Samples: 1, Warmup: -1, Operations: 1}}); err == nil {
					t.Fatal("negative warmup accepted")
				}
				for i := 0; i < 2; i++ { // Each request starts over; each sample does not.
					r, err := c.Call(protocol.Request{Method: "run", Run: &protocol.RunRequest{Scenario: "trajectory", Samples: 3, Warmup: 2, Operations: 99}})
					if err != nil || len(r.Samples) != 5 {
						t.Fatal(r, err)
					}
					for n, s := range r.Samples {
						if s.Index != n || s.Warmup != (n < 2) || s.Operations != 1 || s.SampleType != "individual_operation" || !s.Verified || len(s.Result) != 1 || s.Result[0] != want {
							t.Fatal(s)
						}
					}
				}
				r, err := c.Call(protocol.Request{Method: "run", Run: &protocol.RunRequest{Scenario: "trajectory", Samples: 6, Operations: 1}})
				if err == nil || len(r.Samples) != 0 {
					t.Fatal("instance was reset or sixth invocation not run", r, err)
				}
				prepare("memory")
				if _, err = c.Call(protocol.Request{Method: "run", Run: &protocol.RunRequest{Scenario: "trajectory", Samples: 1, Operations: 1}}); err == nil {
					t.Fatal("memory profile accepted")
				}
				w.Reset = "fresh_instance_per_sample"
				prepare("timing")
				if _, err = c.Call(protocol.Request{Method: "run", Run: &protocol.RunRequest{Scenario: "trajectory", Samples: 1, Operations: 1}}); err == nil {
					t.Fatal("fresh instance trajectory accepted")
				}
			})
		}
	}
}
