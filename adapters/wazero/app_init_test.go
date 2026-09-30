package main

import (
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
	"testing"
)

func TestAppInitFreshState(t *testing.T) {
	for _, interpreter := range []bool{false, true} {
		for _, mode := range []string{"timing", "memory", "wrong", "missing", "trap", "params", "results", "phase"} {
			t.Run(mode+map[bool]string{false: "/compiler", true: "/interpreter"}[interpreter], func(t *testing.T) {
				ws, err := corpus.Generate(t.TempDir(), "lifecycle")
				if err != nil {
					t.Fatal(err)
				}
				w := ws[0]
				switch mode {
				case "wrong":
					w.Oracle.Expected[0]++
				case "missing":
					w.Initialize = ""
				case "trap":
					w.Initialize = "trap_init"
				case "params":
					w.Initialize = "param_init"
				case "results":
					w.Initialize = "result_init"
				}
				profile := "timing"
				if mode == "memory" {
					profile = "memory"
				}
				a := adapter{interpreter: interpreter}
				defer a.close()
				if err = a.prepare(&protocol.Preparation{Artifact: w.Artifact, ArtifactSHA256: w.SHA256, Workload: w, Profile: profile}); err != nil {
					t.Fatal(err)
				}
				samples, err := a.run(&protocol.RunRequest{Scenario: "app-init", Samples: 3, Operations: 100, Warmup: 5, PhaseBarriers: mode == "phase"})
				if mode != "timing" && mode != "memory" {
					if err == nil {
						t.Fatal("accepted invalid initializer")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if len(samples) != 3 {
					t.Fatal(samples)
				}
				for _, s := range samples {
					if s.Warmup || s.Operations != 1 || !s.Verified || s.Result[0] != 7 {
						t.Fatal(s)
					}
					if (len(s.Observations) == 7) != (mode == "memory") {
						t.Fatal(s)
					}
				}
			})
		}
	}
}
