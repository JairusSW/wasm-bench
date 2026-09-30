package publish

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"github.com/wasmbench/wasmbench/analysis"
	"github.com/wasmbench/wasmbench/experiment"
)

const FamilyBuilderVersion = "sealed-report-family-builder-v2"

func verifyArchivedFamily(root, kind string) (bool, error) {
	if _, err := os.Stat(filepath.Join(root, "builder.json")); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return true, err
	}
	if err := experiment.Verify(root); err != nil {
		return true, err
	}
	r, err := loadAnyBuilderReceipt(root)
	if err != nil {
		return true, err
	}
	if r.Kind != kind {
		return true, fmt.Errorf("expected %s report, found %s", kind, r.Kind)
	}
	return true, VerifyAnyReport(root)
}

func familyRenderer(kind string) ([]byte, error) {
	switch kind {
	case "comparison":
		return compareReportAssets.ReadFile("compare_report.html")
	case "history":
		return historyAssets.ReadFile("history.html")
	case "aggregate":
		return aggregateAssets.ReadFile("aggregate.html")
	case "source":
		return sourceAssets.ReadFile("source.html")
	case "source-set":
		return sourceSetAssets.ReadFile("source_set.html")
	case "stack":
		return stackAssets.ReadFile("stack.html")
	case "native-export", "native-disassembly":
		return codeAssets.ReadFile("code.html")
	case "native-comparison":
		return codeCompareAssets.ReadFile("code_compare.html")
	default:
		return nil, fmt.Errorf("unsupported report family %q", kind)
	}
}

// The payload name is selected by a fixed family allowlist, never by a path
// supplied in a receipt. Native exports predate the common data.json convention.
func familyDataPath(kind string) string {
	if kind == "native-export" || kind == "native-disassembly" {
		return "native-code.json"
	}
	return "data.json"
}

func sealFamilyReport(out, kind string) error {
	renderer, err := familyRenderer(kind)
	if err != nil {
		return err
	}
	page, err := experiment.DigestFile(filepath.Join(out, "index.html"))
	if err != nil {
		return err
	}
	if err := archiveBuilderReceipt(out, ReportBuilderReceipt{Version: FamilyBuilderVersion, Kind: kind, PageSHA256: page, OS: runtime.GOOS, Arch: runtime.GOARCH, AnalysisVersion: "report-family-regeneration-v1", RendererSHA256: fmt.Sprintf("%x", sha256.Sum256(renderer))}); err != nil {
		return err
	}
	return experiment.Seal(out)
}

func loadAnyBuilderReceipt(root string) (ReportBuilderReceipt, error) {
	var receipt ReportBuilderReceipt
	if err := experiment.ReadJSON(filepath.Join(root, "builder.json"), &receipt); err != nil {
		if !os.IsNotExist(err) {
			return receipt, err
		}
		var d Dataset
		if err := experiment.ReadJSON(filepath.Join(root, "data.json"), &d); err != nil {
			return receipt, err
		}
		return verifyReportBuilder(root, d)
	}
	if receipt.Version == ReportBuilderVersion {
		var d Dataset
		if err := experiment.ReadJSON(filepath.Join(root, "data.json"), &d); err != nil {
			return receipt, err
		}
		return verifyReportBuilder(root, d)
	}
	if receipt.Version != FamilyBuilderVersion {
		return receipt, fmt.Errorf("unsupported report builder archive version")
	}
	if _, err := familyRenderer(receipt.Kind); err != nil {
		return receipt, err
	}
	for name, want := range map[string]string{reportBuilderPath: receipt.SHA256, familyDataPath(receipt.Kind): receipt.DataSHA256, "index.html": receipt.PageSHA256} {
		got, err := experiment.DigestFile(filepath.Join(root, name))
		if err != nil {
			return receipt, err
		}
		if got != want {
			return receipt, fmt.Errorf("report builder receipt differs from %s", name)
		}
	}
	if receipt.OS == "" || receipt.Arch == "" || receipt.AnalysisVersion != "report-family-regeneration-v1" || receipt.RendererSHA256 == "" {
		return receipt, fmt.Errorf("incomplete report builder receipt")
	}
	return receipt, nil
}

