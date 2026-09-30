package corpus

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/wasmbench/wasmbench/protocol"
)

func checkpointSLEB(n int) []byte {
	var b []byte
	for {
		x := byte(n & 127)
		n >>= 7
		done := (n == 0 && x&64 == 0) || (n == -1 && x&64 != 0)
		if !done {
			x |= 128
		}
		b = append(b, x)
		if done {
			return b
		}
	}
}

// CheckpointModule defines EVERY mutable guest state component supported by
// the eager checkpoint contract. Fixed min=max memory prevents hidden growth
// state. Exact-byte comparison in the adapter disallows other modules.
func CheckpointModule(pages uint32) ([]byte, error) {
	if pages < 1 || pages > 64 {
		return nil, fmt.Errorf("checkpoint pages must be 1 to 64")
	}
	b := []byte{0, 97, 115, 109, 1, 0, 0, 0}
	b = append(b, section(1, []byte{2, 0x60, 0, 0, 0x60, 0, 1, 0x7f})...)
	b = append(b, section(3, []byte{3, 0, 0, 1})...)
	m := []byte{1, 1}
	m = append(m, uleb(int(pages))...)
	m = append(m, uleb(int(pages))...)
	b = append(b, section(5, m)...)
	b = append(b, section(6, []byte{1, 0x7f, 1, 0x41, 0, 0x0b})...)
	exports := []byte{5}
	for _, e := range []struct {
		name        string
		kind, index byte
	}{{"memory", 2, 0}, {"state", 3, 0}, {"initialize", 0, 0}, {"first_write", 0, 1}, {"benchmark", 0, 2}} {
		exports = append(exports, uleb(len(e.name))...)
		exports = append(exports, e.name...)
		exports = append(exports, e.kind, e.index)
	}
	b = append(b, section(7, exports)...)
	bound := checkpointSLEB(int(pages) * 65536)
	loop := []byte{0x02, 0x40, 0x03, 0x40, 0x20, 0, 0x41}
	loop = append(loop, bound...)
	loop = append(loop, 0x4f, 0x0d, 1)
	advance := []byte{0x20, 0, 0x41, 1, 0x6a, 0x21, 0, 0x0c, 0, 0x0b, 0x0b}
	init := []byte{1, 1, 0x7f, 0x41, 42, 0x24, 0}
	init = append(init, loop...)
	init = append(init, 0x20, 0, 0x41, 7, 0x3a, 0, 0)
	init = append(init, advance...)
	init = append(init, 0x0b)
	write := []byte{0, 0x41}
	write = append(write, checkpointSLEB(int(pages)*65536-1)...)
	write = append(write, 0x41, 11, 0x3a, 0, 0, 0x41, 43, 0x24, 0, 0x0b)
	checksum := []byte{1, 2, 0x7f}
	checksum = append(checksum, loop...)
	checksum = append(checksum, 0x20, 1, 0x20, 0, 0x2d, 0, 0, 0x6a, 0x21, 1)
	checksum = append(checksum, advance...)
	checksum = append(checksum, 0x20, 1, 0x23, 0, 0x6a, 0x0b)
	code := []byte{3}
	for _, body := range [][]byte{init, write, checksum} {
		code = append(code, uleb(len(body))...)
		code = append(code, body...)
	}
	return append(b, section(10, code)...), nil
}

func generateCheckpoints(root string) ([]protocol.Workload, error) {
	var out []protocol.Workload
	for _, pages := range []uint32{1, 4, 16, 64} {
		b, err := CheckpointModule(pages)
		if err != nil {
			return nil, err
		}
		id := fmt.Sprintf("checkpoints/memory-scalar/%d", pages)
		path := filepath.Join(root, id+".wasm")
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, b, 0644); err != nil {
			return nil, err
		}
		out = append(out, protocol.Workload{Schema: 1, ID: id, Family: "checkpoints", Artifact: path, SHA256: Hash(b), ABI: "core", Features: []string{"mvp"}, Export: "benchmark", Initialize: "initialize", WorkUnit: "guest_checkpoint", Units: 1, Reset: "fresh_instance_per_sample", Oracle: protocol.Oracle{Kind: "guest_checkpoint_v1", Expected: protocol.Values{uint64(pages)*65536*7 + 42}}, License: "MIT", Source: "corpus/checkpoint.go", Generator: "wasmbench-guest-checkpoint-v1", Dimension: "guest_memory_pages", Size: int(pages), Checkpoint: &protocol.CheckpointContract{Mode: protocol.GuestCheckpointMode, Pages: pages}})
	}
	return out, nil
}
