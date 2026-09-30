package publish

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

func TestNativeExpansionUsesFullIndicesAndRetainsCoverage(t *testing.T) {
	image := &protocol.CodeImage{Version: 2, ModuleSHA256: "digest", Functions: []protocol.CodeFunction{{WasmIndex: 7, Length: 10, Tier: "winch"}, {WasmIndex: 8, Offset: 16, Length: 20, Tier: "winch"}}}
	sources := map[string]nativeBodySource{"digest": {path: "validation/digest.json", version: "core-structure-v3", bodies: map[uint32]uint64{7: 2, 8: 8, 9: 4}}}
	x, err := nativeExpansion(image, 40, sources)
	if err != nil {
		t.Fatal(err)
	}
	if x.Status != "available" || *x.DefinedWasmFunctions != 3 || *x.UnattributedImageBytes != 10 || len(x.Tiers) != 1 || x.Tiers[0].Functions != 2 || x.Tiers[0].RangeBytes != 30 {
		t.Fatal(x)
	}
	if *x.Functions[0].WasmBodyBytes != 2 || *x.Functions[0].RangeBodyRatio != 5 || *x.Functions[1].RangeBodyRatio != 2.5 {
		t.Fatal("joined defined ordinal instead of full index", x)
	}
	page, err := renderCodeHTML(NativeExport{Records: []NativeExportRecord{{Image: image, Expansion: x}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{"2 B · 5.00×", "8 B · 2.50×", "10 image bytes unclassified", "3 defined functions", "raw/validation/digest.json"} {
		if !strings.Contains(string(page), part) {
			t.Fatalf("missing %s", part)
		}
	}
	missing, err := nativeExpansion(image, 40, nil)
	if err != nil {
		t.Fatal(err)
	}
	if missing.Status != "not_recorded" || missing.Functions[0].RangeBodyRatio != nil || missing.DefinedWasmFunctions != nil || missing.Tiers[0].RangeBytes != 30 {
		t.Fatal("missing body evidence was fabricated", missing)
	}
	image.Functions[0].WasmIndex = 0
	if _, err = nativeExpansion(image, 40, sources); err == nil {
		t.Fatal("import/unknown function accepted")
	}
	if _, err = nativeExpansion(image, 2, nil); err == nil {
		t.Fatal("coverage outside image accepted")
	}
	image.Version = 1
	legacy, err := nativeExpansion(image, 40, nil)
	if err != nil || legacy.Status != "unsupported" || legacy.UnattributedImageBytes != nil {
		t.Fatal("legacy attribution fabricated", legacy, err)
	}
}

func TestNativeBodySourcesRequireUnambiguousEvidence(t *testing.T) {
	digest := strings.Repeat("a", 64)
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "validation"), 0700); err != nil {
		t.Fatal(err)
	}
	path := "validation/" + digest + ".json"
	b := experiment.Bundle{Admission: []experiment.ArtifactAdmission{{SHA256: digest, Status: "validated", ReportPath: path}}}
	base := `{"analysis_version":"core-structure-v3","encoding":"core-module","validated":true,"sha256":"` + digest + `","imported_functions":7,"defined_functions":2,"functions":[{"function_index":7,"body_bytes":2},{"function_index":8,"body_bytes":8}]}`
	for _, mode := range []string{"valid", "missing_index", "duplicate", "zero_body", "identity", "coverage", "imports", "legacy"} {
		t.Run(mode, func(t *testing.T) {
			data := base
			switch mode {
			case "missing_index":
				data = strings.Replace(data, `"function_index":7,`, "", 1)
			case "duplicate":
				data = strings.Replace(data, `"function_index":8`, `"function_index":7`, 1)
			case "zero_body":
				data = strings.Replace(data, `"body_bytes":2`, `"body_bytes":0`, 1)
			case "identity":
				data = strings.Replace(data, digest, strings.Repeat("b", 64), 1)
			case "coverage":
				data = strings.Replace(data, `"defined_functions":2`, `"defined_functions":3`, 1)
			case "imports":
				data = strings.Replace(data, `"imported_functions":7`, `"imported_functions":0`, 1)
			case "legacy":
				data = strings.Replace(data, "core-structure-v3", "core-structure-v1", 1)
			}
			if err := os.WriteFile(filepath.Join(root, path), []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			sources, err := nativeBodySources(root, b)
			if mode == "valid" {
				if err != nil || sources[digest].bodies[8] != 8 {
					t.Fatal(sources, err)
				}
			} else if mode == "legacy" {
				if err != nil || len(sources) != 0 {
					t.Fatal("legacy ambiguity accepted", sources, err)
				}
			} else if err == nil {
				t.Fatal("malformed body evidence accepted")
			}
		})
	}
}

func TestLiveNativeExpansionRejectsResealedRatioForgery(t *testing.T) {
	root := os.Getenv("WASMBENCH_NATIVE_EXPANSION_RUN")
	if root == "" {
		t.Skip("set WASMBENCH_NATIVE_EXPANSION_RUN to a sealed attributed code run")
	}
	out := filepath.Join(t.TempDir(), "report")
	if err := ExportNativeCode(root, out); err != nil {
		t.Fatal(err)
	}
	var r NativeExport
	if err := experiment.ReadJSON(filepath.Join(out, "native-code.json"), &r); err != nil {
		t.Fatal(err)
	}
	found := false
	for i := range r.Records {
		x := r.Records[i].Expansion
		if x != nil && x.Status == "available" && len(x.Functions) > 0 {
			ratio := *x.Functions[0].RangeBodyRatio + 1
			x.Functions[0].RangeBodyRatio = &ratio
			found = true
			break
		}
	}
	if !found {
		t.Fatal("live source did not establish any body expansion")
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(out, "native-code.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(filepath.Join(out, "checksums.json")); err != nil {
		t.Fatal(err)
	}
	if err = experiment.Seal(out); err != nil {
		t.Fatal(err)
	}
	if VerifyNativeCode(out) == nil {
		t.Fatal("resealed ratio forgery accepted")
	}
}
