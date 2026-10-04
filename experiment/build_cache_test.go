package experiment

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestWagoBuildCacheReuseAndInvalidation(t *testing.T) {
	root, source := t.TempDir(), t.TempDir()
	t.Setenv("WASMBENCH_TOOL_CACHE", t.TempDir())
	for _, tree := range []string{root, source} {
		if out, err := exec.Command("git", "init", tree).CombinedOutput(); err != nil {
			t.Fatalf("%s: %v", out, err)
		}
		if err := os.WriteFile(filepath.Join(tree, "main.go"), []byte("package main"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	calls := 0
	build := func(path string) error { calls++; return os.WriteFile(path, []byte("compiled adapter"), 0755) }
	run := func() {
		t.Helper()
		if err := cachedWagoBuild(context.Background(), root, source, "revision", "deps", build); err != nil {
			t.Fatal(err)
		}
	}
	run()
	run()
	if calls != 1 {
		t.Fatalf("unchanged adapter rebuilt %d times", calls)
	}
	if err := os.WriteFile(filepath.Join(source, "main.go"), []byte("package changed"), 0644); err != nil {
		t.Fatal(err)
	}
	run()
	if calls != 2 {
		t.Fatal("source change did not invalidate cache")
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package adapter_changed"), 0644); err != nil {
		t.Fatal(err)
	}
	run()
	if calls != 3 {
		t.Fatal("adapter change did not invalidate cache")
	}
	binary := NativeExecutable(filepath.Join(root, "bin", "adapter-wago"))
	if err := os.Chmod(binary, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binary, []byte("corrupt"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := cachedWagoBuild(context.Background(), root, source, "revision", "deps", build); err == nil {
		t.Fatal("accepted changed cached adapter")
	}
}
