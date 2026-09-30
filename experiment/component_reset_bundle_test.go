package experiment

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/protocol"
)

func componentResetFixture(t *testing.T) (protocol.Workload, []Runtime, *AnalyzerLock) {
	t.Helper()
	artifact := os.Getenv("WASMBENCH_COMPONENT_RESET_ARTIFACT")
	adapter := os.Getenv("WASMBENCH_COMPONENT_ADAPTER")
	analyzer := os.Getenv("WASMBENCH_COMPONENT_ANALYZER")
	if artifact == "" || adapter == "" || analyzer == "" {
		t.Skip("requires built P2 reset fixture, adapter and analyzer")
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
	input := []byte("pristine\n")
	w := protocol.Workload{Schema: 1, ID: "p2/filesystem-reset", Family: "applications", Artifact: artifact, SHA256: artifactHash, ABI: "component", HostProfile: "wasi-preview2-temporary-filesystem-v1", Features: []string{"component-model"}, Export: "_start", WorkUnit: "command", Units: 1, Reset: "fresh_instance_per_sample", License: "Apache-2.0", Source: "corpus/testdata/p2-filesystem-reset.rs", Generator: "rustc 1.98.1 wasm32-wasip2", Oracle: protocol.Oracle{Kind: "exact_command"}, Command: &protocol.CommandContract{Argv: []string{"p2-filesystem-reset"}, ExitCode: 0, OutputLimit: 4096, StdoutSHA256: protocol.CommandDigest([]byte("filesystem reset verified\n")), StderrSHA256: protocol.CommandDigest(nil), Files: map[string]protocol.CommandFile{"data/input.txt": {Data: input, SHA256: protocol.CommandDigest(input)}}}}
	runtimes := []Runtime{{ID: "wasmtime", Command: []string{adapter}, Files: map[string]string{adapter: adapterHash}}, {ID: "wasmtime-winch", Command: []string{adapter, "--winch"}, Files: map[string]string{adapter: adapterHash}}}
	return w, runtimes, analyzerLock
}

// Execute a real component that mutates its input and creates a sentinel.
// A reused host filesystem makes the next invocation trap, even with a fresh
// component Store/instance. Every sacrificial and measured sample must pass.
func TestComponentFilesystemResetBundle(t *testing.T) {
	w, runtimes, analyzerLock := componentResetFixture(t)
	artifact := w.Artifact
	// Establish that pristine single invocations work before checking batches.
	for _, r := range runtimes {
		c, err := agent.Start(context.Background(), r.Command, filepath.Join(t.TempDir(), "single.log"), 30*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = c.Call(protocol.Request{Method: "describe"}); err != nil {
			c.Close()
			t.Fatal(err)
		}
		if _, err = c.Call(protocol.Request{Method: "prepare", Prepare: &protocol.Preparation{Artifact: artifact, ArtifactSHA256: w.SHA256, Workload: w, Profile: "timing"}}); err != nil {
			c.Close()
			t.Fatal(err)
		}
		response, err := c.Call(protocol.Request{Method: "run", Run: &protocol.RunRequest{Scenario: "first-call", Samples: 1, Operations: 1}})
		closeErr := c.Close()
		if err != nil || closeErr != nil || len(response.Samples) != 1 || !response.Samples[0].Verified {
			t.Fatalf("single invocation %s failed: %v %v", r.ID, err, closeErr)
		}
		t.Logf("pristine single invocation passes for %s", r.ID)
	}
	lock, err := NewLock(Options{Suite: "component-reset", Profile: "timing", Scenarios: []string{"first-call"}, Launches: 2, Samples: 3, Operations: 1, Warmup: 0, Seed: 1, Timeout: 30 * time.Second}, runtimes, []protocol.Workload{w})
	if err != nil {
		t.Fatal(err)
	}
	lock.Analyzer = analyzerLock
	lock.ArchiveTools = true
	if err := ValidateLock(lock); err != nil {
		t.Fatal(err)
	}
	out := os.Getenv("WASMBENCH_COMPONENT_RESET_EVIDENCE_DIR")
	if out == "" {
		out = filepath.Join(t.TempDir(), "original")
	}
	path, err := Run(context.Background(), lock, filepath.Dir(artifact), out, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	record, err := RestoreTools(path, out+"-restored-tools")
	if err != nil {
		t.Fatal(err)
	}
	var restored Lock
	if err := ReadJSON(record.Lock, &restored); err != nil {
		t.Fatal(err)
	}
	for i, runtime := range restored.Runtimes {
		if runtime.Command[0] == runtimes[i].Command[0] {
			t.Fatal("replay must use the archived adapter")
		}
	}
	replayed, err := Run(context.Background(), restored, filepath.Dir(record.Lock), out+"-replayed", func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	for _, runPath := range []string{path, replayed} {
		b, err := Load(runPath)
		if err != nil {
			t.Fatal(err)
		}
		if len(b.Trials) != 6 {
			t.Fatalf("missing backend/launch coverage: %d", len(b.Trials))
		}
		for _, trial := range b.Trials {
			want := 3
			if trial.Block < 0 {
				want = 2
			}
			if trial.Status != "ok" || len(trial.Samples) != want {
				t.Fatalf("filesystem reset trial %s (%s) failed: %s %s; samples=%d", trial.ID, trial.Runtime, trial.Status, trial.Reason, len(trial.Samples))
			}
			for _, sample := range trial.Samples {
				if !sample.Verified || sample.CommandResult == nil || sample.CommandResult.StdoutSHA256 != w.Command.StdoutSHA256 || sample.CommandResult.StderrSHA256 != w.Command.StderrSHA256 {
					t.Fatal("missing exact output verification")
				}
			}
		}
	}
	if after, err := DigestFile(artifact); err != nil || after != w.SHA256 {
		t.Fatalf("guest execution modified the source component: %s %v", after, err)
	}
}
