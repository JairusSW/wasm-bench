package publish

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

func snapshotViewFixture(t *testing.T) experiment.Bundle {
	ws, err := corpus.Generate(t.TempDir(), "process-snapshots")
	if err != nil {
		t.Fatal(err)
	}
	p := &protocol.ProcessSnapshotResult{Mode: protocol.ProcessSnapshotMode, PreForkThreads: 1, Boundary: protocol.ProcessSnapshotBoundary("process-snapshot-restore"), Source: protocol.SnapshotProcess{PID: 101, StartTimeTicks: "9007199254740993"}, Template: protocol.SnapshotProcess{PID: 102, StartTimeTicks: "9007199254740994"}, Restored: protocol.SnapshotProcess{PID: 103, StartTimeTicks: "9007199254740995"}, AlternateRestored: protocol.SnapshotProcess{PID: 104, StartTimeTicks: "9007199254740996"}, SourceReleased: true, ChildrenReaped: true, SourceAfterMutation: 125, RestoredBeforeWrite: 64, RestoredAfterWrite: 64, PassiveSegmentProbe: 127, MemoryPages: 3, TableElements: 3, IndependentRestorations: 2, MemoryAtRestoreSHA256: protocol.ProcessSnapshotMemorySHA256(false), MemoryAfterFirstWriteSHA256: protocol.ProcessSnapshotMemorySHA256(true)}
	p.ClockProcess = p.Source
	e := &experiment.SnapshotMemoryEvidence{Version: experiment.SnapshotMemoryVersion, ControllerPID: 100, Diagnostics: protocol.SnapshotDiagnostics{Version: protocol.SnapshotInspectionVersion, Profile: "memory", Samples: []protocol.Sample{{Operations: 1, SampleType: "individual_operation", Verified: true, Result: protocol.Values{64}, ProcessSnapshotResult: p}}}}
	var clock int64
	for i, stage := range []string{"template_after_source_release", "restore_ready", "first_write_done", "execution_done", "restore_ready", "first_write_done", "execution_done"} {
		b := protocol.SnapshotBoundary{Version: protocol.SnapshotLiveBoundaryVersion, Stage: stage, Restoration: -1, Source: p.Source, Template: p.Template}
		refs := []protocol.SnapshotProcess{p.Source, p.Template}
		parents := []uint32{100, 101}
		if i > 0 {
			ref := p.Restored
			b.Restoration = 0
			if i > 3 {
				ref = p.AlternateRestored
				b.Restoration = 1
			}
			b.Restored = &ref
			refs = append(refs, ref)
			parents = append(parents, 102)
		}
		record := experiment.SnapshotMemoryRecord{Boundary: b}
		for j, ref := range refs {
			fields := make([]string, 20)
			for k := range fields {
				fields[k] = "0"
			}
			fields[0], fields[1], fields[19] = "S", fmt.Sprint(parents[j]), ref.StartTimeTicks
			stat := fmt.Sprintf("%d (snapshot) %s", ref.PID, strings.Join(fields, " "))
			r := collectors.SnapshotProcessReading{Version: collectors.SnapshotProcessCollectorVersion, Process: ref, ParentPID: parents[j], StartNS: clock, EndNS: clock + 1, StatBefore: stat, StatAfter: stat, Status: fmt.Sprintf("Pid: %d\nPPid: %d\nState: S (sleeping)\nThreads: 1\nVmRSS: 0 kB\nVmSize: 8 kB\n", ref.PID, parents[j]), SmapsStatus: "permission_denied", SmapsReason: "denied fixture"}
			clock += 2
			record.Readings = append(record.Readings, r)
		}
		e.Records = append(e.Records, record)
	}
	return experiment.Bundle{Manifest: experiment.Manifest{ID: "native-run", Lock: experiment.Lock{Workloads: ws, Options: experiment.Options{Profile: "memory", Samples: 1, Operations: 1, PhaseBarriers: true}}}, Trials: []experiment.Trial{{ID: "live", Runtime: "r", Workload: ws[0].ID, Scenario: "process-snapshot-restore", Profile: "memory", Status: "ok", SnapshotMemory: e}}}
}

