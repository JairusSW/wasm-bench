package publish

import (
	"math"
	"path/filepath"
	"testing"

	"github.com/parquet-go/parquet-go"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

func TestObservationParquetPreservesDomainsAndMissing(t *testing.T) {
	o := protocol.Observation{Metric: "host.alloc.bytes", DefinitionVersion: 2, Value: protocol.Value(0), Unit: "bytes", Scope: "go_heap", Phase: "compile/batch", Collector: "go", CollectorVersion: "1.26", Quality: "engine_reported", Profile: "memory", Status: "available", Denominator: "batch"}
	missing := o
	missing.Status = "permission_denied"
	missing.Reason = "no access"
	missing.Metric = "process.pss"
	missing.Value = protocol.Value(999)
	b := experiment.Bundle{Manifest: experiment.Manifest{ID: "run"}, Trials: []experiment.Trial{{ID: "trial", Runtime: "r", Workload: "w", Scenario: "compile", Profile: "memory", Block: 1, Status: "ok", Observations: []protocol.Observation{missing}, Samples: []protocol.Sample{{Index: 3, Operations: 7, Warmup: true, Verified: true, Observations: []protocol.Observation{o}}}}}}
	path := filepath.Join(t.TempDir(), "observations.parquet")
	if err := ExportObservations(b, path); err != nil {
		t.Fatal(err)
	}
	rows, err := parquet.ReadFile[ObservationRow](path)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatal(rows)
	}
	x, y := rows[0], rows[1]
	if x.Value != nil || x.SampleIndex != nil || x.Operations != nil || x.Warmup != nil || x.Verified != nil || x.Status != "permission_denied" || x.Reason != "no access" {
		t.Fatal(x)
	}
	if y.Value == nil || *y.Value != 0 || y.SampleIndex == nil || *y.SampleIndex != 3 || y.Operations == nil || *y.Operations != 7 || !*y.Warmup || !*y.Verified {
		t.Fatal(y)
	}
	if y.Scope != o.Scope || y.Collector != o.Collector || y.CollectorVersion != o.CollectorVersion || y.DefinitionVersion != 2 || y.Phase != o.Phase || y.Denominator != "batch" || y.InstrumentationProfile != "memory" || y.Quality != o.Quality {
		t.Fatal(y)
	}
	if err := ExportObservations(b, path); err == nil {
		t.Fatal("overwrote existing export")
	}
}

func TestObservationParquetRejectsInvalidAvailable(t *testing.T) {
	for _, v := range []*float64{nil, protocol.Value(math.NaN()), protocol.Value(math.Inf(1))} {
		b := experiment.Bundle{Trials: []experiment.Trial{{Observations: []protocol.Observation{{Metric: "broken", Status: "available", Value: v}}}}}
		if err := ExportObservations(b, filepath.Join(t.TempDir(), "bad.parquet")); err == nil {
			t.Fatal("accepted invalid available value")
		}
	}
}
