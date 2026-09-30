package corpus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/wasmbench/wasmbench/protocol"
)

func TestImportCommand(t *testing.T) {
	for _, mode := range []string{"valid", "normalize", "hash", "stdin", "unknown", "oversized", "path", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			base := t.TempDir()
			if err := os.WriteFile(filepath.Join(base, "input"), []byte("abc"), 0600); err != nil {
				t.Fatal(err)
			}
			c := map[string]any{"runtime": "wasi", "export": "_start", "preopen": ".", "inputs": map[string]string{"input": Hash([]byte("abc"))}, "stdin": "input", "stdout_sha256": Hash([]byte("abc"))}
			switch mode {
			case "hash":
				c["inputs"] = map[string]string{"input": Hash(nil)}
			case "stdin":
				c["stdin"] = "absent"
			case "unknown":
				c["stdout_normalize"] = "unknown"
			case "normalize":
				c["stdout_normalize"] = "llvm-ir-preds"
			case "oversized":
				data := make([]byte, protocol.CommandInputLimit+1)
				if err := os.WriteFile(filepath.Join(base, "input"), data, 0600); err != nil {
					t.Fatal(err)
				}
				c["inputs"] = map[string]string{"input": Hash(data)}
			case "path":
				c["preopen"] = "../outside"
			case "symlink":
				outside := filepath.Join(t.TempDir(), "secret")
				os.WriteFile(outside, []byte("abc"), 0600)
				os.Remove(filepath.Join(base, "input"))
				os.Symlink(outside, filepath.Join(base, "input"))
			}
			raw, _ := json.Marshal(c)
			w := protocol.Workload{ABI: "wasi-command", Reset: "fresh_instance_per_sample", Oracle: protocol.Oracle{Kind: "unsupported"}, UnsupportedReason: "unimplemented"}
			err := importWagoCommand(base, "test", raw, &w)
			if mode == "unknown" {
				if err != nil || w.Command != nil || w.UnsupportedReason == "" {
					t.Fatal(w, err)
				}
				return
			}
			if mode != "valid" && mode != "normalize" && mode != "oversized" {
				if err == nil {
					t.Fatal("accepted invalid fixtures")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if w.Command.StdinFile != "input" || len(w.Command.Stdin) != 0 || w.Command.Files["input"].Path == "" || len(w.Command.Files["input"].Data) != 0 || w.Command.Argv[0] != "test" || w.Oracle.Kind != "exact_command" {
				t.Fatal(w)
			}
			if mode == "normalize" && w.Command.StdoutNormalize != "llvm-ir-preds" {
				t.Fatalf("lost stdout normalizer: %+v", w.Command)
			}
		})
	}
}

func TestImportEmscriptenCommand(t *testing.T) {
	stdout := Hash([]byte("expected output\n"))
	raw, err := json.Marshal(map[string]any{
		"runtime": "emscripten", "export": "main", "args": []string{"--flag"}, "stdout_sha256": stdout,
	})
	if err != nil {
		t.Fatal(err)
	}
	w := protocol.Workload{ABI: "emscripten", Reset: "fresh_instance_per_sample", Oracle: protocol.Oracle{Kind: "unsupported"}, UnsupportedReason: "unimplemented"}
	if err := importWagoCommand(t.TempDir(), "fixture", raw, &w); err != nil {
		t.Fatal(err)
	}
	if w.ABI != "emscripten" || w.Export != "main" || w.HostProfile != protocol.EmscriptenStdioProfile || w.Oracle.Kind != "exact_command" || w.UnsupportedReason != "" {
		t.Fatalf("Emscripten command contract was not admitted explicitly: %+v", w)
	}
}
