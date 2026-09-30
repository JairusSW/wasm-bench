package corpus

import (
	"bytes"
	"testing"
)

func TestContinuationModuleIdentity(t *testing.T) {
	seen := make(map[string]bool)
	for _, depth := range []int{0, 1, 8, 32, 128} {
		a, err := ContinuationModule(depth)
		if err != nil {
			t.Fatal(err)
		}
		b, err := ContinuationModule(depth)
		if err != nil || !bytes.Equal(a, b) {
			t.Fatal("nonreproducible continuation artifact")
		}
		if seen[Hash(a)] {
			t.Fatal("different depths have identical artifacts")
		}
		seen[Hash(a)] = true
	}
	for _, depth := range []int{-1, 129} {
		if _, err := ContinuationModule(depth); err == nil {
			t.Fatal("invalid continuation depth")
		}
	}
}
