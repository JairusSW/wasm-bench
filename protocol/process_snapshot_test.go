package protocol_test

import (
	"encoding/json"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
	"reflect"
	"testing"
)

func snapshotWorkload(t *testing.T) protocol.Workload {
	t.Helper()
	ws, err := corpus.Generate(t.TempDir(), "process-snapshots")
	if err != nil {
		t.Fatal(err)
	}
	return ws[0]
}
func snapshotSample(stage string) protocol.Sample {
	e := &protocol.ProcessSnapshotResult{Mode: protocol.ProcessSnapshotMode, Boundary: protocol.ProcessSnapshotBoundary(stage), Source: protocol.SnapshotProcess{PID: 101, StartTimeTicks: "100"}, Template: protocol.SnapshotProcess{PID: 102, StartTimeTicks: "101"}, Restored: protocol.SnapshotProcess{PID: 103, StartTimeTicks: "102"}, AlternateRestored: protocol.SnapshotProcess{PID: 104, StartTimeTicks: "103"}, SourceReleased: true, ChildrenReaped: true, SourceAfterMutation: 125, RestoredBeforeWrite: 64, RestoredAfterWrite: 64, PassiveSegmentProbe: 127, MemoryPages: 3, TableElements: 3, IndependentRestorations: 2, MemoryAtRestoreSHA256: protocol.ProcessSnapshotMemorySHA256(false), MemoryAfterFirstWriteSHA256: protocol.ProcessSnapshotMemorySHA256(true)}
	e.PreForkThreads = 1
	e.ClockProcess = e.Source
	if stage == "process-snapshot-first-write" || stage == "process-snapshot-execute" {
		e.ClockProcess = e.Restored
	}
	return protocol.Sample{Verified: true, Operations: 1, SampleType: "individual_operation", Result: protocol.Values{64}, ProcessSnapshotResult: e}
}
func TestProcessSnapshotContract(t *testing.T) {
	w := snapshotWorkload(t)
	for _, stage := range protocol.ProcessSnapshotScenarios() {
		p := protocol.Preparation{Workload: w, Profile: "timing"}
		r := protocol.RunRequest{Scenario: stage, Samples: 1, Operations: 1}
		if err := protocol.ValidateProcessSnapshot(&p, &r); err != nil {
			t.Fatal(err)
		}
		if err := protocol.VerifyProcessSnapshotSample(w, stage, snapshotSample(stage)); err != nil {
			t.Fatal(err)
		}
		for name, change := range map[string]func(*protocol.RunRequest){"batch": func(r *protocol.RunRequest) { r.Operations = 2 }, "warmup": func(r *protocol.RunRequest) { r.Warmup = 1 }, "empty": func(r *protocol.RunRequest) { r.Samples = 0 }, "oversize": func(r *protocol.RunRequest) { r.Samples = 1001 }, "barriers": func(r *protocol.RunRequest) { r.PhaseBarriers = true }, "foreign_stage": func(r *protocol.RunRequest) { r.Scenario = "checkpoint-restore" }, "sustained": func(r *protocol.RunRequest) { r.SustainedDurationNS = 1 }, "purge": func(r *protocol.RunRequest) { r.SustainedPostCollection = true }} {
			t.Run(stage+"/"+name, func(t *testing.T) {
				bad := r
				change(&bad)
				if protocol.ValidateProcessSnapshot(&p, &bad) == nil {
					t.Fatal("invalid request admitted")
				}
			})
		}
		for _, profile := range []string{"memory", "counters", "code", "profiling"} {
			p.Profile = profile
			if protocol.ValidateProcessSnapshot(&p, &r) == nil {
				t.Fatal("parent-only or diagnostic profile admitted")
			}
		}
	}
	if protocol.ValidateProcessSnapshot(nil, nil) == nil {
		t.Fatal("nil request admitted")
	}
	for name, change := range map[string]func(*protocol.Workload){"digest": func(w *protocol.Workload) { w.SHA256 = "other" }, "mode": func(w *protocol.Workload) {
		w.ProcessSnapshot = &protocol.ProcessSnapshotContract{Mode: "engine_serializer"}
	}, "oracle": func(w *protocol.Workload) { w.Oracle.Expected = protocol.Values{125} }, "abi": func(w *protocol.Workload) { w.ABI = "wasi-command" }, "reset": func(w *protocol.Workload) { w.Reset = "stateless" }, "checkpoint": func(w *protocol.Workload) { w.Checkpoint = &protocol.CheckpointContract{} }, "continuation": func(w *protocol.Workload) { w.Continuation = &protocol.ContinuationContract{} }, "scale": func(w *protocol.Workload) { w.Size = 3 }, "initialization": func(w *protocol.Workload) { w.Initialize = "" }} {
		t.Run(name, func(t *testing.T) {
			bad := w
			change(&bad)
			if protocol.ValidateProcessSnapshotWorkload(bad) == nil {
				t.Fatal("invalid workload admitted")
			}
		})
	}
}
func TestProcessSnapshotProof(t *testing.T) {
	w := snapshotWorkload(t)
	stage := "process-snapshot-restore"
	for name, change := range map[string]func(*protocol.Sample){
		"missing": func(s *protocol.Sample) { s.ProcessSnapshotResult = nil }, "unverified": func(s *protocol.Sample) { s.Verified = false }, "negative_clock": func(s *protocol.Sample) { s.ElapsedNS = -1 }, "batch_average": func(s *protocol.Sample) { s.SampleType = "batch_average" }, "warmup": func(s *protocol.Sample) { s.Warmup = true }, "negative_index": func(s *protocol.Sample) { s.Index = -1 }, "wrong_result": func(s *protocol.Sample) { s.Result = protocol.Values{125} }, "foreign_proof": func(s *protocol.Sample) { s.ContinuationResult = &protocol.ContinuationResult{} }, "memory_in_timing": func(s *protocol.Sample) { s.Observations = []protocol.Observation{{Metric: "process.rss"}} },
		"source_alive": func(s *protocol.Sample) { s.ProcessSnapshotResult.SourceReleased = false }, "unreaped": func(s *protocol.Sample) { s.ProcessSnapshotResult.ChildrenReaped = false }, "memory_copy": func(s *protocol.Sample) { s.ProcessSnapshotResult.Mode = "eager-memory-scalar-v1" }, "api_return": func(s *protocol.Sample) { s.ProcessSnapshotResult.Boundary = "fork_api_return" }, "cross_process_clock": func(s *protocol.Sample) { s.ProcessSnapshotResult.ClockProcess = s.ProcessSnapshotResult.Restored }, "incomplete_restore": func(s *protocol.Sample) { s.ProcessSnapshotResult.IndependentRestorations = 1 }, "wrong_source": func(s *protocol.Sample) { s.ProcessSnapshotResult.SourceAfterMutation = 64 }, "wrong_restore": func(s *protocol.Sample) { s.ProcessSnapshotResult.RestoredBeforeWrite = 125 }, "wrong_write": func(s *protocol.Sample) { s.ProcessSnapshotResult.RestoredAfterWrite = 125 }, "missing_passive": func(s *protocol.Sample) { s.ProcessSnapshotResult.PassiveSegmentProbe = 0 }, "growth": func(s *protocol.Sample) { s.ProcessSnapshotResult.MemoryPages = 2 }, "table": func(s *protocol.Sample) { s.ProcessSnapshotResult.TableElements = 2 }, "partial_memory": func(s *protocol.Sample) { s.ProcessSnapshotResult.MemoryAtRestoreSHA256 = "partial" }, "unchanged_write": func(s *protocol.Sample) {
			s.ProcessSnapshotResult.MemoryAfterFirstWriteSHA256 = s.ProcessSnapshotResult.MemoryAtRestoreSHA256
		}, "duplicate_process": func(s *protocol.Sample) { s.ProcessSnapshotResult.AlternateRestored = s.ProcessSnapshotResult.Restored }, "zero_pid": func(s *protocol.Sample) { s.ProcessSnapshotResult.Template.PID = 0 }, "wide_pid": func(s *protocol.Sample) { s.ProcessSnapshotResult.Template.PID = 2147483648 }, "missing_birth": func(s *protocol.Sample) { s.ProcessSnapshotResult.Template.StartTimeTicks = "" }, "negative_birth": func(s *protocol.Sample) { s.ProcessSnapshotResult.Template.StartTimeTicks = "-1" }, "leading_zero": func(s *protocol.Sample) { s.ProcessSnapshotResult.Template.StartTimeTicks = "0101" }, "overflow_birth": func(s *protocol.Sample) { s.ProcessSnapshotResult.Template.StartTimeTicks = "18446744073709551616" }, "ancestry": func(s *protocol.Sample) { s.ProcessSnapshotResult.Restored.StartTimeTicks = "99" }, "parent_pid_reuse": func(s *protocol.Sample) { s.ProcessSnapshotResult.Restored.PID = s.ProcessSnapshotResult.Source.PID },
	} {
		t.Run(name, func(t *testing.T) {
			s := snapshotSample(stage)
			change(&s)
			if protocol.VerifyProcessSnapshotSample(w, stage, s) == nil {
				t.Fatal("forged proof admitted")
			}
		})
	}
	// Sequential restored children may reuse a PID, but not the same birth identity.
	s := snapshotSample(stage)
	s.ProcessSnapshotResult.AlternateRestored.PID = s.ProcessSnapshotResult.Restored.PID
	if err := protocol.VerifyProcessSnapshotSample(w, stage, s); err != nil {
		t.Fatal(err)
	}
	e := s.ProcessSnapshotResult
	e.Source.StartTimeTicks = "9007199254740993"
	e.Template.StartTimeTicks = "9007199254740994"
	e.Restored.StartTimeTicks = "9007199254740995"
	e.AlternateRestored.StartTimeTicks = "9007199254740996"
	e.ClockProcess = e.Source
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var decoded protocol.Sample
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s, decoded) {
		t.Fatal("wire round trip lost process birth precision")
	}
	if err := protocol.VerifyProcessSnapshotSample(w, stage, decoded); err != nil {
		t.Fatal(err)
	}
}

