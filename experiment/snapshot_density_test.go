package experiment

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/protocol"
)

func densityEvidenceFixture() SnapshotDensityEvidence {
	zero := int64(0)
	source := protocol.SnapshotProcess{PID: 101, StartTimeTicks: "9007199254740993"}
	template := protocol.SnapshotProcess{PID: 102, StartTimeTicks: "9007199254740994"}
	refs := []protocol.SnapshotProcess{{PID: 103, StartTimeTicks: "9007199254740995"}, {PID: 104, StartTimeTicks: "9007199254740996"}}
	e := SnapshotDensityEvidence{Version: SnapshotDensityEvidenceVersion, ControllerPID: 100, Backend: "cranelift", Instances: 2}
	e.Proof = protocol.SnapshotDensityProof{Version: protocol.SnapshotDensityWorkerVersion, Runtime: "wasmtime", RuntimeVersion: "46.0.1", Backend: e.Backend, Architecture: "aarch64", WasmSHA256: protocol.ProcessSnapshotArtifactSHA256, QualificationOnly: true, Profile: "memory", Mode: "simultaneous_linux_process_cow", Instances: 2, PreForkThreads: 1, Source: source, Template: template, SourceReleased: true, SourceAfterMutation: 125, ProvisionClockProcess: source, ProvisionBoundary: "restore_request_to_all_live_ready_identity_frames", ProvisionElapsedNS: &zero, ChildrenReaped: true, TemplateReaped: true, TemplateCheck: 64, TemplateMemorySHA256: protocol.SnapshotDensityMemorySHA256(-1, false), TemplatePassiveSegmentProbe: 127}
	for index, ref := range refs {
		e.Proof.Children = append(e.Proof.Children, protocol.SnapshotDensityChild{Index: index, Process: ref, Before: 64, After: 64, MemoryBeforeSHA256: protocol.SnapshotDensityMemorySHA256(-1, false), MemoryTouchedSHA256: protocol.SnapshotDensityMemorySHA256(index, false), MemoryExecutedSHA256: protocol.SnapshotDensityMemorySHA256(index, true), TouchByte: 22 + index, TouchedOffsets: []int{65535, 131071, 196607}, TouchElapsedNS: &zero, TouchClockProcess: ref, ExecuteElapsedNS: &zero, ExecuteClockProcess: ref, MemoryPages: 3, TableElements: 3, PassiveSegmentProbe: 127})
	}
	var clock int64
	for index, stage := range []string{"template_after_source_release", "idle", "touched", "executed"} {
		b := protocol.SnapshotDensityBoundary{Version: protocol.SnapshotDensityBoundaryVersion, Stage: stage, Instances: 2, Source: source, Template: template}
		if index > 0 {
			b.Restored = append([]protocol.SnapshotProcess{}, refs...)
		}
		record := SnapshotDensityRecord{Boundary: b}
		for index, ref := range append([]protocol.SnapshotProcess{source, template}, b.Restored...) {
			parent := template.PID
			if index == 0 {
				parent = e.ControllerPID
			} else if index == 1 {
				parent = source.PID
			}
			fields := make([]string, 20)
			for i := range fields {
				fields[i] = "0"
			}
			fields[0], fields[1], fields[19] = "S", fmt.Sprint(parent), ref.StartTimeTicks
			stat := fmt.Sprintf("%d (density ) fixture) %s\n", ref.PID, strings.Join(fields, " "))
			record.Readings = append(record.Readings, collectors.SnapshotProcessReading{Version: collectors.SnapshotProcessCollectorVersion, Process: ref, ParentPID: parent, StartNS: clock, EndNS: clock + 1, StatBefore: stat, StatAfter: stat, Status: fmt.Sprintf("Pid: %d\nPPid: %d\nState: S (sleeping)\nThreads: 1\nVmRSS: 0 kB\nVmSize: 8 kB\n", ref.PID, parent), SmapsStatus: "permission_denied", SmapsReason: "fixture denied"})
			clock += 2
		}
		e.Records = append(e.Records, record)
	}
	return e
}

func TestSnapshotDensityEvidenceRawContract(t *testing.T) {
	e := densityEvidenceFixture()
	if err := ValidateSnapshotDensityEvidence(e); err != nil {
		t.Fatal(err)
	}
	for length := 1; length <= 4; length++ {
		prefix := densityEvidenceFixture()
		prefix.Records = prefix.Records[:length]
		prefix.Proof = protocol.SnapshotDensityProof{}
		if err := ValidateSnapshotDensityPrefix(prefix); err != nil {
			t.Fatal(err)
		}
		if ValidateSnapshotDensityEvidence(prefix) == nil {
			t.Fatal("prefix accepted as completed evidence")
		}
	}
	for name, mutate := range map[string]func(*SnapshotDensityEvidence){
		"collector alias":    func(e *SnapshotDensityEvidence) { e.ControllerPID = e.Proof.Template.PID },
		"invalid controller": func(e *SnapshotDensityEvidence) { e.ControllerPID = 0 },
		"foreign backend":    func(e *SnapshotDensityEvidence) { e.Backend = "other" },
		"missing member":     func(e *SnapshotDensityEvidence) { e.Records[1].Readings = e.Records[1].Readings[:3] },
		"extra reading": func(e *SnapshotDensityEvidence) {
			e.Records[1].Readings = append(e.Records[1].Readings, e.Records[1].Readings[0])
		},
		"reading reordered": func(e *SnapshotDensityEvidence) {
			e.Records[1].Readings[2], e.Records[1].Readings[3] = e.Records[1].Readings[3], e.Records[1].Readings[2]
		},
		"wrong parent": func(e *SnapshotDensityEvidence) { e.Records[1].Readings[2].ParentPID = e.Proof.Source.PID },
		"changed birth": func(e *SnapshotDensityEvidence) {
			e.Records[1].Readings[2].StatAfter = strings.Replace(e.Records[1].Readings[2].StatAfter, "9007199254740995", "9007199254740996", 1)
		},
		"extra threads": func(e *SnapshotDensityEvidence) {
			e.Records[1].Readings[2].Status = strings.Replace(e.Records[1].Readings[2].Status, "Threads: 1", "Threads: 2", 1)
		},
		"overlapping clock": func(e *SnapshotDensityEvidence) { e.Records[1].Readings[2].StartNS = 0 },
		"empty prefix":      func(e *SnapshotDensityEvidence) { e.Records = nil },
		"reordered prefix":  func(e *SnapshotDensityEvidence) { e.Records[1], e.Records[2] = e.Records[2], e.Records[1] },
		"changed prefix membership": func(e *SnapshotDensityEvidence) {
			e.Records[2].Boundary.Restored[0].StartTimeTicks = "9999999999999999"
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := densityEvidenceFixture()
			mutate(&e)
			if ValidateSnapshotDensityEvidence(e) == nil {
				t.Fatal("forged completed evidence accepted")
			}
			if ValidateSnapshotDensityPrefix(e) == nil {
				t.Fatal("forged raw prefix accepted")
			}
		})
	}
}
