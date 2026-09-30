package corpus

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/tetratelabs/wazero"
	"github.com/wasmbench/wasmbench/protocol"
	"testing"
)

func TestProcessSnapshotCanonicalGuestState(t *testing.T) {
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	b, err := ProcessSnapshotModule()
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 343 || Hash(b) != protocol.ProcessSnapshotArtifactSHA256 {
		t.Fatal("qualification artifact changed")
	}
	m, err := r.Instantiate(ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	call := func(name string, want uint64) {
		t.Helper()
		v, err := m.ExportedFunction(name).Call(ctx)
		if err != nil || len(v) != 1 || v[0] != want {
			t.Fatalf("%s: %v %v", name, v, err)
		}
	}
	if _, err := m.ExportedFunction("seed").Call(ctx); err != nil {
		t.Fatal(err)
	}
	call("check", 64)
	call("pages", 3)
	call("elements", 3)
	verifyMemory := func(written bool) {
		t.Helper()
		memory, ok := m.Memory().Read(0, 3*65536)
		if !ok {
			t.Fatal("missing grown memory")
		}
		h := sha256.Sum256(memory)
		if hex.EncodeToString(h[:]) != protocol.ProcessSnapshotMemorySHA256(written) {
			t.Fatal("full-memory oracle differs from fixed fixture")
		}
	}
	verifyMemory(false)
	if !m.Memory().WriteByte(65535, 22) {
		t.Fatal("first write failed")
	}
	call("check", 64)
	verifyMemory(true)
	call("probe", 127)
	if _, err := m.ExportedFunction("mutate").Call(ctx); err != nil {
		t.Fatal(err)
	}
	call("check", 125)
	if _, err := m.ExportedFunction("probe").Call(ctx); err == nil {
		t.Fatal("dropped passive segments did not trap")
	}
}
