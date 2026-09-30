package experiment

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/protocol"
)

// TestComponentCommandBundle opts into an end-to-end sealed run against genuine
// Rust wasm32-wasip2 fixtures supplied by the sibling Wago checkout.
func TestComponentCommandBundle(t *testing.T) {
	artifact := os.Getenv("WASMBENCH_COMPONENT_ARTIFACT")
	adapter := os.Getenv("WASMBENCH_COMPONENT_ADAPTER")
	analyzer := os.Getenv("WASMBENCH_COMPONENT_ANALYZER")
	if artifact == "" || adapter == "" || analyzer == "" {
		t.Skip("set WASMBENCH_COMPONENT_ARTIFACT, WASMBENCH_COMPONENT_ADAPTER, and WASMBENCH_COMPONENT_ANALYZER for the sibling Wago P2 smoke")
	}
	artifact, err := filepath.Abs(artifact)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err = filepath.Abs(adapter)
	if err != nil {
		t.Fatal(err)
	}
	artifactHash, err := DigestFile(artifact)
	if err != nil {
		t.Fatal(err)
	}
	adapterHash, err := DigestFile(adapter)
	if err != nil {
		t.Fatal(err)
	}
	analyzerLock, err := PinAnalyzer(analyzer, "default")
	if err != nil {
		t.Fatal(err)
	}
	workload := protocol.Workload{
		Schema: 1, ID: "p2/rust-smoke", Family: "applications", Artifact: artifact,
		SHA256: artifactHash, ABI: "component", HostProfile: "wasi-preview2-readonly-v1",
		Features: []string{"component-model"}, Export: "_start", WorkUnit: "command",
		Units: 1, Reset: "fresh_instance_per_sample", License: "Apache-2.0",
		Source: "Wago wasi/p2 Rust smoke fixture", Generator: "rustc wasm32-wasip2",
		Command: &protocol.CommandContract{
			Argv:  []string{"wasi-p2-smoke", "alpha", "beta"},
			Stdin: []byte("from-rust-stdin\n"), ExitCode: 0,
			StdoutSHA256: "a947d801afe54d4c2d2c048ee853cca4ab4a5f08921698063ba6d287b524b5a8",
			StderrSHA256: "f7718a0427ded6fc8775c92ab79be7a8f6673fcf9a3c6b85274087c5838281dc",
			OutputLimit:  4096,
		},
		Oracle: protocol.Oracle{Kind: "exact_command"},
	}
	runtime := Runtime{ID: "wasmtime", Command: []string{adapter}, Files: map[string]string{adapter: adapterHash}}
	lock, err := NewLock(Options{
		Suite: "component-smoke", Profile: "timing", Scenarios: []string{"first-call"},
		Launches: 1, Samples: 2, Operations: 1, Warmup: 3, Seed: 1, Timeout: 30 * time.Second,
	}, []Runtime{runtime}, []protocol.Workload{workload})
	if err != nil {
		t.Fatal(err)
	}
	lock.Analyzer = analyzerLock
	lock.ArchiveTools = true
	if err := ValidateLock(lock); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "component-run")
	path, err := Run(context.Background(), lock, filepath.Dir(artifact), out, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Trials) != 2 || len(bundle.Manifest.Order) != 1 {
		t.Fatalf("sealed component trial: %+v", bundle.Trials)
	}
	seenCheck, seenMeasurement := false, false
	for _, trial := range bundle.Trials {
		if trial.Status != "ok" || trial.Scenario != "first-call" || len(trial.Samples) != 2 {
			t.Fatalf("component trial did not complete with two verified samples: %+v", trial)
		}
		if len(trial.ID) >= 5 && trial.ID[:5] == "check" {
			seenCheck = true
		} else if trial.ID == "trial-000000" {
			seenMeasurement = true
		}
		for i, sample := range trial.Samples {
			if !sample.Verified || sample.CommandResult == nil || sample.CommandResult.StdoutSHA256 != workload.Command.StdoutSHA256 || sample.CommandResult.StderrSHA256 != workload.Command.StderrSHA256 {
				t.Fatalf("trial %s sample %d did not retain exact verified command evidence: %+v", trial.ID, i, sample)
			}
		}
	}
	if !seenCheck || !seenMeasurement {
		t.Fatalf("bundle lacks sacrificial correctness check or measured launch: %+v", bundle.Trials)
	}
}
