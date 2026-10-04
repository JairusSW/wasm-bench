package experiment

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyTreeIndependentAndExclusive(t *testing.T) {
	root := t.TempDir()
	src, dst := filepath.Join(root, "source"), filepath.Join(root, "copy")
	if err := os.MkdirAll(filepath.Join(src, "tools"), 0755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(src, "tools", "adapter")
	if err := os.WriteFile(file, []byte("exact bytes"), 0555); err != nil {
		t.Fatal(err)
	}
	if err := CopyTree(src, dst); err != nil {
		t.Fatal(err)
	}
	if err := CopyTree(src, dst); !os.IsExist(err) {
		t.Fatalf("overwrite accepted: %v", err)
	}
	if err := os.Chmod(file, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("new bytes"), 0755); err != nil {
		t.Fatal(err)
	}
	copy := filepath.Join(dst, "tools", "adapter")
	b, err := os.ReadFile(copy)
	if err != nil || string(b) != "exact bytes" {
		t.Fatalf("shared mutable contents: %q %v", b, err)
	}
	info, err := os.Stat(copy)
	if err != nil || info.Mode().Perm() != 0755 {
		t.Fatalf("lost executable permission: %v %v", info, err)
	}
}
