package main

import (
	"os"
	"testing"

	"github.com/wasmbench/wasmbench/protocol"
)

func TestEmscriptenMainArgvAndExit(t *testing.T) {
	wasm, err := os.ReadFile("testdata/emscripten-command.wasm")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		argv    string
		wantErr bool
	}{{name: "correct argv", argv: "fixture"}, {name: "incorrect argv", argv: "wrong", wantErr: true}} {
		t.Run(tc.name, func(t *testing.T) {
			w := protocol.Workload{
				ABI: "emscripten", Export: "main", Reset: "fresh_instance_per_sample", HostProfile: protocol.EmscriptenStdioProfile,
				Command: &protocol.CommandContract{Argv: []string{tc.argv}, ExitCode: 0, StdoutSHA256: protocol.CommandDigest(nil), StderrSHA256: protocol.CommandDigest(nil), OutputLimit: 128},
				Oracle:  protocol.Oracle{Kind: "exact_command"},
			}
			a := &adapter{prep: &protocol.Preparation{Workload: w, Profile: "timing"}, wasm: wasm}
			defer a.close()
			samples, err := a.run(&protocol.RunRequest{Scenario: "first-call", Samples: 1, Operations: 1})
			if (err != nil) != tc.wantErr {
				t.Fatalf("run error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if len(samples) != 1 || !samples[0].Verified || samples[0].CommandResult == nil || samples[0].CommandResult.ExitCode != 0 {
				t.Fatalf("unexpected Emscripten result: %+v", samples)
			}
		})
	}
}
