package protocol_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wasmbench/wasmbench/protocol"
)

// Opt-in real-engine evidence check. Run only after the measurement recipe
// terminates, so compiling/running tests cannot perturb its timed operations.
func TestNativeProcessSnapshotWorkerEvidence(t *testing.T) {
	root := os.Getenv("WASMBENCH_SNAPSHOT_WORKER_EVIDENCE")
	if root == "" {
		t.Skip("requires completed native Linux worker evidence")
	}
	w := snapshotWorkload(t)
	for _, backend := range []string{"cranelift", "winch"} {
		for _, stage := range protocol.ProcessSnapshotScenarios() {
			name := backend + "-" + strings.TrimPrefix(stage, "process-snapshot-") + ".json"
			t.Run(name, func(t *testing.T) {
				data, err := os.ReadFile(filepath.Join(root, name))
				if err != nil {
					t.Fatal(err)
				}
				var record struct {
					Version        string            `json:"version"`
					Runtime        string            `json:"runtime"`
					RuntimeVersion string            `json:"runtime_version"`
					Backend        string            `json:"backend"`
					Scenario       string            `json:"scenario"`
					SHA256         string            `json:"wasm_sha256"`
					Development    bool              `json:"development_worker"`
					Registered     bool              `json:"registered_adapter"`
					Samples        []protocol.Sample `json:"samples"`
				}
				if err := json.Unmarshal(data, &record); err != nil {
					t.Fatal(err)
				}
				if record.Version != "linux-process-snapshot-timing-worker-v1" || record.Runtime != "wasmtime" || record.RuntimeVersion != "46.0.1" || record.Backend != backend || record.Scenario != stage || record.SHA256 != protocol.ProcessSnapshotArtifactSHA256 || !record.Development || record.Registered {
					t.Fatal("unexpected native worker identity or scope")
				}
				r := protocol.RunRequest{Scenario: stage, Samples: 2, Operations: 1}
				if err := protocol.VerifyProcessSnapshotSequence(w, r, record.Samples); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