// VerifyAnyReport never executes builders, compilers, adapters or benchmarks.
// Family regeneration uses sealed copied inputs and only produces temporary
// derived files; comparison includes rendered HTML, tables and raw evidence.
func VerifyAnyReport(root string) error {
	if err := experiment.Verify(root); err != nil {
		return err
	}
	receipt, err := loadAnyBuilderReceipt(root)
	if err != nil {
		return err
	}
	if receipt.Kind == "" {
		return VerifyReport(root)
	}
	renderer, err := familyRenderer(receipt.Kind)
	if err != nil {
		return err
	}
	if receipt.RendererSHA256 != fmt.Sprintf("%x", sha256.Sum256(renderer)) {
		return fmt.Errorf("report renderer differs from current build; trusted archives may use verify-report --recorded-builder")
	}
	temp, err := os.MkdirTemp("", "wasmbench-report-regeneration-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	out := filepath.Join(temp, "report")
	if err := regenerateFamilyReport(root, out, receipt.Kind); err != nil {
		return err
	}
	var actual, want map[string]string
	if err := experiment.ReadJSON(filepath.Join(root, "checksums.json"), &actual); err != nil {
		return err
	}
	if err := experiment.ReadJSON(filepath.Join(out, "checksums.json"), &want); err != nil {
		return err
	}
	// Only the top-level builder varies. Nested input archives remain evidence.
	for _, name := range []string{"builder.json", reportBuilderPath} {
		delete(actual, name)
		delete(want, name)
	}
	if len(actual) != len(want) {
		return fmt.Errorf("report file set differs from regenerated evidence")
	}
	for name, digest := range want {
		if actual[name] != digest {
			return fmt.Errorf("report file %s differs from regenerated evidence", name)
		}
	}
	return nil
}

func regenerateFamilyReport(root, out, kind string) error {
	read := func(value any) error { return experiment.ReadJSON(filepath.Join(root, "data.json"), value) }
	switch kind {
	case "comparison":
		var d comparisonPage
		if err := read(&d); err != nil {
			return err
		}
		return CompareReport(filepath.Join(root, "raw/baseline"), filepath.Join(root, "raw/candidate"), d.Comparison.BaselineConfiguration.ID, d.Comparison.CandidateConfiguration.ID, out)
	case "aggregate":
		var d aggregateDataset
		if err := read(&d); err != nil {
			return err
		}
		var set analysis.AggregateSet
		if err := experiment.ReadJSON(filepath.Join(root, "set.json"), &set); err != nil {
			return err
		}
		return AggregateReport(filepath.Join(root, "raw"), set, d.Analysis.Baseline.ID, d.Analysis.Candidate.ID, out)
	case "history":
		var d analysis.HistoryReport
		if err := read(&d); err != nil {
			return err
		}
		paths := []string{}
		for i, e := range d.Evidence {
			want := fmt.Sprintf("raw/%03d", i)
			if e.Bundle != want {
				return fmt.Errorf("invalid history evidence locator")
			}
			paths = append(paths, filepath.Join(root, want))
		}
		return HistoryReport(paths, d.Runtime, out)
	case "source":
		return SourceReport(filepath.Join(root, "raw"), out)
	case "stack":
		var d stackPage
		if err := read(&d); err != nil {
			return err
		}
		return StackReport(filepath.Join(root, "builds/baseline"), filepath.Join(root, "builds/candidate"), filepath.Join(root, "raw/baseline"), filepath.Join(root, "raw/candidate"), d.Comparison.RuntimePerformance.BaselineConfiguration.ID, d.Comparison.RuntimePerformance.CandidateConfiguration.ID, out)
	case "source-set":
		var d sourceSetPage
		if err := read(&d); err != nil {
			return err
		}
		paths := func(side string, links []sourceBuildLink) ([]string, error) {
			out := []string{}
			for i, l := range links {
				want := fmt.Sprintf("builds/%s/%03d", side, i)
				if l.Bundle != want {
					return nil, fmt.Errorf("invalid source build locator")
				}
				out = append(out, filepath.Join(root, want))
			}
			return out, nil
		}
		a, err := paths("baseline", d.BaselineBuildLinks)
		if err != nil {
			return err
		}
		b, err := paths("candidate", d.CandidateBuildLinks)
		if err != nil {
			return err
		}
		return SourceSetReport(a, b, filepath.Join(root, "raw/baseline"), filepath.Join(root, "raw/candidate"), d.Comparison.RuntimePerformance.BaselineConfiguration.ID, out)
	case "native-export":
		if err := VerifyNativeCode(root); err != nil {
			return err
		}
		return ExportNativeCode(filepath.Join(root, "raw"), out)
	case "native-disassembly":
		return regenerateNativeDisassembly(root, out)
	case "native-comparison":
		if err := VerifyNativeComparison(root); err != nil {
			return err
		}
		var d NativeCodeComparison
		if err := read(&d); err != nil {
			return err
		}
		return CompareNativeCode(filepath.Join(root, "evidence/baseline"), filepath.Join(root, "evidence/candidate"), d.BaselineConfiguration.ID, d.CandidateConfiguration.ID, out)
	default:
		return fs.ErrInvalid
	}
}
