package experiment

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestOutputOutsideBundlesPreservesSealedInputs(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "input")
	if err := os.Mkdir(input, 0755); err != nil {
		t.Fatal(err)
	}
	for _, out := range []string{input, filepath.Join(input, "new.json"), filepath.Join(input, "new", "nested.json")} {
		if OutputOutsideBundles(out, input) == nil {
			t.Fatal("nested output accepted", out)
		}
	}
	for _, out := range []string{filepath.Join(root, "new.json"), filepath.Join(root, "input-sibling", "new.json")} {
		if err := OutputOutsideBundles(out, input); err != nil {
			t.Fatal(err)
		}
	}
	if runtime.GOOS != "windows" {
		alias := filepath.Join(root, "alias")
		if err := os.Symlink(input, alias); err != nil {
			t.Fatal(err)
		}
		if OutputOutsideBundles(filepath.Join(alias, "new", "file.json"), input) == nil {
			t.Fatal("symlink ancestor bypassed immutable input")
		}
		if OutputOutsideBundles(filepath.Join(input, "file.json"), alias) == nil {
			t.Fatal("input alias bypassed immutable input")
		}
	}
}
