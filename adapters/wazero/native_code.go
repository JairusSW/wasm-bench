package main

import (
	"encoding/binary"
	"fmt"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

// Pinned to wazero 1.12.0's internal/engine/wazevo/engine_cache.go serializer.
// This retains the native segment, excluding its version/offset/checksum wrapper.
// It does not claim instruction-only bytes or exact function attribution.
func readWazevoSegment(reader io.Reader) ([]byte, uint64, error) {
	header := make([]byte, 7)
	if _, err := io.ReadFull(reader, header); err != nil {
		return nil, 0, err
	}
	if string(header[:6]) != "WAZEVO" {
		return nil, 0, fmt.Errorf("not a Wazevo cache segment")
	}
	version := make([]byte, int(header[6]))
	if _, err := io.ReadFull(reader, version); err != nil {
		return nil, 0, err
	}
	if string(version) != "v1.12.0" && string(version) != "dev" {
		return nil, 0, fmt.Errorf("unsupported Wazevo cache version %q", version)
	}
	var count uint32
	if err := binary.Read(reader, binary.LittleEndian, &count); err != nil {
		return nil, 0, err
	}
	if count > 1_000_000 {
		return nil, 0, fmt.Errorf("Wazevo function count exceeds collector limit")
	}
	offsets := make([]uint64, count)
	for i := range offsets {
		if err := binary.Read(reader, binary.LittleEndian, &offsets[i]); err != nil {
			return nil, 0, err
		}
	}
	var size uint64
	if err := binary.Read(reader, binary.LittleEndian, &size); err != nil {
		return nil, 0, err
	}
	for i, offset := range offsets {
		if offset >= size || (i > 0 && offset <= offsets[i-1]) {
			return nil, 0, fmt.Errorf("invalid Wazevo function offset")
		}
	}
	checksumHash := crc32.New(crc32.MakeTable(crc32.Castagnoli))
	var code []byte
	if size > protocol.MaxCodeImageBytes {
		if size > 1<<30 {
			return nil, 0, fmt.Errorf("Wazevo native segment exceeds collector limit")
		}
		if n, err := io.CopyN(checksumHash, reader, int64(size)); err != nil || uint64(n) != size {
			return nil, 0, fmt.Errorf("incomplete oversized Wazevo native segment")
		}
	} else {
		code = make([]byte, int(size))
		if _, err := io.ReadFull(reader, code); err != nil {
			return nil, 0, err
		}
		checksumHash.Write(code)
	}
	var checksum uint32
	if err := binary.Read(reader, binary.LittleEndian, &checksum); err != nil {
		return nil, 0, err
	}
	if checksumHash.Sum32() != checksum {
		return nil, 0, fmt.Errorf("Wazevo native segment checksum mismatch")
	}
	return code, size, nil
}

func (a *adapter) inspectCode() (*protocol.CodeImage, []protocol.Observation, error) {
	if a.prep == nil || a.prep.Profile != "code" {
		return nil, nil, fmt.Errorf("native export requires code-profile preparation")
	}
	observation := protocol.Observation{Metric: "native.code_image", DefinitionVersion: 1, Unit: "bytes", Scope: "compiled_module", Collector: "wazero.Wazevo serialized executable", CollectorVersion: "wazero-1.12.0-cache-v1", Profile: "code", Quality: "engine_reported", Phase: "compile", Denominator: "module"}
	if a.interpreter {
		observation.Status = "not_applicable"
		observation.Reason = "interpreter does not emit native code"
		return nil, []protocol.Observation{observation}, nil
	}
	directory, err := os.MkdirTemp("", "wasmbench-wazevo-code-")
	if err != nil {
		return nil, nil, err
	}
	defer os.RemoveAll(directory)
	cache, err := wazero.NewCompilationCacheWithDir(directory)
	if err != nil {
		return nil, nil, err
	}
	defer cache.Close(ctx)
	engine := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigCompiler().WithCoreFeatures(api.CoreFeaturesV2).WithCloseOnContextDone(true).WithCompilationCache(cache))
	defer engine.Close(ctx)
	compiled, err := engine.CompileModule(ctx, a.wasm)
	if err != nil {
		return nil, nil, err
	}
	defer compiled.Close(ctx)
	var files []string
	if err := filepath.WalkDir(directory, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		return nil, nil, err
	}
	if len(files) != 1 {
		return nil, nil, fmt.Errorf("expected one isolated Wazevo module cache file, got %d", len(files))
	}
	file, err := os.Open(files[0])
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()
	code, size, err := readWazevoSegment(file)
	if err != nil {
		return nil, nil, err
	}
	observation.Status = "available"
	observation.Value = protocol.Value(float64(size))
	diagnostics := []protocol.Observation{observation}
	if size > protocol.MaxCodeImageBytes {
		diagnostics = append(diagnostics, protocol.Observation{Metric: "native.code_export", DefinitionVersion: 1, Unit: "bytes", Scope: "compiled_module", Collector: observation.Collector, CollectorVersion: observation.CollectorVersion, Profile: "code", Quality: "engine_reported", Status: "unavailable", Reason: "native segment exceeds transport budget; no partial export", Denominator: "module"})
		return nil, diagnostics, nil
	}
	image := &protocol.CodeImage{Version: 1, ModuleSHA256: a.prep.ArtifactSHA256, SHA256: corpus.Hash(code), Architecture: runtime.GOARCH, Backend: "wazevo", Format: "raw-native-image", SectionKind: "mixed_code_and_embedded_data", Event: "compiled_snapshot", FunctionAttribution: "unavailable", Data: code}
	return image, diagnostics, image.Validate(a.prep.ArtifactSHA256)
}
