package publish

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/wasmbench/wasmbench/experiment"
)

// ArtifactStructure is a compact, display-only projection of the sealed,
// independently validated analyzer output. It is never a runtime measurement.
type ArtifactStructure struct {
	SHA256            string        `json:"sha256"`
	Status            string        `json:"status"`
	Encoding          string        `json:"encoding,omitempty"`
	Bytes             uint64        `json:"bytes"`
	CodeSectionBytes  uint64        `json:"code_section_bytes"`
	DataBytes         uint64        `json:"data_bytes"`
	CustomDataBytes   uint64        `json:"custom_data_bytes"`
	DebugDataBytes    uint64        `json:"debug_data_bytes"`
	DefinedFunctions  uint64        `json:"defined_functions"`
	TotalFunctions    uint64        `json:"total_functions"`
	Imports           uint64        `json:"imports"`
	Exports           uint64        `json:"exports"`
	MaxControlDepth   uint64        `json:"max_control_depth"`
	BodyBytesP50      uint64        `json:"body_bytes_p50"`
	BodyBytesP95      uint64        `json:"body_bytes_p95"`
	BodyBytesMax      uint64        `json:"body_bytes_max"`
	ComponentNodes    uint64        `json:"component_nodes"`
	NestedCoreModules uint64        `json:"nested_core_modules"`
	TopOpcodes        []OpcodeCount `json:"top_opcodes,omitempty"`
	ReportPath        string        `json:"report_path,omitempty"`
}

type OpcodeCount struct {
	Name  string `json:"name"`
	Count uint64 `json:"count"`
}

type analyzerStructure struct {
	Encoding         string            `json:"encoding"`
	Bytes            uint64            `json:"bytes"`
	SectionBytes     map[string]uint64 `json:"section_payload_bytes"`
	DataBytes        uint64            `json:"data_bytes"`
	CustomDataBytes  uint64            `json:"custom_data_bytes"`
	DebugDataBytes   uint64            `json:"debug_data_bytes"`
	DefinedFunctions uint64            `json:"defined_functions"`
	TotalFunctions   uint64            `json:"total_functions"`
	ImportCount      uint64            `json:"import_count"`
	Exports          uint64            `json:"exports"`
	MaxControlDepth  uint64            `json:"max_control_depth"`
	Functions        []struct {
		BodyBytes uint64 `json:"body_bytes"`
	} `json:"functions"`
	Nodes []struct {
		Encoding string `json:"encoding"`
	} `json:"nodes"`
	Opcodes map[string]uint64 `json:"opcode_histogram"`
}

func artifactStructures(root string, admissions []experiment.ArtifactAdmission) ([]ArtifactStructure, error) {
	result := make([]ArtifactStructure, 0, len(admissions))
	for _, admission := range admissions {
		entry := ArtifactStructure{SHA256: admission.SHA256, Status: admission.Status}
		if admission.ReportPath == "" {
			result = append(result, entry)
			continue
		}
		// Load has already validated the digest and analyzer result. Retain its
		// constructed relative path rather than accepting an arbitrary path here.
		if admission.ReportPath != "validation/"+admission.SHA256+".json" {
			return nil, fmt.Errorf("unexpected analyzer evidence path %q", admission.ReportPath)
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(admission.ReportPath)))
		if err != nil {
			return nil, err
		}
		var parsed analyzerStructure
		if err := json.Unmarshal(data, &parsed); err != nil {
			return nil, fmt.Errorf("analyzer structure %s: %w", admission.SHA256, err)
		}
		entry.Encoding, entry.Bytes, entry.ReportPath = parsed.Encoding, parsed.Bytes, admission.ReportPath
		switch parsed.Encoding {
		case "core-module":
			entry.CodeSectionBytes = parsed.SectionBytes["10"]
			entry.DataBytes, entry.CustomDataBytes, entry.DebugDataBytes = parsed.DataBytes, parsed.CustomDataBytes, parsed.DebugDataBytes
			entry.DefinedFunctions, entry.TotalFunctions = parsed.DefinedFunctions, parsed.TotalFunctions
			entry.Imports, entry.Exports, entry.MaxControlDepth = parsed.ImportCount, parsed.Exports, parsed.MaxControlDepth
			if len(parsed.Functions) > 0 {
				bodies := make([]uint64, len(parsed.Functions))
				for i, function := range parsed.Functions {
					bodies[i] = function.BodyBytes
				}
				sort.Slice(bodies, func(i, j int) bool { return bodies[i] < bodies[j] })
				entry.BodyBytesP50 = bodies[(len(bodies)-1)/2]
				entry.BodyBytesP95 = bodies[(95*len(bodies)+99)/100-1]
				entry.BodyBytesMax = bodies[len(bodies)-1]
			}
			for name, count := range parsed.Opcodes {
				entry.TopOpcodes = append(entry.TopOpcodes, OpcodeCount{Name: name, Count: count})
			}
			sort.Slice(entry.TopOpcodes, func(i, j int) bool {
				if entry.TopOpcodes[i].Count == entry.TopOpcodes[j].Count {
					return entry.TopOpcodes[i].Name < entry.TopOpcodes[j].Name
				}
				return entry.TopOpcodes[i].Count > entry.TopOpcodes[j].Count
			})
			if len(entry.TopOpcodes) > 6 {
				entry.TopOpcodes = entry.TopOpcodes[:6]
			}
		case "component":
			entry.ComponentNodes = uint64(len(parsed.Nodes))
			for _, node := range parsed.Nodes {
				if node.Encoding == "core-module" {
					entry.NestedCoreModules++
				}
			}
		default:
			return nil, fmt.Errorf("unexpected analyzer encoding %q", parsed.Encoding)
		}
		result = append(result, entry)
	}
	return result, nil
}
