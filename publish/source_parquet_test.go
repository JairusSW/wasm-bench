package publish

import (
	"path/filepath"
	"testing"

	"github.com/parquet-go/parquet-go"
)

func TestSourceTablesPreserveNullZeroAndStages(t *testing.T) {
	z := int64(0)
	peak := float64(0)
	path := filepath.Join(t.TempDir(), "builds.parquet")
	rows := []SourceBuildRow{
		{Version: SourceTableVersion, Stage: "admission", Status: "ok"},
		{Version: SourceTableVersion, Stage: "trial", Status: "timeout", Reason: "deadline"},
		{Version: SourceTableVersion, Stage: "trial", Status: "ok", Warmup: true, Block: &z, WallNS: &z, CPUNS: &z, PeakBytes: &peak},
	}
	if err := writeSourceParquet(path, rows); err != nil {
		t.Fatal(err)
	}
	got, err := parquet.ReadFile[SourceBuildRow](path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Block != nil || got[0].WallNS != nil || got[1].WallNS != nil || got[1].PeakBytes != nil || got[1].Status != "timeout" || !got[2].Warmup || got[2].CPUNS == nil || *got[2].CPUNS != 0 || got[2].PeakBytes == nil || *got[2].PeakBytes != 0 {
		t.Fatal(got)
	}
	if err := writeSourceParquet(path, rows); err == nil {
		t.Fatal("overwrote table")
	}
}

func TestSourceStepDiagnosticRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "steps.parquet")
	oom := true
	raw := `{"resources":{"observations":[{"metric":"source.build.cgroup.peak","unit":"bytes","status":"unavailable","reason":"not exposed"}]}}`
	rows := []SourceStepRow{{Version: SourceTableVersion, Stage: "admission", BuildStatus: "oom", OOM: &oom, CPUStatus: "not_collected", EvidenceJSON: raw}, {Version: SourceTableVersion, Stage: "trial", BuildStatus: "build_failed"}}
	if err := writeSourceParquet(path, rows); err != nil {
		t.Fatal(err)
	}
	got, err := parquet.ReadFile[SourceStepRow](path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].OOM == nil || !*got[0].OOM || got[0].CPUNS != nil || got[0].EvidenceJSON != raw || got[1].OOM != nil {
		t.Fatal(got)
	}
	empty := filepath.Join(t.TempDir(), "empty.parquet")
	if err := writeSourceParquet(empty, []SourceStepRow{}); err != nil {
		t.Fatal(err)
	}
	if got, err := parquet.ReadFile[SourceStepRow](empty); err != nil || len(got) != 0 {
		t.Fatal(got, err)
	}
}
