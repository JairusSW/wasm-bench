// Package corpus supplies deterministic, licensed fixtures and admission checks.
package corpus

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/wasmbench/wasmbench/protocol"
	"os"
	"path/filepath"
)

const GeneratorVersion = "wasmbench-core-v2"

func Hash(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func uleb(n int) []byte {
	var b []byte
	for {
		x := byte(n & 127)
		n >>= 7
		if n != 0 {
			x |= 128
		}
		b = append(b, x)
		if n == 0 {
			return b
		}
	}
}
func section(id byte, body []byte) []byte {
	return append(append([]byte{id}, uleb(len(body))...), body...)
}

// Module returns a core Wasm module exporting benchmark(i32)->i32 and memory.
// Additional functions are distinct definitions to exercise module scaling.
func Module(kind string, size int) []byte {
	b := []byte{0, 97, 115, 109, 1, 0, 0, 0}
	b = append(b, section(1, []byte{1, 0x60, 1, 0x7f, 1, 0x7f})...)
	count := 1
	if kind == "functions" {
		count = size
	}
	funcs := uleb(count)
	for i := 0; i < count; i++ {
		funcs = append(funcs, 0)
	}
	b = append(b, section(3, funcs)...)
	b = append(b, section(5, []byte{1, 0, 1})...)
	b = append(b, section(7, []byte{2, 9, 'b', 'e', 'n', 'c', 'h', 'm', 'a', 'r', 'k', 0, 0, 6, 'm', 'e', 'm', 'o', 'r', 'y', 2, 0})...)
	body := []byte{0, 0x20, 0, 0x0b} // identity calibration
	if kind == "sum" {
		// sum n..1 modulo 2^32, with local accumulator.
		body = []byte{1, 1, 0x7f, 0x02, 0x40, 0x03, 0x40, 0x20, 0, 0x45, 0x0d, 1, 0x20, 1, 0x20, 0, 0x6a, 0x21, 1, 0x20, 0, 0x41, 1, 0x6b, 0x21, 0, 0x0c, 0, 0x0b, 0x0b, 0x20, 1, 0x0b}
	}
	if kind == "body" {
		body = []byte{0, 0x20, 0}
		for i := 0; i < size; i++ {
			body = append(body, 0x41, 1, 0x6a)
		}
		body = append(body, 0x0b)
	}
	code := uleb(count)
	for i := 0; i < count; i++ {
		code = append(code, uleb(len(body))...)
		code = append(code, body...)
	}
	return append(b, section(10, code)...)
}

func Generate(root, suite string) ([]protocol.Workload, error) {
	if suite == "process-snapshot-density" {
		return generateSnapshotDensity(root)
	}
	if suite == "process-snapshots" {
		return generateProcessSnapshots(root)
	}
	if suite == "continuations" {
		return generateContinuations(root)
	}
	if suite == "sustained" {
		b := Module("sum", 1)
		path := filepath.Join(root, "sustained", "integer-sum.wasm")
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, b, 0644); err != nil {
			return nil, err
		}
		return []protocol.Workload{{Schema: 1, ID: "sustained/integer-sum-256", Family: "algorithms", Artifact: path, SHA256: Hash(b), ABI: "core", Features: []string{"mvp"}, Export: "benchmark", Args: protocol.Values{256}, WorkUnit: "integer_sum", Units: 1, Reset: "stateless", Oracle: protocol.Oracle{Kind: "exact_u64", Expected: protocol.Values{32896}}, License: "MIT", Source: "corpus/corpus.go", Generator: "wasmbench-sustained-sum-v1"}}, nil
	}
	if suite == "guest-density" {
		return generateGuestDensity(root)
	}
	if suite == "checkpoints" {
		return generateCheckpoints(root)
	}
	if suite == "reactors" {
		return generateReactors(root)
	}
	if suite == "density" {
		return generateDensity(root)
	}
	if suite == "traps" {
		return generateTraps(root)
	}
	if suite == "lifecycle" {
		return generateLifecycle(root)
	}
	if suite == "floats" {
		return generateFloats(root)
	}
	if suite != "core" && suite != "scaling" {
		return nil, fmt.Errorf("unknown suite %q", suite)
	}
	type fixture struct {
		id, kind, family string
		size             int
		arg, expected    uint64
	}
	fixtures := []fixture{{"mechanisms/identity", "identity", "mechanisms", 1, 7, 7}, {"algorithms/sum", "sum", "algorithms", 1, 10000, 50005000}}
	if suite == "scaling" {
		fixtures = nil
		for _, n := range []int{1, 16, 64, 256, 1024} {
			for _, dimension := range ScalingDimensions {
				expected := uint64(7)
				if dimension == "body" {
					expected += uint64(n)
				}
				if dimension == "data-segments" {
					expected += uint64((n - 1) % 256)
				}
				fixtures = append(fixtures, fixture{fmt.Sprintf("scaling/%s-%d", dimension, n), dimension, "scaling", n, 7, expected})
			}
		}
	}
	var out []protocol.Workload
	for _, f := range fixtures {
		b := Module(f.kind, f.size)
		if suite == "scaling" {
			var err error
			b, err = ScalingModule(f.kind, f.size)
			if err != nil {
				return nil, err
			}
		}
		path := filepath.Join(root, f.id+".wasm")
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, b, 0644); err != nil {
			return nil, err
		}
		out = append(out, protocol.Workload{Schema: 1, ID: f.id, Family: f.family, Artifact: path, SHA256: Hash(b), ABI: "core", Features: []string{"mvp"}, Export: "benchmark", Args: []uint64{f.arg}, WorkUnit: "invocation", Units: 1, Reset: "stateless", Oracle: protocol.Oracle{Kind: "exact_u64", Expected: []uint64{f.expected}}, License: "MIT", Source: "corpus/corpus.go", Generator: GeneratorVersion, Dimension: f.kind, Size: f.size})
		if f.kind == "imports" {
			out[len(out)-1].HostProfile = "identity-v1"
		}
	}
	return out, nil
}

