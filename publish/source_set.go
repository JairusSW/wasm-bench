package publish

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/wasmbench/wasmbench/analysis"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/sourcebuild"
)

//go:embed source_set.html
var sourceSetAssets embed.FS

type sourceSetEvidence struct {
	BaselineRun   string `json:"baseline_run"`
	CandidateRun  string `json:"candidate_run"`
	BaselineHash  string `json:"baseline_checksums_sha256"`
	CandidateHash string `json:"candidate_checksums_sha256"`
}

type sourceSetPage struct {
	Comparison          analysis.SourceSetComparison `json:"comparison"`
	Evidence            sourceSetEvidence            `json:"evidence"`
	BaselineBuildLinks  []sourceBuildLink            `json:"baseline_build_links"`
	CandidateBuildLinks []sourceBuildLink            `json:"candidate_build_links"`
}

type sourceBuildLink struct {
	Workload string `json:"workload"`
	Bundle   string `json:"bundle"`
}

// SourceSetReport recomputes a fixed multi-workload comparison from sealed,
// verified build and runtime bundles, then packages those inputs for offline use.
func SourceSetReport(baselineBuildPaths, candidateBuildPaths []string, baselineRun, candidateRun, runtime, out string) error {
	inputs := append(append([]string{}, baselineBuildPaths...), candidateBuildPaths...)
	inputs = append(inputs, baselineRun, candidateRun)
	if len(baselineBuildPaths) == 0 || len(candidateBuildPaths) == 0 || baselineRun == "" || candidateRun == "" {
		return fmt.Errorf("source set report requires nonempty build sets and both runtime runs")
	}
	if err := reportOutsideInputs(inputs, out); err != nil {
		return err
	}
	loadBuilds := func(paths []string) ([]sourcebuild.Result, error) {
		builds := make([]sourcebuild.Result, 0, len(paths))
		for _, path := range paths {
			build, err := sourcebuild.Verify(path)
			if err != nil {
				return nil, err
			}
			builds = append(builds, build)
		}
		return builds, nil
	}
	baselineBuilds, err := loadBuilds(baselineBuildPaths)
	if err != nil {
		return fmt.Errorf("verify baseline source builds: %w", err)
	}
	candidateBuilds, err := loadBuilds(candidateBuildPaths)
	if err != nil {
		return fmt.Errorf("verify candidate source builds: %w", err)
	}
	baseline, err := experiment.Load(baselineRun)
	if err != nil {
		return fmt.Errorf("verify baseline runtime run: %w", err)
	}
	candidate, err := experiment.Load(candidateRun)
	if err != nil {
		return fmt.Errorf("verify candidate runtime run: %w", err)
	}
	comparison, err := analysis.CompareSourceRunSet(baseline, candidate, baselineBuilds, candidateBuilds, runtime)
	if err != nil {
		return err
	}
	baselineChecksums, err := experiment.DigestFile(filepath.Join(baselineRun, "checksums.json"))
	if err != nil {
		return err
	}
	candidateChecksums, err := experiment.DigestFile(filepath.Join(candidateRun, "checksums.json"))
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		return err
	}
	if err = os.Mkdir(out, 0755); err != nil {
		return err
	}
	copyVerified := func(src, dst, checksumPath string, verify func(string) error) error {
		if err := os.CopyFS(dst, os.DirFS(src)); err != nil {
			return err
		}
		if err := verify(dst); err != nil {
			return err
		}
		digest, err := experiment.DigestFile(filepath.Join(dst, checksumPath))
		if err != nil {
			return err
		}
		original, err := experiment.DigestFile(filepath.Join(src, checksumPath))
		if err != nil {
			return err
		}
		if digest != original {
			return fmt.Errorf("evidence changed while copying %s", src)
		}
		return nil
	}
	if err = copyVerified(baselineRun, filepath.Join(out, "raw/baseline"), "checksums.json", func(path string) error { _, e := experiment.Load(path); return e }); err != nil {
		return err
	}
	if err = copyVerified(candidateRun, filepath.Join(out, "raw/candidate"), "checksums.json", func(path string) error { _, e := experiment.Load(path); return e }); err != nil {
		return err
	}
	copyBuilds := func(paths []string, pathBuilds, orderedBuilds []sourcebuild.Result, side string) ([]sourceBuildLink, error) {
		pathByID := make(map[string]string, len(paths))
		for i, build := range pathBuilds {
			pathByID[build.Lock.Recipe.Workload.ID] = paths[i]
		}
		links := make([]sourceBuildLink, 0, len(orderedBuilds))
		for i, build := range orderedBuilds {
			id := build.Lock.Recipe.Workload.ID
			src, ok := pathByID[id]
			if !ok {
				return nil, fmt.Errorf("source build path missing workload %q", id)
			}
			dst := filepath.Join(out, "builds", side, fmt.Sprintf("%03d", i))
			if err := copyVerified(src, dst, "checksums.json", func(path string) error { _, e := sourcebuild.Verify(path); return e }); err != nil {
				return nil, err
			}
			links = append(links, sourceBuildLink{Workload: id, Bundle: filepath.ToSlash(filepath.Join("builds", side, fmt.Sprintf("%03d", i)))})
		}
		return links, nil
	}
	baselineBuildLinks, err := copyBuilds(baselineBuildPaths, baselineBuilds, comparison.BaselineBuilds, "baseline")
	if err != nil {
		return err
	}
	candidateBuildLinks, err := copyBuilds(candidateBuildPaths, candidateBuilds, comparison.CandidateBuilds, "candidate")
	if err != nil {
		return err
	}
	page := sourceSetPage{Comparison: comparison, Evidence: sourceSetEvidence{BaselineRun: baseline.Manifest.ID, CandidateRun: candidate.Manifest.ID, BaselineHash: baselineChecksums, CandidateHash: candidateChecksums}, BaselineBuildLinks: baselineBuildLinks, CandidateBuildLinks: candidateBuildLinks}
	if err = experiment.WriteJSON(filepath.Join(out, "data.json"), page); err != nil {
		return err
	}
	data, err := json.Marshal(page)
	if err != nil {
		return err
	}
	t, err := template.ParseFS(sourceSetAssets, "source_set.html")
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
	return sealFamilyReport(out, "source-set")
}

