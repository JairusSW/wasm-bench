package main

import (
	"context"
	"github.com/wago-org/wago"
	"github.com/wago-org/wasi/p1"
	"os"
	"path/filepath"
	"testing"
)

func wasiOpenProbe() []byte {
	leb := func(n int) []byte {
		var b []byte
		for {
			v := byte(n & 127)
			n >>= 7
			if n != 0 {
				v |= 128
			}
			b = append(b, v)
			if n == 0 {
				return b
			}
		}
	}
	text := func(s string) []byte { return append(leb(len(s)), []byte(s)...) }
	wasm := []byte{0, 97, 115, 109, 1, 0, 0, 0}
	section := func(id byte, b []byte) {
		wasm = append(wasm, id)
		wasm = append(wasm, leb(len(b))...)
		wasm = append(wasm, b...)
	}
	section(1, []byte{2, 0x60, 9, 0x7f, 0x7f, 0x7f, 0x7f, 0x7f, 0x7e, 0x7e, 0x7f, 0x7f, 1, 0x7f, 0x60, 4, 0x7f, 0x7e, 0x7e, 0x7f, 1, 0x7f})
	imports := append([]byte{1}, text(p1.Module)...)
	imports = append(imports, text("path_open")...)
	section(2, append(imports, 0, 0))
	section(3, []byte{1, 1})
	section(5, []byte{1, 0, 1})
	exports := append([]byte{2}, text("run")...)
	exports = append(exports, 0, 1)
	exports = append(exports, text("memory")...)
	section(7, append(exports, 2, 0))
	body := []byte{0, 0x41, 3, 0x41, 0, 0x41, 0, 0x41, 11, 0x20, 0, 0x20, 1, 0x20, 2, 0x20, 3, 0x41, 32, 0x10, 0, 0x0b}
	section(10, append(append([]byte{1}, leb(len(body))...), body...))
	section(11, append([]byte{1, 0, 0x41, 0, 0x0b, 11}, []byte("fixture.txt")...))
	return wasm
}

func TestReadonlyWasiGoRightsPermitReadsAndStillDenyMutation(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "fixture.txt")
	if err := os.WriteFile(file, []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	compiled, err := wago.Compile(nil, wasiOpenProbe())
	if err != nil {
		t.Fatal(err)
	}
	defer compiled.Close()
	for _, tc := range []struct {
		name                  string
		adapt                 bool
		oflags, fdflags, want uint64
	}{
		{"provider rejects broad Go rights", false, 0, 0, 76},
		{"adapter permits readonly broad rights", true, 0, 0, 0},
		{"create denied", true, 1, 0, 76}, {"truncate denied", true, 8, 0, 76}, {"exclusive denied", true, 4, 0, 76},
		{"append denied", true, 0, 1, 76}, {"sync denied", true, 0, 16, 76},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := wago.NewRuntime()
			defer rt.Close()
			mod, err := rt.Module(compiled)
			if err != nil {
				t.Fatal(err)
			}
			defer mod.Close()
			imports := p1.Imports(p1.Config{Mounts: []p1.Preopen{{GuestPath: ".", HostPath: root, Read: true}}})
			opts := []wago.InstantiateOption{wago.WithImports(imports)}
			if tc.adapt {
				override, err := readonlyP1OpenOverrides(imports)
				if err != nil {
					t.Fatal(err)
				}
				opts = append(opts, wago.WithImports(override))
			}
			instance, err := rt.Instantiate(context.Background(), mod, opts...)
			if err != nil {
				t.Fatal(err)
			}
			defer instance.Close()
			result, err := instance.Invoke("run", tc.oflags, ^uint64(0), ^uint64(0), tc.fdflags)
			if err != nil || len(result) != 1 || result[0] != tc.want {
				t.Fatalf("result=%v err=%v want=%d", result, err, tc.want)
			}
			content, err := os.ReadFile(file)
			if err != nil || string(content) != "unchanged" {
				t.Fatalf("fixture changed: %q %v", content, err)
			}
		})
	}
}
