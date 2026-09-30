package experiment_test

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

func TestBuiltAdapterOutputPointer(t *testing.T) {
	ids := os.Getenv("WASMBENCH_PHASE_TEST_RUNTIMES")
	if ids == "" {
		t.Skip("set WASMBENCH_PHASE_TEST_RUNTIMES after building adapters")
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	runtimes, err := experiment.ResolveRuntimes(root, strings.Split(ids, ","))
	if err != nil {
		t.Fatal(err)
	}
	// () -> i32 returns 16; exported memory contains ab at address 16.
	wasm, err := hex.DecodeString("0061736d010000000105016000017f0302010005030100010716020962656e63686d61726b0000066d656d6f727902000a0601040041100b0b07010041100b01ab")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "pointer.wasm")
	if err := os.WriteFile(path, wasm, 0600); err != nil {
		t.Fatal(err)
	}
	for _, runtime := range runtimes {
		for _, scenario := range []string{"compile", "instantiate", "first-call", "steady"} {
			for _, mode := range []string{"correct", "correct-input", "correct-absolute-input", "input-missing-pointer", "input-overflow", "input-out-of-bounds", "input-invalid-hex", "missing-pointer", "wrong-bytes", "overflow"} {
				t.Run(runtime.ID+"/"+scenario+"/"+mode, func(t *testing.T) {
					w := protocol.Workload{Schema: 1, ID: "pointer", Artifact: path, SHA256: corpus.Hash(wasm), ABI: "core", Export: "benchmark", Args: protocol.Values{}, Reset: "fresh_instance_per_sample", WorkUnit: "invocation", Units: 1,
						Oracle: protocol.Oracle{Kind: "exact_u64", Expected: protocol.Values{16}, OutputPointerExport: "benchmark", Memory: []protocol.MemoryCheck{{Offset: 0, Hex: "ab"}}}}
					switch mode {
					case "correct-input", "correct-absolute-input", "input-missing-pointer", "input-overflow", "input-out-of-bounds", "input-invalid-hex":
						w.Input = &protocol.MemoryInput{PointerExport: "benchmark", Hex: "cd"}
						w.Oracle.Memory[0].Hex = "cd"
						if mode == "correct-absolute-input" {
							w.Input.PointerExport = ""
							w.Input.Offset = 16
						}
						if mode == "input-missing-pointer" {
							w.Input.PointerExport = "absent"
						}
						if mode == "input-overflow" {
							w.Input.Offset = 0xfffffff0
						}
						if mode == "input-out-of-bounds" {
							w.Input.Offset = 65536
						}
						if mode == "input-invalid-hex" {
							w.Input.Hex = "c"
						}
					case "missing-pointer":
						w.Oracle.OutputPointerExport = "absent"
					case "wrong-bytes":
						w.Oracle.Memory[0].Hex = "ac"
					case "overflow":
						w.Oracle.Memory[0].Offset = 0xfffffff0
					}
					c, err := agent.Start(context.Background(), runtime.Command, filepath.Join(t.TempDir(), "log"), 10*time.Second)
					if err != nil {
						t.Fatal(err)
					}
					defer c.Close()
					_, err = c.Call(protocol.Request{Method: "prepare", Prepare: &protocol.Preparation{Artifact: path, ArtifactSHA256: w.SHA256, Workload: w, Profile: "timing"}})
					if err != nil {
						t.Fatal(err)
					}
					response, err := c.Call(protocol.Request{Method: "run", Run: &protocol.RunRequest{Scenario: scenario, Samples: 2, Operations: 2}})
					if !strings.HasPrefix(mode, "correct") {
						if err == nil {
							t.Fatal("accepted invalid memory oracle")
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					if len(response.Samples) != 2 {
						t.Fatal(response)
					}
					for _, sample := range response.Samples {
						if !sample.Verified {
							t.Fatal("unverified sample")
						}
					}
				})
			}
		}
	}
}
