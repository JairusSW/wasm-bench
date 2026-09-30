package publish

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/sourcebuild"
)

type reportReplayBuildInput struct {
	Workload, Pass string
}

func preflightSourceReportPass(pass *reportReplayPass) error {
	current, err := os.Executable()
	if err != nil {
		return err
	}
	// Source benchmarks archive their runner in each admitted build, avoiding
	// a separate, duplicate top-level source-benchmark archive.
	archiveRoot := pass.Source
	if pass.Kind == "source-benchmark" {
		archiveRoot = filepath.Join(pass.Source, "admission/00/build")
	}
	pass.RunnerArchive = filepath.Join(archiveRoot, "tools/runner/wasmbench")
	pass.Runner, err = exactReportRunner(current, archiveRoot, pass.Lock.RunnerSHA256)
	if err != nil {
		return err
	}
	switch pass.Kind {
	case "source-build":
		original, err := sourcebuild.PreflightRebuild(pass.Source, pass.Runner)
		if err != nil {
			return err
		}
		if pass.Timeout != original.ToolTimeoutNS {
			return fmt.Errorf("source replay tool timeout differs from recorded protocol")
		}
	case "source-benchmark":
		_, err := sourcebuild.PreflightBenchmarkReplay(pass.Source, pass.Runner)
		return err
	default:
		return fmt.Errorf("unsupported source replay pass %q", pass.Kind)
	}
	return nil
}

func newSourceReportReplayPlan(source, kind string) (reportReplayPlan, error) {
	var plan reportReplayPlan
	addBuild := func(name, relative string) (sourcebuild.Result, error) {
		path := filepath.Join(source, relative)
		b, err := sourcebuild.Verify(path)
		if err != nil {
			return b, err
		}
		plan.Passes = append(plan.Passes, reportReplayPass{Name: name, Source: path, Kind: "source-build", Timeout: b.ToolTimeoutNS, Lock: experiment.Lock{RunnerSHA256: b.Lock.RunnerSHA256}})
		return b, nil
	}
	addRuntime := func(name, relative string, builds []reportReplayBuildInput) error {
		path := filepath.Join(source, relative)
		b, err := experiment.Load(path)
		if err != nil {
			return err
		}
		// Artifact staging must stay inside the new owned input copy. This is
		// checked before any compiler measurement starts, not after rebuilding.
		for _, w := range b.Manifest.Lock.Workloads {
			if !filepath.IsLocal(w.Artifact) || filepath.Clean(w.Artifact) != w.Artifact {
				return fmt.Errorf("source runtime replay needs a bundle-local artifact path")
			}
		}
		plan.Passes = append(plan.Passes, reportReplayPass{Name: name, Source: path, Lock: b.Manifest.Lock, BuildInputs: builds})
		return nil
	}
	switch kind {
	case "source":
		path := filepath.Join(source, "raw")
		b, err := sourcebuild.VerifyBenchmark(path)
		if err != nil {
			return plan, err
		}
		plan.Passes = []reportReplayPass{{Name: "source", Source: path, Kind: "source-benchmark", Lock: experiment.Lock{RunnerSHA256: b.Config.Variants[0].RunnerSHA256}}}
		plan.Finish = func(_ context.Context, paths map[string]string, out string) error {
			return SourceReport(paths["source"], out)
		}
	case "stack":
		var d stackPage
		if err := experiment.ReadJSON(filepath.Join(source, "data.json"), &d); err != nil {
			return plan, err
		}
		inputs := map[string][]reportReplayBuildInput{}
		for _, side := range []string{"baseline", "candidate"} {
			name := "build-" + side
			b, err := addBuild(name, "builds/"+side)
			if err != nil {
				return plan, err
			}
			inputs[side] = []reportReplayBuildInput{{Workload: b.Lock.Recipe.Workload.ID, Pass: name}}
		}
		for _, side := range []string{"baseline", "candidate"} {
			if err := addRuntime(side, "raw/"+side, inputs[side]); err != nil {
				return plan, err
			}
		}
		plan.Finish = func(_ context.Context, paths map[string]string, out string) error {
			return StackReport(paths["build-baseline"], paths["build-candidate"], paths["baseline"], paths["candidate"], d.Comparison.RuntimePerformance.BaselineConfiguration.ID, d.Comparison.RuntimePerformance.CandidateConfiguration.ID, out)
		}
	case "source-set":
		var d sourceSetPage
		if err := experiment.ReadJSON(filepath.Join(source, "data.json"), &d); err != nil {
			return plan, err
		}
		inputs := map[string][]reportReplayBuildInput{}
		for i, side := range []string{"baseline", "candidate"} {
			links := [][]sourceBuildLink{d.BaselineBuildLinks, d.CandidateBuildLinks}[i]
			for j, link := range links {
				want := fmt.Sprintf("builds/%s/%03d", side, j)
				if link.Bundle != want {
					return plan, fmt.Errorf("invalid source set build locator")
				}
				name := fmt.Sprintf("build-%s-%03d", side, j)
				b, err := addBuild(name, want)
				if err != nil {
					return plan, err
				}
				inputs[side] = append(inputs[side], reportReplayBuildInput{Workload: b.Lock.Recipe.Workload.ID, Pass: name})
			}
		}
		for _, side := range []string{"baseline", "candidate"} {
			if err := addRuntime(side, "raw/"+side, inputs[side]); err != nil {
				return plan, err
			}
		}
		plan.Finish = func(_ context.Context, paths map[string]string, out string) error {
			builds := func(side string) []string {
				result := []string{}
				for _, input := range inputs[side] {
					result = append(result, paths[input.Pass])
				}
				return result
			}
			return SourceSetReport(builds("baseline"), builds("candidate"), paths["baseline"], paths["candidate"], d.Comparison.RuntimePerformance.BaselineConfiguration.ID, out)
		}
	default:
		return plan, fmt.Errorf("unsupported source report kind")
	}
	return plan, nil
}

