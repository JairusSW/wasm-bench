package publish

import "github.com/parquet-go/parquet-go"

// The library's default CreatedBy depends on debug.ReadBuildInfo: ordinary CLI
// and test executables can encode identical rows with different file footers.
// Pin export metadata independently of the executable's build-info availability.
// Exact runner/dependency provenance remains in the sealed input and builder.
const ParquetWriterVersion = "parquet-export-v1"

func reportParquetWriterOption() parquet.WriterOption {
	return parquet.CreatedBy("wasmbench", ParquetWriterVersion, "")
}
