package experiment

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// OutputOutsideBundles resolves existing symlink ancestors before allowing a
// future output. New sidecars and reports must never alter their sealed inputs.
func OutputOutsideBundles(out string, inputs ...string) error {
	abs, err := filepath.Abs(out)
	if err != nil {
		return err
	}
	parent, suffix := abs, ""
	var resolvedOut string
	for {
		resolved, err := filepath.EvalSymlinks(parent)
		if err == nil {
			resolvedOut = filepath.Join(resolved, suffix)
			break
		}
		if !os.IsNotExist(err) {
			return err
		}
		next := filepath.Dir(parent)
		if next == parent {
			return err
		}
		suffix = filepath.Join(filepath.Base(parent), suffix)
		parent = next
	}
	for _, input := range inputs {
		resolved, err := filepath.EvalSymlinks(input)
		if err != nil {
			return err
		}
		resolved, err = filepath.Abs(resolved)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(resolved, resolvedOut)
		if err != nil {
			return err
		}
		if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			return fmt.Errorf("output must not be inside an input bundle")
		}
	}
	return nil
}
