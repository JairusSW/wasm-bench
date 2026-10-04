package main

import (
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
	"testing"
)

func TestCompileDoesNotRetainUntimedModule(t *testing.T) {
	workloads, err := corpus.Generate(t.TempDir(), "core")
	if err != nil {
		t.Fatal(err)
	}
	a := &adapter{}
	defer a.close()
	w := workloads[0]
	if err = a.prepare(&protocol.Preparation{Artifact: w.Artifact, ArtifactSHA256: w.SHA256, Workload: w, Profile: "timing"}); err != nil {
		t.Fatal(err)
	}
	// A previous execution in this process must not seed the compile timer's cache.
	if err = a.setup(); err != nil {
		t.Fatal(err)
	}
	samples, err := a.run(&protocol.RunRequest{Scenario: "compile", Samples: 3, Operations: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 3 {
		t.Fatalf("samples=%d", len(samples))
	}
	if a.compiled != nil || a.instance != nil || a.fn != nil {
		t.Fatal("compile retains an untimed module or instance, allowing cached compilation")
	}
}
