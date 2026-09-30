package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
)

// MaxCodeImageBytes leaves headroom for base64 and other fields in the 64 MiB
// control-message limit. Oversize images are unavailable, never truncated.
const MaxCodeImageBytes = 16 << 20

// CodeImage is a retained compile-time snapshot, not a lifetime emission log or
// a guest-instruction-only range. Data is base64 in JSON and sealed with trials.
type CodeImage struct {
	Materialization     *CompileMaterialization `json:"materialization,omitempty"`
	Version             int                     `json:"version"`
	ModuleSHA256        string                  `json:"module_sha256"`
	SHA256              string                  `json:"sha256"`
	Architecture        string                  `json:"architecture"`
	Backend             string                  `json:"backend"`
	Format              string                  `json:"format"`
	SectionKind         string                  `json:"section_kind"`
	Event               string                  `json:"event"`
	FunctionAttribution string                  `json:"function_attribution"`
	Data                []byte                  `json:"data,omitempty"`
	Functions           []CodeFunction          `json:"functions,omitempty"`
}

// CodeFunction identifies an engine-reported range; it is not an instruction
// count and may contain function-local padding or embedded constants.
type CodeFunction struct {
	ModuleIndex uint32 `json:"module_index"`
	WasmIndex   uint32 `json:"wasm_index"`
	Name        string `json:"name,omitempty"`
	Offset      uint64 `json:"offset"`
	Length      uint64 `json:"length"`
	Tier        string `json:"tier"`
	Generation  uint32 `json:"generation"`
}

func (c CodeImage) Validate(module string) error {
	digest, err := hex.DecodeString(module)
	if err != nil || len(digest) != sha256.Size || c.ModuleSHA256 != module {
		return fmt.Errorf("native code module identity mismatch")
	}
	legacy := c.Version == 1 && c.FunctionAttribution == "unavailable" && len(c.Functions) == 0
	attributed := (c.Version == 2 || c.Version == 3) && c.FunctionAttribution == "engine_reported"
	if (!legacy && !attributed) || c.Format != "raw-native-image" || c.SectionKind != "mixed_code_and_embedded_data" || c.Event != "compiled_snapshot" || c.Backend == "" {
		return fmt.Errorf("unsupported native code image contract")
	}
	if c.Architecture != "arm64" && c.Architecture != "amd64" {
		return fmt.Errorf("unsupported native code architecture %q", c.Architecture)
	}
	if len(c.Data) > MaxCodeImageBytes {
		return fmt.Errorf("native code image exceeds transport budget")
	}
	if c.Version == 3 {
		if c.Materialization == nil {
			return fmt.Errorf("materialized native image lacks completion evidence")
		}
		if err := c.Materialization.Validate(c); err != nil {
			return err
		}
	} else if c.Materialization != nil {
		return fmt.Errorf("legacy code image cannot declare materialization")
	}
	if attributed {
		if c.Backend != "cranelift" && c.Backend != "winch" {
			return fmt.Errorf("unsupported attributed backend")
		}
		seen := make(map[uint32]bool)
		ranges := append([]CodeFunction(nil), c.Functions...)
		for _, f := range ranges {
			if f.ModuleIndex != 0 || seen[f.WasmIndex] || f.Tier != c.Backend || f.Generation != 0 || f.Length == 0 || f.Offset > uint64(len(c.Data)) || f.Length > uint64(len(c.Data))-f.Offset {
				return fmt.Errorf("invalid native function range")
			}
			seen[f.WasmIndex] = true
		}
		sort.Slice(ranges, func(i, j int) bool { return ranges[i].Offset < ranges[j].Offset })
		for i := 1; i < len(ranges); i++ {
			if ranges[i].Offset < ranges[i-1].Offset+ranges[i-1].Length {
				return fmt.Errorf("overlapping native function ranges")
			}
		}
	}
	sum := sha256.Sum256(c.Data)
	if hex.EncodeToString(sum[:]) != c.SHA256 {
		return fmt.Errorf("native code image digest mismatch")
	}
	return nil
}
