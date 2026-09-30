package corpus

import (
	_ "embed"
	"github.com/wasmbench/wasmbench/protocol"
	"os"
	"path/filepath"
)

//go:embed testdata/app-init.wasm
var appInitModule []byte

func generateLifecycle(root string) ([]protocol.Workload, error) {
	path := filepath.Join(root, "mechanisms", "app-init.wasm")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, appInitModule, 0644); err != nil {
		return nil, err
	}
	return []protocol.Workload{{Schema: 1, ID: "mechanisms/app-init", Family: "mechanisms", Artifact: path, SHA256: Hash(appInitModule), ABI: "core", Features: []string{"mvp"}, Export: "benchmark", Args: protocol.Values{}, Initialize: "initialize", Input: &protocol.MemoryInput{PointerExport: "input_ptr", Hex: "07000000"}, WorkUnit: "invocation", Units: 1, Reset: "fresh_instance_per_sample", Oracle: protocol.Oracle{Kind: "exact_u64", Expected: protocol.Values{7}, Memory: []protocol.MemoryCheck{{Offset: 64, Hex: "2a000000"}}}, License: "MIT", Source: "corpus/testdata/app-init.wat", Generator: "wasmbench-lifecycle-v1"}}, nil
}
