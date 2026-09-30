package main

import (
	"github.com/wasmbench/wasmbench/protocol"
	"os"
	"strings"
	"testing"
)

func TestAssemblyScriptAbortIsNotNoop(t *testing.T) {
	wasm, err := os.ReadFile("../../corpus/testdata/assemblyscript-abort.wasm")
	if err != nil {
		t.Fatal(err)
	}
	for _, interpreter := range []bool{false, true} {
		for _, scenario := range []string{"compile", "instantiate", "first-call", "steady", "app-init", "teardown"} {
			for _, export := range []string{"benchmark", "abort_benchmark"} {
				a := adapter{interpreter: interpreter, wasm: wasm, prep: &protocol.Preparation{Profile: "timing", Workload: protocol.Workload{ABI: "core", HostProfile: protocol.AssemblyScriptAbortProfile, Initialize: "initialize", Export: export, Reset: "fresh_instance_per_sample", Oracle: protocol.Oracle{Kind: "exact_u64", Expected: protocol.Values{7}}}}}
				_, err := a.run(&protocol.RunRequest{Scenario: scenario, Samples: 2, Operations: 1})
				a.close()
				if export == "benchmark" {
					if err != nil {
						t.Fatal(err)
					}
				} else if err == nil || !strings.Contains(err.Error(), "AssemblyScript abort: message_ptr=4294967295") {
					t.Fatalf("lost abort: %v", err)
				}
			}
		}
	}
}
