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

func TestBuiltAdapterHarnessCalibration(t *testing.T) {
	ids := os.Getenv("WASMBENCH_HARNESS_TEST_RUNTIMES")
	if ids == "" {
		t.Skip("build adapters and set WASMBENCH_HARNESS_TEST_RUNTIMES")
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	runtimes, err := experiment.ResolveRuntimes(root, strings.Split(ids, ","))
	if err != nil {
		t.Fatal(err)
	}
	workloads, err := corpus.Generate(t.TempDir(), "core")
	if err != nil {
		t.Fatal(err)
	}
	for _, runtime := range runtimes {
		if runtime.ID == "v8" {
			if _, ok := runtime.Files[filepath.Join(root, "adapters/v8/harness.mjs")]; !ok {
				t.Fatal("calibration helper must be pinned and archived")
			}
		}
		for _, mode := range []string{"individual", "batch", "wrong-oracle", "memory", "warmup", "barriers", "empty", "oversized", "zero-samples"} {
			t.Run(runtime.ID+"/"+mode, func(t *testing.T) {
				w := workloads[0]
				w.Oracle.Expected = append(protocol.Values(nil), w.Oracle.Expected...)
				profile := "timing"
				r := protocol.RunRequest{Scenario: protocol.HarnessCalibrationScenario, Samples: 2, Operations: 257}
				switch mode {
				case "individual":
					r.Operations = 1
				case "wrong-oracle":
					w.Oracle.Expected[0] ^= 1
				case "memory":
					profile = "memory"
				case "warmup":
					r.Warmup = 1
				case "barriers":
					r.PhaseBarriers = true
				case "empty":
					r.Operations = 0
				case "oversized":
					r.Operations = 1000001
				case "zero-samples":
					r.Samples = 0
				}
				c, err := agent.Start(context.Background(), runtime.Command, filepath.Join(t.TempDir(), "adapter.log"), 15*time.Second)
				if err != nil {
					t.Fatal(err)
				}
				defer c.Close()
				description, err := c.Call(protocol.Request{Method: "describe"})
				if err != nil || description.Description == nil || description.Description.Configuration["harness_calibration_policy"] != protocol.HarnessCalibrationPolicy {
					t.Fatal("missing exact calibration policy", err)
				}
				if _, err = c.Call(protocol.Request{Method: "prepare", Prepare: &protocol.Preparation{Artifact: w.Artifact, ArtifactSHA256: w.SHA256, Workload: w, Profile: profile}}); err != nil {
					t.Fatal(err)
				}
				response, err := c.Call(protocol.Request{Method: "run", Run: &r})
				if mode != "individual" && mode != "batch" {
					if err == nil || len(response.Samples) != 0 {
						t.Fatal("invalid calibration returned measurements", mode, err)
					}
					if mode == "wrong-oracle" && !strings.Contains(err.Error(), "incorrect result") {
						t.Fatal("unrelated failure masked missing workload verification", err)
					}
					return
				}
				if err != nil || len(response.Samples) != 2 {
					t.Fatal("calibration batch failed", err)
				}
				for i, s := range response.Samples {
					kind := "batch_average"
					if r.Operations == 1 {
						kind = "individual_operation"
					}
					if s.Index != i || s.Warmup || !s.Verified || s.SampleType != kind || s.Operations != r.Operations || len(s.Result) != 1 || s.Result[0] != uint64(r.Operations) || len(s.Observations) != 0 {
						t.Fatal("invalid calibration bookkeeping/sample shape", s)
					}
				}
			})
		}
	}
}
