package experiment_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/publish"
)

func TestWasmtimeMaterializationControllerRoundTrip(t *testing.T) {
	if os.Getenv("WASMBENCH_MATERIALIZATION_TEST") != "1" {
		t.Skip("set WASMBENCH_MATERIALIZATION_TEST=1 after building both Wasmtime adapters")
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	runtimes, err := experiment.ResolveRuntimes(root, []string{"wasmtime", "wasmtime-winch"})
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	workloads, err := corpus.Generate(filepath.Join(tmp, "corpus"), "core")
	if err != nil {
		t.Fatal(err)
	}
	lock, err := experiment.NewLock(experiment.Options{Suite: "core", Profile: "code", Scenarios: []string{"compile-materialized"}, Launches: 1, Samples: 1, Operations: 1, Timeout: 15 * time.Second}, runtimes, workloads)
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
	measured := 0
	for i, trial := range b.Trials {
		if trial.Status != "ok" {
			t.Fatal(trial.Runtime, trial.Reason)
		}
		if trial.Block < 0 {
			continue
		}
		measured++
		if trial.CodeImage == nil || trial.CodeImage.Version != 3 || len(trial.Samples) != 1 {
			t.Fatal("missing materialized evidence")
		}
		original := b.Trials[i].CodeImage
		image, proof := *original, *original.Materialization
		image.Materialization = &proof
		// Internally coherent shifted native indices still must fail the independent input gate.
		image.Functions = append(image.Functions[:0:0], original.Functions...)
		proof.ImportedFunctions++
		for j := range image.Functions {
			image.Functions[j].WasmIndex++
		}
		if err := image.Validate(image.ModuleSHA256); err != nil {
			t.Fatal("forgery must satisfy internal range contract", err)
		}
		b.Trials[i].CodeImage = &image
		if experiment.ValidateMaterializationEvidence(out, b) == nil {
			t.Fatal("independent analyzer accepted shifted coverage")
		}
		b.Trials[i].CodeImage = nil
		if experiment.ValidateMaterializationEvidence(out, b) == nil {
			t.Fatal("missing proof accepted")
		}
		b.Trials[i].CodeImage = original
	}
	if measured != len(runtimes)*len(workloads) {
		t.Fatal("incomplete runtime/workload coverage", measured)
	}
	report := filepath.Join(tmp, "native-report")
	if err := publish.ExportNativeCode(out, report); err != nil {
		t.Fatal(err)
	}
	if err := publish.VerifyNativeCode(report); err != nil {
		t.Fatal(err)
	}
	html, err := os.ReadFile(filepath.Join(report, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(html), "Materialized compile:") || !strings.Contains(string(html), "not headline latency") {
		t.Fatal("materialization proof missing from native viewer")
	}
}
