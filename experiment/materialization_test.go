package experiment

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wasmbench/wasmbench/protocol"
)

func TestMaterializationRequiresIndependentInputCoverage(t *testing.T) {
	sha := strings.Repeat("a", 64)
	m := &protocol.CompileMaterialization{ImportedFunctions: 1, DefinedFunctions: 2}
	for _, mode := range []string{"valid", "missing", "imports", "defined", "identity", "version", "encoding", "validation", "absent-count"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Mkdir(filepath.Join(root, "validation"), 0755); err != nil {
				t.Fatal(err)
			}
			source := map[string]any{"sha256": sha, "analysis_version": "core-structure-v3", "encoding": "core-module", "validated": true, "imported_functions": 1, "defined_functions": 2}
			switch mode {
			case "imports":
				source["imported_functions"] = 0
			case "defined":
				source["defined_functions"] = 3
			case "identity":
				source["sha256"] = strings.Repeat("b", 64)
			case "version":
				source["analysis_version"] = "old"
			case "encoding":
				source["encoding"] = "component"
			case "validation":
				source["validated"] = false
			case "absent-count":
				delete(source, "defined_functions")
			}
			if mode != "missing" {
				if err := WriteJSON(filepath.Join(root, "validation", sha+".json"), source); err != nil {
					t.Fatal(err)
				}
			}
			err := validateMaterializationInput(root, sha, m)
			if (err == nil) != (mode == "valid") {
				t.Fatalf("unexpected coverage result: %v", err)
			}
		})
	}
	if validateMaterializationInput(t.TempDir(), "../escape", m) == nil {
		t.Fatal("invalid input path accepted")
	}
}
