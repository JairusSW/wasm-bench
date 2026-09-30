package experiment

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/corpus"
)

func TestRecordedProducerPathDialects(t *testing.T) {
	for _, source := range []string{`C:\Program Files\nodejs\node.exe`, `D:/repo/adapters/v8/adapter.mjs`, `\\server\share\repo\adapter.exe`, "/opt/tools/adapter", "/opt/path with spaces/adapter"} {
		if _, err := parseRecordedPath(source); err != nil {
			t.Fatal(source, err)
		}
	}
	for _, source := range []string{"relative", "C:relative.exe", `\relative.exe`, `\\?\C:\adapter.exe`, `\\.\pipe\adapter`, `C:\foo\..\adapter.exe`, `C:\foo\adapter.exe:stream`, `C:\foo\NUL.exe`, `C:\foo\adapter.`, `C:\foo\adapter `, `\\server\share`, "/tmp/../adapter", "/tmp//adapter", "/", "C:/", "C:/foo\x00.exe"} {
		if _, err := parseRecordedPath(source); err == nil {
			t.Fatal("unsafe producer path accepted", source)
		}
	}
}

func TestPortableArchiveProducerTrees(t *testing.T) {
	for _, tc := range []struct {
		keys []string
		want map[string]string
	}{
		{[]string{"/repo/js/adapter.mjs", "/repo/js/helper.mjs", "/repo/bin/node"}, map[string]string{"/repo/js/adapter.mjs": "js/adapter.mjs", "/repo/js/helper.mjs": "js/helper.mjs", "/repo/bin/node": "bin/node"}},
		{[]string{`C:\node\node.exe`, `D:\repo\js\adapter.mjs`, `D:\repo\js\helper.mjs`}, map[string]string{`C:\node\node.exe`: "volume-0/node.exe", `D:\repo\js\adapter.mjs`: "volume-1/adapter.mjs", `D:\repo\js\helper.mjs`: "volume-1/helper.mjs"}},
		{[]string{`\\server\share\js\adapter.mjs`, `\\server\share\js\helper.mjs`}, map[string]string{`\\server\share\js\adapter.mjs`: "adapter.mjs", `\\server\share\js\helper.mjs`: "helper.mjs"}},
	} {
		got, err := archiveRelativePaths(tc.keys)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Fatal(got, tc.want, err)
		}
	}
	for _, keys := range [][]string{{"/repo/adapter", `C:\repo\helper`}, {`C:\repo\helper.mjs`, `c:\REPO\HELPER.mjs`}, {`C:\repo\helper.mjs`, `C:/repo/helper.mjs`}, {`C:\repo\Σ.mjs`, `C:\repo\ς.mjs`}} {
		if _, err := archiveRelativePaths(keys); err == nil {
			t.Fatal("mixed or colliding producer tree accepted", keys)
		}
	}
}

func TestOfflineWindowsArchiveOnAnyReader(t *testing.T) {
	original, l, source := archiveFixture(t)
	r := &l.Runtimes[0]
	node, script, helper := `C:\nodejs\node.exe`, `D:\repo\js\adapter.mjs`, `D:\repo\js\helper.mjs`
	refs := map[string]string{node: filepath.Join(source, "bin/node"), script: filepath.Join(source, "js/adapter.mjs"), helper: filepath.Join(source, "js/helper.mjs")}
	r.Command = []string{node, script, "--flag"}
	r.Files = map[string]string{}
	for recorded, local := range refs {
		r.Files[recorded], _ = DigestFile(local)
	}
	root := t.TempDir()
	entries, err := toolArchiveSpec(l)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		local := refs[entry.Source]
		if entry.Runtime == -1 {
			local = filepath.Join(source, "runner")
		}
		if err := copyTool(local, filepath.Join(root, entry.Relative), entry.SHA256, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := CopyExclusive(filepath.Join(original, "artifacts/test.wasm"), filepath.Join(root, "artifacts/test.wasm")); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(l)
	if err := WriteJSON(filepath.Join(root, "manifest.json"), Manifest{Lock: l, LockSHA256: corpus.Hash(raw), Host: agent.Host{OS: "windows", Arch: "amd64"}}); err != nil {
		t.Fatal(err)
	}
	if err := Seal(root); err != nil {
		t.Fatal(err)
	}
	before, _ := DigestFile(filepath.Join(root, "checksums.json"))
	if _, err := Load(root); err != nil {
		t.Fatal("portable offline Windows archive rejected", err)
	}
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		out := filepath.Join(t.TempDir(), "restore")
		if _, err := RestoreTools(root, out); err == nil || !strings.Contains(err.Error(), "recorded OS and architecture") {
			t.Fatal("incompatible reader allowed restoration", err)
		}
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			t.Fatal("wrote incompatible restoration output")
		}
	}
	after, _ := DigestFile(filepath.Join(root, "checksums.json"))
	if before != after {
		t.Fatal("offline read mutated archive")
	}
}

func TestPortableAnalyzerExecutableEvidence(t *testing.T) {
	a := &AnalyzerLock{Executable: `C:\tools\wasm-analyze.exe`, SHA256: strings.Repeat("a", 64), Profile: "default", Name: "wasmparser", Version: "0.251.0", AnalysisVersion: "artifact-structure-v1"}
	if err := a.validate(); err != nil {
		t.Fatal("producer analyzer identity rejected", err)
	}
	a.Executable = `C:wasm-analyze.exe`
	if a.validate() == nil {
		t.Fatal("drive-relative analyzer accepted")
	}
}
