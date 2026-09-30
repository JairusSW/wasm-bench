package publish

import (
	"math"
	"path/filepath"
	"testing"

	"github.com/parquet-go/parquet-go"
	"github.com/wasmbench/wasmbench/analysis"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

func TestThroughputParquetExactEvidenceAndExclusions(t *testing.T) {
	for _, profile := range []string{"timing", "memory"} {
		t.Run(profile, func(t *testing.T) {
			b := experiment.Bundle{Manifest: experiment.Manifest{Kind: "measurement", Lock: experiment.Lock{Options: experiment.Options{Profile: profile}, Workloads: []protocol.Workload{{ID: "w", WorkUnit: "byte", Units: math.MaxUint64}}}}}
			for i, status := range []string{"ok", "timeout", "ok"} {
				trial := experiment.Trial{ID: status, Block: i, Status: status, Profile: profile, Runtime: "r", Workload: "w", Scenario: "steady", Samples: []protocol.Sample{
					{Warmup: true, Operations: 100, ElapsedNS: 5, Verified: true, SampleType: "batch_average"},
					{Operations: 2, ElapsedNS: math.MaxInt64, Verified: true, SampleType: "batch_average"},
					{Operations: 2, ElapsedNS: math.MaxInt64, Verified: true, SampleType: "batch_average"},
				}}
				if i == 2 {
					trial.Samples[2].Verified = false
				}
				if status == "timeout" {
					trial.Reason = "deadline exceeded"
				}
				b.Trials = append(b.Trials, trial)
			}
			path := filepath.Join(t.TempDir(), "throughput.parquet")
			if err := ExportThroughput("run", analysis.Throughput(b), path); err != nil {
				t.Fatal(err)
			}
			rows, err := parquet.ReadFile[ThroughputRow](path)
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 3 {
				t.Fatal("lost excluded launches", rows)
			}
			for i, row := range rows {
				if row.ExportVersion != ThroughputExportVersion || row.AnalysisVersion != analysis.ThroughputVersion || row.TrialStatus != b.Trials[i].Status || row.UnitsPerInvocation != "18446744073709551615" {
					t.Fatal(row)
				}
				if row.TrialReason != b.Trials[i].Reason {
					t.Fatal("lost failure reason", row)
				}
				if profile == "timing" && i == 0 {
					if row.Rate == nil || row.Operations == nil || *row.Operations != "4" || row.WorkUnits == nil || *row.WorkUnits != "73786976294838206460" || row.ElapsedNS == nil || *row.ElapsedNS != "18446744073709551614" {
						t.Fatalf("lost exact totals: %+v", row)
					}
				} else if row.Rate != nil || row.WorkUnits != nil || row.Operations != nil || row.ElapsedNS != nil {
					t.Fatal("fabricated excluded values", row)
				}
			}
			if err := ExportThroughput("run", analysis.Throughput(b), path); err == nil {
				t.Fatal("overwrote evidence")
			}
		})
	}
}
