package publish

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

func TestReportRejectsOutputInsideEitherInput(t *testing.T) {
	timing, _ := aggregateBundle(t, false)
	memory, _ := aggregateBundle(t, false)
	alias := filepath.Join(t.TempDir(), "timing-alias")
	if err := os.Symlink(timing, alias); err != nil {
		t.Fatal(err)
	}
	for name, args := range map[string][2]string{
		"timing bundle": {"", filepath.Join(timing, "nested", "report")},
		"timing alias":  {"", filepath.Join(alias, "nested", "report")},
		"memory bundle": {memory, filepath.Join(memory, "nested", "report")},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ReportWithMemory(timing, args[0], args[1]); err == nil || !strings.Contains(err.Error(), "inside an input bundle") {
				t.Fatalf("unsafe output accepted or misreported: %v", err)
			}
			if _, err := os.Stat(args[1]); !os.IsNotExist(err) {
				t.Fatalf("unsafe output was created: %v", err)
			}
		})
	}
	for _, root := range []string{timing, memory} {
		if err := experiment.Verify(root); err != nil {
			t.Fatalf("input evidence modified: %v", err)
		}
	}
}

func TestReportWithCodePassVerifiesCopiedNativeEvidence(t *testing.T) {
	timing, _ := aggregateBundle(t, false)
	b, err := experiment.Load(timing)
	if err != nil {
		t.Fatal(err)
	}
	code := filepath.Join(t.TempDir(), "code")
	if err := os.MkdirAll(filepath.Join(code, "trials"), 0755); err != nil {
		t.Fatal(err)
	}
	m := b.Manifest
	m.ID = "matched-code-fixture"
	m.Lock.Options.Profile = "code"
	if err := experiment.WriteJSON(filepath.Join(code, "manifest.json"), m); err != nil {
		t.Fatal(err)
	}
	trial := experiment.Trial{ID: "code-1", Runtime: "baseline", Workload: "required", Scenario: "compile", Profile: "code", Block: 0, Status: "ok", Observations: []protocol.Observation{{Metric: "native.guest_code", Status: "unsupported", Reason: "no public export"}}}
	if err := experiment.WriteJSON(filepath.Join(code, "trials", "code-1.json"), trial); err != nil {
		t.Fatal(err)
	}
	if err := experiment.Seal(code); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "report")
	if err := ReportWithPasses(timing, "", code, out); err != nil {
		t.Fatal(err)
	}
	if err := VerifyReport(out); err != nil {
		t.Fatal(err)
	}
	var d Dataset
	if err := experiment.ReadJSON(filepath.Join(out, "data.json"), &d); err != nil {
		t.Fatal(err)
	}
	if d.CodeSource == nil || d.CodeSource.ID != m.ID || len(d.CodeRecords) != 1 || d.CodeRecords[0].Status != "unsupported" || d.CodeRecords[0].ImageBytes != nil || d.CodeRecords[0].Index != 0 {
		t.Fatal(d.CodeSource, d.CodeRecords)
	}
	html, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil || !strings.Contains(string(html), "Generated native output · separate code pass") {
		t.Fatal("code drilldown missing from report page", err)
	}
	if err := ReportWithPasses(timing, "", code, filepath.Join(code, "nested")); err == nil {
		t.Fatal("report output inside code input accepted")
	}
}

func TestReportSealAndRecomputedDataset(t *testing.T) {
	run, _ := aggregateBundle(t, false)
	out := filepath.Join(t.TempDir(), "report")
	if err := Report(run, out); err != nil {
		t.Fatal(err)
	}
	if err := VerifyReport(out); err != nil {
		t.Fatalf("newly generated report failed verification: %v", err)
	}
	var data Dataset
	if err := experiment.ReadJSON(filepath.Join(out, "data.json"), &data); err != nil {
		t.Fatal(err)
	}
	originalVersion := data.AnalysisVersion
	data.AnalysisVersion = "forged-analysis"
	if err := os.Remove(filepath.Join(out, "data.json")); err != nil {
		t.Fatal(err)
	}
	if err := experiment.WriteJSON(filepath.Join(out, "data.json"), data); err != nil {
		t.Fatal(err)
	}
	if err := VerifyReport(out); err == nil {
		t.Fatal("tampered report seal accepted")
	}
	if err := os.Remove(filepath.Join(out, "checksums.json")); err != nil {
		t.Fatal(err)
	}
	if err := experiment.Seal(out); err != nil {
		t.Fatal(err)
	}
	if err := VerifyReport(out); err == nil || !strings.Contains(err.Error(), "dataset differs") {
		t.Fatalf("resealed forged analysis accepted: %v", err)
	}
	// Restore the valid dataset, then forge only the rendered page and reseal.
	data.AnalysisVersion = originalVersion
	if err := os.Remove(filepath.Join(out, "data.json")); err != nil {
		t.Fatal(err)
	}
	if err := experiment.WriteJSON(filepath.Join(out, "data.json"), data); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(out, "index.html")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "index.html"), []byte("forged page"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(out, "checksums.json")); err != nil {
		t.Fatal(err)
	}
	if err := experiment.Seal(out); err != nil {
		t.Fatal(err)
	}
	if err := VerifyReport(out); err == nil || !strings.Contains(err.Error(), "page differs") {
		t.Fatalf("resealed forged page accepted: %v", err)
	}
}