// VerifySourceSetReport validates the outer seal, copied bundles, build-to-run
// bindings and analysis recomputed from those portable inputs.
func VerifySourceSetReport(root string) error {
	if found, err := verifyArchivedFamily(root, "source-set"); found {
		return err
	}
	if err := experiment.Verify(root); err != nil {
		return err
	}
	var saved sourceSetPage
	if err := experiment.ReadJSON(filepath.Join(root, "data.json"), &saved); err != nil {
		return err
	}
	loadBuilds := func(side string, links []sourceBuildLink) ([]sourcebuild.Result, error) {
		builds := make([]sourcebuild.Result, 0, len(links))
		for i, link := range links {
			want := filepath.ToSlash(filepath.Join("builds", side, fmt.Sprintf("%03d", i)))
			if link.Bundle != want || link.Workload == "" {
				return nil, fmt.Errorf("invalid %s build link", side)
			}
			build, err := sourcebuild.Verify(filepath.Join(root, filepath.FromSlash(link.Bundle)))
			if err != nil {
				return nil, err
			}
			if build.Lock.Recipe.Workload.ID != link.Workload {
				return nil, fmt.Errorf("%s build link workload mismatch", side)
			}
			builds = append(builds, build)
		}
		return builds, nil
	}
	baselineBuilds, err := loadBuilds("baseline", saved.BaselineBuildLinks)
	if err != nil {
		return err
	}
	candidateBuilds, err := loadBuilds("candidate", saved.CandidateBuildLinks)
	if err != nil {
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
	comparison, err := analysis.CompareSourceRunSet(baseline, candidate, baselineBuilds, candidateBuilds, saved.Comparison.RuntimePerformance.BaselineConfiguration.ID)
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
	want := sourceSetPage{Comparison: comparison, Evidence: sourceSetEvidence{BaselineRun: baseline.Manifest.ID, CandidateRun: candidate.Manifest.ID, BaselineHash: baselineHash, CandidateHash: candidateHash}, BaselineBuildLinks: saved.BaselineBuildLinks, CandidateBuildLinks: saved.CandidateBuildLinks}
	if !reflect.DeepEqual(saved, want) {
		return fmt.Errorf("source set report analysis does not match its verified inputs")
	}
	return nil
}

// SplitSourceBuildPaths parses the CLI's comma-separated path convention while
// rejecting empty entries instead of silently changing the requested set.
func SplitSourceBuildPaths(paths string) ([]string, error) {
	if strings.TrimSpace(paths) == "" {
		return nil, fmt.Errorf("empty source build path list")
	}
	out := strings.Split(paths, ",")
	for i := range out {
		out[i] = strings.TrimSpace(out[i])
		if out[i] == "" {
			return nil, fmt.Errorf("empty path in source build list")
		}
	}
	return out, nil
}