// Fresh source outputs must be byte-identical and retain the original source
// lock/contract. Replace only artifact bytes in a new, sealed input copy before
// invoking the original runtime runner; never mutate the source report/lock.
func stageRebuiltRuntimeInputs(pass reportReplayPass, paths map[string]string, out string) (string, error) {
	b, err := experiment.Load(pass.Source)
	if err != nil {
		return "", err
	}
	if len(pass.BuildInputs) != len(b.Manifest.Lock.Workloads) {
		return "", fmt.Errorf("source build bindings do not cover the runtime workload set")
	}
	byID := map[string]string{}
	for _, input := range pass.BuildInputs {
		if _, exists := byID[input.Workload]; exists || paths[input.Pass] == "" {
			return "", fmt.Errorf("duplicate or missing rebuilt source binding")
		}
		byID[input.Workload] = paths[input.Pass]
	}
	// Verify every rebuilt artifact before creating the runtime input copy.
	for _, w := range b.Manifest.Lock.Workloads {
		if byID[w.ID] == "" {
			return "", fmt.Errorf("missing rebuilt source binding for %s", w.ID)
		}
		built, err := sourcebuild.Verify(byID[w.ID])
		if err != nil {
			return "", err
		}
		if !built.MatchesWorkload(w) || built.ReproducesSHA256 != w.SHA256 {
			return "", fmt.Errorf("rebuilt source artifact or contract differs for %s", w.ID)
		}
		if !filepath.IsLocal(w.Artifact) || filepath.Clean(w.Artifact) != w.Artifact {
			return "", fmt.Errorf("runtime artifact must remain bundle-local")
		}
	}
	if err := os.CopyFS(out, os.DirFS(pass.Source)); err != nil {
		return "", err
	}
	for _, w := range b.Manifest.Lock.Workloads {
		target := filepath.Join(out, w.Artifact)
		if err := os.Remove(target); err != nil {
			return "", err
		}
		if err := experiment.CopyExclusive(filepath.Join(byID[w.ID], "module.wasm"), target); err != nil {
			return "", err
		}
	}
	// The replacement bytes match the original digests: its seal must remain
	// valid without rewriting it. Any deviation fails before runtime execution.
	if _, err := experiment.Load(out); err != nil {
		return "", err
	}
	return out, nil
}
