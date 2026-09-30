package analysis

import (
	"encoding/json"
	"testing"

	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
	"github.com/wasmbench/wasmbench/sourcebuild"
)

func sourceFixture(id string, values []int64) (experiment.Bundle, sourcebuild.Result) {
	b := regressionFixture(id, values)
	w := b.Manifest.Lock.Workloads[0]
	w.SHA256 = ""
	r := sourcebuild.Result{Schema: 1, Kind: "source_build", Status: "validated_not_correctness_checked", LockSHA256: "lock-" + id, ArtifactSHA256: "artifact-" + id,
		Lock: sourcebuild.Lock{Schema: 1, Recipe: sourcebuild.Recipe{Schema: 1, ID: id, SourceRevision: "source-v1", License: "MIT", Workload: w,
			Inputs: map[string]sourcebuild.File{"source.c": {Path: "/source/" + id, SHA256: "fixed-source"}}, Tools: map[string]sourcebuild.File{"compiler": {Path: "/compiler/" + id, SHA256: "fixed-compiler"}}, Steps: []sourcebuild.Step{{Tool: "compiler", Args: []string{"-O" + id}}}}, Analyzer: &experiment.AnalyzerLock{Profile: "wasm1", SHA256: "fixed-analyzer"}},
	}
	w.SHA256, w.Source, w.License, w.Generator = r.ArtifactSHA256, "source-v1", "MIT", "source-build-v1"
	// Match the canonical wire order emitted by the producer.
	w.Provenance, _ = json.Marshal(struct {
		Kind           string           `json:"kind"`
		LockSHA256     string           `json:"lock_sha256"`
		ArtifactSHA256 string           `json:"artifact_sha256"`
		Lock           sourcebuild.Lock `json:"lock"`
	}{"source_build", r.LockSHA256, r.ArtifactSHA256, r.Lock})
	b.Manifest.Lock.Workloads = []protocol.Workload{w}
	return b, r
}

func TestSourceComparisonKeepsRuntimeComparisonStrict(t *testing.T) {
	a, ab := sourceFixture("a", []int64{10, 10, 10})
	b, bb := sourceFixture("b", []int64{20, 20, 20})
	r, err := CompareSourceRuns(a, b, ab, bb, "runtime")
	if err != nil {
		t.Fatal(err)
	}
	if r.Track != "source_toolchain_output" || len(r.RuntimePerformance.Results) != 1 || r.RuntimePerformance.Results[0].Ratio == nil || *r.RuntimePerformance.Results[0].Ratio != 2 {
		t.Fatal(r)
	}
	strict, err := CompareRuns(a, b, "runtime", "runtime")
	if err != nil || strict.Results[0].Status != "incomparable" || strict.Results[0].Ratio != nil {
		t.Fatal(strict, err)
	}
}

