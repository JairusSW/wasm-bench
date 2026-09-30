package publish

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/parquet-go/parquet-go"
	"github.com/wasmbench/wasmbench/experiment"
)

func TestEveryParquetExportPinsBuildIndependentMetadata(t *testing.T) {
	b := experiment.Bundle{}
	for _, export := range []struct {
		name  string
		write func(string) error
	}{
		{"samples", func(p string) error { return ExportParquet(b, p) }},
		{"observations", func(p string) error { return ExportObservations(b, p) }},
		{"counters", func(p string) error { return ExportCounters(b, p) }},
		{"throughput", func(p string) error { return ExportThroughput("run", nil, p) }},
		{"source-builds", func(p string) error { return writeSourceParquet(p, []SourceBuildRow{}) }},
		{"source-steps", func(p string) error { return writeSourceParquet(p, []SourceStepRow{}) }},
	} {
		t.Run(export.name, func(t *testing.T) {
			var first []byte
			for i := 0; i < 2; i++ {
				path := filepath.Join(t.TempDir(), "export.parquet")
				if err := export.write(path); err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				f, err := parquet.OpenFile(bytes.NewReader(data), int64(len(data)))
				if err != nil {
					t.Fatal(err)
				}
				if got := f.Metadata().CreatedBy; got != "wasmbench version parquet-export-v1(build )" {
					t.Fatalf("export metadata depends on executable build info: %q", got)
				}
				if i == 1 && !bytes.Equal(first, data) {
					t.Fatal("identical exports have different bytes")
				}
				first = data
			}
		})
	}
}