func TestProcessSnapshotFreshSequence(t *testing.T) {
	w := snapshotWorkload(t)
	stage := "process-snapshot-restore"
	r := protocol.RunRequest{Scenario: stage, Samples: 2, Operations: 1}
	first, second := snapshotSample(stage), snapshotSample(stage)
	second.Index = 1
	second.ProcessSnapshotResult.Template = protocol.SnapshotProcess{PID: 105, StartTimeTicks: "104"}
	second.ProcessSnapshotResult.Restored = protocol.SnapshotProcess{PID: 106, StartTimeTicks: "105"}
	second.ProcessSnapshotResult.AlternateRestored = protocol.SnapshotProcess{PID: 107, StartTimeTicks: "106"}
	if err := protocol.VerifyProcessSnapshotSequence(w, r, []protocol.Sample{first, second}); err != nil {
		t.Fatal(err)
	}
	if protocol.VerifyProcessSnapshotSequence(w, r, []protocol.Sample{first}) == nil {
		t.Fatal("partial batch admitted")
	}
	bad := snapshotSample(stage)
	bad.Index = 1
	if protocol.VerifyProcessSnapshotSequence(w, r, []protocol.Sample{first, bad}) == nil {
		t.Fatal("reused snapshot admitted")
	}
	second.Index = 0
	if protocol.VerifyProcessSnapshotSequence(w, r, []protocol.Sample{first, second}) == nil {
		t.Fatal("reordered batch admitted")
	}
	second.Index = 1
	second.ProcessSnapshotResult.Source.StartTimeTicks = "101"
	second.ProcessSnapshotResult.ClockProcess = second.ProcessSnapshotResult.Source
	if protocol.VerifyProcessSnapshotSequence(w, r, []protocol.Sample{first, second}) == nil {
		t.Fatal("changed root process admitted")
	}
}