func TestStackComparisonAllowsBothComponentsToChange(t *testing.T) {
	a, ab := sourceFixture("a", []int64{10, 10, 10})
	b, bb := sourceFixture("b", []int64{20, 20, 20})
	b.Manifest.Lock.Runtimes[0].ID = "other-runtime"
	b.Manifest.Lock.Runtimes[0].Files = map[string]string{"adapter": "different-runtime"}
	for i := range b.Trials {
		b.Trials[i].Runtime = "other-runtime"
	}
	got, err := CompareStackRuns(a, b, ab, bb, "runtime", "other-runtime")
	if err != nil {
		t.Fatal(err)
	}
	if got.Track != "end_to_end_stack" || got.RuntimePerformance.BaselineConfiguration.ID != "runtime" || got.RuntimePerformance.CandidateConfiguration.ID != "other-runtime" || len(got.RuntimePerformance.CommonSubset) != 1 || got.RuntimePerformance.Results[0].Ratio == nil || *got.RuntimePerformance.Results[0].Ratio != 2 {
		t.Fatal(got)
	}
	if _, err := CompareSourceRuns(a, b, ab, bb, "runtime"); err == nil {
		t.Fatal("fixed-runtime source track accepted changed runtime")
	}
	if _, err := CompareStackRuns(a, b, ab, ab, "runtime", "other-runtime"); err == nil {
		t.Fatal("stack track accepted unchanged source build")
	}
	labelOnly := bb
	labelOnly.Lock.Recipe.Steps = ab.Lock.Recipe.Steps
	if _, err := CompareStackRuns(a, b, ab, labelOnly, "runtime", "other-runtime"); err == nil {
		t.Fatal("stack track accepted build-label-only change")
	}
	unchangedRuntime, unchangedBuild := sourceFixture("b", []int64{20, 20, 20})
	if _, err := CompareStackRuns(a, unchangedRuntime, ab, unchangedBuild, "runtime", "runtime"); err == nil {
		t.Fatal("stack track accepted unchanged runtime")
	}
	for _, change := range []string{"oracle", "source", "host", "runner", "artifact"} {
		t.Run(change, func(t *testing.T) {
			badRun, badBuild := sourceFixture("b", []int64{20, 20, 20})
			badRun.Manifest.Lock.Runtimes[0].ID = "other-runtime"
			for i := range badRun.Trials {
				badRun.Trials[i].Runtime = "other-runtime"
			}
			switch change {
			case "oracle":
				badBuild.Lock.Recipe.Workload.Oracle.Expected = protocol.Values{8}
			case "source":
				badBuild.Lock.Recipe.Inputs["source.c"] = sourcebuild.File{SHA256: "changed"}
			case "host":
				badRun.Manifest.Host.Arch = "changed"
			case "runner":
				badRun.Manifest.Lock.RunnerSHA256 = "changed"
			case "artifact":
				badRun.Manifest.Lock.Workloads[0].SHA256 = "changed"
			}
			if _, err := CompareStackRuns(a, badRun, ab, badBuild, "runtime", "other-runtime"); err == nil {
				t.Fatal("accepted changed", change)
			}
		})
	}
}

func TestSourceComparisonRejectsUnmatchedEvidence(t *testing.T) {
	for _, field := range []string{"source", "revision", "oracle", "policy", "analyzer", "artifact", "provenance", "runtime", "runtime configuration", "runner", "host", "protocol", "extra workload"} {
		t.Run(field, func(t *testing.T) {
			a, ab := sourceFixture("a", []int64{10, 10, 10})
			b, bb := sourceFixture("b", []int64{20, 20, 20})
			switch field {
			case "source":
				bb.Lock.Recipe.Inputs["source.c"] = sourcebuild.File{SHA256: "changed"}
			case "revision":
				bb.Lock.Recipe.SourceRevision = "changed"
			case "oracle":
				bb.Lock.Recipe.Workload.Oracle.Expected = protocol.Values{8}
			case "policy":
				bb.Lock.Analyzer.Profile = "wasm2"
			case "analyzer":
				bb.Lock.Analyzer.SHA256 = "changed"
			case "artifact":
				b.Manifest.Lock.Workloads[0].SHA256 = "wrong"
			case "provenance":
				b.Manifest.Lock.Workloads[0].Provenance = nil
			case "runtime":
				b.Manifest.Lock.Runtimes[0].ID = "changed"
			case "runtime configuration":
				b.Manifest.Lock.Runtimes[0].Files = map[string]string{"adapter": "changed"}
			case "runner":
				b.Manifest.Lock.RunnerSHA256 = "changed"
			case "host":
				b.Manifest.Host.Arch = "different"
			case "protocol":
				b.Manifest.Lock.Options.Warmup++
			case "extra workload":
				b.Manifest.Lock.Workloads = append(b.Manifest.Lock.Workloads, b.Manifest.Lock.Workloads[0])
			}
			if _, err := CompareSourceRuns(a, b, ab, bb, "runtime"); err == nil {
				t.Fatal("accepted", field)
			}
		})
	}
}

func TestSourceComparisonRetainsFailures(t *testing.T) {
	a, ab := sourceFixture("a", []int64{10, 10, 10})
	b, bb := sourceFixture("b", []int64{20, 20, 20})
	for i := range b.Trials {
		b.Trials[i].Status = "incorrect"
	}
	r, err := CompareSourceRuns(a, b, ab, bb, "runtime")
	if err != nil {
		t.Fatal(err)
	}
	row := r.RuntimePerformance.Results[0]
	if row.Ratio != nil || row.CandidateOutcomes["incorrect"] != 3 || len(r.RuntimePerformance.CommonSubset) != 0 {
		t.Fatal(r)
	}
}

