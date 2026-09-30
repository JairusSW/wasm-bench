package protocol_test

import (
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
	"testing"
)

func TestContinuationContracts(t *testing.T) {
	ws, err := corpus.Generate(t.TempDir(), "continuations")
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range ws {
		for _, change := range []func(*protocol.Workload){func(w *protocol.Workload) { w.Size++ }, func(w *protocol.Workload) { w.Dimension = "native_stack_bytes" }} {
			bad := w
			change(&bad)
			if protocol.ValidateContinuationWorkload(bad) == nil {
				t.Fatal("forged native continuation scaling dimension admitted")
			}
		}
		for _, stage := range protocol.ContinuationScenarios() {
			p := &protocol.Preparation{Workload: w, Profile: "timing"}
			r := protocol.RunRequest{Scenario: stage, Samples: 1, Operations: 1}
			if err := protocol.ValidateContinuation(p, &r); err != nil {
				t.Fatal(err)
			}
			for _, change := range []func(*protocol.RunRequest){func(r *protocol.RunRequest) { r.Operations = 2 }, func(r *protocol.RunRequest) { r.Warmup = 1 }, func(r *protocol.RunRequest) { r.Samples = 0 }, func(r *protocol.RunRequest) { r.Samples = 10001 }, func(r *protocol.RunRequest) { r.PhaseBarriers = true }, func(r *protocol.RunRequest) { r.Scenario = "checkpoint-restore" }, func(r *protocol.RunRequest) { r.SustainedDurationNS = 1 }} {
				bad := r
				change(&bad)
				if protocol.ValidateContinuation(p, &bad) == nil {
					t.Fatal("invalid continuation request")
				}
			}
			s := protocol.Sample{Verified: true, Operations: 1, SampleType: "individual_operation", Result: protocol.Values{133}, ContinuationResult: &protocol.ContinuationResult{Mode: protocol.NativeContinuationMode, Depth: w.Continuation.Depth, StackResult: uint32(7 + w.Continuation.Depth*(w.Continuation.Depth+1)/2), GuestResumed: true, MemoryAfterRestoreSHA256: protocol.ContinuationMemorySHA256(false), MemoryAfterWriteSHA256: protocol.ContinuationMemorySHA256(true), GlobalAfterRestore: 99, GlobalAfterWrite: 100}}
			if err := protocol.VerifyContinuationSample(w, stage, s); err != nil {
				t.Fatal(err)
			}
			for _, change := range []func(*protocol.ContinuationResult){func(e *protocol.ContinuationResult) { e.Mode = "whole-instance" }, func(e *protocol.ContinuationResult) { e.Depth++ }, func(e *protocol.ContinuationResult) { e.StackResult++ }, func(e *protocol.ContinuationResult) { e.GuestResumed = false }, func(e *protocol.ContinuationResult) { e.MemoryAfterRestoreSHA256 = "fake" }, func(e *protocol.ContinuationResult) { e.MemoryAfterWriteSHA256 = "fake" }, func(e *protocol.ContinuationResult) { e.GlobalAfterRestore = 0 }, func(e *protocol.ContinuationResult) { e.GlobalAfterWrite = 0 }} {
				proof := *s.ContinuationResult
				change(&proof)
				bad := s
				bad.ContinuationResult = &proof
				if protocol.VerifyContinuationSample(w, stage, bad) == nil {
					t.Fatal("forged stack/non-stack proof accepted")
				}
			}
		}
	}
}
