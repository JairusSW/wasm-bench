package experiment

import (
	"context"
	"github.com/wasmbench/wasmbench/protocol"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidatorFeatureCompatibility(t *testing.T) {
	p := &protocol.ValidatorFeaturePolicy{Namespace: "wasmparser/0.251.0", Evidence: "pinned config", Supported: map[string]bool{"TAIL_CALL": false, "SIMD": true}}
	for _, tc := range []struct {
		name     string
		policy   *protocol.ValidatorFeaturePolicy
		required []string
		reject   bool
	}{
		{"explicit disabled", p, []string{"TAIL_CALL"}, true},
		{"enabled", p, []string{"SIMD"}, false},
		{"unknown", p, []string{"GC"}, false},
		{"no requirements", p, nil, false},
		{"no policy", nil, []string{"TAIL_CALL"}, false},
		{"wrong vocabulary", &protocol.ValidatorFeaturePolicy{Namespace: "other", Evidence: "config", Supported: p.Supported}, []string{"TAIL_CALL"}, false},
		{"no evidence", &protocol.ValidatorFeaturePolicy{Namespace: p.Namespace, Supported: p.Supported}, []string{"TAIL_CALL"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := incompatibleValidatorFeatures(tc.policy, tc.required); (got != "") != tc.reject {
				t.Fatalf("reason=%q reject=%v", got, tc.reject)
			}
		})
	}
	w := protocol.Workload{SHA256: "artifact", ABI: "core"}
	r := Runtime{ID: "test", Command: []string{"must-not-launch"}, Description: &protocol.Description{ABIs: []string{"core"}, ValidatorFeatures: p}}
	trial := runTrial(context.Background(), t.TempDir(), Options{requiredValidatorFeatures: map[string][]string{"artifact": {"TAIL_CALL"}}}, r, w, "compile", 0, "feature-check")
	if trial.Status != "unsupported" || !strings.Contains(trial.Reason, "TAIL_CALL") {
		t.Fatalf("%+v", trial)
	}
}

func TestBuiltFeatureAdmission(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	analyzer := filepath.Join(root, "adapters/wasmtime/target/release/wasm-analyze")
	for _, path := range []string{analyzer, filepath.Join(root, "bin/adapter-wazero")} {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Skip("build analyzer and adapter first")
		}
	}
	var workloads []protocol.Workload
	for _, name := range []string{"feature-tail-call", "analyzer-features", "feature-tail-call"} {
		path := filepath.Join(root, "corpus/testdata", name+".wasm")
		hash, err := DigestFile(path)
		if err != nil {
			t.Fatal(err)
		}
		id := name
		if len(workloads) == 2 {
			id += "-duplicate"
		}
		workloads = append(workloads, protocol.Workload{Schema: 1, ID: id, Family: "mechanisms", Artifact: path, SHA256: hash, ABI: "core", Export: "benchmark", WorkUnit: "invocation", Units: 1, Reset: "fresh_instance_per_sample", Oracle: protocol.Oracle{Kind: "exact_u64", Expected: protocol.Values{7}}, License: "MIT"})
	}
	ids := []string{"wazero", "wazero-interpreter"}
	if requested := os.Getenv("WASMBENCH_FEATURE_TEST_RUNTIMES"); requested != "" {
		ids = strings.Split(requested, ",")
	}
	runtimes, err := ResolveRuntimes(root, ids)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := NewLock(Options{Suite: "feature-admission", Profile: "timing", Scenarios: []string{"compile", "first-call"}, Launches: 2, Samples: 1, Operations: 1, Timeout: 10 * time.Second}, runtimes, workloads)
	if err != nil {
		t.Fatal(err)
	}
	lock.Analyzer, err = PinAnalyzer(analyzer, "default")
	if err != nil {
		t.Fatal(err)
	}
	base := t.TempDir()
	for i := 0; i < 2; i++ {
		out := filepath.Join(t.TempDir(), "run")
		if _, err := Run(context.Background(), lock, base, out, func(string) {}); err != nil {
			t.Fatal(err)
		}
		bundle, err := Load(out)
		if err != nil {
			t.Fatal(err)
		}
		if len(bundle.Trials) != 15*len(ids) { // three artifacts, one check plus four measured cells per runtime
			t.Fatal(len(bundle.Trials))
		}
		for _, trial := range bundle.Trials {
			tailDisabled := trial.Runtime == "wazero" || trial.Runtime == "wazero-interpreter" || trial.Runtime == "wasmtime-winch"
			if strings.HasPrefix(trial.Workload, "feature-tail-call") && tailDisabled {
				if trial.Status != "unsupported" || !strings.Contains(trial.Reason, "TAIL_CALL") {
					t.Fatalf("%+v", trial)
				}
				if len(trial.Samples) != 0 {
					t.Fatal("unsupported has samples")
				}
			} else if trial.Status != "ok" {
				t.Fatalf("%+v", trial)
			}
		}
		// Reproduction must derive the same decisions from newly validated bytes,
		// despite the private requirements not being serialized in the lock.
		lock, base = bundle.Manifest.Lock, out
	}
}

func TestRequiredValidatorFeatures(t *testing.T) {
	data := []byte(`{"analysis_version":"core-structure-v3","feature_probes":{"probes":[{"feature":"SIMD","valid_without":true},{"feature":"TAIL_CALL","valid_without":false}]}}`)
	got := requiredValidatorFeatures(data)
	if len(got) != 1 || got[0] != "TAIL_CALL" {
		t.Fatal(got)
	}
	if got := requiredValidatorFeatures([]byte(`{"analysis_version":"core-structure-v2"}`)); len(got) != 0 {
		t.Fatal(got)
	}
}
