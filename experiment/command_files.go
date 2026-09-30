package experiment

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/wasmbench/wasmbench/protocol"
)

func verifyCommandFiles(w protocol.Workload, base string) error {
	if w.Command == nil {
		return nil
	}
	if err := protocol.ValidateCommand(w); err != nil {
		return err
	}
	for name, file := range w.Command.Files {
		if err := protocol.CopyCommandFile(io.Discard, file, base); err != nil {
			return fmt.Errorf("command input %s: %w", name, err)
		}
	}
	return nil
}

func bundleCommandFiles(w *protocol.Workload, base, out string) error {
	if w.Command == nil {
		return nil
	}
	for name, file := range w.Command.Files {
		if file.Path == "" {
			continue
		}
		rel := filepath.Join("inputs", file.SHA256)
		dst := filepath.Join(out, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return err
		}
		f, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err == nil {
			err = protocol.CopyCommandFile(f, file, base)
			closeErr := f.Close()
			if err != nil {
				return err
			}
			if closeErr != nil {
				return closeErr
			}
		} else if !os.IsExist(err) {
			return err
		}
		file.Path = filepath.ToSlash(rel)
		if err := protocol.CopyCommandFile(io.Discard, file, out); err != nil {
			return err
		}
		w.Command.Files[name] = file
	}
	return nil
}

// Preparation gets resolved absolute paths, while the locked workload retains
// portable bundle-relative references. Never mutate the manifest's shared map.
func resolveCommandFiles(w protocol.Workload, base string) protocol.Workload {
	if w.Command == nil {
		return w
	}
	c := *w.Command
	c.Files = make(map[string]protocol.CommandFile, len(w.Command.Files))
	for name, file := range w.Command.Files {
		if file.Path != "" && !filepath.IsAbs(file.Path) {
			file.Path = filepath.Join(base, file.Path)
		}
		c.Files[name] = file
	}
	w.Command = &c
	return w
}
