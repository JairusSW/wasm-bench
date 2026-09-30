package corpus

import (
	"context"
	"encoding/json"
	"github.com/tetratelabs/wazero"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestGeneratedCorpusOracles(t *testing.T) {
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	if _, e := r.NewHostModuleBuilder("wasmbench").NewFunctionBuilder().WithFunc(func(v uint32) uint32 { return v }).Export("identity").Instantiate(ctx); e != nil {
		t.Fatal(e)
	}
	for _, suite := range []string{"core", "scaling"} {
		workloads, e := Generate(t.TempDir(), suite)
		if e != nil {
			t.Fatal(e)
		}
		for _, w := range workloads {
			t.Run(w.ID, func(t *testing.T) {
				b, e := os.ReadFile(w.Artifact)
				if e != nil {
					t.Fatal(e)
				}
				structure, e := Analyze(b)
				if e != nil {
					t.Fatal(e)
				}
				if structure.SHA256 != w.SHA256 || len(structure.FunctionBodyBytes) == 0 {
					t.Fatal("invalid structure")
				}
				m, e := r.InstantiateWithConfig(ctx, b, wazero.NewModuleConfig().WithName(""))
				if e != nil {
					t.Fatal(e)
				}
				defer m.Close(ctx)
				got, e := m.ExportedFunction(w.Export).Call(ctx, w.Args...)
				if e != nil {
					t.Fatal(e)
				}
				if !slices.Equal(got, w.Oracle.Expected) {
					t.Fatalf("got %v want %v", got, w.Oracle.Expected)
				}
			})
		}
	}
}
func TestAnalyzerRejectsTruncation(t *testing.T) {
	b := Module("sum", 1)
	for _, data := range [][]byte{nil, b[:7], append(append([]byte(nil), b[:8]...), 10, 0xff, 0xff, 0xff, 0xff, 0xff), b[:len(b)-1]} {
		if _, e := Analyze(data); e == nil {
			t.Fatal("accepted malformed module")
		}
	}
}
func TestWagoImportPreservesUnsupportedContracts(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "corpus")
	os.MkdirAll(base, 0755)
	wasm := Module("identity", 1)
	os.WriteFile(filepath.Join(base, "test.wasm"), wasm, 0644)
	catalog := map[string]any{"schema": 1, "benchmarks": []any{map[string]any{"id": "direct", "artifact": "test.wasm", "artifact_sha256": Hash(wasm), "exec": []any{map[string]any{"export": "benchmark", "args": []int{7}, "want": []int{7}}}}, map[string]any{"id": "vector", "artifact": "test.wasm", "artifact_sha256": Hash(wasm), "semantic_exec": []string{"vector/test"}}}, "checks": []any{map[string]any{"id": "vector/test", "invoke": map[string]any{"export": "benchmark", "vectors": map[string]int{"count": 3}}, "expect": map[string]any{"return": []string{"0x7"}}}}}
	b, _ := json.Marshal(catalog)
	os.WriteFile(filepath.Join(base, "catalog.json"), b, 0644)
	ws, e := ImportWago(root, nil)
	if e != nil {
		t.Fatal(e)
	}
	if len(ws) != 2 || ws[0].Oracle.Kind != "exact_u64" || ws[1].Oracle.Kind != "unsupported" || ws[1].UnsupportedReason == "" {
		t.Fatalf("lost contract: %+v", ws)
	}
	os.WriteFile(filepath.Join(base, "test.wasm"), []byte("changed"), 0644)
	if _, e = ImportWago(root, nil); e == nil {
		t.Fatal("accepted changed artifact")
	}
}

func TestWagoImportOutputPointer(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "corpus")
	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatal(err)
	}
	wasm := Module("identity", 1)
	if err := os.WriteFile(filepath.Join(base, "test.wasm"), wasm, 0644); err != nil {
		t.Fatal(err)
	}
	catalog := map[string]any{"schema": 1, "benchmarks": []any{map[string]any{"id": "pointer", "artifact": "test.wasm", "artifact_sha256": Hash(wasm), "semantic_exec": []string{"pointer/test"}}}, "checks": []any{map[string]any{"id": "pointer/test", "invoke": map[string]any{"export": "benchmark", "output_ptr_export": "output"}, "expect": map[string]any{"return": []string{"0x1"}, "memory": []any{map[string]any{"offset": 7, "hex": "abcd"}}}}}}
	invoke := catalog["checks"].([]any)[0].(map[string]any)["invoke"].(map[string]any)
	invoke["input"] = "0123"
	invoke["input_ptr_export"] = "input_buffer"
	b, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "catalog.json"), b, 0644); err != nil {
		t.Fatal(err)
	}
	ws, err := ImportWago(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 1 || ws[0].UnsupportedReason != "" || ws[0].Oracle.OutputPointerExport != "output" || ws[0].Oracle.Memory[0].Offset != 7 || ws[0].Oracle.Memory[0].Hex != "abcd" {
		t.Fatalf("lost pointer contract: %+v", ws)
	}
	if ws[0].Input == nil || ws[0].Input.Hex != "0123" || ws[0].Input.PointerExport != "input_buffer" {
		t.Fatalf("lost input contract: %+v", ws[0])
	}
}
