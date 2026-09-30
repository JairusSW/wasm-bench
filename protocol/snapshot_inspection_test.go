package protocol_test

import (
	"github.com/wasmbench/wasmbench/protocol"
	"testing"
)

func inspectionFixture() ([]protocol.SnapshotBoundary, protocol.SnapshotDiagnostics, protocol.RunRequest) {
	r := protocol.RunRequest{Scenario: "process-snapshot-execute", Samples: 1, Operations: 1, PhaseBarriers: true}
	s := snapshotSample(r.Scenario)
	p := s.ProcessSnapshotResult
	var events []protocol.SnapshotBoundary
	for i, stage := range []string{"template_after_source_release", "restore_ready", "first_write_done", "execution_done", "restore_ready", "first_write_done", "execution_done"} {
		b := protocol.SnapshotBoundary{Version: protocol.SnapshotLiveBoundaryVersion, Stage: stage, Restoration: -1, Source: p.Source, Template: p.Template}
		if i > 0 {
			ref := p.Restored
			b.Restoration = 0
			if i > 3 {
				ref = p.AlternateRestored
				b.Restoration = 1
			}
			b.Restored = &ref
		}
		events = append(events, b)
	}
	return events, protocol.SnapshotDiagnostics{Version: protocol.SnapshotInspectionVersion, Profile: "memory", Samples: []protocol.Sample{s}}, r
}
func TestSnapshotInspectionContract(t *testing.T) {
	w := snapshotWorkload(t)
	events, d, r := inspectionFixture()
	if err := protocol.VerifySnapshotInspection(w, r, events, d); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*[]protocol.SnapshotBoundary, *protocol.SnapshotDiagnostics, *protocol.RunRequest){
		"timing profile": func(_ *[]protocol.SnapshotBoundary, d *protocol.SnapshotDiagnostics, _ *protocol.RunRequest) {
			d.Profile = "timing"
		},
		"headline eligible": func(_ *[]protocol.SnapshotBoundary, d *protocol.SnapshotDiagnostics, _ *protocol.RunRequest) {
			d.LatencyEligible = true
		},
		"version": func(_ *[]protocol.SnapshotBoundary, d *protocol.SnapshotDiagnostics, _ *protocol.RunRequest) {
			d.Version = "unknown"
		},
		"no barriers": func(_ *[]protocol.SnapshotBoundary, _ *protocol.SnapshotDiagnostics, r *protocol.RunRequest) {
			r.PhaseBarriers = false
		},
		"incomplete": func(e *[]protocol.SnapshotBoundary, _ *protocol.SnapshotDiagnostics, _ *protocol.RunRequest) {
			*e = (*e)[:6]
		},
		"duplicate": func(e *[]protocol.SnapshotBoundary, _ *protocol.SnapshotDiagnostics, _ *protocol.RunRequest) {
			(*e)[2] = (*e)[1]
		},
		"reordered": func(e *[]protocol.SnapshotBoundary, _ *protocol.SnapshotDiagnostics, _ *protocol.RunRequest) {
			(*e)[2], (*e)[3] = (*e)[3], (*e)[2]
		},
		"sample index": func(e *[]protocol.SnapshotBoundary, _ *protocol.SnapshotDiagnostics, _ *protocol.RunRequest) {
			(*e)[1].SampleIndex = 1
		},
		"source": func(e *[]protocol.SnapshotBoundary, _ *protocol.SnapshotDiagnostics, _ *protocol.RunRequest) {
			(*e)[1].Source.PID = 999
		},
		"template": func(e *[]protocol.SnapshotBoundary, _ *protocol.SnapshotDiagnostics, _ *protocol.RunRequest) {
			(*e)[1].Template.PID = 999
		},
		"restoration": func(e *[]protocol.SnapshotBoundary, _ *protocol.SnapshotDiagnostics, _ *protocol.RunRequest) {
			(*e)[4].Restoration = 0
		},
		"restored birth": func(e *[]protocol.SnapshotBoundary, _ *protocol.SnapshotDiagnostics, _ *protocol.RunRequest) {
			(*e)[2].Restored.StartTimeTicks = "999"
		},
		"capture child": func(e *[]protocol.SnapshotBoundary, _ *protocol.SnapshotDiagnostics, _ *protocol.RunRequest) {
			(*e)[0].Restored = (*e)[1].Restored
		},
		"missing live child": func(e *[]protocol.SnapshotBoundary, _ *protocol.SnapshotDiagnostics, _ *protocol.RunRequest) {
			(*e)[1].Restored = nil
		},
		"unknown stage": func(e *[]protocol.SnapshotBoundary, _ *protocol.SnapshotDiagnostics, _ *protocol.RunRequest) {
			(*e)[1].Stage = "made-up"
		},
		"missing final samples": func(_ *[]protocol.SnapshotBoundary, d *protocol.SnapshotDiagnostics, _ *protocol.RunRequest) {
			d.Samples = nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			e, d, r := inspectionFixture()
			change(&e, &d, &r)
			if err := protocol.VerifySnapshotInspection(w, r, e, d); err == nil {
				t.Fatal("accepted forged/incomplete inspection")
			}
		})
	}
}
