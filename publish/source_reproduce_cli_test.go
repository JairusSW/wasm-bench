package publish

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/parquet-go/parquet-go"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/sourcebuild"
)

// Unlike the API integration fixture, this exercises the real source-rebuild,
// source-bench-replay and reproduce commands from staged executable archives.
// Build tooling finishes before any measurement; do not run this in parallel.
func TestLLVMSourceReportReplayArchivedCLI(t *testing.T) {
	if os.Getenv("WASMBENCH_SOURCE_LLVM_TEST") != "1" {
		t.Skip("set WASMBENCH_SOURCE_LLVM_TEST=1 with LLVM, analyzer and wazero")
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	work := t.TempDir()
	// Resolve installed adapters through a disposable workspace. CLI run indexing
	// must not insert test fixtures into the user's repository-local database.
	if err := os.Symlink(filepath.Join(root, "adapters"), filepath.Join(work, "adapters")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "bin"), filepath.Join(work, "bin")); err != nil {
		t.Fatal(err)
	}
	cli := filepath.Join(work, "wasmbench")
	run := func(t *testing.T, executable string, args ...string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, executable, args...)
		cmd.Dir = work
		if executable == "go" {
			cmd.Dir = root
		}
		var diagnostic bytes.Buffer
		cmd.Stdout, cmd.Stderr = io.Discard, &diagnostic
		if err := cmd.Run(); err != nil {
			t.Fatalf("%s: %v\n%s", strings.Join(args, " "), err, diagnostic.String())
		}
	}
	run(t, "go", "build", "-trimpath", "-o", cli, "./cmd/wasmbench")
	digest, err := experiment.DigestFile(cli)
	if err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	selfDigest, err := experiment.DigestFile(self)
	if err != nil || selfDigest == digest {
		t.Fatal("fixture must use a controller different from the locked CLI", err)
	}
	var locks, builds, runs []string
	for i, name := range []string{"xorshift-llvm-o0.json", "xorshift-llvm.json"} {
		lock := filepath.Join(work, name+".lock")
		build := filepath.Join(work, name+".build")
		runtime := filepath.Join(work, name+".run")
		run(t, cli, "source-lock", "--recipe", filepath.Join(root, "recipes/source", name), "--validation-profile", "wasm1", "--out", lock)
		run(t, cli, "source-build", "--lock", lock, "--out", build, "--timeout", "1m")
		id := []string{"wazero", "wazero-interpreter"}[i]
		run(t, cli, "run", "--suite", filepath.Join(build, "suite.json"), "--runtimes", id, "--scenarios", "compile", "--validation-profile", "wasm1", "--launches", "1", "--samples", "1", "--operations", "1", "--warmup", "0", "--out", runtime)
		locks, builds, runs = append(locks, lock), append(builds, build), append(runs, runtime)
	}
	// The source-set track holds the runtime configuration fixed.
	setRun := filepath.Join(work, "set-candidate.run")
	run(t, cli, "run", "--suite", filepath.Join(builds[1], "suite.json"), "--runtimes", "wazero", "--scenarios", "compile", "--validation-profile", "wasm1", "--launches", "1", "--samples", "1", "--operations", "1", "--warmup", "0", "--out", setRun)
	benchmark := filepath.Join(work, "benchmark")
	run(t, cli, "source-bench", "--locks", strings.Join(locks, ","), "--blocks", "1", "--warmup", "0", "--timeout", "1m", "--out", benchmark)
	for _, kind := range []string{"source", "source-set", "stack"} {
		t.Run(kind, func(t *testing.T) {
			source := filepath.Join(work, kind+".report")
			switch kind {
			case "source":
				run(t, cli, "source-bench-html", "--bundle", benchmark, "--out", source)
			case "source-set":
				run(t, cli, "source-compare-set-report", "--baseline-builds", builds[0], "--candidate-builds", builds[1], "--baseline-run", runs[0], "--candidate-run", setRun, "--runtime", "wazero", "--out", source)
			case "stack":
				run(t, cli, "stack-report", "--baseline-build", builds[0], "--candidate-build", builds[1], "--baseline-run", runs[0], "--candidate-run", runs[1], "--baseline-runtime", "wazero", "--candidate-runtime", "wazero-interpreter", "--out", source)
			}
			before, err := experiment.DigestFile(filepath.Join(source, "checksums.json"))
			if err != nil {
				t.Fatal(err)
			}
			if kind == "source" {
				regenerated := t.TempDir()
				if err := ExportSourceTables(filepath.Join(source, "raw"), regenerated); err != nil {
					t.Fatal(err)
				}
				a, err := parquet.ReadFile[SourceStepRow](filepath.Join(source, "compiler-steps.parquet"))
				if err != nil {
					t.Fatal(err)
				}
				b, err := parquet.ReadFile[SourceStepRow](filepath.Join(regenerated, "compiler-steps.parquet"))
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(a, b) {
					t.Fatalf("source step row mismatch: original=%+v regenerated=%+v", a, b)
				}
				for _, name := range []string{"compiler-builds.parquet", "compiler-steps.parquet"} {
					original, err := os.ReadFile(filepath.Join(source, name))
					if err != nil {
						t.Fatal(err)
					}
					current, err := os.ReadFile(filepath.Join(regenerated, name))
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(original, current) {
						t.Fatalf("cross-build Parquet byte mismatch: %s", name)
					}
				}
			}
			out := filepath.Join(work, kind+".replay")
			if err := ReproduceReport(ctx, source, out, nil); err != nil {
				t.Fatal(err)
			}
			if err := VerifyAnyReport(filepath.Join(out, "report")); err != nil {
				t.Fatal(err)
			}
			plan, err := newSourceReportReplayPlan(source, kind)
			if err != nil {
				t.Fatal(err)
			}
			for _, pass := range plan.Passes {
				staged := filepath.Join(out, "runners", pass.Name, "wasmbench")
				got, err := experiment.DigestFile(staged)
				if err != nil || got != digest {
					t.Fatal("did not stage the exact original CLI", pass.Name, err)
				}
				info, err := os.Stat(staged)
				if err != nil || info.Mode().Perm()&0111 == 0 {
					t.Fatal("staged runner is not executable", err)
				}
				if pass.Kind == "source-build" {
					result, err := sourcebuild.Verify(filepath.Join(out, pass.Name))
					if err != nil || result.ReproducesSHA256 == "" || result.ToolTimeoutNS != time.Minute {
						t.Fatal("missing fresh build or original timeout evidence", err)
					}
				}
			}
			after, err := experiment.DigestFile(filepath.Join(source, "checksums.json"))
			if err != nil || after != before {
				t.Fatal("changed immutable source report", err)
			}
			if err := VerifyAnyReport(source); err != nil {
				t.Fatal("source seal or runner archive changed", err)
			}
		})
	}
}
