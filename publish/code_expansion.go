package publish

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

const NativeExpansionVersion = "native-function-expansion-v1"

type NativeFunctionExpansion struct {
	Function       protocol.CodeFunction `json:"function"`
	WasmBodyBytes  *uint64               `json:"wasm_body_bytes"`
	RangeBodyRatio *float64              `json:"range_body_ratio"`
}

type NativeTierCoverage struct {
	Tier       string `json:"tier"`
	Generation uint32 `json:"generation"`
	Functions  uint64 `json:"functions"`
	RangeBytes uint64 `json:"range_bytes"`
}

type NativeExpansion struct {
	Version               string                    `json:"version"`
	Status                string                    `json:"status"`
	Reason                string                    `json:"reason,omitempty"`
	AnalysisPath          string                    `json:"analysis_path,omitempty"`
	SourceAnalysisVersion string                    `json:"source_analysis_version,omitempty"`
	DefinedWasmFunctions  *uint64                   `json:"defined_wasm_functions"`
	Functions             []NativeFunctionExpansion `json:"functions"`
	Tiers                 []NativeTierCoverage      `json:"tiers"`
	// This remainder is not classified as trampolines, metadata or capacity.
	UnattributedImageBytes *uint64 `json:"unattributed_image_bytes"`
}

type nativeBodySource struct {
	path, version string
	bodies        map[uint32]uint64
}

// Consume only sealed independent analyzer evidence. No runtime executable or
// current artifact path is required; function indices include imported functions.
func nativeBodySources(root string, b experiment.Bundle) (map[string]nativeBodySource, error) {
	sources := map[string]nativeBodySource{}
	for _, a := range b.Admission {
		if a.Status != "validated" || a.ReportPath == "" {
			continue
		}
		if a.ReportPath != "validation/"+a.SHA256+".json" {
			return nil, fmt.Errorf("unexpected native expansion analyzer path")
		}
		data, err := os.ReadFile(filepath.Join(root, a.ReportPath))
		if err != nil {
			return nil, err
		}
		var parsed struct {
			SHA256    string  `json:"sha256"`
			Validated bool    `json:"validated"`
			Version   string  `json:"analysis_version"`
			Encoding  string  `json:"encoding"`
			Imported  *uint32 `json:"imported_functions"`
			Defined   *uint32 `json:"defined_functions"`
			Functions []struct {
				Index *uint32 `json:"function_index"`
				Bytes uint64  `json:"body_bytes"`
			} `json:"functions"`
		}
		if err = json.Unmarshal(data, &parsed); err != nil {
			return nil, err
		}
		if parsed.Version != "core-structure-v3" || parsed.Encoding != "core-module" {
			continue
		}
		if parsed.SHA256 != a.SHA256 || !parsed.Validated || parsed.Imported == nil || parsed.Defined == nil || uint64(len(parsed.Functions)) != uint64(*parsed.Defined) {
			return nil, fmt.Errorf("incomplete native expansion source identity or body coverage")
		}
		source := nativeBodySource{path: a.ReportPath, version: parsed.Version, bodies: map[uint32]uint64{}}
		for i, f := range parsed.Functions {
			if f.Index == nil || uint64(*f.Index) != uint64(*parsed.Imported)+uint64(i) || f.Bytes == 0 {
				return nil, fmt.Errorf("invalid native expansion body index or size")
			}
			source.bodies[*f.Index] = f.Bytes
		}
		sources[a.SHA256] = source
	}
	return sources, nil
}

func nativeExpansion(image *protocol.CodeImage, size uint64, sources map[string]nativeBodySource) (*NativeExpansion, error) {
	if image == nil {
		return nil, nil
	}
	x := &NativeExpansion{Version: NativeExpansionVersion, Status: "unsupported", Reason: "native image has no engine-reported function attribution", Functions: []NativeFunctionExpansion{}, Tiers: []NativeTierCoverage{}}
	if image.Version != 2 && image.Version != 3 {
		return x, nil
	}
	x.Status = "not_recorded"
	x.Reason = "independent function-body evidence with full Wasm indices is unavailable"
	source, matched := sources[image.ModuleSHA256]
	if matched {
		x.Status = "available"
		x.Reason = ""
		x.AnalysisPath = source.path
		x.SourceAnalysisVersion = source.version
		count := uint64(len(source.bodies))
		x.DefinedWasmFunctions = &count
	}
	tiers := map[string]*NativeTierCoverage{}
	var attributed uint64
	for _, f := range image.Functions {
		if attributed > size || f.Length > size-attributed {
			return nil, fmt.Errorf("native function coverage exceeds image size")
		}
		entry := NativeFunctionExpansion{Function: f}
		if matched {
			body, ok := source.bodies[f.WasmIndex]
			if !ok || body == 0 {
				return nil, fmt.Errorf("native function %d has no independently analyzed defined Wasm body", f.WasmIndex)
			}
			ratio := float64(f.Length) / float64(body)
			entry.WasmBodyBytes = &body
			entry.RangeBodyRatio = &ratio
		}
		x.Functions = append(x.Functions, entry)
		key := fmt.Sprintf("%s/%d", f.Tier, f.Generation)
		if tiers[key] == nil {
			tiers[key] = &NativeTierCoverage{Tier: f.Tier, Generation: f.Generation}
		}
		tiers[key].Functions++
		tiers[key].RangeBytes += f.Length
		attributed += f.Length
	}
	if attributed > size {
		return nil, fmt.Errorf("native function coverage exceeds image size")
	}
	remainder := size - attributed
	x.UnattributedImageBytes = &remainder
	keys := make([]string, 0, len(tiers))
	for key := range tiers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		x.Tiers = append(x.Tiers, *tiers[key])
	}
	return x, nil
}
