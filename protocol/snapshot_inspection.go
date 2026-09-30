package protocol

import "fmt"

const SnapshotLiveBoundaryVersion = "linux-process-snapshot-live-boundary-v1"
const SnapshotInspectionVersion = "linux-process-snapshot-live-inspection-v1"

type SnapshotBoundary struct {
	Version     string           `json:"version"`
	SampleIndex int              `json:"sample_index"`
	Stage       string           `json:"stage"`
	Restoration int              `json:"restoration"`
	Source      SnapshotProcess  `json:"source"`
	Template    SnapshotProcess  `json:"template"`
	Restored    *SnapshotProcess `json:"restored"`
}

// Diagnostic timings are retained for provenance, never headline eligibility.
// This payload is deliberately separate from Response.Samples.
type SnapshotDiagnostics struct {
	Version         string   `json:"version"`
	Profile         string   `json:"profile"`
	LatencyEligible bool     `json:"latency_eligible"`
	Samples         []Sample `json:"samples"`
}

func ValidateSnapshotInspection(p Preparation, r RunRequest) error {
	if p.Profile != "memory" || !r.PhaseBarriers {
		return fmt.Errorf("snapshot inspection requires memory and explicit barriers")
	}
	// Same operations/state policy, with a distinct instrumentation profile.
	p.Profile, r.PhaseBarriers = "timing", false
	return ValidateProcessSnapshot(&p, &r)
}

func ValidateSnapshotBoundary(b SnapshotBoundary) error {
	if b.Version != SnapshotLiveBoundaryVersion || b.SampleIndex < 0 || b.Source.PID == 0 || b.Template.PID == 0 || b.Source.PID == b.Template.PID {
		return fmt.Errorf("invalid snapshot live boundary")
	}
	if b.Stage == "template_after_source_release" {
		if b.Restoration != -1 || b.Restored != nil {
			return fmt.Errorf("invalid template boundary")
		}
		return nil
	}
	if b.Stage != "restore_ready" && b.Stage != "first_write_done" && b.Stage != "execution_done" {
		return fmt.Errorf("unknown snapshot live stage")
	}
	if b.Restoration < 0 || b.Restoration > 1 || b.Restored == nil || b.Restored.PID == 0 || b.Restored.PID == b.Source.PID || b.Restored.PID == b.Template.PID {
		return fmt.Errorf("invalid restored boundary")
	}
	return nil
}

func VerifySnapshotInspection(w Workload, r RunRequest, events []SnapshotBoundary, d SnapshotDiagnostics) error {
	if err := ValidateSnapshotInspection(Preparation{Workload: w, Profile: "memory"}, r); err != nil {
		return err
	}
	if d.Version != SnapshotInspectionVersion || d.Profile != "memory" || d.LatencyEligible {
		return fmt.Errorf("invalid inspection scope/eligibility")
	}
	r.PhaseBarriers = false
	if err := VerifyProcessSnapshotSequence(w, r, d.Samples); err != nil {
		return err
	}
	if len(events) != 7*r.Samples {
		return fmt.Errorf("incomplete snapshot live boundaries")
	}
	stages := []string{"template_after_source_release", "restore_ready", "first_write_done", "execution_done", "restore_ready", "first_write_done", "execution_done"}
	for i, b := range events {
		if err := ValidateSnapshotBoundary(b); err != nil {
			return err
		}
		index, part := i/7, i%7
		proof := d.Samples[index].ProcessSnapshotResult
		if b.SampleIndex != index || b.Stage != stages[part] || b.Source != proof.Source || b.Template != proof.Template {
			return fmt.Errorf("live boundary differs from completed snapshot proof")
		}
		if part == 0 {
			continue
		}
		want := proof.Restored
		restoration := 0
		if part > 3 {
			want = proof.AlternateRestored
			restoration = 1
		}
		if b.Restoration != restoration || *b.Restored != want {
			return fmt.Errorf("live restoration differs from completed proof")
		}
	}
	return nil
}
