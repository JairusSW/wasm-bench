package publish

import (
	"bytes"
	"fmt"
	"github.com/wasmbench/wasmbench/experiment"
	"os"
	"path/filepath"
)

func exportEngineTraces(b experiment.Bundle, root string) error {
	seen := map[string]bool{}
	for _, t := range b.Trials {
		x := t.EngineTrace
		if x == nil || x.Status == "unavailable" || seen[x.SHA256] {
			continue
		}
		if err := x.Validate(x.ModuleSHA256); err != nil {
			return err
		}
		if len(seen) == 0 {
			if err := os.Mkdir(filepath.Join(root, "traces"), 0755); err != nil {
				return err
			}
		}
		f, err := os.OpenFile(filepath.Join(root, "traces", x.SHA256+".trace.json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		_, writeErr := f.Write(x.Data)
		closeErr := f.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return closeErr
		}
		seen[x.SHA256] = true
	}
	return nil
}

func verifyEngineTraceExports(b experiment.Bundle, root string) error {
	for _, t := range b.Trials {
		x := t.EngineTrace
		if x == nil || x.Status == "unavailable" {
			continue
		}
		if err := x.Validate(x.ModuleSHA256); err != nil {
			return err
		}
		got, err := os.ReadFile(filepath.Join(root, "traces", x.SHA256+".trace.json"))
		if err != nil {
			return err
		}
		if !bytes.Equal(got, x.Data) {
			return fmt.Errorf("engine trace download differs from raw evidence")
		}
	}
	return nil
}
