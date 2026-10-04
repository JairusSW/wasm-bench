package experiment

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
)

func TestWebsiteCodePassVectorPreflight(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "bin", "adapter-wazero")
	if _, err := os.Stat(binary); os.IsNotExist(err) {
		t.Skip("build wazero adapter first")
	}
	runtimes, err := ResolveRuntimes(root, []string{"wazero"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := agent.Start(context.Background(), runtimes[0].Command, filepath.Join(t.TempDir(), "describe"), 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	description, err := client.Call(protocol.Request{Method: "describe"})
	client.Close()
	if err != nil {
		t.Fatal(err)
	}
	runtime := runtimes[0]
	runtime.Description = description.Description
	data, err := os.ReadFile(filepath.Join(root, "corpus/testdata/vector-lifecycle.wasm"))
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	if err := os.Mkdir(filepath.Join(directory, "logs"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "vector.wasm"), data, 0600); err != nil {
		t.Fatal(err)
	}
	workload := protocol.Workload{Schema: 1, ID: "vector", Artifact: "vector.wasm", SHA256: corpus.Hash(data), ABI: "core", Export: "benchmark", Reset: "fresh_instance_per_sample", WorkUnit: "vector_sequence", Units: 1,
		Vectors: &protocol.VectorContract{InputOffset: 32, OutputOffset: 16, OutputLen: 1, Mod: 3, Cases: []protocol.VectorCase{{Out: "ab"}, {Len: 7, Out: "ab"}}}, VectorByteBudget: 1024, Oracle: protocol.Oracle{Kind: "exact_vectors"}}
	options := Options{Profile: "code", Samples: 1, Operations: 1, Timeout: 10 * time.Second}
	check := runTrial(context.Background(), directory, options, runtime, workload, "steady", -1, "preflight")
	if check.Status != "ok" {
		t.Fatalf("code-profile correctness preflight failed: %+v", check)
	}
	measured := runTrial(context.Background(), directory, options, runtime, workload, "compile", 0, "code")
	if measured.Status != "unsupported" || len(measured.Samples) != 0 {
		t.Fatalf("unsupported vector code extraction produced measurements: %+v", measured)
	}
}
