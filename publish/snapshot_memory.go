package publish

import (
	"fmt"
	"strconv"

	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

const SnapshotMemoryViewVersion = "snapshot-process-footprint-view-v1"

type SnapshotFootprintRow struct {
	SampleIndex     int                      `json:"sample_index"`
	Stage           string                   `json:"stage"`
	Restoration     int                      `json:"restoration"`
	Role            string                   `json:"role"`
	Process         protocol.SnapshotProcess `json:"process"`
	ParentPID       uint32                   `json:"parent_pid"`
	StartNS         string                   `json:"start_ns"`
	EndNS           string                   `json:"end_ns"`
	RSSBytes        string                   `json:"rss_bytes"`
	VirtualBytes    string                   `json:"virtual_bytes"`
	PSSBytes        *string                  `json:"pss_bytes"`
	PrivateBytes    *string                  `json:"private_bytes"`
	SmapsStatus     string                   `json:"smaps_status"`
	SmapsReason     string                   `json:"smaps_reason,omitempty"`
	RawRecordIndex  int                      `json:"raw_record_index"`
	RawReadingIndex int                      `json:"raw_reading_index"`
}

type SnapshotMemoryView struct {
	Run                string                 `json:"run"`
	Trial              string                 `json:"trial"`
	Runtime            string                 `json:"runtime"`
	Workload           string                 `json:"workload"`
	Scenario           string                 `json:"scenario"`
	Status             string                 `json:"status"`
	Reason             string                 `json:"reason,omitempty"`
	RawTrial           string                 `json:"raw_trial"`
	RetainedBoundaries int                    `json:"retained_boundaries"`
	Rows               []SnapshotFootprintRow `json:"rows"`
}

func snapshotMemoryViews(b experiment.Bundle, prefix string, matched map[string]bool) ([]SnapshotMemoryView, error) {
	var views []SnapshotMemoryView
	workloads := map[string]protocol.Workload{}
	for _, w := range b.Manifest.Lock.Workloads {
		if w.ProcessSnapshot != nil {
			workloads[w.ID] = w
		}
	}
	for _, t := range b.Trials {
		w, known := workloads[t.Workload]
		if !known || t.Profile != "memory" || t.Block < 0 || (matched != nil && !matched[t.Runtime+"\x00"+t.Workload]) {
			continue
		}
		view := SnapshotMemoryView{Run: b.Manifest.ID, Trial: t.ID, Runtime: t.Runtime, Workload: t.Workload, Scenario: t.Scenario, Status: t.Status, Reason: t.Reason, RawTrial: prefix + "/trials/" + t.ID + ".json"}
		if t.SnapshotMemory != nil {
			view.RetainedBoundaries = len(t.SnapshotMemory.Records)
		}
		// Failed prefixes remain raw diagnostics; never derive successful numbers
		// from partial/failed state proofs or silently score their surviving rows.
		if t.Status != "ok" {
			views = append(views, view)
			continue
		}
		if t.SnapshotMemory == nil {
			return nil, fmt.Errorf("successful snapshot memory trial lacks evidence")
		}
		o := b.Manifest.Lock.Options
		r := protocol.RunRequest{Scenario: t.Scenario, Samples: o.Samples, Operations: o.Operations, Warmup: o.Warmup, PhaseBarriers: o.PhaseBarriers}
		if err := experiment.ValidateSnapshotMemoryEvidence(w, r, *t.SnapshotMemory); err != nil {
			return nil, err
		}
		for recordIndex, record := range t.SnapshotMemory.Records {
			for readingIndex, reading := range record.Readings {
				footprint, err := collectors.ValidateSnapshotProcessReading(reading, reading.Process, reading.ParentPID)
				if err != nil {
					return nil, err
				}
				role := []string{"source", "template", "restored"}[readingIndex]
				row := SnapshotFootprintRow{SampleIndex: record.Boundary.SampleIndex, Stage: record.Boundary.Stage, Restoration: record.Boundary.Restoration, Role: role, Process: reading.Process, ParentPID: reading.ParentPID,
					StartNS: strconv.FormatInt(reading.StartNS, 10), EndNS: strconv.FormatInt(reading.EndNS, 10), RSSBytes: strconv.FormatUint(footprint.RSSBytes, 10), VirtualBytes: strconv.FormatUint(footprint.VirtualBytes, 10),
					PSSBytes: decimalBytes(footprint.PSSBytes), PrivateBytes: decimalBytes(footprint.PrivateBytes), SmapsStatus: reading.SmapsStatus, SmapsReason: reading.SmapsReason, RawRecordIndex: recordIndex, RawReadingIndex: readingIndex}
				view.Rows = append(view.Rows, row)
			}
		}
		views = append(views, view)
	}
	return views, nil
}

func decimalBytes(n *uint64) *string {
	if n == nil {
		return nil
	}
	s := strconv.FormatUint(*n, 10)
	return &s
}
