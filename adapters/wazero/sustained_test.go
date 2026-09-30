package main

import (
	"fmt"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
	"testing"
)

func TestSustainedRetainedSessions(t *testing.T) {
	ws, err := corpus.Generate(t.TempDir(), "core")
	if err != nil {
		t.Fatal(err)
	}
	for _, interpreter := range []bool{false, true} {
		for _, profile := range []string{"timing", "memory"} {
			t.Run(fmt.Sprintf("%t/%s", interpreter, profile), func(t *testing.T) {
				w := ws[0]
				a := &adapter{interpreter: interpreter}
				defer a.close()
				if err := a.prepare(&protocol.Preparation{Artifact: w.Artifact, ArtifactSHA256: w.SHA256, Workload: w, Profile: profile}); err != nil {
					t.Fatal(err)
				}
				r := &protocol.RunRequest{Scenario: "sustained", Samples: 20, Operations: 100, Warmup: 2, SustainedDurationNS: 1000000}
				samples, err := a.run(r)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := protocol.VerifySustainedSequence(w, *r, samples); err != nil {
					t.Fatal(err)
				}
				if a.engine != nil || a.instance != nil || a.fn != nil {
					t.Fatal("resources not logically released")
				}
				for _, s := range samples {
					if profile == "timing" && len(s.Observations) != 0 {
						t.Fatal("timing instrumentation")
					}
					if profile == "memory" && len(s.Observations) < 8 {
						t.Fatal("missing memory observations")
					}
				}
				if profile == "memory" {
					if len(samples[len(samples)-1].Observations) != 15 {
						t.Fatal("missing release window")
					}
				}
			})
		}
	}
}

func TestSustainedPostCollection(t *testing.T) {
	ws, err := corpus.Generate(t.TempDir(), "sustained")
	if err != nil {
		t.Fatal(err)
	}
	for _, interpreter := range []bool{false, true} {
		a := &adapter{interpreter: interpreter}
		w := ws[0]
		if err := a.prepare(&protocol.Preparation{Artifact: w.Artifact, ArtifactSHA256: w.SHA256, Workload: w, Profile: "memory"}); err != nil {
			t.Fatal(err)
		}
		r := protocol.RunRequest{Scenario: "sustained", Samples: 2, Operations: 100, SustainedDurationNS: 1000000, SustainedPostCollection: true}
		samples, err := a.run(&r)
		if err != nil {
			a.close()
			t.Fatal(err)
		}
		if _, err := protocol.VerifySustainedSequence(w, r, samples); err != nil {
			t.Fatal(err)
		}
		c := samples[len(samples)-1].SustainedRelease.PostCollection
		if c == nil || len(c.Observations) != 7 {
			t.Fatal("missing collection evidence")
		}
		if len(samples[len(samples)-1].Observations) != 15 {
			t.Fatal("collection mixed with operation/release windows")
		}
		if a.engine != nil || a.compiled != nil || a.instance != nil || a.fn != nil {
			t.Fatal("runtime reference retained")
		}
		a.close()
	}
}
