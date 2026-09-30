package publish

import (
	"math"
	"path/filepath"
	"testing"

	"github.com/parquet-go/parquet-go"
	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

func TestCounterParquetExactIntegersAndOutcomes(t *testing.T) {
	zero, max, enabled, running := uint64(0), uint64(math.MaxUint64), uint64(100), uint64(50)
	reading := collectors.PerfReading{CollectorVersion: "perf-test", Scope: "cgroup_cpu", PrivilegeScope: "user_kernel", CPU: 7, Event: collectors.PerfEvent{Name: "cycles", Type: 0, Config: math.MaxUint64}, Count: &max, EnabledNS: &enabled, RunningNS: &running, Status: "multiplexed"}
	z := reading
	z.Count = &zero
	z.RunningNS = &enabled
	z.Status = "available"
	z.CPU = 8
	denied := reading
	denied.Count = nil
	denied.EnabledNS = nil
	denied.RunningNS = nil
	denied.Status = "permission_denied"
	denied.Reason = "not permitted"
	denied.CPU = 9
	changed := reading
	changed.Status = "coverage_changed"
	changed.Reason = "CPU set changed"
	b := experiment.Bundle{Manifest: experiment.Manifest{ID: "run"}, Trials: []experiment.Trial{
		{ID: "raw", Profile: "counters", Status: "error", Reason: "later failure", Runtime: "runtime", Workload: "work", Scenario: "steady", Block: 2, Samples: []protocol.Sample{{Index: 3, Verified: true, Warmup: true, Operations: 5}}, CounterPhases: []agent.CounterPhase{{CollectorVersion: "window-v1", Sample: 3, Phase: "steady/barrier_window", Status: "partial", Readings: []collectors.PerfReading{reading, z, denied, changed}}}},
		{ID: "no-cgroup", Profile: "counters", Status: "ok", CounterPhases: []agent.CounterPhase{{Sample: 0, Status: "unavailable", Reason: "no cgroup"}}},
		{ID: "unsupported", Profile: "counters", Status: "unsupported", Reason: "no capability"},
		{ID: "admission", Profile: "counters", Status: "ok", Block: -1},
		{ID: "timing", Profile: "timing", Status: "ok"},
	}}
	path := filepath.Join(t.TempDir(), "counters.parquet")
	if err := ExportCounters(b, path); err != nil {
		t.Fatal(err)
	}
	rows, err := parquet.ReadFile[CounterRow](path)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 7 {
		t.Fatal(len(rows))
	}
	r := rows[0]
	if r.SampleWarmup == nil || !*r.SampleWarmup || r.SampleOperations == nil || *r.SampleOperations != 5 || rows[4].SampleWarmup != nil || rows[4].SampleOperations != nil {
		t.Fatal("batch context lost or inferred", rows)
	}
	if *r.RawCount != math.MaxUint64 || *r.EventConfig != math.MaxUint64 || *r.EnabledNS != 100 || *r.RunningNS != 50 || r.ReadingStatus != "multiplexed" || *r.CPU != 7 || *r.EventType != 0 {
		t.Fatal(r)
	}
	if r.ExportVersion != CounterExportVersion || r.TrialStatus != "error" || r.TrialReason != "later failure" || *r.WindowIndex != 0 || *r.ReadingIndex != 0 || *r.SampleIndex != 3 || !*r.SampleVerified || r.WindowStatus != "partial" || r.Scope != reading.Scope || r.PrivilegeScope != reading.PrivilegeScope || r.CollectorVersion != "perf-test" || r.WindowCollectorVersion != "window-v1" {
		t.Fatal(r)
	}
	if rows[1].RawCount == nil || *rows[1].RawCount != 0 {
		t.Fatal("zero lost")
	}
	if rows[2].RawCount != nil || rows[2].EnabledNS != nil || rows[2].ReadingReason != "not permitted" {
		t.Fatal(rows[2])
	}
	if rows[3].RawCount == nil || *rows[3].RawCount != max || rows[3].ReadingStatus != "coverage_changed" {
		t.Fatal("diagnostic evidence lost")
	}
	if rows[4].RowKind != "window_outcome" || rows[4].CPU != nil || rows[4].RawCount != nil || rows[4].ReadingIndex != nil || rows[4].SampleVerified != nil || rows[4].WindowReason != "no cgroup" {
		t.Fatal(rows[4])
	}
	if rows[5].RowKind != "trial_outcome" || rows[5].SampleIndex != nil || rows[5].TrialStatus != "unsupported" || rows[6].Block != -1 {
		t.Fatal(rows[5:])
	}
	if err = ExportCounters(b, path); err == nil {
		t.Fatal("overwrote evidence")
	}
}

func TestCounterParquetEmptyTypedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.parquet")
	if err := ExportCounters(experiment.Bundle{}, path); err != nil {
		t.Fatal(err)
	}
	rows, err := parquet.ReadFile[CounterRow](path)
	if err != nil || len(rows) != 0 {
		t.Fatal(rows, err)
	}
}
