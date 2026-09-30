package analysis

import (
	"fmt"
	"reflect"

	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
	"github.com/wasmbench/wasmbench/sourcebuild"
)

type SourceComparison struct {
	Schema             int                `json:"schema"`
	Track              string             `json:"track"`
	AnalysisVersion    string             `json:"analysis_version"`
	Interpretation     string             `json:"interpretation"`
	BaselineBuild      sourcebuild.Result `json:"baseline_build"`
	CandidateBuild     sourcebuild.Result `json:"candidate_build"`
	RuntimePerformance RegressionReport   `json:"runtime_performance"`
}

// StackComparison keeps source compilation and runtime execution evidence in
// one task comparison while allowing both the compiler and runtime to change.
// It does not turn a one-off build duration into a statistical latency sample.
type StackComparison struct {
	Schema             int                `json:"schema"`
	Track              string             `json:"track"`
	AnalysisVersion    string             `json:"analysis_version"`
	Interpretation     string             `json:"interpretation"`
	BaselineBuild      sourcebuild.Result `json:"baseline_build"`
	CandidateBuild     sourcebuild.Result `json:"candidate_build"`
	RuntimePerformance RegressionReport   `json:"runtime_performance"`
}

// CompareStackRuns compares two source compiler + runtime configurations on
// one fixed application task. Inputs must be loaded from verified bundles.
func CompareStackRuns(a, b experiment.Bundle, ab, bb sourcebuild.Result, baselineRuntime, candidateRuntime string) (StackComparison, error) {
	out := StackComparison{Schema: 1, Track: "end_to_end_stack", AnalysisVersion: "stack-independent-launch-v2", BaselineBuild: ab, CandidateBuild: bb,
		Interpretation: "Fixed source task and correctness policy; both source-build and runtime configurations change. The measured values are runtime API phase latencies of each toolchain's exact Wasm output, not an observed source-to-result end-to-end latency. One-off source build step durations are operational evidence, not replicated performance samples. Independent launch intervals describe observed stack differences, not causal attribution to either component."}
	if err := sourcebuild.SameTask(ab, bb); err != nil {
		return out, err
	}
	if sameSourceBuildConfiguration(ab, bb) {
		return out, fmt.Errorf("stack track requires distinct source-build configurations; use the runtime track when only the runtime changes")
	}
	if len(a.Manifest.Lock.Workloads) != 1 || len(b.Manifest.Lock.Workloads) != 1 || !ab.MatchesWorkload(a.Manifest.Lock.Workloads[0]) || !bb.MatchesWorkload(b.Manifest.Lock.Workloads[0]) {
		return out, fmt.Errorf("runtime workload does not match its source build output and contract")
	}
	r, err := compareRuns(a, b, baselineRuntime, candidateRuntime, func(x, y protocol.Workload) bool {
		return ab.MatchesWorkload(x) && bb.MatchesWorkload(y)
	})
	if err != nil {
		return out, err
	}
	if r.RunnerChanged {
		return out, fmt.Errorf("stack comparison requires identical measurement runners")
	}
	if reflect.DeepEqual(r.BaselineConfiguration, r.CandidateConfiguration) {
		return out, fmt.Errorf("stack track requires distinct runtime configurations; use the source-output track when only the source toolchain changes")
	}
	out.RuntimePerformance = r
	return out, nil
}

// Build IDs and path locators are provenance, not evidence that the toolchain
// configuration changed. Compare the pinned tool bytes, steps and environment.
func sameSourceBuildConfiguration(a, b sourcebuild.Result) bool {
	tools := func(r sourcebuild.Result) map[string]string {
		out := make(map[string]string, len(r.Lock.Recipe.Tools))
		for name, tool := range r.Lock.Recipe.Tools {
			out[name] = tool.SHA256
		}
		return out
	}
	return reflect.DeepEqual(tools(a), tools(b)) && reflect.DeepEqual(a.Lock.Recipe.Steps, b.Lock.Recipe.Steps) && reflect.DeepEqual(a.Lock.Recipe.Environment, b.Lock.Recipe.Environment)
}

// CompareSourceRuns compares execution of different source-toolchain outputs
// on the same runtime. Callers must load verified runtime and source bundles.
// It does not compare the one-off operational compiler durations.
func CompareSourceRuns(a, b experiment.Bundle, ab, bb sourcebuild.Result, runtime string) (SourceComparison, error) {
	out := SourceComparison{Schema: 1, Track: "source_toolchain_output", AnalysisVersion: "source-output-independent-launch-v1", BaselineBuild: ab, CandidateBuild: bb,
		Interpretation: "Runtime API latency of independently built Wasm outputs for an identical source task and validation policy, using a fixed runtime configuration. Compiler step times are not performance samples. Run ordering is not paired across bundles; intervals describe observed differences, not attribution to particular flags or tool changes. Builds are not hermetic."}
	if err := sourcebuild.SameTask(ab, bb); err != nil {
		return out, err
	}
	// One source build emits one workload. Reject accidental extra workloads,
	// rather than allowing a common-subset comparison to hide them.
	if len(a.Manifest.Lock.Workloads) != 1 || len(b.Manifest.Lock.Workloads) != 1 || !ab.MatchesWorkload(a.Manifest.Lock.Workloads[0]) || !bb.MatchesWorkload(b.Manifest.Lock.Workloads[0]) {
		return out, fmt.Errorf("runtime workload does not match its source build output and contract")
	}
	r, err := compareRuns(a, b, runtime, runtime, func(x, y protocol.Workload) bool {
		return ab.MatchesWorkload(x) && bb.MatchesWorkload(y)
	})
	if err != nil {
		return out, err
	}
	if !reflect.DeepEqual(r.BaselineConfiguration, r.CandidateConfiguration) {
		return out, fmt.Errorf("source-toolchain comparison requires identical runtime configurations")
	}
	if r.RunnerChanged {
		return out, fmt.Errorf("source-toolchain comparison requires identical measurement runners")
	}
	out.RuntimePerformance = r
	return out, nil
}
