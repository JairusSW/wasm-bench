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

// Drive the real adapter batch directly: controller sacrificial checks must not
// mask an AOT operation that skips verification of its own produced module.
func TestBuiltAdapterAOTOracles(t *testing.T) {
	ids := os.Getenv("WASMBENCH_AOT_TEST_RUNTIMES")
	if ids == "" {
		t.Skip("build adapters and set WASMBENCH_AOT_TEST_RUNTIMES")
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
		for _, scenario := range []string{"aot-produce", "aot-load"} {
			for _, profile := range []string{"timing", "memory"} {
				for _, oracle := range []string{"correct", "wrong-result", "wrong-memory"} {
					name := r.ID + "/" + scenario + "/" + profile + "/" + oracle
					t.Run(name, func(t *testing.T) {
						w := workloads[0]
						w.Oracle.Expected = append(protocol.Values(nil), w.Oracle.Expected...)
						if oracle == "wrong-result" {
							w.Oracle.Expected[0] ^= 1
						}
						if oracle == "wrong-memory" {
							// The identity fixture exports zero-initialized memory.
							// A valid scalar result cannot mask a failed memory oracle.
							w.Oracle.Memory = []protocol.MemoryCheck{{Offset: 0, Hex: "01"}}
						}
						c, err := agent.Start(context.Background(), r.Command, filepath.Join(t.TempDir(), "adapter.log"), 15*time.Second)
						if err != nil {
							t.Fatal(err)
						}
						defer c.Close()
						description, err := c.Call(protocol.Request{Method: "describe"})
						if err != nil || description.Description == nil || description.Description.Configuration["aot_policy"] == "" {
							t.Fatal("missing AOT timing and native-artifact trust policy", r.ID, err)
						}
						if _, err = c.Call(protocol.Request{Method: "prepare", Prepare: &protocol.Preparation{Artifact: w.Artifact, ArtifactSHA256: w.SHA256, Workload: w, Profile: profile}}); err != nil {
							t.Fatal(err)
						}
						response, err := c.Call(protocol.Request{Method: "run", Run: &protocol.RunRequest{Scenario: scenario, Samples: 2, Operations: 3}})
						if oracle != "correct" {
							if err == nil || !strings.Contains(err.Error(), "incorrect result") || len(response.Samples) != 0 {
								t.Fatal("AOT samples falsely verified without checking the produced/loaded artifact", err, response.Samples)
							}
							return
						}
						if err != nil || len(response.Samples) != 2 {
							t.Fatal("correct AOT batch failed", err, response.Samples)
						}
						for i, s := range response.Samples {
							if !s.Verified || s.Index != i || s.Warmup || s.Operations != 3 || s.SampleType != "batch_average" {
								t.Fatal("invalid AOT sample shape", s)
							}
						}
					})
				}
			}
		}
	}
}
