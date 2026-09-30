package publish

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wasmbench/wasmbench/experiment"
)

func TestFamilyBuildersRecomputePagesAndPreserveArchives(t *testing.T) {
	a, set := aggregateBundle(t, false)
	b := filepath.Join(t.TempDir(), "candidate")
	if err := os.CopyFS(b, os.DirFS(a)); err != nil {
		t.Fatal(err)
	}
	bundle, err := experiment.Load(b)
	if err != nil {
		t.Fatal(err)
	}
	bundle.Manifest.ID = "candidate-history"
	if err := os.Remove(filepath.Join(b, "manifest.json")); err != nil {
		t.Fatal(err)
	}
	if err := experiment.WriteJSON(filepath.Join(b, "manifest.json"), bundle.Manifest); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(b, "checksums.json")); err != nil {
		t.Fatal(err)
	}
	if err := experiment.Seal(b); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"comparison", "history", "aggregate"} {
		t.Run(kind, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "report")
			var err error
			switch kind {
			case "comparison":
				err = CompareReport(a, b, "baseline", "candidate", out)
			case "history":
				err = HistoryReport([]string{a, b}, "baseline", out)
			case "aggregate":
				err = AggregateReport(a, set, "baseline", "candidate", out)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := VerifyAnyReport(out); err != nil {
				t.Fatal(err)
			}
			r, err := loadAnyBuilderReceipt(out)
			if err != nil || r.Kind != kind || r.Version != FamilyBuilderVersion {
				t.Fatal(r, err)
			}
			called := false
			if err := recordedReportBuilder(context.Background(), out, []string{"verify-report", "--dir", out}, func(_ context.Context, path string, args []string) error {
				called = true
				digest, _ := experiment.DigestFile(path)
				if digest != r.SHA256 {
					t.Fatal("changed staged builder")
				}
				return nil
			}); err != nil || !called {
				t.Fatal(err)
			}
			// Even changing the page, updating its receipt and resealing cannot turn
			// unrelated HTML into the renderer's exact output from the saved inputs.
			if err := os.WriteFile(filepath.Join(out, "index.html"), []byte("<h1>Forged graph</h1>"), 0644); err != nil {
				t.Fatal(err)
			}
			r.PageSHA256, _ = experiment.DigestFile(filepath.Join(out, "index.html"))
			if err := os.Remove(filepath.Join(out, "builder.json")); err != nil {
				t.Fatal(err)
			}
			if err := experiment.WriteJSON(filepath.Join(out, "builder.json"), r); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(filepath.Join(out, "checksums.json")); err != nil {
				t.Fatal(err)
			}
			if err := experiment.Seal(out); err != nil {
				t.Fatal(err)
			}
			if err := VerifyAnyReport(out); err == nil || !strings.Contains(err.Error(), "index.html differs") {
				t.Fatal("accepted forged page", err)
			}
		})
	}
}

func TestFamilyBuilderRejectsUnknownKindsAndUnsafeLocators(t *testing.T) {
	for _, kind := range []string{"../outside", "arbitrary-command", ""} {
		if _, err := familyRenderer(kind); err == nil {
			t.Fatal("accepted kind", kind)
		}
	}
	root := t.TempDir()
	if err := experiment.WriteJSON(filepath.Join(root, "data.json"), map[string]any{"runtime": "r", "evidence": []map[string]string{{"bundle_locator": "../../outside"}}}); err != nil {
		t.Fatal(err)
	}
	if err := regenerateFamilyReport(root, filepath.Join(t.TempDir(), "out"), "history"); err == nil || !strings.Contains(err.Error(), "invalid history evidence") {
		t.Fatal(err)
	}
}

func TestAggregateSetExportHonorsOutputAndProtectsEvidence(t *testing.T) {
	root, _ := aggregateBundle(t, false)
	out := filepath.Join(t.TempDir(), "nested", "set.json")
	if err := ExportAggregateSet(root, "set-v1", "compile", out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatal(err)
	}
	if err := ExportAggregateSet(root, "set-v1", "compile", out); err == nil {
		t.Fatal("overwrote set")
	}
	if err := ExportAggregateSet(root, "set-v1", "compile", filepath.Join(root, "new-set.json")); err == nil {
		t.Fatal("wrote into sealed input")
	}
	if err := experiment.Verify(root); err != nil {
		t.Fatal(err)
	}
}