func bindSourceFixture(bundle experiment.Bundle, build sourcebuild.Result, id string) (experiment.Bundle, sourcebuild.Result) {
	build.Lock.Recipe.Workload.ID = id
	w := build.Lock.Recipe.Workload
	w.Artifact = "module.wasm"
	w.SHA256 = build.ArtifactSHA256
	w.License = build.Lock.Recipe.License
	w.Source = build.Lock.Recipe.SourceRevision
	w.Generator = "source-build-v1"
	w.Provenance, _ = json.Marshal(struct {
		Kind           string           `json:"kind"`
		LockSHA256     string           `json:"lock_sha256"`
		ArtifactSHA256 string           `json:"artifact_sha256"`
		Lock           sourcebuild.Lock `json:"lock"`
	}{"source_build", build.LockSHA256, build.ArtifactSHA256, build.Lock})
	bundle.Manifest.Lock.Workloads[0] = w
	for i := range bundle.Trials {
		bundle.Trials[i].Workload = id
	}
	return bundle, build
}

func TestSourceSetComparisonRequiresAndComparesEveryOutput(t *testing.T) {
	a1, ab1 := sourceFixture("a1", []int64{10, 10, 10})
	a1, ab1 = bindSourceFixture(a1, ab1, "task/one")
	a2, ab2 := sourceFixture("a2", []int64{30, 30, 30})
	a2, ab2 = bindSourceFixture(a2, ab2, "task/two")
	b1, bb1 := sourceFixture("b1", []int64{20, 20, 20})
	b1, bb1 = bindSourceFixture(b1, bb1, "task/one")
	b2, bb2 := sourceFixture("b2", []int64{60, 60, 60})
	b2, bb2 = bindSourceFixture(b2, bb2, "task/two")
	baseline := a1
	baseline.Manifest.ID = "baseline"
	baseline.Manifest.Lock.Workloads = append(baseline.Manifest.Lock.Workloads, a2.Manifest.Lock.Workloads[0])
	baseline.Trials = append(baseline.Trials, a2.Trials...)
	candidate := b1
	candidate.Manifest.ID = "candidate"
	candidate.Manifest.Lock.Workloads = append(candidate.Manifest.Lock.Workloads, b2.Manifest.Lock.Workloads[0])
	candidate.Trials = append(candidate.Trials, b2.Trials...)

	got, err := CompareSourceRunSet(baseline, candidate, []sourcebuild.Result{ab2, ab1}, []sourcebuild.Result{bb1, bb2}, "runtime")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.BaselineBuilds) != 2 || len(got.RuntimePerformance.Results) != 2 || len(got.RuntimePerformance.CommonSubset) != 2 {
		t.Fatalf("incomplete source set result: %+v", got)
	}
	for _, result := range got.RuntimePerformance.Results {
		if result.Ratio == nil || *result.Ratio != 2 {
			t.Fatalf("wrong per-workload ratio: %+v", result)
		}
	}
	if got.BaselineBuilds[0].Lock.Recipe.Workload.ID != "task/one" || got.BaselineBuilds[1].Lock.Recipe.Workload.ID != "task/two" {
		t.Fatal("build outputs are not deterministically ordered")
	}
	if _, err = CompareSourceRunSet(baseline, candidate, []sourcebuild.Result{ab1}, []sourcebuild.Result{bb1, bb2}, "runtime"); err == nil {
		t.Fatal("accepted mismatched source build set")
	}
	candidate.Manifest.Lock.Workloads = candidate.Manifest.Lock.Workloads[:1]
	if _, err = CompareSourceRunSet(baseline, candidate, []sourcebuild.Result{ab1, ab2}, []sourcebuild.Result{bb1, bb2}, "runtime"); err == nil {
		t.Fatal("accepted runtime run omitting a source output")
	}
}
