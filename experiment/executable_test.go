package experiment

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNativeExecutableNames(t *testing.T) {
	for _, tc := range []struct{ path, goos, want string }{
		{"adapter-wazero", "windows", "adapter-wazero.exe"},
		{`C:\tools\adapter-wago`, "windows", `C:\tools\adapter-wago.exe`},
		{"wasm-analyze.exe", "windows", "wasm-analyze.exe"},
		{"Node.EXE", "windows", "Node.EXE"},
		{"wasmbench", "linux", "wasmbench"},
		{"wasmbench", "darwin", "wasmbench"},
	} {
		if got := ExecutableForOS(tc.path, tc.goos); got != tc.want {
			t.Fatalf("%s/%s: %q != %q", tc.goos, tc.path, got, tc.want)
		}
	}
}

func TestResolvedNativeRuntimeExecutableIsPinned(t *testing.T) {
	root := t.TempDir()
	paths := []string{
		filepath.Join(root, "bin", "adapter-wazero"),
		filepath.Join(root, "bin", "adapter-wago"),
		filepath.Join(root, "adapters", "wasmtime", "target", "release", "adapter-wasmtime"),
	}
	for _, path := range paths {
		path = NativeExecutable(path)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("planning-only fixture"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"wago", "wazero", "wazero-interpreter", "wasmtime", "wasmtime-winch"} {
		runtimes, err := ResolveRuntimes(root, []string{id})
		if err != nil {
			t.Fatal(err)
		}
		r := runtimes[0]
		if r.Files[r.Command[0]] == "" {
			t.Fatal("resolved executable not pinned", r)
		}
		if runtime.GOOS == "windows" && filepath.Ext(r.Command[0]) != ".exe" {
			t.Fatal("missing Windows suffix", r.Command)
		}
	}
}

func TestWindowsMultiVolumeArchivePreservesImports(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("native Windows path semantics")
	}
	paths := []string{`C:\Program Files\nodejs\node.exe`, `D:\repo\adapters\v8\adapter.mjs`, `D:\repo\adapters\v8\helper.mjs`}
	relative, err := archiveRelativePaths(paths)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(relative[paths[1]]) != filepath.Dir(relative[paths[2]]) || relative[paths[0]] == relative[paths[1]] {
		t.Fatal("cross-volume relocation lost script import layout", relative)
	}
	for _, rel := range relative {
		if !filepath.IsLocal(rel) {
			t.Fatal("escaping archive path", rel)
		}
	}
}
