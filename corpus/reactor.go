package corpus

import (
	_ "embed"
	"github.com/wasmbench/wasmbench/protocol"
	"os"
	"path/filepath"
)

//go:embed testdata/wasi-reactor.wasm
var reactorModule []byte

func generateReactors(root string) ([]protocol.Workload, error) {
	file := filepath.Join(root, "mechanisms", "wasi-reactor.wasm")
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(file, reactorModule, 0644); err != nil {
		return nil, err
	}
	var out []protocol.Workload
	for _, mode := range []struct{ name, export, reset string }{{"stateless", "benchmark", "stateless"}, {"fresh", "once", "fresh_instance_per_sample"}} {
		out = append(out, protocol.Workload{Schema: 1, ID: "mechanisms/wasi-reactor-" + mode.name, Family: "mechanisms", Artifact: file, SHA256: Hash(reactorModule), ABI: "wasi-reactor", HostProfile: protocol.WASIReactorProfile, Features: []string{"mvp"}, Export: mode.export, Initialize: "_initialize", Args: protocol.Values{35}, Input: &protocol.MemoryInput{PointerExport: "input_ptr", Hex: "07000000"}, Reset: mode.reset, WorkUnit: "invocation", Units: 1, Oracle: protocol.Oracle{Kind: "exact_u64", Expected: protocol.Values{42}, Memory: []protocol.MemoryCheck{{Offset: 64, Hex: "01000000"}}}, License: "MIT", Source: "corpus/testdata/wasi-reactor.wat", Generator: "wasmbench-wasi-reactor-v1"})
	}
	return out, nil
}
