package publish

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
	"github.com/wasmbench/wasmbench/sourcebuild"
)

func TestLLVMSourceReportFamiliesReplayBuildsBeforeRuntime(t *testing.T) {
	if os.Getenv("WASMBENCH_SOURCE_LLVM_TEST") != "1" {
		t.Skip("set WASMBENCH_SOURCE_LLVM_TEST=1 with LLVM, analyzer and wazero")
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	a, err := experiment.PinAnalyzer(filepath.Join(root, "adapters/wasmtime/target/release/wasm-analyze"), "wasm1")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	runtimes, err := experiment.ResolveRuntimes(root, []string{"wazero", "wazero-interpreter"})
	if err != nil {
		t.Fatal(err)
	}
	config := sourcebuild.BenchmarkConfig{Schema: 1, Profile: "timing", Blocks: 1, Seed: 1, Timeout: time.Minute, CorrectnessRuntimes: runtimes[:1]}
	builds := []string{}
	for _, name := range []string{"xorshift-llvm-o0.json", "xorshift-llvm.json"} {
		var recipe sourcebuild.Recipe
		if err := experiment.ReadJSON(filepath.Join(root, "recipes/source", name), &recipe); err != nil {
			t.Fatal(err)
		}
		lock, err := sourcebuild.Pin(recipe, filepath.Join(root, "recipes/source"), a)
		if err != nil {
			t.Fatal(err)
		}
		config.Variants = append(config.Variants, lock)
		out := filepath.Join(t.TempDir(), "build")
		if _, err := sourcebuild.Build(ctx, lock, out, time.Minute); err != nil {
			t.Fatal(err)
		}
		builds = append(builds, out)
	}
	makeRun := func(buildPaths []string, runtimeIndex int) string {
		var suite []protocol.Workload
		for _, build := range buildPaths {
			var part []protocol.Workload
			if err := experiment.ReadJSON(filepath.Join(build, "suite.json"), &part); err != nil {
				t.Fatal(err)
			}
			suite = append(suite, part...)
		}
		lock, err := experiment.NewLock(experiment.Options{Suite: "source-replay-integration", Profile: "timing", Scenarios: []string{"compile"}, Launches: 1, Samples: 1, Operations: 1, Timeout: time.Minute}, runtimes[runtimeIndex:runtimeIndex+1], suite)
		if err != nil {
			t.Fatal(err)
		}
		lock.Analyzer = a
		out := filepath.Join(t.TempDir(), fmt.Sprintf("run-%d", runtimeIndex))
		if _, err := experiment.Run(ctx, lock, buildPaths[0], out, func(string) {}); err != nil {
			t.Fatal(err)
		}
		return out
	}
	runA, runC := makeRun(builds[:1], 0), makeRun(builds[1:], 1)
	// Exercise a real two-workload set, including two byte-identical artifacts
	// sharing one bundled artifact path but distinct source/task bindings.
	multi := [][]string{{builds[0]}, {builds[1]}}
	for i, original := range config.Variants {
		recipe := original.Recipe
		recipe.Workload.ID = "algorithms/xorshift-copy"
		lock, err := sourcebuild.Pin(recipe, filepath.Join(root, "recipes/source"), a)
		if err != nil {
			t.Fatal(err)
		}
		out := filepath.Join(t.TempDir(), "additional-build")
		if _, err := sourcebuild.Build(ctx, lock, out, time.Minute); err != nil {
			t.Fatal(err)
		}
		multi[i] = append(multi[i], out)
	}
	setRunA, setRunB := makeRun(multi[0], 0), makeRun(multi[1], 0)
	benchmark := filepath.Join(t.TempDir(), "benchmark")
	if built, err := sourcebuild.Benchmark(ctx, config, benchmark); err != nil {
		t.Fatal(err, built.Admissions)
	}
	for _, kind := range []string{"source", "source-set", "stack"} {
		t.Run(kind, func(t *testing.T) {
			source := filepath.Join(t.TempDir(), "report")
			var err error
			switch kind {
			case "source":
				err = SourceReport(benchmark, source)
			case "source-set":
				err = SourceSetReport(multi[0], multi[1], setRunA, setRunB, "wazero", source)
			case "stack":
				err = StackReport(builds[0], builds[1], runA, runC, "wazero", "wazero-interpreter", source)
			}
			if err != nil {
				t.Fatal(err)
			}
			before, _ := experiment.DigestFile(filepath.Join(source, "checksums.json"))
			out := filepath.Join(t.TempDir(), "replay")
			var checked, executed []string
			err = reproduceReport(ctx, source, out, nil, func(p *reportReplayPass) error {
				if len(executed) != 0 {
					t.Fatal("measurement before full preflight")
				}
				checked = append(checked, p.Name)
				return preflightReportPass(p)
			}, func(ctx context.Context, p reportReplayPass, path string, _ func(string)) error {
				executed = append(executed, p.Name)
				// A test executable has no wasmbench CLI. Execute the same
				// pinned APIs here; real staged CLI replay is checked live.
				switch p.Kind {
				case "source-build":
					_, err := sourcebuild.Rebuild(ctx, p.Source, path, p.Timeout)
					return err
				case "source-benchmark":
					_, err := sourcebuild.ReplayBenchmark(ctx, p.Source, path)
					return err
				default:
					if filepath.Dir(p.Source) != filepath.Join(out, "inputs") {
						t.Fatal("runtime did not use staged rebuilt artifacts", p.Source)
					}
					_, err := experiment.Run(ctx, p.Lock, p.Source, path, func(string) {})
					return err
				}
			})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(checked, executed) {
				t.Fatal("changed pass order", checked, executed)
			}
			if err := VerifyAnyReport(filepath.Join(out, "report")); err != nil {
				t.Fatal(err)
			}
			if kind == "stack" && !reflect.DeepEqual(executed, []string{"build-baseline", "build-candidate", "baseline", "candidate"}) {
				t.Fatal("compiler/runtime ordering", executed)
			}
			if kind == "source-set" && !reflect.DeepEqual(executed, []string{"build-baseline-000", "build-baseline-001", "build-candidate-000", "build-candidate-001", "baseline", "candidate"}) {
				t.Fatal("multi-workload ordering", executed)
			}
			after, _ := experiment.DigestFile(filepath.Join(source, "checksums.json"))
			if before != after {
				t.Fatal("changed original report")
			}
			// A late missing runner must refuse before any compiler starts.
			refused := filepath.Join(t.TempDir(), "refused")
			err = reproduceReport(ctx, source, refused, nil, func(p *reportReplayPass) error {
				if p.Name == checked[len(checked)-1] {
					return fmt.Errorf("late exact runner missing")
				}
				return nil
			}, func(context.Context, reportReplayPass, string, func(string)) error {
				t.Fatal("measurement before complete preflight")
				return nil
			})
			if err == nil {
				t.Fatal("accepted failed preflight")
			}
			if _, err := os.Stat(refused); !os.IsNotExist(err) {
				t.Fatal("created output during failed preflight", err)
			}
			if kind != "source" {
				stale := filepath.Join(t.TempDir(), "stale-build-replay")
				err := reproduceReport(ctx, source, stale, nil, func(*reportReplayPass) error { return nil }, func(_ context.Context, p reportReplayPass, path string, _ func(string)) error {
					if p.Kind != "source-build" {
						t.Fatal("runtime started with old, unreplayed build evidence")
					}
					return os.CopyFS(path, os.DirFS(p.Source))
				})
				if err == nil {
					t.Fatal("substituted old builds for replayed outputs")
				}
				if _, err := os.Stat(filepath.Join(stale, "report")); !os.IsNotExist(err) {
					t.Fatal("published stale build comparison", err)
				}
			}
		})
	}
}