type Structure struct {
	Analyzer          string       `json:"analyzer"`
	SHA256            string       `json:"sha256"`
	Bytes             int          `json:"bytes"`
	Sections          map[byte]int `json:"section_payload_bytes"`
	FunctionBodyBytes []int        `json:"function_body_bytes"`
}

// Analyze is intentionally structural, not a feature validator or opcode parser.
func Analyze(b []byte) (Structure, error) {
	s := Structure{Analyzer: "wasmbench-structure-v1", SHA256: Hash(b), Bytes: len(b), Sections: map[byte]int{}}
	if len(b) < 8 || string(b[:8]) != "\x00asm\x01\x00\x00\x00" {
		return s, fmt.Errorf("not a core Wasm v1 module")
	}
	read := func(data []byte, pos *int) (int, error) {
		n := uint64(0)
		for shift := uint(0); shift < 35; shift += 7 {
			if *pos >= len(data) {
				return 0, fmt.Errorf("truncated LEB")
			}
			c := data[*pos]
			*pos++
			n |= uint64(c&127) << shift
			if c < 128 {
				if n > 1<<32-1 {
					return 0, fmt.Errorf("LEB overflow")
				}
				return int(n), nil
			}
		}
		return 0, fmt.Errorf("invalid LEB")
	}
	for p := 8; p < len(b); {
		id := b[p]
		p++
		n, err := read(b, &p)
		if err != nil {
			return s, err
		}
		if n > len(b)-p {
			return s, fmt.Errorf("truncated section")
		}
		end := p + n
		s.Sections[id] += n
		if id == 10 {
			q := p
			count, e := read(b[:end], &q)
			if e != nil {
				return s, e
			}
			for i := 0; i < count; i++ {
				length, e := read(b[:end], &q)
				if e != nil {
					return s, e
				}
				if length > end-q {
					return s, fmt.Errorf("truncated body")
				}
				s.FunctionBodyBytes = append(s.FunctionBodyBytes, length)
				q += length
			}
			if q != end {
				return s, fmt.Errorf("trailing code bytes")
			}
		}
		p = end
	}
	return s, nil
}
