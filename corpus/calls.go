package corpus

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/wasmbench/wasmbench/protocol"
)

// HostCallModule is the deliberately tiny guest used to measure one call
// crossing a runtime boundary. kind is host-to-wasm or wasm-to-host.
func HostCallModule(kind string) ([]byte, error) {
	if kind == "host-to-wasm" {
		return Module("identity", 1), nil
	}
	if kind != "wasm-to-host" {
		return nil, fmt.Errorf("unknown host-call direction %q", kind)
	}
	b := []byte{0, 97, 115, 109, 1, 0, 0, 0}
	b = append(b, section(1, []byte{1, 0x60, 1, 0x7f, 1, 0x7f})...) // (i32)->i32
	importName := []byte{1, 9, 'w', 'a', 's', 'm', 'b', 'e', 'n', 'c', 'h', 8, 'i', 'd', 'e', 'n', 't', 'i', 't', 'y', 0, 0}
	b = append(b, section(2, importName)...)
	b = append(b, section(3, []byte{1, 0})...) // one defined function, type 0
	b = append(b, section(7, []byte{1, 9, 'b', 'e', 'n', 'c', 'h', 'm', 'a', 'r', 'k', 0, 1})...)
	b = append(b, section(10, []byte{1, 6, 0, 0x20, 0, 0x10, 0, 0x0b})...) // local.get 0; call import 0
	return b, nil
}

func generateCalls(root string) ([]protocol.Workload, error) {
	workloads := make([]protocol.Workload, 0, 2)
	for _, direction := range []string{"host-to-wasm", "wasm-to-host"} {
		b, err := HostCallModule(direction)
		if err != nil {
			return nil, err
		}
		path := filepath.Join(root, "calls", direction+".wasm")
		if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return nil, err
		}
		if err = os.WriteFile(path, b, 0644); err != nil {
			return nil, err
		}
		w := protocol.Workload{Schema: 1, ID: "mechanisms/" + direction + "-call", Family: "mechanisms", Artifact: path, SHA256: Hash(b), ABI: "core", Features: []string{"mvp"}, Export: "benchmark", Args: protocol.Values{7}, WorkUnit: direction + "_call", Units: 1, Reset: "stateless", Oracle: protocol.Oracle{Kind: "exact_u64", Expected: protocol.Values{7}}, License: "MIT", Source: "corpus/calls.go", Generator: "wasmbench-host-call-v1"}
		if direction == "wasm-to-host" {
			w.HostProfile = "identity-v1"
		}
		workloads = append(workloads, w)
	}
	return workloads, nil
}
