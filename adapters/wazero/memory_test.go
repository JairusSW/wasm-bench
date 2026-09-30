package main

import (
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
	"testing"
)

func TestMemoryProfileWithoutLinearMemory(t *testing.T) {
	for _, interpreter := range []bool{false, true} {
		workloads, err := corpus.Generate(t.TempDir(), "floats")
		if err != nil {
			t.Fatal(err)
		}
		w := workloads[0]
		a := adapter{interpreter: interpreter}
		defer a.close()
		if err = a.prepare(&protocol.Preparation{Artifact: w.Artifact, ArtifactSHA256: w.SHA256, Workload: w, Profile: "memory"}); err != nil {
			t.Fatal(err)
		}
		samples, err := a.run(&protocol.RunRequest{Scenario: "first-call", Samples: 2, Operations: 1})
		if err != nil {
			t.Fatal(err)
		}
		if hasMemory(a.instance) {
			t.Fatal("module unexpectedly has linear memory")
		}
		if len(samples) != 2 || !samples[0].Verified {
			t.Fatal(samples)
		}
		for _, s := range samples {
			for _, o := range s.Observations {
				if o.Metric == "guest.memory.logical" && o.Status == "available" {
					t.Fatal("invented linear memory", o)
				}
			}
		}
	}
}
