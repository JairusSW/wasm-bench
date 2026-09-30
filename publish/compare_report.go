package publish

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"reflect"

	"github.com/wasmbench/wasmbench/analysis"
	"github.com/wasmbench/wasmbench/experiment"
)

//go:embed compare_report.html
var compareReportAssets embed.FS

type comparisonEvidence struct {
	BaselineRun   string `json:"baseline_run"`
	CandidateRun  string `json:"candidate_run"`
	BaselineHash  string `json:"baseline_checksums_sha256"`
	CandidateHash string `json:"candidate_checksums_sha256"`
}

type comparisonPage struct {
	Comparison analysis.RegressionReport `json:"comparison"`
	Evidence   comparisonEvidence        `json:"evidence"`
}

// CompareReport produces a self-contained comparison site from verified run
// bundles. The analysis is recomputed and all underlying evidence is copied.
func CompareReport(baselineRun, candidateRun, baselineRuntime, candidateRuntime, out string) error {
	if baselineRun == "" || candidateRun == "" || out == "" {
		return fmt.Errorf("comparison report requires two runs and an output directory")
	}
	if err := reportOutsideInputs([]string{baselineRun, candidateRun}, out); err != nil {
		return err
	}
	baseline, err := experiment.Load(baselineRun)
	if err != nil {
		return fmt.Errorf("verify baseline run: %w", err)
	}
	candidate, err := experiment.Load(candidateRun)
	if err != nil {
		return fmt.Errorf("verify candidate run: %w", err)
	}
	comparison, err := analysis.CompareRuns(baseline, candidate, baselineRuntime, candidateRuntime)
	if err != nil {
		return err
	}
	baselineHash, err := experiment.DigestFile(filepath.Join(baselineRun, "checksums.json"))
	if err != nil {
		return err
	}
	candidateHash, err := experiment.DigestFile(filepath.Join(candidateRun, "checksums.json"))
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		return err
	}
	if err = os.Mkdir(out, 0755); err != nil {
		return err
	}
	copyRun := func(src, side, want string) error {
		dst := filepath.Join(out, "raw", side)
		if err := os.CopyFS(dst, os.DirFS(src)); err != nil {
			return err
		}
		if _, err := experiment.Load(dst); err != nil {
			return err
		}
		got, err := experiment.DigestFile(filepath.Join(dst, "checksums.json"))
		if err != nil {
			return err
		}
		if got != want {
			return fmt.Errorf("%s run evidence changed while copying", side)
		}
		return nil
	}
	if err = copyRun(baselineRun, "baseline", baselineHash); err != nil {
		return err
	}
	if err = copyRun(candidateRun, "candidate", candidateHash); err != nil {
		return err
	}
	page := comparisonPage{Comparison: comparison, Evidence: comparisonEvidence{BaselineRun: baseline.Manifest.ID, CandidateRun: candidate.Manifest.ID, BaselineHash: baselineHash, CandidateHash: candidateHash}}
	if err = experiment.WriteJSON(filepath.Join(out, "data.json"), page); err != nil {
		return err
	}
	data, err := json.Marshal(page)
	if err != nil {
		return err
	}
	t, err := template.ParseFS(compareReportAssets, "compare_report.html")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(out, "index.html"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	err = t.Execute(f, struct{ Data template.JS }{template.JS(data)})
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return sealFamilyReport(out, "comparison")
}

// VerifyCompareReport checks the outer seal, both copied run bundles, and that
// the displayed regression analysis is exactly recomputable from those runs.
func VerifyCompareReport(root string) error {
	if found, err := verifyArchivedFamily(root, "comparison"); found {
		return err
	}
	if err := experiment.Verify(root); err != nil {
		return err
	}
	var saved comparisonPage
	if err := experiment.ReadJSON(filepath.Join(root, "data.json"), &saved); err != nil {
		return err
	}
	baseline, err := experiment.Load(filepath.Join(root, "raw/baseline"))
	if err != nil {
		return err
	}
	candidate, err := experiment.Load(filepath.Join(root, "raw/candidate"))
	if err != nil {
		return err
	}
	comparison, err := analysis.CompareRuns(baseline, candidate, saved.Comparison.BaselineConfiguration.ID, saved.Comparison.CandidateConfiguration.ID)
	if err != nil {
		return err
	}
	baselineHash, err := experiment.DigestFile(filepath.Join(root, "raw/baseline/checksums.json"))
	if err != nil {
		return err
	}
	candidateHash, err := experiment.DigestFile(filepath.Join(root, "raw/candidate/checksums.json"))
	if err != nil {
		return err
	}
	want := comparisonPage{Comparison: comparison, Evidence: comparisonEvidence{BaselineRun: baseline.Manifest.ID, CandidateRun: candidate.Manifest.ID, BaselineHash: baselineHash, CandidateHash: candidateHash}}
	if !reflect.DeepEqual(saved, want) {
		return fmt.Errorf("comparison report analysis does not match its verified inputs")
	}
	return nil
}
