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

func TestBuiltAdapterReactors(t *testing.T) {
	ids := os.Getenv("WASMBENCH_REACTOR_TEST_RUNTIMES")
	if ids == "" {
		t.Skip("build adapters and set WASMBENCH_REACTOR_TEST_RUNTIMES")
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	runtimes, err := experiment.ResolveRuntimes(root, strings.Split(ids, ","))
	if err != nil {
		t.Fatal(err)
	}
	for _, runtime := range runtimes {
		for _, profile := range []string{"timing", "memory"} {
			for _, scenario := range []string{"compile", "instantiate", "app-init", "first-call", "steady", "teardown"} {
				t.Run(runtime.ID+"/"+profile+"/"+scenario, func(t *testing.T) {
					workloads, err := corpus.Generate(t.TempDir(), "reactors")
					if err != nil {
						t.Fatal(err)
					}
					for _, w := range workloads {
						c, err := agent.Start(context.Background(), runtime.Command, filepath.Join(t.TempDir(), "log"), 15*time.Second)
						if err != nil {
							t.Fatal(err)
						}
						defer c.Close()
						if _, err = c.Call(protocol.Request{Method: "prepare", Prepare: &protocol.Preparation{Artifact: w.Artifact, ArtifactSHA256: w.SHA256, Workload: w, Profile: profile}}); err != nil {
							t.Fatal(err)
						}
						response, err := c.Call(protocol.Request{Method: "run", Run: &protocol.RunRequest{Scenario: scenario, Samples: 3, Operations: 4, Warmup: 2}})
						if err != nil {
							t.Fatal(err)
						}
						count := 3
						if scenario == "steady" {
							count += 2
						}
						if len(response.Samples) != count {
							t.Fatal(response)
						}
						for i, s := range response.Samples {
							ops := 1
							if scenario == "steady" && w.Reset == "stateless" {
								ops = 4
							}
							if !s.Verified || s.Index != i || s.Operations != ops || s.Warmup != (scenario == "steady" && i < 2) || len(s.Result) != 1 || s.Result[0] != 42 {
								t.Fatal(s)
							}
						}
					}
				})
			}
		}
	}
}
