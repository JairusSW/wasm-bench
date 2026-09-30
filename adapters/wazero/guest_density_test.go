package main

import (
	"fmt"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
	"testing"
)

func TestGuestDensityFreshAndRestore(t *testing.T) {
	ws, err := corpus.Generate(t.TempDir(), "guest-density")
	if err != nil {
		t.Fatal(err)
	}
	for _, interpreter := range []bool{false, true} {
		for _, w := range ws {
			for _, profile := range []string{"timing", "memory"} {
				t.Run(fmt.Sprintf("%t/%s/%s", interpreter, profile, w.ID), func(t *testing.T) {
					a := &adapter{interpreter: interpreter}
					defer a.close()
					if err := a.prepare(&protocol.Preparation{Artifact: w.Artifact, ArtifactSHA256: w.SHA256, Workload: w, Profile: profile}); err != nil {
						t.Fatal(err)
					}
					var events []protocol.PhaseEvent
					a.barrier = func(e protocol.PhaseEvent) error { events = append(events, e); return nil }
					r := &protocol.RunRequest{Scenario: "guest-density", Samples: 2, Operations: 1, PhaseBarriers: profile == "memory"}
					ss, err := a.run(r)
					if err != nil {
						t.Fatal(err)
					}
					if len(ss) != 2 {
						t.Fatal("missing independent groups")
					}
					for i, s := range ss {
						if err := protocol.VerifyGuestDensitySample(w, s); err != nil {
							t.Fatal(err)
						}
						if profile == "timing" && len(s.Observations) != 0 {
							t.Fatal("memory in timing")
						}
						if profile == "memory" {
							if len(s.Observations) != 8 {
								t.Fatal("missing allocation/logical memory")
							}
							for j, stage := range protocol.PhaseStages(r.Scenario) {
								if events[i*3+j] != (protocol.PhaseEvent{SampleIndex: i, Stage: stage}) {
									t.Fatal("phase order")
								}
							}
						}
					}
					if profile == "timing" && len(events) != 0 {
						t.Fatal("unexpected timing barriers")
					}
					a.wasm = append(a.wasm, 0, 1, 0)
					if _, err := a.run(r); err == nil {
						t.Fatal("accepted arbitrary guest state shape")
					}
				})
			}
		}
	}
}
