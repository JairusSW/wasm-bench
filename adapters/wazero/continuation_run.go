package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
)

func (a *adapter) runContinuation(r *protocol.RunRequest) ([]protocol.Sample, error) {
	if err := protocol.ValidateContinuation(a.prep, r); err != nil {
		return nil, err
	}
	if a.interpreter {
		return nil, fmt.Errorf("native continuation unsupported: pinned interpreter fails guest-resumption qualification")
	}
	canonical, err := corpus.ContinuationModule(a.prep.Workload.Continuation.Depth)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(canonical, a.wasm) {
		return nil, fmt.Errorf("native continuation requires exact canonical artifact bytes")
	}
	if r.PhaseBarriers && a.barrier == nil {
		return nil, fmt.Errorf("native continuation barrier handler unavailable")
	}
	// Dedicated runtime/module setup is outside every selected stage. It is
	// reused within this batch; fresh guest instances are created per sample.
	engine := a.newEngine()
	defer engine.Close(ctx)
	e, err := newContinuationExperiment(ctx, engine, a.prep.Workload.Continuation.Depth)
	if err != nil {
		return nil, err
	}
	defer e.close(ctx)
	out := make([]protocol.Sample, 0, r.Samples)
	stages := protocol.PhaseStages(r.Scenario)
	for i := 0; i < r.Samples; i++ {
		var obs []protocol.Observation
		var finish func() []protocol.Observation
		var started, completed bool
		barrier := func(stage string) error {
			if r.PhaseBarriers {
				return a.barrier(protocol.PhaseEvent{SampleIndex: i, Stage: stage})
			}
			return nil
		}
		window := func(_ string, start bool) error {
			if start {
				if started {
					return fmt.Errorf("duplicate native continuation stage start")
				}
				if err := barrier(stages[0]); err != nil {
					return err
				}
				if a.prep.Profile == "memory" {
					finish = collectors.GoMemoryWindow(r.Scenario+"/operation_window", protocol.ContinuationDenominator)
				}
				started = true
			} else {
				if !started || completed {
					return fmt.Errorf("invalid native continuation stage end")
				}
				if finish != nil {
					obs = finish()
				}
				completed = true
				if err := barrier(stages[1]); err != nil {
					return err
				}
			}
			return nil
		}
		m, err := e.measureStage(ctx, r.Scenario, window)
		if err != nil {
			return out, err
		}
		if !started || !completed {
			return out, fmt.Errorf("native continuation stage not observed")
		}
		elapsed := m.CreateNS
		switch r.Scenario {
		case "continuation-resume":
			elapsed = m.RestoreResumptionNS
		case "continuation-first-write":
			elapsed = m.FirstWriteNS
		case "continuation-execute":
			elapsed = m.PostRestoreExecutionNS
		}
		proof := &protocol.ContinuationResult{Mode: protocol.NativeContinuationMode, Depth: m.Depth, StackResult: uint32(m.StackResult), GuestResumed: m.Verified, MemoryAfterRestoreSHA256: hex.EncodeToString(m.MemoryAfterRestore[:]), MemoryAfterWriteSHA256: hex.EncodeToString(m.MemoryAfterWrite[:]), GlobalAfterRestore: uint32(m.GlobalAfterRestore), GlobalAfterWrite: uint32(m.GlobalAfterWrite)}
		s := protocol.Sample{Index: i, ElapsedNS: elapsed, Operations: 1, SampleType: "individual_operation", Verified: m.Verified, Result: protocol.Values{m.PostRestoreResult}, ContinuationResult: proof, Observations: obs}
		if err := protocol.VerifyContinuationSample(a.prep.Workload, r.Scenario, s); err != nil {
			return out, err
		}
		// measureStage has verified and closed the instance, and discarded its
		// active stack capture. Engine/module stay retained; no GC is forced.
		if err := barrier(stages[2]); err != nil {
			return out, err
		}
		out = append(out, s)
	}
	return out, nil
}
