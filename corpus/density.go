package corpus

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wasmbench/wasmbench/protocol"
)

//go:embed testdata/density.wasm
var densityModule []byte

func generateDensity(root string) ([]protocol.Workload, error) {
	path := filepath.Join(root, "density", "density.wasm")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, densityModule, 0644); err != nil {
		return nil, err
	}
	var out []protocol.Workload
	for _, mode := range []string{"shared_module", "separate_engines"} {
		for _, touched := range []uint64{0, 65536} {
			state := "idle"
			if touched != 0 {
				state = "touched_65536"
			}
			for _, count := range []int{1, 4, 16, 64} {
				out = append(out, protocol.Workload{Schema: 1, ID: fmt.Sprintf("density/%s/%s/%d", mode, state, count), Family: "density", Artifact: path, SHA256: Hash(densityModule), ABI: "core", Features: []string{"mvp"}, Export: "benchmark", Args: protocol.Values{touched}, WorkUnit: "instance_group", Units: 1, Reset: "fresh_instance_per_sample", Oracle: protocol.Oracle{Kind: "exact_u64", Expected: protocol.Values{touched + 1}}, License: "MIT", Source: "corpus/testdata/density.wat", Generator: "wasmbench-density-v1/" + mode + "/" + state, Dimension: "instances", Size: count, Density: &protocol.DensityContract{Instances: count, Sharing: mode}})
			}
		}
	}
	return out, nil
}
