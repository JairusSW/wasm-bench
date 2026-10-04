package experiment

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFinderMetadataSeal(t *testing.T) {
	root := t.TempDir()
	put := func(name, value string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	put("data.json", "evidence")
	if err := os.Mkdir(filepath.Join(root, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	put(".DS_Store", "finder before")
	put("nested/.DS_Store", "nested finder before")
	if err := Seal(root); err != nil {
		t.Fatal(err)
	}
	put(".DS_Store", "finder after")
	put("nested/.DS_Store", "nested finder after")
	if err := Verify(root); err != nil {
		t.Fatalf("Finder metadata invalidated evidence: %v", err)
	}
	put("data.json", "tampered")
	if err := Verify(root); err == nil {
		t.Fatal("accepted changed evidence")
	}
	put("data.json", "evidence")
	put("nested/unknown", "unexpected")
	if err := Verify(root); err == nil {
		t.Fatal("accepted unsealed file")
	}
}
func TestLegacyFinderSeal(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "data.json"), []byte("evidence"), 0600); err != nil {
		t.Fatal(err)
	}
	hash, err := DigestFile(filepath.Join(root, "data.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteJSON(filepath.Join(root, "checksums.json"), map[string]string{"data.json": hash, ".DS_Store": "old Finder digest"}); err != nil {
		t.Fatal(err)
	}
	if err := Verify(root); err != nil {
		t.Fatal(err)
	}
}
