package experiment

import (
	"os"
	"path/filepath"
	"testing"
)

func TestImmutableBundleTampering(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "evidence.json")
	if e := WriteJSON(path, map[string]int{"value": 1}); e != nil {
		t.Fatal(e)
	}
	if e := WriteJSON(path, map[string]int{"value": 2}); e == nil {
		t.Fatal("overwrote evidence")
	}
	if e := Seal(root); e != nil {
		t.Fatal(e)
	}
	if e := Verify(root); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(path, []byte("altered"), 0644)
	if e := Verify(root); e == nil {
		t.Fatal("accepted altered evidence")
	}
}
func TestUnsealedFilesRejected(t *testing.T) {
	root := t.TempDir()
	WriteJSON(filepath.Join(root, "manifest.json"), map[string]int{"schema": 1})
	Seal(root)
	os.WriteFile(filepath.Join(root, "extra"), []byte("extra"), 0644)
	if e := Verify(root); e == nil {
		t.Fatal("accepted unsealed artifact")
	}
}
