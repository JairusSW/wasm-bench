package experiment

import (
	"fmt"
	"os"
	"path/filepath"
)

// Archived tools are regular hard links to immutable content-addressed blobs.
// Existing bundle verification and portable exports keep their exact byte paths.
// Keep this cache on the same filesystem as experiment outputs.
func linkCachedTool(source, destination, digest string) error {
	if got, err := DigestFile(source); err == nil && got != digest {
		return fmt.Errorf("tool changed before archiving: %s", source)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	root := os.Getenv("WASMBENCH_TOOL_CACHE")
	if root == "" {
		var err error
		root, err = os.UserCacheDir()
		if err != nil {
			return err
		}
		root = filepath.Join(root, "wasm-bench", "tools")
	}
	if len(digest) != 64 {
		return fmt.Errorf("invalid tool cache digest")
	}
	blob := filepath.Join(root, "sha256", digest[:2], digest)
	if err := os.MkdirAll(filepath.Dir(blob), 0755); err != nil {
		return err
	}
	if _, err := os.Lstat(blob); os.IsNotExist(err) {
		temp, err := os.CreateTemp(filepath.Dir(blob), ".tool-*")
		if err != nil {
			return err
		}
		name := temp.Name()
		temp.Close()
		os.Remove(name)
		defer os.Remove(name)
		if err := copyTool(source, name, digest, false); err != nil {
			return err
		}
		// Link publishes atomically, without replacing a concurrent writer's blob.
		if err := os.Link(name, blob); err != nil && !os.IsExist(err) {
			return err
		}
	} else if err != nil {
		return err
	}
	info, err := os.Lstat(blob)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0222 != 0 {
		return fmt.Errorf("tool cache blob is not an immutable regular file: %s", blob)
	}
	got, err := DigestFile(blob)
	if err != nil {
		return err
	}
	if got != digest {
		return fmt.Errorf("tool cache blob differs from digest: %s", blob)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return err
	}
	if err := os.Link(blob, destination); err != nil {
		if os.IsExist(err) {
			return err
		}
		return fmt.Errorf("link shared tool cache: %w; set WASMBENCH_TOOL_CACHE to a directory on the experiment filesystem", err)
	}
	return nil
}
