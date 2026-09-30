package publish

import (
	"fmt"
	"strconv"

	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

const SnapshotDensityViewVersion = "snapshot-density-process-footprints-v1"

type DensityFootprintRow struct {
	SnapshotFootprintRow
	GroupIndex int  `json:"group_index"`
	ChildIndex *int `json:"child_index"`
}

type SnapshotDensityView struct {
	Run                string                `json:"run"`
	Trial              string                `json:"trial"`
	Runtime            string                `json:"runtime"`
	Workload           string                `json:"workload"`
	Status             string                `json:"status"`
	Reason             string                `json:"reason,omitempty"`
	RawTrial           string                `json:"raw_trial"`
	RetainedBoundaries int                   `json:"retained_boundaries"`
	Rows               []DensityFootprintRow `json:"rows"`
}

// Derive exact per-process footprints only after the full trial proof passes.
// Failed prefixes retain raw links and boundary counts, never usable values.
func snapshotDensityViews(b experiment.Bundle) ([]SnapshotDensityView, error) {
	workloads := map[string]protocol.Workload{}
	for _, w := range b.Manifest.Lock.Workloads {
		if w.SnapshotDensity != nil {
			workloads[w.ID] = w
		}
	}
	var out []SnapshotDensityView
	for _, t := range b.Trials {
		w, known := workloads[t.Workload]
		if !known || t.Block < 0 || t.Profile != "memory" || t.Scenario != protocol.SnapshotDensityScenario {
			continue
		}
		view := SnapshotDensityView{Run: b.Manifest.ID, Trial: t.ID, Runtime: t.Runtime, Workload: t.Workload, Status: t.Status, Reason: t.Reason, RawTrial: "raw/trials/" + t.ID + ".json"}
		if t.SnapshotDensity != nil {
			for _, g := range t.SnapshotDensity.Groups {
				view.RetainedBoundaries += len(g.Records)
			}
		}
		if t.Status != "ok" {
			out = append(out, view)
			continue
		}
		if t.SnapshotDensity == nil {
			return nil, fmt.Errorf("density trial has no footprints")
		}
		backend := ""
		for _, r := range b.Manifest.Lock.Runtimes {
			if r.ID == t.Runtime && r.Description != nil {
				backend = r.Description.Backend
			}
		}
		o := b.Manifest.Lock.Options
		request := protocol.RunRequest{Scenario: t.Scenario, Samples: o.Samples, Operations: o.Operations, Warmup: o.Warmup, PhaseBarriers: o.PhaseBarriers}
		if err := experiment.ValidateSnapshotDensityTrial(w, request, backend, *t.SnapshotDensity); err != nil {
			return nil, err
		}
		for groupIndex, g := range t.SnapshotDensity.Groups {
			for recordIndex, record := range g.Records {
				for readingIndex, reading := range record.Readings {
					f, err := collectors.ValidateSnapshotProcessReading(reading, reading.Process, reading.ParentPID)
					if err != nil {
						return nil, err
					}
					role := "restored"
					var childIndex *int
					if readingIndex == 0 {
						role = "source"
					} else if readingIndex == 1 {
						role = "template"
					} else {
						n := readingIndex - 2
						childIndex = &n
					}
					outRow := DensityFootprintRow{GroupIndex: groupIndex, ChildIndex: childIndex, SnapshotFootprintRow: SnapshotFootprintRow{SampleIndex: record.Boundary.SampleIndex, Stage: record.Boundary.Stage, Role: role, Process: reading.Process, ParentPID: reading.ParentPID, StartNS: strconv.FormatInt(reading.StartNS, 10), EndNS: strconv.FormatInt(reading.EndNS, 10), RSSBytes: strconv.FormatUint(f.RSSBytes, 10), VirtualBytes: strconv.FormatUint(f.VirtualBytes, 10), PSSBytes: decimalBytes(f.PSSBytes), PrivateBytes: decimalBytes(f.PrivateBytes), SmapsStatus: reading.SmapsStatus, SmapsReason: reading.SmapsReason, RawRecordIndex: recordIndex, RawReadingIndex: readingIndex}}
					view.Rows = append(view.Rows, outRow)
				}
			}
		}
		out = append(out, view)
	}
	return out, nil
}
