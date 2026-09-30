package experiment

import (
	"fmt"
	"slices"

	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/protocol"
)

const SnapshotDensityEvidenceVersion = "native-snapshot-density-evidence-v1"

type SnapshotDensityRecord struct {
	Boundary protocol.SnapshotDensityBoundary    `json:"boundary"`
	Readings []collectors.SnapshotProcessReading `json:"readings"`
}

type SnapshotDensityEvidence struct {
	Version       string                        `json:"version"`
	ControllerPID uint32                        `json:"controller_pid"`
	Backend       string                        `json:"backend"`
	Instances     int                           `json:"instances"`
	Proof         protocol.SnapshotDensityProof `json:"proof"`
	Records       []SnapshotDensityRecord       `json:"records"`
}

// Qualification-only, not a sealed product trial or publication gate. The
// retained raw readings attest each entire declared group at all four barriers.
func ValidateSnapshotDensityEvidence(e SnapshotDensityEvidence) error {
	events := make([]protocol.SnapshotDensityBoundary, len(e.Records))
	for i, r := range e.Records {
		events[i] = r.Boundary
	}
	if err := protocol.VerifySnapshotDensityQualification(e.Backend, e.Instances, events, e.Proof); err != nil {
		return err
	}
	return ValidateSnapshotDensityPrefix(e)
}

// ValidateSnapshotDensityPrefix checks retained barriers and raw procfs records
// only. It deliberately does not attest completion, state proofs or cleanup.
func ValidateSnapshotDensityPrefix(e SnapshotDensityEvidence) error {
	if e.Version != SnapshotDensityEvidenceVersion || e.ControllerPID == 0 || e.ControllerPID > 2147483647 || !slices.Contains([]string{"cranelift", "winch"}, e.Backend) || e.Instances < 1 || e.Instances > 32 || len(e.Records) < 1 || len(e.Records) > 4 {
		return fmt.Errorf("invalid density collector envelope/prefix")
	}
	stages := []string{"template_after_source_release", "idle", "touched", "executed"}
	first := e.Records[0].Boundary
	var previousEnd int64
	for index, record := range e.Records {
		b := record.Boundary
		if err := protocol.ValidateSnapshotDensityBoundary(b); err != nil {
			return err
		}
		if b.SampleIndex != first.SampleIndex || b.Stage != stages[index] || b.Instances != e.Instances || b.Source != first.Source || b.Template != first.Template || (index > 1 && !slices.Equal(b.Restored, e.Records[1].Boundary.Restored)) {
			return fmt.Errorf("density prefix changes order, lineage or held membership")
		}
		refs := append([]protocol.SnapshotProcess{b.Source, b.Template}, b.Restored...)
		if len(record.Readings) != len(refs) {
			return fmt.Errorf("incomplete simultaneously held process group")
		}
		for i, ref := range refs {
			if ref.PID == e.ControllerPID {
				return fmt.Errorf("density member aliases collector")
			}
			parent := b.Template.PID
			if i == 0 {
				parent = e.ControllerPID
			} else if i == 1 {
				parent = b.Source.PID
			}
			r := record.Readings[i]
			f, err := collectors.ValidateSnapshotProcessReading(r, ref, parent)
			if err != nil {
				return err
			}
			if f.Threads != 1 || r.StartNS < previousEnd {
				return fmt.Errorf("invalid density process threads/collector ordering")
			}
			previousEnd = r.EndNS
		}
	}
	return nil
}
