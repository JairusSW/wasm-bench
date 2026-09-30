package experiment

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
)

func archiveFixture(t *testing.T) (string, Lock, string) {
	t.Helper()
	source := t.TempDir()
	write := func(name, data string) string {
		path := filepath.Join(source, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0755); err != nil {
			t.Fatal(err)
		}
		return path
	}
	runner := write("runner", "runner exact bytes")
	node := write("bin/node", "interpreter exact bytes")
	script := write("js/adapter.mjs", "import './helper.mjs';")
	helper := write("js/helper.mjs", "export const n = 42;")
	files := map[string]string{}
	for _, path := range []string{node, script, helper} {
		files[path], _ = DigestFile(path)
	}
	digest, _ := DigestFile(runner)
	root := t.TempDir()
	artifact := []byte{0, 97, 115, 109, 1, 0, 0, 0}
	if err := os.Mkdir(filepath.Join(root, "artifacts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "artifacts/test.wasm"), artifact, 0644); err != nil {
		t.Fatal(err)
	}
	l := Lock{ArchiveTools: true, RunnerSHA256: digest, Runtimes: []Runtime{{ID: "script", Command: []string{node, script, "--flag"}, Files: files}}, Workloads: []protocol.Workload{{Artifact: "artifacts/test.wasm", SHA256: corpus.Hash(artifact)}}}
	if err := archiveTools(l, runner, root); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(l)
	if err := WriteJSON(filepath.Join(root, "manifest.json"), Manifest{Lock: l, Host: agent.IdentifyHost(), LockSHA256: corpus.Hash(raw)}); err != nil {
		t.Fatal(err)
	}
	if err := Seal(root); err != nil {
		t.Fatal(err)
	}
	return root, l, source
}

func TestArchiveRestorationPreservesToolsAndImports(t *testing.T) {
	root, l, source := archiveFixture(t)
	// Simulate an upgraded installation; no original tool is needed thereafter.
	for path := range l.Runtimes[0].Files {
		if err := os.WriteFile(path, []byte("upgraded"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(source, "runner"), []byte("upgraded"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root); err != nil {
		t.Fatal(err)
	}
	before, _ := DigestFile(filepath.Join(root, "checksums.json"))
	out := filepath.Join(t.TempDir(), "replay")
	record, err := RestoreTools(root, out)
	if err != nil {
		t.Fatal(err)
	}
	var restored Lock
	if err = ReadJSON(record.Lock, &restored); err != nil {
		t.Fatal(err)
	}
	if err = VerifyInputs(restored, out); err != nil {
		t.Fatal(err)
	}
	r := restored.Runtimes[0]
	if r.Command[2] != "--flag" {
		t.Fatal(r.Command)
	}
	if _, err = os.Stat(filepath.Join(filepath.Dir(r.Command[1]), "helper.mjs")); err != nil {
		t.Fatal("relative import lost", err)
	}
	got, _ := DigestFile(record.Runner)
	if got != l.RunnerSHA256 || record.SourceChecksumsSHA256 != before {
		t.Fatal(record)
	}
	if _, err = RestoreTools(root, out); err == nil {
		t.Fatal("overwrote existing restoration")
	}
	if _, err = RestoreTools(root, filepath.Join(root, "replay")); err == nil {
		t.Fatal("modified source bundle")
	}
	after, _ := DigestFile(filepath.Join(root, "checksums.json"))
	if before != after {
		t.Fatal("source changed")
	}
	entries, _ := toolArchiveSpec(l)
	for _, e := range entries {
		info, err := os.Stat(filepath.Join(root, e.Relative))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0111 != 0 {
			t.Fatal("archived tool executable")
		}
	}
}

func TestArchiveValidationBindsLockEvenAfterReseal(t *testing.T) {
	root, l, _ := archiveFixture(t)
	entries, _ := toolArchiveSpec(l)
	path := filepath.Join(root, entries[0].Relative)
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("tampered"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root); err == nil {
		t.Fatal("accepted corrupt seal")
	}
	if err := os.Remove(filepath.Join(root, "checksums.json")); err != nil {
		t.Fatal(err)
	}
	if err := Seal(root); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root); err == nil {
		t.Fatal("accepted archive inconsistent with lock")
	}
}

func TestArchiveRejectsUnpinnedAndChangedTools(t *testing.T) {
	_, l, source := archiveFixture(t)
	if err := archiveTools(l, filepath.Join(source, "bin/node"), t.TempDir()); err == nil {
		t.Fatal("accepted wrong runner")
	}
	l.Runtimes[0].Command = append(l.Runtimes[0].Command, filepath.Join(source, "not-pinned"))
	if _, err := toolArchiveSpec(l); err == nil {
		t.Fatal("accepted unpinned command file")
	}
	l.Runtimes[0].Command = l.Runtimes[0].Command[:2]
	l.Runtimes[0].Files["relative.mjs"] = l.RunnerSHA256
	if _, err := toolArchiveSpec(l); err == nil {
		t.Fatal("accepted ambiguous relative source")
	}
}
