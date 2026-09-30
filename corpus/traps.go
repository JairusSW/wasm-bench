package corpus

import (
	_ "embed"
	"github.com/wasmbench/wasmbench/protocol"
	"os"
	"path/filepath"
)

//go:embed testdata/traps.wasm
var trapModule []byte

func generateTraps(root string) ([]protocol.Workload, error) {
	path := filepath.Join(root, "mechanisms", "traps.wasm")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, trapModule, 0644); err != nil {
		return nil, err
	}
	var workloads []protocol.Workload
	for _, code := range protocol.TrapCodes {
		workloads = append(workloads, protocol.Workload{Schema: 1, ID: "mechanisms/trap/" + code, Family: "mechanisms", Artifact: path, SHA256: Hash(trapModule), ABI: "core", Features: []string{"mvp"}, Export: code, Args: protocol.Values{}, WorkUnit: "trapping_invocation", Units: 1, Reset: "fresh_instance_per_sample", Oracle: protocol.Oracle{Kind: "expected_trap", ExpectedTrap: code}, License: "MIT", Source: "corpus/testdata/traps.wat", Generator: "wasmbench-traps-v1/WABT-1.0.41"})
	}
	return workloads, nil
}
