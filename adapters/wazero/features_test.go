package main

import (
	"os"
	"testing"
)

func TestPinnedFeaturePolicy(t *testing.T) {
	for _, interpreter := range []bool{false, true} {
		a := &adapter{interpreter: interpreter}
		e := a.newEngine()
		for _, tc := range []struct {
			file  string
			valid bool
		}{
			{"analyzer-features.wasm", true}, {"feature-tail-call.wasm", false},
		} {
			b, err := os.ReadFile("../../corpus/testdata/" + tc.file)
			if err != nil {
				t.Fatal(err)
			}
			m, err := e.CompileModule(ctx, b)
			if (err == nil) != tc.valid {
				t.Fatalf("interpreter=%v file=%s: %v", interpreter, tc.file, err)
			}
			if m != nil {
				m.Close(ctx)
			}
		}
		e.Close(ctx)
	}
}
