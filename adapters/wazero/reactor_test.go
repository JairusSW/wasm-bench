package main

import (
	"bytes"
	"os"
	"testing"

	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
)

func TestReactorLifecycle(t *testing.T) {
	for _, interpreter := range []bool{false, true} {
		for _, profile := range []string{"timing", "memory"} {
			for _, scenario := range []string{"compile", "instantiate", "app-init", "first-call", "steady", "teardown"} {
				t.Run(profile+"/"+scenario+map[bool]string{false: "/compiler", true: "/interpreter"}[interpreter], func(t *testing.T) {
					workloads, err := corpus.Generate(t.TempDir(), "reactors")
					if err != nil {
						t.Fatal(err)
					}
					for _, w := range workloads {
						a := adapter{interpreter: interpreter}
						defer a.close()
						if err := a.prepare(&protocol.Preparation{Artifact: w.Artifact, ArtifactSHA256: w.SHA256, Workload: w, Profile: profile}); err != nil {
							t.Fatal(err)
						}
						samples, err := a.run(&protocol.RunRequest{Scenario: scenario, Samples: 3, Operations: 4, Warmup: 2})
						if err != nil {
							t.Fatal(w.ID, err)
						}
						count := 3
						if scenario == "steady" {
							count = 5
						}
						if len(samples) != count {
							t.Fatal("missing samples", samples)
						}
						for i, s := range samples {
							ops := 1
							if scenario == "steady" && w.Reset == "stateless" {
								ops = 4
							}
							if !s.Verified || len(s.Result) != 1 || s.Result[0] != 42 || s.Operations != ops || s.Warmup != (scenario == "steady" && i < 2) {
								t.Fatal(w.ID, s)
							}
							if (len(s.Observations) > 0) != (profile == "memory") {
								t.Fatal("incorrect instrumentation", s)
							}
						}
					}
				})
			}
		}
	}
}

func TestReactorInvalidOraclesAndInitialization(t *testing.T) {
	for _, mode := range []string{"wrong", "missing", "params", "results", "stream", "oracle-stream", "repeat-stateful", "wrong-middle"} {
		t.Run(mode, func(t *testing.T) {
			ws, err := corpus.Generate(t.TempDir(), "reactors")
			if err != nil {
				t.Fatal(err)
			}
			w := ws[0]
			switch mode {
			case "wrong":
				w.Oracle.Expected[0]++
			case "stream":
				w.Export = "write_output"
			case "oracle-stream":
				w.Oracle.OutputPointerExport = "write_pointer"
				w.Oracle.Memory[0].Offset = 0
			case "wrong-middle":
				w.Export = "wrong_middle"
			case "repeat-stateful":
				w.Export = "once" // Lying stateless contract must fail on the second call.
			default:
				data, err := os.ReadFile(w.Artifact)
				if err != nil {
					t.Fatal(err)
				}
				data = bytes.ReplaceAll(data, []byte("_initialize"), []byte("unused_init"))
				if mode == "params" {
					data = bytes.ReplaceAll(data, []byte("param_init_"), []byte("_initialize"))
				}
				if mode == "results" {
					data = bytes.ReplaceAll(data, []byte("result_init"), []byte("_initialize"))
				}
				if err := os.WriteFile(w.Artifact, data, 0600); err != nil {
					t.Fatal(err)
				}
				w.SHA256 = corpus.Hash(data)
			}
			a := adapter{}
			defer a.close()
			if err := a.prepare(&protocol.Preparation{Artifact: w.Artifact, ArtifactSHA256: w.SHA256, Workload: w, Profile: "timing"}); err != nil {
				t.Fatal(err)
			}
			if _, err := a.run(&protocol.RunRequest{Scenario: "steady", Samples: 2, Operations: 3}); err == nil {
				t.Fatal("invalid reactor accepted")
			}
		})
	}
}
