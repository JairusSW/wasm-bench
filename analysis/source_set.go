package analysis

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
	"github.com/wasmbench/wasmbench/sourcebuild"
)

// SourceSetComparison compares corresponding outputs from two toolchain variants
// as one fixed workload set. Every runtime workload must bind to exactly one
// verified source-build result; no common-subset filtering can hide a missing
// build or workload.
type SourceSetComparison struct {
	Schema             int                  `json:"schema"`
	Track              string               `json:"track"`
	AnalysisVersion    string               `json:"analysis_version"`
	Interpretation     string               `json:"interpretation"`
	BaselineBuilds     []sourcebuild.Result `json:"baseline_builds"`
	CandidateBuilds    []sourcebuild.Result `json:"candidate_builds"`
	RuntimePerformance RegressionReport     `json:"runtime_performance"`
}

func CompareSourceRunSet(a, b experiment.Bundle, baseline, candidate []sourcebuild.Result, runtime string) (SourceSetComparison, error) {
	out := SourceSetComparison{Schema: 1, Track: "source_toolchain_output_set", AnalysisVersion: "source-output-set-independent-launch-v1",
		Interpretation: "Runtime API latency for a fixed set of independently built Wasm outputs from identical per-workload source tasks, using one fixed runtime configuration. Compiler step times are not performance samples. Run ordering is not paired across bundles; intervals describe observed differences, not attribution to particular flags or tool changes. Builds are not hermetic.",
		BaselineBuilds: append([]sourcebuild.Result(nil), baseline...), CandidateBuilds: append([]sourcebuild.Result(nil), candidate...)}
	baseByID, err := indexSourceBuilds(baseline)
	if err != nil {
		return out, fmt.Errorf("baseline builds: %w", err)
	}
	candidateByID, err := indexSourceBuilds(candidate)
	if err != nil {
		return out, fmt.Errorf("candidate builds: %w", err)
	}
	if len(baseByID) == 0 || len(baseByID) != len(candidateByID) {
		return out, fmt.Errorf("source build sets must be nonempty and have identical workload IDs")
	}
	ids := make([]string, 0, len(baseByID))
	for id, baselineBuild := range baseByID {
		candidateBuild, ok := candidateByID[id]
		if !ok {
			return out, fmt.Errorf("candidate source build missing workload %q", id)
		}
		if err := sourcebuild.SameTask(baselineBuild, candidateBuild); err != nil {
			return out, fmt.Errorf("workload %q: %w", id, err)
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out.BaselineBuilds = orderedSourceBuilds(ids, baseByID)
	out.CandidateBuilds = orderedSourceBuilds(ids, candidateByID)
	if err := matchSourceBuildSet(a, baseByID); err != nil {
		return out, fmt.Errorf("baseline runtime run: %w", err)
	}
	if err := matchSourceBuildSet(b, candidateByID); err != nil {
		return out, fmt.Errorf("candidate runtime run: %w", err)
	}
	performance, err := compareRuns(a, b, runtime, runtime, func(x, y protocol.Workload) bool {
		bx, xok := baseByID[x.ID]
		by, yok := candidateByID[y.ID]
		return xok && yok && bx.MatchesWorkload(x) && by.MatchesWorkload(y)
	})
	if err != nil {
		return out, err
	}
	if !reflect.DeepEqual(performance.BaselineConfiguration, performance.CandidateConfiguration) {
		return out, fmt.Errorf("source-toolchain comparison requires identical runtime configurations")
	}
	if performance.RunnerChanged {
		return out, fmt.Errorf("source-toolchain comparison requires identical measurement runners")
	}
	out.RuntimePerformance = performance
	return out, nil
}

func indexSourceBuilds(builds []sourcebuild.Result) (map[string]sourcebuild.Result, error) {
	out := make(map[string]sourcebuild.Result, len(builds))
	for _, build := range builds {
		id := build.Lock.Recipe.Workload.ID
		if id == "" || build.Kind != "source_build" || build.Status != "validated_not_correctness_checked" || build.ArtifactSHA256 == "" || build.LockSHA256 == "" {
			return nil, fmt.Errorf("invalid or incomplete source build result")
		}
		if _, exists := out[id]; exists {
			return nil, fmt.Errorf("duplicate source build for workload %q", id)
		}
		out[id] = build
	}
	return out, nil
}

func orderedSourceBuilds(ids []string, builds map[string]sourcebuild.Result) []sourcebuild.Result {
	out := make([]sourcebuild.Result, 0, len(ids))
	for _, id := range ids {
		out = append(out, builds[id])
	}
	return out
}

func matchSourceBuildSet(bundle experiment.Bundle, builds map[string]sourcebuild.Result) error {
	if len(bundle.Manifest.Lock.Workloads) != len(builds) {
		return fmt.Errorf("runtime workload count does not match source build set")
	}
	seen := make(map[string]bool, len(builds))
	for _, workload := range bundle.Manifest.Lock.Workloads {
		build, ok := builds[workload.ID]
		if !ok || seen[workload.ID] || !build.MatchesWorkload(workload) {
			return fmt.Errorf("runtime workload %q does not match exactly one source build", workload.ID)
		}
		seen[workload.ID] = true
	}
	return nil
}
