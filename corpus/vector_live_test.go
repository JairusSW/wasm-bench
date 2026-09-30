package corpus_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
)

// This optional test uses exact upstream digests and contracts without copying
// their expected results into the test or treating a successful call as proof.
func TestWagoPublishedVectors(t *testing.T) {
	root := os.Getenv("WASMBENCH_WAGO_CORPUS")
	if root == "" {
		t.Skip("set WASMBENCH_WAGO_CORPUS to an existing Wago checkout")
	}
	workloads, err := corpus.ImportWago(root, []string{"blake3"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	count := 0
	for _, w := range workloads {
		var contract struct {
			Invoke struct {
				Export  string                  `json:"export"`
				Vectors protocol.VectorContract `json:"vectors"`
			} `json:"invoke"`
		}
		if err := json.Unmarshal(w.OriginalContract, &contract); err != nil {
			t.Fatal(err)
		}
		t.Run(w.ID, func(t *testing.T) {
			p, err := protocol.PrepareVectors(contract.Invoke.Vectors, 64<<20)
			if err != nil {
				t.Fatal(err)
			}
			wasm, err := os.ReadFile(w.Artifact)
			if err != nil {
				t.Fatal(err)
			}
			if corpus.Hash(wasm) != w.SHA256 {
				t.Fatal("artifact changed after import")
			}
			compiled, err := r.CompileModule(ctx, wasm)
			if err != nil {
				t.Fatal(err)
			}
			defer compiled.Close(ctx)
			for repeat := 0; repeat < 2; repeat++ {
				m, err := r.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName("").WithStartFunctions())
				if err != nil {
					t.Fatal(err)
				}
				f := m.ExportedFunction(contract.Invoke.Export)
				if f == nil || m.Memory() == nil {
					m.Close(ctx)
					t.Fatal("missing vector exports")
				}
				err = p.Run(protocol.VectorInstance{
					Pointer: func(name string) ([]uint64, error) {
						f := m.ExportedFunction(name)
						if f == nil {
							return nil, fmt.Errorf("missing pointer %s", name)
						}
						return f.Call(ctx)
					},
					Write:  m.Memory().Write,
					Invoke: func(in, n, out uint32) error { _, err := f.Call(ctx, uint64(in), uint64(n), uint64(out)); return err },
					Read:   m.Memory().Read,
				})
				m.Close(ctx)
				if err != nil {
					t.Fatal(err)
				}
				count += len(contract.Invoke.Vectors.Cases)
			}
		})
	}
	if count != 210 {
		t.Fatalf("checked %d vector results, expected 3 modes * 35 cases * 2 fresh instances", count)
	}
}
