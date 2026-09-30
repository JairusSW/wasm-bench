package experiment

import (
	"context"
	"encoding/json"
	"github.com/wasmbench/wasmbench/corpus"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAnalyzerEvidenceIdentity(t *testing.T) {
	a := &AnalyzerLock{Name: "wasmparser", Version: "0.251.0", AnalysisVersion: "core-structure-v2", Profile: "wasm2"}
	base := map[string]any{"schema": 2, "analyzer": a.Name, "analyzer_version": a.Version, "analysis_version": a.AnalysisVersion, "sha256": "digest", "validation_profile": a.Profile, "encoding": "core-module", "validated": true, "validation_features": []string{"SIMD"}}
	data, _ := json.Marshal(base)
	if err := a.validateResult(data, "digest"); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"schema", "analyzer", "analyzer_version", "analysis_version", "sha256", "validation_profile", "encoding", "validated", "validation_features"} {
		copy := map[string]any{}
		for k, v := range base {
			copy[k] = v
		}
		delete(copy, key)
		data, _ := json.Marshal(copy)
		if a.validateResult(data, "digest") == nil {
			t.Fatal("accepted missing identity", key)
		}
	}
	if a.validateResult([]byte("{} {}"), "digest") == nil {
		t.Fatal("accepted trailing data")
	}
	b := &boundedAnalysisOutput{limit: 3}
	if _, err := b.Write([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Write([]byte("d")); err == nil {
		t.Fatal("unbounded output")
	}
	if _, err := io.Copy(&boundedAnalysisOutput{limit: 3}, strings.NewReader("four")); err == nil {
		t.Fatal("copy bypassed output bound")
	}
}

func TestAnalyzerPinRejectsChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "analyzer")
	if err := os.WriteFile(path, []byte("original"), 0700); err != nil {
		t.Fatal(err)
	}
	a, err := PinAnalyzer(path, "wasm1")
	if err != nil {
		t.Fatal(err)
	}
	if err = a.verify(); err != nil {
		t.Fatal(err)
	}
	if _, err = PinAnalyzer(path, "unknown"); err == nil {
		t.Fatal("unknown policy")
	}
	if err = os.WriteFile(path, []byte("changed"), 0700); err != nil {
		t.Fatal(err)
	}
	if a.verify() == nil {
		t.Fatal("changed executable accepted")
	}
}

func TestFeatureProbeEvidence(t *testing.T) {
	base := `{"method":"single-flag-removal-v1","policy_bits":"3","probes":[{"feature":"A","disabled_bits":"1","remaining_bits":"2","valid_without":false,"failure":"instruction requires A","failure_offset":0},{"feature":"B","disabled_bits":"2","remaining_bits":"1","valid_without":true,"failure":null,"failure_offset":null}]}`
	for _, tc := range []struct{ name, old, new string }{
		{"correct", "", ""},
		{"wrong method", "single-flag-removal-v1", "unknown"},
		{"bad mask", `"disabled_bits":"1"`, `"disabled_bits":"4"`},
		{"wrong remainder", `"remaining_bits":"2"`, `"remaining_bits":"3"`},
		{"duplicate", `"feature":"B"`, `"feature":"A"`},
		{"no outcome", `"valid_without":false`, `"valid_without":null`},
		{"no witness", `"failure_offset":0`, `"failure_offset":null`},
		{"contradiction", `"valid_without":false`, `"valid_without":true`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := base
			if tc.old != "" {
				data = strings.Replace(data, tc.old, tc.new, 1)
			}
			var p featureProbeEvidence
			if err := json.Unmarshal([]byte(data), &p); err != nil {
				t.Fatal(err)
			}
			err := p.validate([]string{"A", "B"})
			if (err == nil) != (tc.name == "correct") {
				t.Fatal(err)
			}
		})
	}
	var absent *featureProbeEvidence
	if absent.validate([]string{"A"}) == nil {
		t.Fatal("missing probes accepted")
	}
}

func TestIndependentAdmissionBundle(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	analyzer := filepath.Join(root, "adapters", "wasmtime", "target", "release", "wasm-analyze")
	adapter := filepath.Join(root, "bin", "adapter-wazero")
	for _, path := range []string{analyzer, adapter} {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Skip("build analyzer and wazero adapter")
		}
	}
	tmp := t.TempDir()
	workloads, err := corpus.Generate(filepath.Join(tmp, "corpus"), "core")
	if err != nil {
		t.Fatal(err)
	}
	runtimes, err := ResolveRuntimes(root, []string{"wazero"})
	if err != nil {
		t.Fatal(err)
	}
	lock, err := NewLock(Options{Suite: "core", Profile: "timing", Scenarios: []string{"first-call"}, Launches: 1, Samples: 1, Operations: 1, Timeout: 10 * time.Second}, runtimes, workloads)
	if err != nil {
		t.Fatal(err)
	}
	lock.Analyzer, err = PinAnalyzer(analyzer, "wasm1")
	if err != nil {
		t.Fatal(err)
	}
	out, err := Run(context.Background(), lock, tmp, filepath.Join(tmp, "run"), func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := Load(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, trial := range bundle.Trials {
		if trial.Status != "ok" {
			t.Fatal(trial)
		}
	}
	if bundle.Manifest.Lock.Analyzer.SHA256 != lock.Analyzer.SHA256 {
		t.Fatal("lost analyzer pin")
	}
	// Re-run the saved lock against its portable artifact paths.
	if _, err = Run(context.Background(), bundle.Manifest.Lock, out, filepath.Join(tmp, "reproduced"), func(string) {}); err != nil {
		t.Fatal(err)
	}
	// A correctly sealed but semantically wrong report must still be rejected.
	path := filepath.Join(out, "validation", workloads[0].SHA256+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), `"validation_profile": "wasm1"`, `"validation_profile": "wasm2"`, 1))
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(filepath.Join(out, "checksums.json")); err != nil {
		t.Fatal(err)
	}
	if err = Seal(out); err != nil {
		t.Fatal(err)
	}
	if _, err = Load(out); err == nil {
		t.Fatal("mismatched policy evidence accepted")
	}
	// Feature failure happens before any adapter process/trial is started.
	w := workloads[0]
	w.Artifact = filepath.Join(root, "corpus", "testdata", "analyzer-signatures.wasm")
	w.SHA256, err = DigestFile(w.Artifact)
	if err != nil {
		t.Fatal(err)
	}
	lock.Workloads = lock.Workloads[:1]
	lock.Workloads[0] = w
	bad := filepath.Join(tmp, "rejected")
	if _, err = Run(context.Background(), lock, tmp, bad, func(string) {}); err == nil {
		t.Fatal("multi-value admitted under wasm1")
	}
	entries, err := os.ReadDir(filepath.Join(bad, "logs"))
	if err != nil || len(entries) != 0 {
		t.Fatal("adapter started before admission", entries, err)
	}
}
