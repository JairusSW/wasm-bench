//go:build linux || darwin

package sourcebuild

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

func fixture(t *testing.T) (Recipe, string, *experiment.AnalyzerLock) {
	t.Helper()
	dir := t.TempDir()
	write := func(name, data string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(data), 0700); err != nil {
			t.Fatal(err)
		}
		return path
	}
	write("source", "source fixture")
	// This protocol stub is not a Wasm validator; real validation is separately
	// covered by the opt-in LLVM integration test below.
	report, _ := json.Marshal(map[string]any{
		"schema": 2, "analyzer": "wasmparser", "analyzer_version": "0.251.0",
		"analysis_version": "core-structure-v2", "sha256": corpus.Hash([]byte("\x00asm\x01\x00\x00\x00")),
		"validation_profile": "wasm1", "encoding": "core-module", "validated": true,
		"validation_features": []string{"MVP"},
	})
	a, err := experiment.PinAnalyzer(write("analyzer", "#!/bin/sh\nprintf '%s' '"+string(report)+"'\n"), "wasm1")
	if err != nil {
		t.Fatal(err)
	}
	a.AnalysisVersion = "core-structure-v2"
	r := Recipe{Schema: 1, ID: "fixture", SourceRevision: "fixture-v1", License: "MIT",
		Inputs: map[string]File{"source": {Path: "source"}}, Tools: map[string]File{"compiler": {Path: "/bin/sh"}},
		Steps: []Step{{Tool: "compiler", Args: []string{"-c", "printf '\\000asm\\001\\000\\000\\000' > module.wasm"}}}, Output: "module.wasm",
		Workload: protocol.Workload{Schema: 1, ID: "fixture", ABI: "core", Export: "benchmark", WorkUnit: "invocation", Units: 1, Reset: "stateless", Oracle: protocol.Oracle{Kind: "exact_u64", Expected: protocol.Values{1}}},
	}
	return r, dir, a
}

func TestBuildEvidence(t *testing.T) {
	r, dir, a := fixture(t)
	l, err := Pin(r, dir, a)
	if err != nil {
		t.Fatal(err)
	}
	if r.Inputs["source"].Path != "source" || r.Inputs["source"].SHA256 != "" {
		t.Fatal("Pin mutated recipe")
	}
	out := filepath.Join(dir, "bundle")
	result, err := Build(context.Background(), l, out, time.Second*10)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "validated_not_correctness_checked" || len(result.Steps) != 1 || result.LockSHA256 == "" {
		t.Fatal(result)
	}
	if err := experiment.Verify(out); err != nil {
		t.Fatal(err)
	}
	var suite []protocol.Workload
	if err := experiment.ReadJSON(filepath.Join(out, "suite.json"), &suite); err != nil {
		t.Fatal(err)
	}
	if len(suite) != 1 || suite[0].SHA256 != result.ArtifactSHA256 || !strings.Contains(string(suite[0].Provenance), `"lock_sha256"`) {
		t.Fatal(suite)
	}
	entries, _ := os.ReadDir(out)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "work-") {
			t.Fatal("scratch not cleaned")
		}
	}
	if _, err := Build(context.Background(), l, out, time.Second); err == nil {
		t.Fatal("overwrote bundle")
	}
	if err := experiment.Verify(out); err != nil {
		t.Fatal("existing evidence damaged", err)
	}
	if err := os.Remove(filepath.Join(dir, "source")); err != nil {
		t.Fatal(err)
	}
	replayed, err := Rebuild(context.Background(), out, filepath.Join(dir, "replayed"), 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.ArtifactSHA256 != result.ArtifactSHA256 || replayed.LockSHA256 != result.LockSHA256 || replayed.ReproducesSHA256 != result.ArtifactSHA256 {
		t.Fatal("lost reproduction identity", replayed)
	}
	if err := experiment.Verify(filepath.Join(dir, "replayed")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "sources/source"), []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if experiment.Verify(out) == nil {
		t.Fatal("source snapshot tampering accepted")
	}
	if _, err := Rebuild(context.Background(), out, filepath.Join(dir, "tampered-replay"), time.Second); err == nil {
		t.Fatal("tampered replay accepted")
	}
}

