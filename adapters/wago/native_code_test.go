package main

import (
	"encoding/hex"
	"github.com/wago-org/wago"
	"testing"
)

func TestNativeCompilationDoesNotInstantiateImports(t *testing.T) {
	// Imported m.f is deliberately unbound; exported g is a defined function.
	wasm, err := hex.DecodeString("0061736d010000000105016000017f020701016d0166000003020100070501016700010a0601040041070b")
	if err != nil {
		t.Fatal(err)
	}
	a := &adapter{wasm: wasm}
	defer a.closeAll()
	if err := a.compilePrepared(); err != nil {
		t.Fatal(err)
	}
	if a.instance != nil || a.compiled.CodeSize() == 0 {
		t.Fatal("native compilation instantiated a guest or omitted generated code")
	}
	if instance, err := wago.Instantiate(a.compiled, wago.InstantiateOptions{}); err == nil {
		instance.Close()
		t.Fatal("fixture unexpectedly allows instantiation without its host import")
	}
	compiled := a.compiled
	if err := a.compilePrepared(); err != nil {
		t.Fatal(err)
	}
	if a.compiled != compiled {
		t.Fatal("prepared compilation was repeated")
	}
}
