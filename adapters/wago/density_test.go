package main

import (
	"context"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
	"os"
	"testing"
)

func TestDensityGroups(t *testing.T) {
	ws, err := corpus.Generate(t.TempDir(), "density")
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range ws {
		t.Run(w.ID, func(t *testing.T) {
			wasm, err := os.ReadFile(w.Artifact)
			if err != nil {
				t.Fatal(err)
			}
			a := &adapter{prep: &protocol.Preparation{Artifact: w.Artifact, ArtifactSHA256: w.SHA256, Workload: w, Profile: "memory"}, wasm: wasm}
			defer a.closeAll()
			g := &densityGroup{}
			defer g.close()
			if err := a.buildDensityGroup(g); err != nil {
				t.Fatal(err)
			}
			engines := 1
			if w.Density.Sharing == "separate_engines" {
				engines = w.Density.Instances
			}
			if len(g.engines) != engines || len(g.modules) != engines || len(g.instances) != w.Density.Instances || len(g.results) != w.Density.Instances {
				t.Fatal("wrong group shape")
			}
			for i, m := range g.instances {
				if err := a.verifyCompiled(nil, m, g.results[i]); err != nil {
					t.Fatal(err)
				}
				if _, ok := m.Read(0, 65536); !ok {
					t.Fatal("member not live")
				}
				for j := 0; j < i; j++ {
					if m == g.instances[j] {
						t.Fatal("instance reused")
					}
				}
			}
			if err := g.close(); err != nil {
				t.Fatal(err)
			}
			for _, e := range g.engines {
				select {
				case <-e.Closed():
				default:
					t.Fatal("runtime not closed at release")
				}
			}
			for _, m := range g.instances {
				if err := m.WaitClosed(context.Background()); err != nil {
					t.Fatal(err)
				}
			}
			var events []protocol.PhaseEvent
			a.barrier = func(e protocol.PhaseEvent) error { events = append(events, e); return nil }
			request := &protocol.RunRequest{Scenario: "density", Samples: 2, Operations: 1, PhaseBarriers: true}
			samples, err := a.run(request)
			if err != nil {
				t.Fatal(err)
			}
			if len(samples) != 2 || len(events) != 6 {
				t.Fatal("missing samples/barriers")
			}
			for i, e := range events {
				if e.SampleIndex != i/3 || e.Stage != protocol.PhaseStages("density")[i%3] {
					t.Fatal("wrong barrier order")
				}
			}
			for _, s := range samples {
				if !s.Verified || s.Operations != 1 || s.Warmup || s.SampleType != "individual_operation" {
					t.Fatal("wrong sample semantics")
				}
				found := false
				for _, o := range s.Observations {
					if o.Metric == "density.guest_memory.logical" {
						found = true
						if o.Value == nil || *o.Value != float64(w.Density.Instances*65536) {
							t.Fatal("wrong logical group memory")
						}
					}
				}
				if !found {
					t.Fatal("logical memory missing")
				}
			}
			a.prep.Workload.Oracle.Expected = protocol.Values{999999}
			events = nil
			if _, err := a.run(request); err == nil {
				t.Fatal("incorrect group accepted")
			}
			if len(events) != 1 {
				t.Fatal("incorrect group reported ready")
			}
		})
	}
}
