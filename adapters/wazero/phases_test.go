package main

import (
	"strings"
	"testing"

	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
)

func TestCompilePhasesVerifyMeasuredModule(t *testing.T) {
	workloads, err := corpus.Generate(t.TempDir(), "core")
	if err != nil {
		t.Fatal(err)
	}
	for _, wrong := range []bool{false, true} {
		a := &adapter{}
		defer a.close()
		w := workloads[0]
		if wrong {
			w.Oracle.Expected = protocol.Values{123456}
		}
		if err = a.prepare(&protocol.Preparation{Artifact: w.Artifact, ArtifactSHA256: w.SHA256, Workload: w, Profile: "memory"}); err != nil {
			t.Fatal(err)
		}
		var events []protocol.PhaseEvent
		a.barrier = func(e protocol.PhaseEvent) error { events = append(events, e); return nil }
		samples, err := a.run(&protocol.RunRequest{Scenario: "compile", Samples: 2, Operations: 17, PhaseBarriers: true})
		if wrong {
			if err == nil || !strings.Contains(err.Error(), "incorrect result") {
				t.Fatal("wrong oracle accepted", err)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(events) != 6 || len(samples) != 2 {
			t.Fatal(events, samples)
		}
		for i, s := range samples {
			if s.Operations != 1 || !s.Verified || s.Index != i || len(s.Observations) != 7 {
				t.Fatal(s)
			}
		}
		if events[0].Stage != "before_compile" || events[1].Stage != "compiled" || events[2].Stage != "released" {
			t.Fatal(events)
		}
		a.prep.Profile = "timing"
		if _, err = a.run(&protocol.RunRequest{Scenario: "compile", Samples: 1, Operations: 1, PhaseBarriers: true}); err == nil {
			t.Fatal("instrumented headline timing")
		}
	}
}
