package experiment

import (
	"os"
	"path/filepath"
	"testing"
)

func TestToolCacheSharesBytesAndRejectsCorruption(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("WASMBENCH_TOOL_CACHE", cache)
	source := filepath.Join(t.TempDir(), "adapter")
	if err := os.WriteFile(source, []byte("one exact adapter"), 0755); err != nil {
		t.Fatal(err)
	}
	digest, err := DigestFile(source)
	if err != nil {
		t.Fatal(err)
	}
	a := filepath.Join(t.TempDir(), "tools", "adapter")
	b := filepath.Join(t.TempDir(), "tools", "adapter")
	if err := linkCachedTool(source, a, digest); err != nil {
		t.Fatal(err)
	}
	// The original installation can disappear; subsequent reports use the cache.
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if err := linkCachedTool(source, b, digest); err != nil {
		t.Fatal(err)
	}
	ai, _ := os.Stat(a)
	bi, _ := os.Stat(b)
	if !os.SameFile(ai, bi) {
		t.Fatal("reports duplicated cached adapter bytes")
	}
	snapshot := filepath.Join(t.TempDir(), "report")
	if err := CopyTree(filepath.Dir(filepath.Dir(a)), snapshot); err != nil {
		t.Fatal(err)
	}
	si, err := os.Stat(filepath.Join(snapshot, "tools", "adapter"))
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(ai, si) {
		t.Fatal("report snapshot duplicated tool bytes")
	}
	blob := filepath.Join(cache, "sha256", digest[:2], digest)
	ci, _ := os.Stat(blob)
	if !os.SameFile(ai, ci) {
		t.Fatal("report is not linked to shared cache")
	}
	if err := os.Chmod(blob, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blob, []byte("corrupted"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(blob, 0444); err != nil {
		t.Fatal(err)
	}
	if err := linkCachedTool(source, filepath.Join(t.TempDir(), "third"), digest); err == nil {
		t.Fatal("accepted corrupt cached bytes")
	}
}
