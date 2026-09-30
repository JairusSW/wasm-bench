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

// Construction success alone does not prove that the measured engine is usable.
// Drive batches directly so controller preflight cannot mask missing validation.
func TestBuiltAdapterEngineInitOracles(t *testing.T) {
	ids := os.Getenv("WASMBENCH_ENGINE_INIT_TEST_RUNTIMES")
	if ids == "" {
		t.Skip("build adapters and set WASMBENCH_ENGINE_INIT_TEST_RUNTIMES")
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
	for _, r := range runtimes {
		for _, profile := range []string{"timing", "memory"} {
			for _, oracle := range []string{"correct", "wrong-result", "wrong-memory"} {
				t.Run(r.ID+"/"+profile+"/"+oracle, func(t *testing.T) {
					w := workloads[0]
					w.Oracle.Expected = append(protocol.Values(nil), w.Oracle.Expected...)
					if oracle == "wrong-result" {
						w.Oracle.Expected[0] ^= 1
					}
					if oracle == "wrong-memory" {
						w.Oracle.Memory = []protocol.MemoryCheck{{Offset: 0, Hex: "01"}}
					}
					c, err := agent.Start(context.Background(), r.Command, filepath.Join(t.TempDir(), "adapter.log"), 15*time.Second)
					if err != nil {
						t.Fatal(err)
					}
					defer c.Close()
					description, err := c.Call(protocol.Request{Method: "describe"})
					if err != nil || description.Description == nil || description.Description.Configuration["engine_init_policy"] == "" {
						t.Fatal("missing engine initialization boundary policy", err)
					}
					if _, err = c.Call(protocol.Request{Method: "prepare", Prepare: &protocol.Preparation{Artifact: w.Artifact, ArtifactSHA256: w.SHA256, Workload: w, Profile: profile}}); err != nil {
						t.Fatal(err)
					}
					response, err := c.Call(protocol.Request{Method: "run", Run: &protocol.RunRequest{Scenario: "engine-init", Samples: 2, Operations: 3}})
					if oracle != "correct" {
						if err == nil || !strings.Contains(err.Error(), "incorrect result") || len(response.Samples) != 0 {
							t.Fatal("engine construction falsely verified without its behavioral oracle", err, response.Samples)
						}
						return
					}
					if err != nil || len(response.Samples) != 2 {
						t.Fatal("correct engine initialization failed", err, response.Samples)
					}
					for i, s := range response.Samples {
						if !s.Verified || s.Index != i || s.Warmup || s.Operations != 3 || s.SampleType != "batch_average" {
							t.Fatal("invalid engine initialization sample", s)
						}
						for _, o := range s.Observations {
							if o.Metric == "guest.memory.logical" {
								t.Fatal("engine initialization attributed an unrelated prepared instance's memory", o)
							}
							if strings.HasPrefix(o.Metric, "host.alloc.") && o.Denominator != "batch_including_untimed_usability_verification_and_release" {
								t.Fatal("allocator window hides untimed usability verification", o)
							}
						}
					}
				})
			}
		}
	}
}
