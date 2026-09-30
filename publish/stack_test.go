package publish

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wasmbench/wasmbench/experiment"
)

func TestStackTemplateEscapesEmbeddedSourceMetadata(t *testing.T) {
	page := stackPage{}
	page.Comparison.BaselineBuild.Lock.Recipe.ID = "</script><script>alert(1)</script>"
	markup, err := renderStackPage(page)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(markup, []byte("</script><script>alert")) || !bytes.Contains(markup, []byte(`\u003c/script\u003e`)) {
		t.Fatal("unsafe embedded stack metadata")
	}
}

func TestStackReportRejectsNestedOutput(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"baseline", "candidate", "run-a", "run-b"} {
		if err := os.Mkdir(filepath.Join(root, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(root, "baseline", "report")
	if err := StackReport(filepath.Join(root, "baseline"), filepath.Join(root, "candidate"), filepath.Join(root, "run-a"), filepath.Join(root, "run-b"), "a", "b", out); err == nil || !strings.Contains(err.Error(), "inside") {
		t.Fatal("nested report accepted", err)
	}
}

func TestStackReportLiveEvidenceAndResealedTamper(t *testing.T) {
	root := filepath.Join("..", "reports", "stack-wago-vs-wasmtime-v3")
	if _, err := os.Stat(filepath.Join(root, "checksums.json")); os.IsNotExist(err) {
		t.Skip("optional generated live evidence not present")
	}
	if err := VerifyStackReport(root); err != nil {
		t.Fatal(err)
	}
	copy := filepath.Join(t.TempDir(), "report")
	if err := os.CopyFS(copy, os.DirFS(root)); err != nil {
		t.Fatal(err)
	}
	var page stackPage
	if err := experiment.ReadJSON(filepath.Join(copy, "data.json"), &page); err != nil {
		t.Fatal(err)
	}
	page.Comparison.RuntimePerformance.Results[0].Status = "forged_status"
	if err := os.Remove(filepath.Join(copy, "data.json")); err != nil {
		t.Fatal(err)
	}
	if err := experiment.WriteJSON(filepath.Join(copy, "data.json"), page); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(copy, "checksums.json")); err != nil {
		t.Fatal(err)
	}
	if err := experiment.Seal(copy); err != nil {
		t.Fatal(err)
	}
	if err := VerifyStackReport(copy); err == nil {
		t.Fatal("resealed false comparison accepted")
	}
}