func TestSnapshotFootprintViewsValidateAndPreserveDomains(t *testing.T) {
	b := snapshotViewFixture(t)
	views, err := snapshotMemoryViews(b, "raw", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || len(views[0].Rows) != 20 || views[0].RawTrial != "raw/trials/live.json" {
		t.Fatal("coverage/raw provenance lost", views)
	}
	row := views[0].Rows[4]
	if row.Role != "restored" || row.PSSBytes != nil || row.PrivateBytes != nil || row.RSSBytes != "0" || row.VirtualBytes != "8192" || row.Process.StartTimeTicks != "9007199254740995" || row.StartNS != "8" || row.EndNS != "9" || row.SmapsStatus != "permission_denied" || row.RawRecordIndex != 1 || row.RawReadingIndex != 2 {
		t.Fatal("zero, exact identity, clock or missing smaps changed", row)
	}
	paired, err := snapshotMemoryViews(b, "raw-memory", map[string]bool{"r\x00" + b.Trials[0].Workload: true})
	if err != nil || paired[0].RawTrial != "raw-memory/trials/live.json" {
		t.Fatal("paired provenance lost", err)
	}
	filtered, err := snapshotMemoryViews(b, "raw-memory", map[string]bool{})
	if err != nil || len(filtered) != 0 {
		t.Fatal("unmatched workload included")
	}
	b.Trials[0].SnapshotMemory.Records[1].Readings[2].Process.StartTimeTicks = "wrong"
	if _, err := snapshotMemoryViews(b, "raw", nil); err == nil {
		t.Fatal("forged lineage rendered")
	}
}

func TestSnapshotFootprintFailureNeverRendersPrefixNumbers(t *testing.T) {
	b := snapshotViewFixture(t)
	b.Trials[0].Status = "error"
	b.Trials[0].Reason = "child failed"
	views, err := snapshotMemoryViews(b, "raw", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || len(views[0].Rows) != 0 || views[0].RetainedBoundaries != 7 || views[0].Reason != "child failed" {
		t.Fatal("failed prefix promoted or hidden")
	}
	b.Trials[0].Status = "unsupported"
	b.Trials[0].SnapshotMemory = nil
	views, err = snapshotMemoryViews(b, "raw", nil)
	if err != nil || len(views) != 1 || views[0].RetainedBoundaries != 0 {
		t.Fatal("unsupported omitted", err)
	}
	b.Trials[0].Block = -1
	views, err = snapshotMemoryViews(b, "raw", nil)
	if err != nil || len(views) != 0 {
		t.Fatal("sacrificial timing shown as memory")
	}
	n := uint64(9007199254740993)
	if *decimalBytes(&n) != "9007199254740993" || decimalBytes(nil) != nil {
		t.Fatal("integer precision or null changed")
	}
}

func TestRetainedSnapshotMemoryReport(t *testing.T) {
	root := os.Getenv("WASMBENCH_SNAPSHOT_MEMORY_REPORT_RUN")
	if root == "" {
		t.Skip("requires sealed native memory bundle")
	}
	d, err := buildReportDataset(root, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if d.SnapshotMemoryViewVersion != SnapshotMemoryViewVersion || len(d.SnapshotMemoryViews) != 16 {
		t.Fatal("native coverage lost")
	}
	count := 0
	for _, view := range d.SnapshotMemoryViews {
		if view.Status != "ok" || len(view.Rows) != 40 {
			t.Fatal("native coverage lost")
		}
		count += len(view.Rows)
	}
	if count != 640 {
		t.Fatal("raw process count changed")
	}
	if timing := os.Getenv("WASMBENCH_SNAPSHOT_MEMORY_REPORT_TIMING_RUN"); timing != "" {
		paired, err := buildReportDataset(timing, root, "")
		if err != nil {
			t.Fatal(err)
		}
		if paired.MemorySource == nil || len(paired.SnapshotMemoryViews) != 16 || paired.SnapshotMemoryViewVersion != SnapshotMemoryViewVersion {
			t.Fatal("paired snapshot views missing")
		}
		for _, view := range paired.SnapshotMemoryViews {
			if !strings.HasPrefix(view.RawTrial, "raw-memory/trials/") || view.Run != paired.MemorySource.ID {
				t.Fatal("paired view links wrong source")
			}
		}
	}
	html, err := renderReportHTML(d)
	if err != nil || !strings.Contains(string(html), "Snapshot process footprints") {
		t.Fatal("viewer absent", err)
	}
}
