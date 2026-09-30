package publish

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

func memoryTimelineBundle() experiment.Bundle {
	return experiment.Bundle{Manifest: experiment.Manifest{Kind: "measurement", Lock: experiment.Lock{Options: experiment.Options{Profile: "memory"}}}, Trials: []experiment.Trial{{ID: "same-id", Runtime: "r", Workload: "w", Scenario: "density", Profile: "memory", Status: "ok", Samples: []protocol.Sample{{Index: 0, Operations: 1, SampleType: "individual_operation", Verified: true, Observations: []protocol.Observation{
		{Metric: "host.heap.end", Unit: "bytes", Quality: "engine_reported", Status: "available", Value: protocol.Value(0), Phase: "density", Scope: "go_heap", Profile: "memory"},
		{Metric: "process.rss", Unit: "bytes", Quality: "boundary_snapshot_only", Status: "unsupported", Phase: "density/before_density", Scope: "adapter_process", Profile: "memory", Reason: "fixture"},
		{Metric: "process.rss", Unit: "bytes", Quality: "boundary_snapshot_only", Status: "unsupported", Phase: "density/density_ready", Scope: "adapter_process", Profile: "memory", Reason: "fixture"},
		{Metric: "process.rss", Unit: "bytes", Quality: "boundary_snapshot_only", Status: "unsupported", Phase: "density/density_released", Scope: "adapter_process", Profile: "memory", Reason: "fixture"},
	}}}}}}
}

func TestPairedMemoryTimelinesFilterContractsAndKeepMissingDomains(t *testing.T) {
	b := memoryTimelineBundle()
	other := b.Trials[0]
	other.Runtime = "different"
	b.Trials = append(b.Trials, other)
	lines, footprints := pairedMemoryTimelines(b, map[string]bool{"r\x00w": true})
	if len(lines) != 4 || len(footprints) != 1 {
		t.Fatal("missing domains or unmatched runtime", lines, footprints)
	}
	for _, line := range lines {
		if line.Runtime != "r" || len(line.Points) != 1 {
			t.Fatal(line)
		}
		if line.Measurement.Metric == "host.heap.end" && (line.Points[0].Value == nil || *line.Points[0].Value != 0) {
			t.Fatal("measured zero became unavailable")
		}
	}
	if footprints[0].Points[0].ProvisionChange != nil || footprints[0].Points[0].ReleaseChange != nil {
		t.Fatal("unavailable snapshots became zero")
	}
	if len(b.Trials) != 2 || len(b.Trials[0].Samples[0].Observations) != 4 {
		t.Fatal("input mutated")
	}
}

func TestPairedMemoryReportExportsAndResealedTamper(t *testing.T) {
	timing, _ := aggregateBundle(t, false)
	primary, err := experiment.Load(timing)
	if err != nil {
		t.Fatal(err)
	}
	memory := t.TempDir()
	if err := os.Mkdir(filepath.Join(memory, "trials"), 0755); err != nil {
		t.Fatal(err)
	}
	m := primary.Manifest
	m.ID = "separate-memory"
	m.Lock.Options.Profile = "memory"
	if err := experiment.WriteJSON(filepath.Join(memory, "manifest.json"), m); err != nil {
		t.Fatal(err)
	}
	tr := memoryTimelineBundle().Trials[0]
	tr.ID = "t0-0"
	tr.Runtime = "baseline"
	tr.Workload = "required"
	tr.Scenario = "compile"
	if err := experiment.WriteJSON(filepath.Join(memory, "trials", tr.ID+".json"), tr); err != nil {
		t.Fatal(err)
	}
	if err := experiment.Seal(memory); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "report")
	if err := ReportWithMemory(timing, memory, out); err != nil {
		t.Fatal(err)
	}
	var d Dataset
	if err := experiment.ReadJSON(filepath.Join(out, "data.json"), &d); err != nil {
		t.Fatal(err)
	}
	if d.MemoryEvidenceVersion != MemoryEvidenceVersion || len(d.PairedMemoryTimelines) != 4 || len(d.MemoryTimelines) != 0 {
		t.Fatal("lost separate paired timeline source")
	}
	rows, err := parquet.ReadFile[SampleRow](filepath.Join(out, "memory-samples.parquet"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Run != m.ID || rows[0].Profile != "memory" || rows[0].LatencyEligible {
		t.Fatal("memory mistaken for timing", rows)
	}
	obs, err := parquet.ReadFile[ObservationRow](filepath.Join(out, "memory-observations.parquet"))
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 4 || obs[0].Run != m.ID {
		t.Fatal("lost memory observations")
	}
	if err := VerifyReport(out); err != nil {
		t.Fatal(err)
	}
	// The timing and memory trials intentionally use the same textual ID.
	path := filepath.Join(out, "memory-samples.parquet")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := ExportParquet(primary, path); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(out, "checksums.json")); err != nil {
		t.Fatal(err)
	}
	if err := experiment.Seal(out); err != nil {
		t.Fatal(err)
	}
	if err := VerifyReport(out); err == nil || !strings.Contains(err.Error(), "memory-samples.parquet differs") {
		t.Fatal("resealed substituted timing rows accepted", err)
	}
}
