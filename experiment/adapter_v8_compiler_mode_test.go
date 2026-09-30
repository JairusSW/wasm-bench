package experiment_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

func TestV8ControlledCompilerModes(t *testing.T) {
	if os.Getenv("WASMBENCH_V8_COMPILER_MODE_TEST") != "1" {
		t.Skip("set WASMBENCH_V8_COMPILER_MODE_TEST=1 with supported Node/V8")
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	runtimes, err := experiment.ResolveRuntimes(root, []string{"v8", "v8-liftoff-only", "v8-optimizing-only"})
	if err != nil {
		t.Fatal(err)
	}
	for _, rt := range runtimes {
		t.Run(rt.ID, func(t *testing.T) {
			helper := filepath.Join(root, "adapters/v8/compiler-mode.mjs")
			if rt.Files[helper] == "" {
				t.Fatal("compiler-mode helper is not pinned")
			}
			for _, scenario := range []string{"first-call", "trajectory"} {
				c, err := agent.Start(context.Background(), rt.Command, filepath.Join(t.TempDir(), "log"), 15*time.Second)
				if err != nil {
					t.Fatal(err)
				}
				defer c.Close()
				d, err := c.Call(protocol.Request{Method: "describe"})
				if err != nil || d.Description == nil {
					t.Fatal(d, err)
				}
				configuration := d.Description.Configuration
				if d.Description.Capabilities["can_control_compiler_mode"] != (rt.ID != "v8") {
					t.Fatal("incorrect controlled compiler capability", d.Description)
				}
				if d.Description.Capabilities["can_observe_tiers"] {
					t.Fatal("calibration is not workload tier observation")
				}
				if rt.ID == "v8" {
					if d.Description.Backend != "production-default-tiering" || configuration["compiler_mode_probe"] != "" {
						t.Fatal("changed default mode", d.Description)
					}
				} else {
					mode := "liftoff-only"
					if rt.ID == "v8-optimizing-only" {
						mode = "optimizing-only"
					}
					var probe struct {
						Version    string `json:"version"`
						SHA256     string `json:"module_sha256"`
						Liftoff    bool   `json:"liftoff"`
						Optimizing bool   `json:"optimizing"`
					}
					if err := json.Unmarshal([]byte(configuration["compiler_mode_probe"]), &probe); err != nil {
						t.Fatal(err)
					}
					if d.Description.Backend != mode || configuration["tiering"] != mode || configuration["lazy_compilation"] != "disabled" || probe.Version != "v8-compiler-mode-probe-v1" || len(probe.SHA256) != 64 || probe.Liftoff != (mode == "liftoff-only") || probe.Optimizing != (mode == "optimizing-only") {
						t.Fatal("unverified mode", d.Description)
					}
				}
				// This guest stops returning the expected value after exactly five
				// calls. A hidden workload call during probe/setup would fail.
				artifact := filepath.Join(root, "corpus/testdata/trajectory-limit.wasm")
				digest, err := experiment.DigestFile(artifact)
				if err != nil {
					t.Fatal(err)
				}
				w := protocol.Workload{ABI: "core", Reset: "stateless", Export: "run", Args: protocol.Values{}, Oracle: protocol.Oracle{Kind: "exact_u64", Expected: protocol.Values{7}}}
				_, err = c.Call(protocol.Request{Method: "prepare", Prepare: &protocol.Preparation{Artifact: artifact, ArtifactSHA256: digest, Workload: w, Profile: "timing"}})
				if err != nil {
					t.Fatal(err)
				}
				count := 5
				if scenario == "first-call" {
					count = 1
				}
				resp, err := c.Call(protocol.Request{Method: "run", Run: &protocol.RunRequest{Scenario: scenario, Samples: count, Operations: 1, Warmup: 0}})
				if err != nil || len(resp.Samples) != count {
					t.Fatal(resp, err)
				}
				for _, sample := range resp.Samples {
					if !sample.Verified || len(sample.Observations) != 0 {
						t.Fatal("instrumented or incorrect timing", sample)
					}
				}
			}
		})
	}
}
