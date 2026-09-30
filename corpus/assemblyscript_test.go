package corpus

import (
	"encoding/json"
	"github.com/wasmbench/wasmbench/protocol"
	"os"
	"path/filepath"
	"testing"
)

func TestAssemblyScriptFamilyHostContract(t *testing.T) {
	root := t.TempDir()
	data := Module("identity", 1)
	var entries []map[string]any
	for _, path := range []string{"workloads/assemblyscript/example.wasm", "other/example.wasm"} {
		name := filepath.Join(root, "corpus", path)
		if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, data, 0600); err != nil {
			t.Fatal(err)
		}
		entries = append(entries, map[string]any{"id": path, "artifact": path, "artifact_sha256": Hash(data), "init": "_initialize", "exec": []map[string]any{{"export": "benchmark", "args": []int{7}, "want": []int{7}}}})
	}
	b, err := json.Marshal(map[string]any{"schema": 1, "benchmarks": entries})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "corpus", "catalog.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	ws, err := ImportWago(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 2 || ws[0].HostProfile != protocol.AssemblyScriptAbortProfile || ws[1].HostProfile != "" {
		t.Fatal(ws)
	}
}
