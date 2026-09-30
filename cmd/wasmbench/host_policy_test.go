package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCaptureHostPolicy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "host.json")
	if err := captureHostPolicy([]string{"--out", path}); err != nil {
		t.Fatal(err)
	}
	if _, err := loadHostPolicy(path); err != nil {
		t.Fatal(err)
	}
	if err := captureHostPolicy([]string{"--out", path}); err == nil {
		t.Fatal("overwrote baseline")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{" {}", " null", " garbage"} {
		bad := filepath.Join(t.TempDir(), "bad.json")
		if err := os.WriteFile(bad, append(append([]byte{}, data...), []byte(suffix)...), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadHostPolicy(bad); err == nil {
			t.Fatal("trailing data accepted")
		}
	}
}
