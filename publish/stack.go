package publish

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"reflect"

	"github.com/wasmbench/wasmbench/analysis"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/sourcebuild"
)

//go:embed stack.html
var stackAssets embed.FS

type stackEvidence struct {
	BaselineBuildHash  string `json:"baseline_build_checksums_sha256"`
	CandidateBuildHash string `json:"candidate_build_checksums_sha256"`
	BaselineRunHash    string `json:"baseline_run_checksums_sha256"`
	CandidateRunHash   string `json:"candidate_run_checksums_sha256"`
}

type stackPage struct {
	Comparison analysis.StackComparison `json:"comparison"`
	Evidence   stackEvidence            `json:"evidence"`
}

func stackEvidenceFromPaths(baseBuild, candidateBuild, baseRun, candidateRun string) (stackEvidence, error) {
	paths := []string{baseBuild, candidateBuild, baseRun, candidateRun}
	values := make([]string, 0, len(paths))
	for _, root := range paths {
		hash, err := experiment.DigestFile(filepath.Join(root, "checksums.json"))
		if err != nil {
			return stackEvidence{}, err
		}
		values = append(values, hash)
	}
	return stackEvidence{values[0], values[1], values[2], values[3]}, nil
}

func renderStackPage(p stackPage) ([]byte, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	t, err := template.ParseFS(stackAssets, "stack.html")
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := t.Execute(&out, struct{ Data template.JS }{template.JS(data)}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func copyStackInput(src, dst string, verify func(string) error) error {
	if err := os.CopyFS(dst, os.DirFS(src)); err != nil {
		return err
	}
	if err := verify(dst); err != nil {
		return err
	}
	want, err := experiment.DigestFile(filepath.Join(src, "checksums.json"))
	if err != nil {
		return err
	}
	got, err := experiment.DigestFile(filepath.Join(dst, "checksums.json"))
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("stack evidence changed while copying %s", src)
	}
	return nil
}

// StackReport packages exact source-build and runtime-run evidence for an
// offline comparison. It does not execute either trusted compiler or adapter.
func StackReport(baseBuild, candidateBuild, baseRun, candidateRun, baseRuntime, candidateRuntime, out string) error {
	inputs := []string{baseBuild, candidateBuild, baseRun, candidateRun}
	for _, input := range inputs {
		if input == "" {
			return fmt.Errorf("stack report requires two builds and two runtime runs")
		}
	}
	if err := reportOutsideInputs(inputs, out); err != nil {
		return err
	}
	ab, err := sourcebuild.Verify(baseBuild)
	if err != nil {
		return err
	}
	bb, err := sourcebuild.Verify(candidateBuild)
	if err != nil {
		return err
	}
	a, err := experiment.Load(baseRun)
	if err != nil {
		return err
	}
	b, err := experiment.Load(candidateRun)
	if err != nil {
		return err
	}
	comparison, err := analysis.CompareStackRuns(a, b, ab, bb, baseRuntime, candidateRuntime)
	if err != nil {
		return err
	}
	evidence, err := stackEvidenceFromPaths(baseBuild, candidateBuild, baseRun, candidateRun)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		return err
	}
	if err := os.Mkdir(out, 0755); err != nil {
		return err
	}
	for _, spec := range []struct {
		src, dst string
		verify   func(string) error
	}{
		{baseBuild, "builds/baseline", func(p string) error { _, e := sourcebuild.Verify(p); return e }},
		{candidateBuild, "builds/candidate", func(p string) error { _, e := sourcebuild.Verify(p); return e }},
		{baseRun, "raw/baseline", func(p string) error { _, e := experiment.Load(p); return e }},
		{candidateRun, "raw/candidate", func(p string) error { _, e := experiment.Load(p); return e }},
	} {
		if err := copyStackInput(spec.src, filepath.Join(out, spec.dst), spec.verify); err != nil {
			return err
		}
	}
	page := stackPage{Comparison: comparison, Evidence: evidence}
	if err := experiment.WriteJSON(filepath.Join(out, "data.json"), page); err != nil {
		return err
	}
	html, err := renderStackPage(page)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, "index.html"), html, 0644); err != nil {
		return err
	}
	return sealFamilyReport(out, "stack")
}

// VerifyStackReport re-derives all analysis from copied sealed bundles and
// requires the HTML to match the current renderer byte-for-byte.
func VerifyStackReport(root string) error {
	if found, err := verifyArchivedFamily(root, "stack"); found {
		return err
	}
	if err := experiment.Verify(root); err != nil {
		return err
	}
	var saved stackPage
	if err := experiment.ReadJSON(filepath.Join(root, "data.json"), &saved); err != nil {
		return err
	}
	baseBuild, err := sourcebuild.Verify(filepath.Join(root, "builds/baseline"))
	if err != nil {
		return err
	}
	candidateBuild, err := sourcebuild.Verify(filepath.Join(root, "builds/candidate"))
	if err != nil {
		return err
	}
	baseRun, err := experiment.Load(filepath.Join(root, "raw/baseline"))
	if err != nil {
		return err
	}
	candidateRun, err := experiment.Load(filepath.Join(root, "raw/candidate"))
	if err != nil {
		return err
	}
	baselineID := saved.Comparison.RuntimePerformance.BaselineConfiguration.ID
	candidateID := saved.Comparison.RuntimePerformance.CandidateConfiguration.ID
	comparison, err := analysis.CompareStackRuns(baseRun, candidateRun, baseBuild, candidateBuild, baselineID, candidateID)
	if err != nil {
		return err
	}
	evidence, err := stackEvidenceFromPaths(filepath.Join(root, "builds/baseline"), filepath.Join(root, "builds/candidate"), filepath.Join(root, "raw/baseline"), filepath.Join(root, "raw/candidate"))
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(saved, stackPage{Comparison: comparison, Evidence: evidence}) {
		return fmt.Errorf("stack report differs from verified evidence")
	}
	want, err := renderStackPage(saved)
	if err != nil {
		return err
	}
	got, err := os.ReadFile(filepath.Join(root, "index.html"))
	if err != nil {
		return err
	}
	if !bytes.Equal(got, want) {
		return fmt.Errorf("stack report page differs from dataset and renderer")
	}
	return nil
}
