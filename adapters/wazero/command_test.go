package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wasmbench/wasmbench/protocol"
)

func TestWASICommand(t *testing.T) {
	wasm, err := os.ReadFile("testdata/command.wasm")
	if err != nil {
		t.Fatal(err)
	}
	for _, interpreter := range []bool{false, true} {
		for _, scenario := range []string{"compile", "instantiate", "first-call", "steady", "teardown"} {
			for _, mode := range []string{"correct", "memory", "file", "wrong-file", "stdout", "stderr", "exit", "overflow", "short-input", "mixed", "phase"} {
				t.Run(scenario+"/"+mode+map[bool]string{false: "/compiler", true: "/interpreter"}[interpreter], func(t *testing.T) {
					c := &protocol.CommandContract{Argv: []string{"test", "arg"}, Stdin: []byte("abc"), ExitCode: 7, StdoutSHA256: protocol.CommandDigest([]byte("abc")), StderrSHA256: protocol.CommandDigest([]byte("err")), OutputLimit: 16}
					w := protocol.Workload{ABI: "wasi-command", Export: "_start", Reset: "fresh_instance_per_sample", HostProfile: "wasi-preview1-readonly-v1", Oracle: protocol.Oracle{Kind: "exact_command"}, Command: c}
					if mode == "file" || mode == "wrong-file" {
						name := filepath.Join(t.TempDir(), "input")
						data := []byte("abc")
						if mode == "wrong-file" {
							data = []byte("bad")
						}
						if err := os.WriteFile(name, data, 0600); err != nil {
							t.Fatal(err)
						}
						c.Files = map[string]protocol.CommandFile{"input": {Path: name, Size: 3, SHA256: protocol.CommandDigest([]byte("abc"))}}
						c.Stdin = nil
						c.StdinFile = "input"
					}
					switch mode {
					case "stdout":
						c.StdoutSHA256 = protocol.CommandDigest(nil)
					case "stderr":
						c.StderrSHA256 = protocol.CommandDigest(nil)
					case "exit":
						c.ExitCode = 0
					case "overflow":
						c.OutputLimit = 1
					case "short-input":
						c.Stdin = nil
					case "mixed":
						w.Args = protocol.Values{1}
					}
					profile := "timing"
					if mode == "memory" {
						profile = "memory"
					}
					a := &adapter{prep: &protocol.Preparation{Workload: w, Profile: profile}, wasm: wasm, interpreter: interpreter}
					defer a.close()
					samples, err := a.run(&protocol.RunRequest{Scenario: scenario, Samples: 2, Warmup: 1, Operations: 5, PhaseBarriers: mode == "phase"})
					if mode != "correct" && mode != "memory" && mode != "file" {
						if err == nil {
							t.Fatal("accepted invalid command")
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					want := 2
					if scenario == "steady" {
						want++
					}
					if len(samples) != want {
						t.Fatal(samples)
					}
					for _, s := range samples {
						if !s.Verified || s.CommandResult == nil || s.CommandResult.ExitCode != 7 || s.CommandResult.StdoutBytes != 3 || s.CommandResult.StderrBytes != 3 || s.Operations != 1 {
							t.Fatal(s)
						}
						if (len(s.Observations) > 0) != (profile == "memory") {
							t.Fatal("wrong instrumentation", s)
						}
					}
				})
			}
		}
	}
}
