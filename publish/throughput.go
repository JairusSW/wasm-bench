package publish

import (
	"github.com/parquet-go/parquet-go"
	"github.com/wasmbench/wasmbench/analysis"
	"os"
)

const ThroughputExportVersion = "timed-work-evidence-parquet-v1"

type ThroughputRow struct {
	ExportVersion      string   `parquet:"export_version"`
	AnalysisVersion    string   `parquet:"analysis_version"`
	Run                string   `parquet:"run"`
	Runtime            string   `parquet:"runtime"`
	Workload           string   `parquet:"workload"`
	Scenario           string   `parquet:"scenario"`
	Profile            string   `parquet:"profile"`
	WorkUnit           string   `parquet:"work_unit"`
	UnitsPerInvocation string   `parquet:"units_per_invocation_decimal"`
	Trial              string   `parquet:"trial"`
	Block              int64    `parquet:"block"`
	TrialStatus        string   `parquet:"trial_status"`
	TrialReason        string   `parquet:"trial_reason"`
	Status             string   `parquet:"throughput_status"`
	Operations         *string  `parquet:"measured_operations_decimal,optional"`
	WorkUnits          *string  `parquet:"measured_work_units_decimal,optional"`
	ElapsedNS          *string  `parquet:"measured_elapsed_ns_decimal,optional"`
	Rate               *float64 `parquet:"work_units_per_second,optional"`
}

// ExportThroughput writes every measured trial, including exclusions. Warmup
// and sacrificial trials remain in samples.parquet, not launch work totals.
func ExportThroughput(run string, summaries []analysis.ThroughputSummary, path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	w := parquet.NewGenericWriter[ThroughputRow](f, reportParquetWriterOption())
	for _, s := range summaries {
		for _, e := range s.Evidence {
			r := ThroughputRow{ExportVersion: ThroughputExportVersion, AnalysisVersion: analysis.ThroughputVersion, Run: run, Runtime: s.Runtime, Workload: s.Workload, Scenario: s.Scenario, Profile: s.Profile, WorkUnit: s.WorkUnit, UnitsPerInvocation: s.UnitsPerInvocation, Trial: e.Trial, Block: int64(e.Block), TrialStatus: e.TrialStatus, Status: e.Status, Operations: e.Operations, WorkUnits: e.WorkUnits, ElapsedNS: e.ElapsedNS, Rate: e.Rate}
			r.TrialReason = e.TrialReason
			if _, err = w.Write([]ThroughputRow{r}); err != nil {
				return err
			}
		}
	}
	return w.Close()
}
