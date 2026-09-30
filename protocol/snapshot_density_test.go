package protocol

import "testing"

func densityQualificationFixture() ([]SnapshotDensityBoundary, SnapshotDensityProof) {
	zero := int64(0)
	source := SnapshotProcess{PID: 101, StartTimeTicks: "9007199254740993"}
	template := SnapshotProcess{PID: 102, StartTimeTicks: "9007199254740994"}
	p := SnapshotDensityProof{Version: SnapshotDensityWorkerVersion, Runtime: "wasmtime", RuntimeVersion: "46.0.1", Backend: "cranelift", Architecture: "aarch64", WasmSHA256: ProcessSnapshotArtifactSHA256, QualificationOnly: true, Profile: "memory", Mode: "simultaneous_linux_process_cow", Instances: 2, PreForkThreads: 1, Source: source, Template: template, SourceReleased: true, SourceAfterMutation: 125, ProvisionClockProcess: source, ProvisionBoundary: "restore_request_to_all_live_ready_identity_frames", ProvisionElapsedNS: &zero, ChildrenReaped: true, TemplateReaped: true, TemplateCheck: 64, TemplateMemorySHA256: SnapshotDensityMemorySHA256(-1, false), TemplatePassiveSegmentProbe: 127}
	refs := []SnapshotProcess{{PID: 103, StartTimeTicks: "9007199254740995"}, {PID: 104, StartTimeTicks: "9007199254740996"}}
	for i, ref := range refs {
		p.Children = append(p.Children, SnapshotDensityChild{Index: i, Process: ref, Before: 64, After: 64, MemoryBeforeSHA256: SnapshotDensityMemorySHA256(-1, false), MemoryTouchedSHA256: SnapshotDensityMemorySHA256(i, false), MemoryExecutedSHA256: SnapshotDensityMemorySHA256(i, true), TouchByte: 22 + i, TouchedOffsets: []int{65535, 131071, 196607}, TouchElapsedNS: &zero, TouchClockProcess: ref, ExecuteElapsedNS: &zero, ExecuteClockProcess: ref, MemoryPages: 3, TableElements: 3, PassiveSegmentProbe: 127})
	}
	var events []SnapshotDensityBoundary
	for i, stage := range []string{"template_after_source_release", "idle", "touched", "executed"} {
		b := SnapshotDensityBoundary{Version: SnapshotDensityBoundaryVersion, Stage: stage, Instances: 2, Source: source, Template: template}
		if i > 0 {
			b.Restored = append([]SnapshotProcess{}, refs...)
		}
		events = append(events, b)
	}
	return events, p
}

func TestSnapshotDensityQualification(t *testing.T) {
	e, p := densityQualificationFixture()
	if err := VerifySnapshotDensityQualification("cranelift", 2, e, p); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*[]SnapshotDensityBoundary, *SnapshotDensityProof){
		"missing event":       func(e *[]SnapshotDensityBoundary, _ *SnapshotDensityProof) { *e = (*e)[:3] },
		"reordered":           func(e *[]SnapshotDensityBoundary, _ *SnapshotDensityProof) { (*e)[1], (*e)[2] = (*e)[2], (*e)[1] },
		"missing idle member": func(e *[]SnapshotDensityBoundary, _ *SnapshotDensityProof) { (*e)[1].Restored = (*e)[1].Restored[:1] },
		"sequential PID reuse": func(e *[]SnapshotDensityBoundary, p *SnapshotDensityProof) {
			p.Children[1].Process.PID = p.Children[0].Process.PID
			for i := 1; i < 4; i++ {
				(*e)[i].Restored[1] = p.Children[1].Process
			}
		},
		"changed group": func(e *[]SnapshotDensityBoundary, _ *SnapshotDensityProof) {
			(*e)[2].Restored[1].StartTimeTicks = "999"
		},
		"source alive":      func(_ *[]SnapshotDensityBoundary, p *SnapshotDensityProof) { p.SourceReleased = false },
		"unreaped":          func(_ *[]SnapshotDensityBoundary, p *SnapshotDensityProof) { p.ChildrenReaped = false },
		"template unreaped": func(_ *[]SnapshotDensityBoundary, p *SnapshotDensityProof) { p.TemplateReaped = false },
		"wrong template": func(_ *[]SnapshotDensityBoundary, p *SnapshotDensityProof) {
			p.TemplateMemorySHA256 = SnapshotDensityMemorySHA256(0, false)
		},
		"wrong initial memory": func(_ *[]SnapshotDensityBoundary, p *SnapshotDensityProof) {
			p.Children[1].MemoryBeforeSHA256 = SnapshotDensityMemorySHA256(0, false)
		},
		"aliased marker": func(_ *[]SnapshotDensityBoundary, p *SnapshotDensityProof) {
			p.Children[1].MemoryTouchedSHA256 = p.Children[0].MemoryTouchedSHA256
		},
		"wrong post probe": func(_ *[]SnapshotDensityBoundary, p *SnapshotDensityProof) {
			p.Children[1].MemoryExecutedSHA256 = p.Children[1].MemoryTouchedSHA256
		},
		"wrong touched pages": func(_ *[]SnapshotDensityBoundary, p *SnapshotDensityProof) {
			p.Children[0].TouchedOffsets = []int{65535}
		},
		"wrong touch clock": func(_ *[]SnapshotDensityBoundary, p *SnapshotDensityProof) {
			p.Children[0].TouchClockProcess = p.Source
		},
		"missing timing":           func(_ *[]SnapshotDensityBoundary, p *SnapshotDensityProof) { p.Children[0].TouchElapsedNS = nil },
		"missing provision timing": func(_ *[]SnapshotDensityBoundary, p *SnapshotDensityProof) { p.ProvisionElapsedNS = nil },
		"negative timing": func(_ *[]SnapshotDensityBoundary, p *SnapshotDensityProof) {
			n := int64(-1)
			p.Children[0].ExecuteElapsedNS = &n
		},
		"headline":        func(_ *[]SnapshotDensityBoundary, p *SnapshotDensityProof) { p.LatencyEligible = true },
		"claimed product": func(_ *[]SnapshotDensityBoundary, p *SnapshotDensityProof) { p.RegisteredAdapter = true },
		"multithreaded":   func(_ *[]SnapshotDensityBoundary, p *SnapshotDensityProof) { p.PreForkThreads = 2 },
		"wrong fixture":   func(_ *[]SnapshotDensityBoundary, p *SnapshotDensityProof) { p.WasmSHA256 = "wrong" },
		"backend":         func(_ *[]SnapshotDensityBoundary, p *SnapshotDensityProof) { p.Backend = "winch" },
	} {
		t.Run(name, func(t *testing.T) {
			e, p := densityQualificationFixture()
			change(&e, &p)
			if VerifySnapshotDensityQualification("cranelift", 2, e, p) == nil {
				t.Fatal("forged simultaneous density admitted")
			}
		})
	}
}
