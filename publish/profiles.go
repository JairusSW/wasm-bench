package publish

import (
	"github.com/wasmbench/wasmbench/experiment"
	"os"
	"path/filepath"
)

// exportProfiles materializes unmodified content-addressed diagnostic downloads.
// The containing report directory is new; the raw bundle remains untouched.
func exportProfiles(b experiment.Bundle, root string) error {
	seen := map[string]bool{}
	for _, t := range b.Trials {
		p := t.CPUProfile
		if p == nil || p.Status != "collected" || seen[p.SHA256] {
			continue
		}
		if err := p.Validate(p.ModuleSHA256); err != nil {
			return err
		}
		if len(seen) == 0 {
			if err := os.Mkdir(filepath.Join(root, "profiles"), 0755); err != nil {
				return err
			}
		}
		f, err := os.OpenFile(filepath.Join(root, "profiles", p.SHA256+p.Extension()), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		_, writeErr := f.Write(p.Data)
		closeErr := f.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return closeErr
		}
		seen[p.SHA256] = true
	}
	return nil
}
