package agent

import (
	"github.com/wasmbench/wasmbench/protocol"
	"testing"
)

func TestSnapshotInspectionRejectsUnpreparedOrUnscopedCall(t *testing.T) {
	for _, name := range []string{"profile", "method", "request", "barriers", "observer"} {
		t.Run(name, func(t *testing.T) {
			c := &Client{preparedProfile: "memory"}
			r := protocol.Request{Method: "inspect", Run: &protocol.RunRequest{PhaseBarriers: true}}
			observer := func(protocol.SnapshotBoundary) error { return nil }
			switch name {
			case "profile":
				c.preparedProfile = "timing"
			case "method":
				r.Method = "run"
			case "request":
				r.Run = nil
			case "barriers":
				r.Run.PhaseBarriers = false
			case "observer":
				observer = nil
			}
			// No pipes/process: rejection must happen before emitting any request.
			if _, err := c.CallSnapshotInspection(r, observer); err == nil {
				t.Fatal("accepted unscoped inspection")
			}
		})
	}
}

func TestSnapshotDensityInspectionRejectsUnscopedCall(t *testing.T) {
	for _, name := range []string{"profile", "method", "request", "barriers", "scenario", "observer"} {
		t.Run(name, func(t *testing.T) {
			c := &Client{preparedProfile: "memory"}
			r := protocol.Request{Method: "inspect", Run: &protocol.RunRequest{Scenario: protocol.SnapshotDensityScenario, PhaseBarriers: true}}
			observer := func(protocol.SnapshotDensityBoundary) error { return nil }
			switch name {
			case "profile":
				c.preparedProfile = "timing"
			case "method":
				r.Method = "run"
			case "request":
				r.Run = nil
			case "barriers":
				r.Run.PhaseBarriers = false
			case "scenario":
				r.Run.Scenario = "process-snapshot-restore"
			case "observer":
				observer = nil
			}
			if _, err := c.CallSnapshotDensityInspection(r, observer); err == nil {
				t.Fatal("unscoped density reached control pipe")
			}
		})
	}
}
