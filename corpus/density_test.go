package corpus

import (
	"context"
	"os"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/wasmbench/wasmbench/protocol"
)

func TestDensityFixtures(t *testing.T) {
	ws, err := Generate(t.TempDir(), "density")
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 16 {
		t.Fatalf("got %d contracts", len(ws))
	}
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	seen := map[string]bool{}
	groups := map[string]int{}
	for _, w := range ws {
		t.Run(w.ID, func(t *testing.T) {
			if err := protocol.ValidateDensityWorkload(w); err != nil {
				t.Fatal(err)
			}
			if seen[w.ID] {
				t.Fatal("duplicate identity")
			}
			seen[w.ID] = true
			groups[w.Generator]++
			if w.Size != w.Density.Instances {
				t.Fatal("scaling axis differs from group count")
			}
			b, err := os.ReadFile(w.Artifact)
			if err != nil {
				t.Fatal(err)
			}
			if Hash(b) != w.SHA256 {
				t.Fatal("hash mismatch")
			}
			m, err := r.InstantiateWithConfig(ctx, b, wazero.NewModuleConfig().WithName(""))
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close(ctx)
			v, err := m.ExportedFunction(w.Export).Call(ctx, w.Args...)
			if err != nil || len(v) != 1 || v[0] != w.Oracle.Expected[0] {
				t.Fatalf("first call %v: %v", v, err)
			}
			memory, ok := m.Memory().Read(0, 65536)
			if !ok {
				t.Fatal("missing memory")
			}
			want := byte(0)
			if w.Args[0] != 0 {
				want = 1
			}
			for i, b := range memory {
				if b != want {
					t.Fatalf("byte %d = %d, want %d", i, b, want)
				}
			}
			v, err = m.ExportedFunction(w.Export).Call(ctx, w.Args...)
			if err != nil || len(v) != 1 || v[0] == w.Oracle.Expected[0] {
				t.Fatalf("instance reuse undetected: %v %v", v, err)
			}
		})
	}
	if len(groups) != 4 {
		t.Fatal("sharing/touch policies merged")
	}
	for _, n := range groups {
		if n != 4 {
			t.Fatal("missing scaling points")
		}
	}
}
