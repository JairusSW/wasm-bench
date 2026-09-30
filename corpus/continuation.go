package corpus

import (
	"fmt"
	"github.com/wasmbench/wasmbench/protocol"
	"os"
	"path/filepath"
)

// ContinuationModule exercises engine-native execution-stack restoration. Depth
// is the number of recursive ancestors above the capture frame, not stack bytes.
// Memory and globals deliberately change before restore and MUST stay changed:
// this artifact must never be admitted as a whole-instance snapshot workload.
func ContinuationModule(depth int) ([]byte, error) {
	if depth < 0 || depth > 128 {
		return nil, fmt.Errorf("continuation depth must be 0 to 128")
	}
	b := []byte{0, 97, 115, 109, 1, 0, 0, 0}
	b = append(b, section(1, []byte{3, 0x60, 0, 1, 0x7f, 0x60, 0, 0, 0x60, 1, 0x7f, 1, 0x7f})...)
	imports := []byte{3}
	for i, name := range []string{"capture", "restore", "resumed"} {
		module := "wasmbench_continuation_v1"
		imports = append(imports, uleb(len(module))...)
		imports = append(imports, module...)
		imports = append(imports, uleb(len(name))...)
		imports = append(imports, name...)
		typeIndex := byte(1)
		if i == 0 {
			typeIndex = 0
		}
		imports = append(imports, 0, typeIndex)
	}
	b = append(b, section(2, imports)...)
	b = append(b, section(3, []byte{4, 2, 0, 1, 0})...)
	b = append(b, section(5, []byte{1, 1, 1, 1})...)
	b = append(b, section(6, []byte{1, 0x7f, 1, 0x41, 0, 0x0b})...)
	exports := []byte{5}
	for _, e := range []struct {
		name        string
		kind, index byte
	}{{"memory", 2, 0}, {"state", 3, 0}, {"run", 0, 4}, {"first_write", 0, 5}, {"benchmark", 0, 6}} {
		exports = append(exports, uleb(len(e.name))...)
		exports = append(exports, e.name...)
		exports = append(exports, e.kind, e.index)
	}
	b = append(b, section(7, exports)...)
	// At capture return 0, mutate non-stack state and restore. A restored return
	// of 1 branches to the marker; normal return from restore traps. Ancestors
	// retain distinct parameters and add them while unwinding the restored stack.
	recurse := []byte{0, 0x20, 0, 0x45, 0x04, 0x7f, 0x10, 0, 0x04, 0x7f,
		0x10, 2, 0x41, 7, 0x05, 0x41, 0, 0x41, 11, 0x3a, 0, 0, 0x41}
	recurse = append(recurse, checkpointSLEB(99)...)
	recurse = append(recurse, 0x24, 0, 0x10, 1, 0, 0x0b, 0x05,
		0x20, 0, 0x41, 1, 0x6b, 0x10, 3, 0x20, 0, 0x6a, 0x0b, 0x0b)
	run := append([]byte{0, 0x41}, checkpointSLEB(depth)...)
	run = append(run, 0x10, 3, 0x0b)
	write := append([]byte{0, 0x41}, checkpointSLEB(65535)...)
	write = append(write, 0x41, 22, 0x3a, 0, 0, 0x41)
	write = append(write, checkpointSLEB(100)...)
	write = append(write, 0x24, 0, 0x0b)
	// Full-memory checksum, not just the locations written by the fixture.
	checksum := []byte{1, 2, 0x7f, 0x02, 0x40, 0x03, 0x40, 0x20, 0, 0x41}
	checksum = append(checksum, checkpointSLEB(65536)...)
	checksum = append(checksum, 0x4f, 0x0d, 1, 0x20, 1, 0x20, 0, 0x2d, 0, 0,
		0x6a, 0x21, 1, 0x20, 0, 0x41, 1, 0x6a, 0x21, 0, 0x0c, 0, 0x0b, 0x0b,
		0x20, 1, 0x23, 0, 0x6a, 0x0b)
	code := []byte{4}
	for _, body := range [][]byte{recurse, run, write, checksum} {
		code = append(code, uleb(len(body))...)
		code = append(code, body...)
	}
	return append(b, section(10, code)...), nil
}

func generateContinuations(root string) ([]protocol.Workload, error) {
	var out []protocol.Workload
	for _, depth := range []int{0, 1, 8, 32, 128} {
		b, err := ContinuationModule(depth)
		if err != nil {
			return nil, err
		}
		id := fmt.Sprintf("continuations/native-stack/%d", depth)
		path := filepath.Join(root, id+".wasm")
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, b, 0644); err != nil {
			return nil, err
		}
		out = append(out, protocol.Workload{Schema: 1, ID: id, Family: "continuations", Artifact: path, SHA256: Hash(b), ABI: "core", Features: []string{"mvp"}, Export: "run", WorkUnit: "native_continuation_stage", Units: 1, Reset: "fresh_instance_per_sample", Oracle: protocol.Oracle{Kind: "native_continuation_v1", Expected: protocol.Values{133}}, License: "MIT", Source: "corpus/continuation.go", Generator: "wasmbench-native-continuation-v1", Dimension: "recursive_ancestor_depth", Size: depth, Continuation: &protocol.ContinuationContract{Mode: protocol.NativeContinuationMode, Depth: depth}})
	}
	return out, nil
}
