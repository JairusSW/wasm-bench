package experiment

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wasmbench/wasmbench/protocol"
)

func TestPortableCommandFiles(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	data := []byte("portable input")
	if err := os.WriteFile(source, data, 0600); err != nil {
		t.Fatal(err)
	}
	w := protocol.Workload{ABI: "wasi-command", Export: "_start", Reset: "fresh_instance_per_sample", HostProfile: "wasi-preview1-readonly-v1", Oracle: protocol.Oracle{Kind: "exact_command"}, Command: &protocol.CommandContract{Argv: []string{"test"}, StdinFile: "input", StdoutSHA256: protocol.CommandDigest(data), OutputLimit: 100, Files: map[string]protocol.CommandFile{"input": {Path: source, Size: uint64(len(data)), SHA256: protocol.CommandDigest(data)}}}}
	if err := verifyCommandFiles(w, ""); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "bundle")
	if err := bundleCommandFiles(&w, "", out); err != nil {
		t.Fatal(err)
	}
	rel := w.Command.Files["input"].Path
	if filepath.IsAbs(rel) || !filepath.IsLocal(rel) {
		t.Fatal("not portable", rel)
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if err := Seal(out); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(root, "moved")
	if err := os.Rename(out, moved); err != nil {
		t.Fatal(err)
	}
	if err := Verify(moved); err != nil {
		t.Fatal(err)
	}
	if err := verifyCommandFiles(w, moved); err != nil {
		t.Fatal(err)
	}
	prepared := resolveCommandFiles(w, moved)
	if prepared.Command.Files["input"].Path != filepath.Join(moved, rel) || w.Command.Files["input"].Path != rel {
		t.Fatal("resolution mutated locked workload")
	}
	if err := os.WriteFile(filepath.Join(moved, rel), []byte("changed input!"), 0600); err != nil {
		t.Fatal(err)
	}
	if verifyCommandFiles(w, moved) == nil || Verify(moved) == nil {
		t.Fatal("accepted changed bundle input")
	}
}
