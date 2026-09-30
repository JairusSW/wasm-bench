package experiment_test

import (
	"context"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/publish"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestWasmtimeCodeLifetimeControllerRoundTrip(t *testing.T) {
	if os.Getenv("WASMBENCH_CODE_LIFETIME_TEST") != "1" {
		t.Skip("build the Linux diagnostic binary and set WASMBENCH_CODE_LIFETIME_TEST=1")
	}
	if runtime.GOOS != "linux" {
		t.Skip("native code lifetime is Linux-only")
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	runtimes, err := experiment.ResolveRuntimes(root, []string{"wasmtime-code-lifetime", "wasmtime-winch-code-lifetime"})
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	workloads, err := corpus.Generate(filepath.Join(tmp, "corpus"), "core")
	if err != nil {
		t.Fatal(err)
	}
	lock, err := experiment.NewLock(experiment.Options{Suite: "core", Profile: "code", Scenarios: []string{"code-lifetime"}, Launches: 1, Samples: 1, Operations: 1, Timeout: 15 * time.Second}, runtimes, workloads)
	if err != nil {
		t.Fatal(err)
	}
	lock.Analyzer, err = experiment.PinAnalyzer(filepath.Join(root, "adapters/wasmtime/target/release/wasm-analyze"), "default")
	if err != nil {
		t.Fatal(err)
	}
	out, err := experiment.Run(context.Background(), lock, tmp, filepath.Join(tmp, "run"), func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	b, err := experiment.Load(out)
	if err != nil {
		t.Fatal(err)
	}
	successful := 0
	for _, trial := range b.Trials {
		if trial.Block < 0 {
			continue
		}
		if trial.Status != "ok" || trial.CodeLifetime == nil || len(trial.Samples) != 0 {
			t.Fatalf("incomplete trial %s: %s %s", trial.ID, trial.Status, trial.Reason)
		}
		successful++
	}
	if successful != len(workloads)*len(runtimes) {
		t.Fatal("missing diagnostic coverage")
	}
	if err := publish.Report(out, filepath.Join(tmp, "report")); err != nil {
		t.Fatal(err)
	}
	if err := publish.VerifyReport(filepath.Join(tmp, "report")); err != nil {
		t.Fatal(err)
	}
}
