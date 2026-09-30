package main

import (
	"fmt"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
	"testing"
)

func TestDensityGroups(t *testing.T) {
	ws, err := corpus.Generate(t.TempDir(), "density")
	if err != nil {
		t.Fatal(err)
	}
	for _, interpreter := range []bool{false, true} {
		for _, w := range ws {
			t.Run(fmt.Sprintf("%t/%s", interpreter, w.ID), func(t *testing.T) {
				a := &adapter{interpreter: interpreter}
				defer a.close()
				if err := a.prepare(&protocol.Preparation{Artifact: w.Artifact, ArtifactSHA256: w.SHA256, Workload: w, Profile: "memory"}); err != nil {
					t.Fatal(err)
				}
				g := &densityGroup{}
				defer g.close()
				if err := a.buildDensityGroup(g); err != nil {
					t.Fatal(err)
				}
				engines := 1
				if w.Density.Sharing == "separate_engines" {
					engines = w.Density.Instances
				}
				if len(g.engines) != engines || len(g.modules) != w.Density.Instances || len(g.results) != w.Density.Instances {
					t.Fatal("wrong group shape")
				}
				for i, m := range g.modules {
					if m.IsClosed() || !a.verifyInstance(m, g.results[i]) {
						t.Fatal("member not simultaneously live and verified")
					}
					for j := 0; j < i; j++ {
						if m == g.modules[j] {
							t.Fatal("instance reused")
						}
					}
				}
				if err := g.close(); err != nil {
					t.Fatal(err)
				}
				for _, m := range g.modules {
					if !m.IsClosed() {
						t.Fatal("instance not released")
					}
				}
				var events []protocol.PhaseEvent
				a.barrier = func(e protocol.PhaseEvent) error { events = append(events, e); return nil }
				r := &protocol.RunRequest{Scenario: "density", Samples: 2, Operations: 1, PhaseBarriers: true}
				samples, err := a.run(r)
				if err != nil {
					t.Fatal(err)
				}
				if len(samples) != 2 || len(events) != 6 {
					t.Fatal("incomplete group samples or barriers")
				}
				for i, e := range events {
					if e.SampleIndex != i/3 || e.Stage != protocol.PhaseStages("density")[i%3] {
						t.Fatal("wrong phase order")
					}
				}
				for i, s := range samples {
					if s.Index != i || !s.Verified || s.Operations != 1 || s.Warmup || s.SampleType != "individual_operation" || len(s.Observations) == 0 {
						t.Fatalf("invalid sample %+v", s)
					}
				}
				a.prep.Workload.Oracle.Expected = protocol.Values{999999}
				events = nil
				if _, err := a.run(r); err == nil {
					t.Fatal("incorrect group accepted")
				}
				if len(events) != 1 || events[0].Stage != "before_density" {
					t.Fatal("incorrect group reported ready")
				}
			})
		}
	}
}
