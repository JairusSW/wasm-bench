package publish

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/wasmbench/wasmbench/analysis"
	"github.com/wasmbench/wasmbench/experiment"
)

const MemoryEvidenceVersion = "paired-memory-evidence-v1"

func pairedMemoryTimelines(memory experiment.Bundle, matched map[string]bool) ([]analysis.MemoryTimeline, []analysis.DensityFootprint) {
	var timelines []analysis.MemoryTimeline
	for _, line := range analysis.MemoryTimelines(memory) {
		if matched[line.Runtime+"\x00"+line.Workload] {
			timelines = append(timelines, line)
		}
	}
	return timelines, analysis.DensityFootprints(timelines)
}

// A seal alone cannot detect intentionally resealed false derived exports.
// Recreate the full memory-pass exports from copied raw evidence and compare
// their digests. No timing/memory concatenation or numeric rescaling occurs.
func verifyMemoryExports(root string, memory experiment.Bundle) error {
	dir, err := os.MkdirTemp("", "wasmbench-memory-export-verify-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	for _, item := range []struct {
		name   string
		export func(experiment.Bundle, string) error
	}{{"memory-samples.parquet", ExportParquet}, {"memory-observations.parquet", ExportObservations}} {
		path := filepath.Join(dir, item.name)
		if err := item.export(memory, path); err != nil {
			return err
		}
		want, err := experiment.DigestFile(path)
		if err != nil {
			return err
		}
		got, err := experiment.DigestFile(filepath.Join(root, item.name))
		if err != nil {
			return err
		}
		if want != got {
			return fmt.Errorf("%s differs from copied raw memory evidence", item.name)
		}
	}
	return nil
}
