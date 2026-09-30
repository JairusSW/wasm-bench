package experiment

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
)

func TestLoadCodeImageEvidence(t *testing.T) {
	for _, mode := range []string{"valid", "corrupt", "wrong_module", "timing", "failed", "admission", "unknown_workload"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Mkdir(filepath.Join(root, "trials"), 0755); err != nil {
				t.Fatal(err)
			}
			data := []byte{1, 2, 3}
			digest := corpus.Hash(data)
			m := Manifest{Lock: Lock{Workloads: []protocol.Workload{{ID: "w", SHA256: digest}}}}
			trial := Trial{ID: "t", Workload: "w", Profile: "code", Status: "ok", CodeImage: &protocol.CodeImage{Version: 1, ModuleSHA256: digest, SHA256: digest, Architecture: "arm64", Backend: "railshot", Format: "raw-native-image", SectionKind: "mixed_code_and_embedded_data", Event: "compiled_snapshot", FunctionAttribution: "unavailable", Data: data}}
			switch mode {
			case "corrupt":
				trial.CodeImage.Data[0] = 9
			case "wrong_module":
				trial.CodeImage.ModuleSHA256 = corpus.Hash([]byte("other"))
			case "timing":
				trial.Profile = "timing"
			case "failed":
				trial.Status = "error"
			case "admission":
				trial.Block = -1
			case "unknown_workload":
				trial.Workload = "missing"
			}
			if err := WriteJSON(filepath.Join(root, "manifest.json"), m); err != nil {
				t.Fatal(err)
			}
			if err := WriteJSON(filepath.Join(root, "trials", "t.json"), trial); err != nil {
				t.Fatal(err)
			}
			if err := Seal(root); err != nil {
				t.Fatal(err)
			}
			b, err := Load(root)
			if mode == "valid" {
				if err != nil || len(b.Trials) != 1 || b.Trials[0].CodeImage == nil {
					t.Fatalf("%+v %v", b, err)
				}
			} else if err == nil {
				t.Fatal("accepted resealed invalid code evidence")
			}
		})
	}
}
