package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wasmbench/wasmbench/experiment"
)

func TestPlanAdmissionDefaultAndLockedPolicy(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	// Planning pins bytes without executing them. Execution tests live in
	// experiment and use the actual built analyzer and adapters.
	for _, path := range []string{"bin/adapter-wazero", "adapters/wasmtime/target/release/wasm-analyze"} {
		path = experiment.NativeExecutable(path)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("planning-only executable fixture"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	for _, profile := range []string{"default", "wasm1", "wasm2", "wasm3"} {
		args := []string{"plan", "--runtimes", "wazero", "--out", profile + ".lock"}
		if profile != "default" {
			args = append(args, "--validation-profile", profile)
		}
		if err := run(ctx, args); err != nil {
			t.Fatal(err)
		}
		var lock experiment.Lock
		if err := experiment.ReadJSON(profile+".lock", &lock); err != nil {
			t.Fatal(err)
		}
		if lock.Analyzer == nil || lock.Analyzer.Profile != profile || lock.Analyzer.SHA256 == "" {
			t.Fatalf("%+v", lock.Analyzer)
		}
		if !lock.ArchiveTools {
			t.Fatal("new plan did not preserve tools by default")
		}
		if err := run(ctx, []string{"plan", "--lock", profile + ".lock", "--out", profile + "-copy.lock"}); err != nil {
			t.Fatal(err)
		}
		var copied experiment.Lock
		if err := experiment.ReadJSON(profile+"-copy.lock", &copied); err != nil {
			t.Fatal(err)
		}
		if *copied.Analyzer != *lock.Analyzer {
			t.Fatal("changed locked analyzer")
		}
		if copied.ArchiveTools != lock.ArchiveTools {
			t.Fatal("changed archive policy")
		}
	}
	if err := run(ctx, []string{"plan", "--runtimes", "wazero", "--archive-tools=false", "--out", "no-archive.lock"}); err != nil {
		t.Fatal(err)
	}
	var noArchive experiment.Lock
	if err := experiment.ReadJSON("no-archive.lock", &noArchive); err != nil || noArchive.ArchiveTools {
		t.Fatal("opt-out ignored", err)
	}
	for _, value := range []string{"true", "false"} {
		if err := run(ctx, []string{"plan", "--lock", "default.lock", "--archive-tools=" + value}); err == nil || !strings.Contains(err.Error(), "cannot override") {
			t.Fatal(err)
		}
	}
	for _, profile := range []string{"", "bogus"} {
		if err := run(ctx, []string{"plan", "--runtimes", "wazero", "--validation-profile", profile, "--out", "bad.lock"}); err == nil {
			t.Fatal("accepted invalid policy")
		}
	}
	for _, profile := range []string{"", "default", "wasm1"} {
		if err := run(ctx, []string{"plan", "--lock", "default.lock", "--validation-profile", profile}); err == nil || !strings.Contains(err.Error(), "cannot override") {
			t.Fatal(err)
		}
	}
	var legacy experiment.Lock
	if err := experiment.ReadJSON("default.lock", &legacy); err != nil {
		t.Fatal(err)
	}
	legacy.Analyzer = nil
	if err := experiment.WriteJSON("legacy.lock", legacy); err != nil {
		t.Fatal(err)
	}
	if err := run(ctx, []string{"plan", "--lock", "legacy.lock", "--out", "legacy-copy.lock"}); err != nil {
		t.Fatal(err)
	}
	if err := experiment.ReadJSON("legacy-copy.lock", &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.Analyzer != nil {
		t.Fatal("silently upgraded legacy lock")
	}
	if err := os.Remove(experiment.NativeExecutable("adapters/wasmtime/target/release/wasm-analyze")); err != nil {
		t.Fatal(err)
	}
	if err := run(ctx, []string{"plan", "--runtimes", "wazero", "--out", "missing.lock"}); err == nil || !strings.Contains(err.Error(), "make build") {
		t.Fatal(err)
	}
	if _, err := os.Stat("missing.lock"); !os.IsNotExist(err) {
		t.Fatal("wrote plan without analyzer")
	}
}