func TestRebuildRejectsResealedIdentityMismatch(t *testing.T) {
	r, dir, a := fixture(t)
	l, err := Pin(r, dir, a)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "original")
	if _, err := Build(context.Background(), l, out, time.Second*10); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "module.wasm"), []byte("different"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(out, "checksums.json")); err != nil {
		t.Fatal(err)
	}
	if err := experiment.Seal(out); err != nil {
		t.Fatal(err)
	}
	if _, err := Rebuild(context.Background(), out, filepath.Join(dir, "replayed"), time.Second); err == nil {
		t.Fatal("resealed inconsistent artifact accepted")
	}
}

func TestBuildFailures(t *testing.T) {
	for _, tc := range []struct {
		name, command string
		timeout       time.Duration
	}{
		{"exit", "echo failure >&2; exit 7", time.Second * 10},
		{"missing", "true", time.Second * 10},
		{"symlink", "/bin/ln -s source module.wasm", time.Second * 10},
		{"timeout", "while :; do :; done", 100 * time.Millisecond},
		{"log overflow", "i=0; while [ $i -lt 20000 ]; do printf '%0100d' 0; i=$((i+1)); done", time.Second * 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, dir, a := fixture(t)
			r.Steps[0].Args[1] = tc.command
			l, err := Pin(r, dir, a)
			if err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(dir, "failed")
			result, err := Build(context.Background(), l, out, tc.timeout)
			if err == nil || result.Status != "incomplete" {
				t.Fatal(result, err)
			}
			if _, err := os.Stat(filepath.Join(out, "checksums.json")); !os.IsNotExist(err) {
				t.Fatal("failed build sealed")
			}
			entries, _ := os.ReadDir(out)
			for _, e := range entries {
				if strings.HasPrefix(e.Name(), "work-") {
					t.Fatal("failed scratch not cleaned")
				}
			}
		})
	}
}

func TestPinsAndRecipeRejection(t *testing.T) {
	r, dir, a := fixture(t)
	l, err := Pin(r, dir, a)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "source"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "no-build")
	if _, err := Build(context.Background(), l, out, time.Second); err == nil {
		t.Fatal("changed source accepted")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("wrote before pin verification")
	}
	for _, path := range []string{"../outside", "/absolute", ".", ".tmp", ".tmp/file", "a/../source", "module.wasm"} {
		r.Inputs = map[string]File{path: {Path: "source"}}
		if r.validate() == nil {
			t.Fatal("accepted path", path)
		}
	}
	r, dir, a = fixture(t)
	r.Environment = map[string]string{"HOME": "override"}
	if r.validate() == nil {
		t.Fatal("reserved environment accepted")
	}
	r.Environment = nil
	a.Profile = "not-a-policy"
	if _, err := Pin(r, dir, a); err == nil {
		t.Fatal("malformed analyzer accepted")
	}
}

func TestPinPreservesDriverName(t *testing.T) {
	r, dir, a := fixture(t)
	driver := filepath.Join(dir, "wasm-driver")
	if err := os.Symlink("/bin/sh", driver); err != nil {
		t.Fatal(err)
	}
	r.Tools["compiler"] = File{Path: driver}
	l, err := Pin(r, dir, a)
	if err != nil {
		t.Fatal(err)
	}
	if l.Recipe.Tools["compiler"].Path != driver {
		t.Fatal("lost multi-call driver invocation name")
	}
	if err := verifyFile(l.Recipe.Tools["compiler"]); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(driver); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(driver, []byte("changed executable"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := verifyFile(l.Recipe.Tools["compiler"]); err == nil {
		t.Fatal("changed driver bytes accepted")
	}
}

func TestLLVMIntegration(t *testing.T) {
	if os.Getenv("WASMBENCH_SOURCE_LLVM_TEST") != "1" {
		t.Skip("set WASMBENCH_SOURCE_LLVM_TEST=1 with recipe LLVM paths and built analyzer")
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	var r Recipe
	if err := experiment.ReadJSON(filepath.Join(root, "recipes/source/xorshift-llvm.json"), &r); err != nil {
		t.Fatal(err)
	}
	a, err := experiment.PinAnalyzer(filepath.Join(root, "adapters/wasmtime/target/release/wasm-analyze"), "wasm1")
	if err != nil {
		t.Fatal(err)
	}
	l, err := Pin(r, filepath.Join(root, "recipes/source"), a)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	first, err := Build(context.Background(), l, filepath.Join(dir, "first"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Build(context.Background(), l, filepath.Join(dir, "second"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if first.ArtifactSHA256 != second.ArtifactSHA256 {
		t.Fatal("non-reproducible output", first.ArtifactSHA256, second.ArtifactSHA256)
	}
	for _, name := range []string{"first", "second"} {
		if err := experiment.Verify(filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
}
