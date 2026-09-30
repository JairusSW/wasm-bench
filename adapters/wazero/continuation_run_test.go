package main

import (
	"fmt"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
	"reflect"
	"testing"
)

func TestContinuationAdapterStages(t *testing.T) {
	ws, err := corpus.Generate(t.TempDir(), "continuations")
	if err != nil {
		t.Fatal(err)
	}
	for _, profile := range []string{"timing", "memory"} {
		for _, phased := range []bool{false, true} {
			if phased && profile == "timing" {
				continue
			}
			for _, w := range ws {
				for _, stage := range protocol.ContinuationScenarios() {
					t.Run(fmt.Sprintf("%s/%t/%d/%s", profile, phased, w.Continuation.Depth, stage), func(t *testing.T) {
						a := &adapter{}
						defer a.close()
						if err := a.prepare(&protocol.Preparation{Artifact: w.Artifact, ArtifactSHA256: w.SHA256, Workload: w, Profile: profile}); err != nil {
							t.Fatal(err)
						}
						var events []protocol.PhaseEvent
						a.barrier = func(e protocol.PhaseEvent) error { events = append(events, e); return nil }
						samples, err := a.run(&protocol.RunRequest{Scenario: stage, Samples: 2, Operations: 1, PhaseBarriers: phased})
						if err != nil {
							t.Fatal(err)
						}
						if len(samples) != 2 {
							t.Fatal("sample count")
						}
						for i, s := range samples {
							if s.Index != i {
								t.Fatal("sample identity")
							}
							if err := protocol.VerifyContinuationSample(w, stage, s); err != nil {
								t.Fatal(err)
							}
							want := 0
							if profile == "memory" {
								want = 7
							}
							if len(s.Observations) != want {
								t.Fatal("instrumentation profile mismatch")
							}
							for _, o := range s.Observations {
								if o.Phase != stage+"/operation_window" || o.Denominator != protocol.ContinuationDenominator {
									t.Fatal("memory window scope")
								}
							}
						}
						if phased {
							if len(events) != 6 {
								t.Fatal("missing barriers")
							}
							for i, e := range events {
								if !reflect.DeepEqual(e, protocol.PhaseEvent{SampleIndex: i / 3, Stage: protocol.PhaseStages(stage)[i%3]}) {
									t.Fatal("barrier order")
								}
							}
						} else if len(events) != 0 {
							t.Fatal("unsolicited barriers")
						}
						if phased {
							a.barrier = func(protocol.PhaseEvent) error { return fmt.Errorf("transport failed") }
							if _, err := a.run(&protocol.RunRequest{Scenario: stage, Samples: 1, Operations: 1, PhaseBarriers: true}); err == nil {
								t.Fatal("ignored failed barrier")
							}
						}
						a.wasm = append(a.wasm, 0, 1, 0)
						if _, err := a.run(&protocol.RunRequest{Scenario: stage, Samples: 1, Operations: 1}); err == nil {
							t.Fatal("noncanonical artifact accepted")
						}
					})
				}
			}
		}
	}
	for _, w := range ws {
		a := &adapter{interpreter: true}
		if err := a.prepare(&protocol.Preparation{Artifact: w.Artifact, ArtifactSHA256: w.SHA256, Workload: w, Profile: "timing"}); err != nil {
			t.Fatal(err)
		}
		if _, err := a.run(&protocol.RunRequest{Scenario: "continuation-resume", Samples: 1, Operations: 1}); err == nil {
			t.Fatal("unqualified interpreter accepted")
		}
		a.close()
	}
}
