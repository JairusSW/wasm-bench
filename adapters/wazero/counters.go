package main

import (
	"fmt"
	"time"

	"github.com/wasmbench/wasmbench/protocol"
)

func (a *adapter) steadyCounters(r *protocol.RunRequest) ([]protocol.Sample, error) {
	if err := protocol.ValidateCounterRun(a.prep, r); err != nil {
		return nil, err
	}
	if r.Scenario != "steady" || a.barrier == nil {
		return nil, fmt.Errorf("unsupported steady counter request")
	}
	// Each request starts fresh; only the explicitly requested warmup batches
	// execute before measured batches. The instance remains alive across both.
	a.close()
	if err := a.setup(); err != nil {
		return nil, err
	}
	defer a.close()
	out := make([]protocol.Sample, 0, r.Samples+r.Warmup)
	for i := 0; i < r.Samples+r.Warmup; i++ {
		results := make([][]uint64, r.Operations)
		if err := a.barrier(protocol.PhaseEvent{SampleIndex: i, Stage: "before_steady_batch"}); err != nil {
			return out, err
		}
		var callErr error
		start := time.Now()
		for j := range results {
			results[j], callErr = a.fn.Call(ctx, a.prep.Workload.Args...)
			if callErr != nil {
				break
			}
		}
		elapsed := time.Since(start).Nanoseconds()
		if err := a.barrier(protocol.PhaseEvent{SampleIndex: i, Stage: "steady_batch_returned"}); err != nil {
			return out, err
		}
		if callErr != nil {
			return out, callErr
		}
		for _, result := range results {
			if !a.verify(result) {
				return out, fmt.Errorf("incorrect result")
			}
		}
		kind := "batch_average"
		if r.Operations == 1 {
			kind = "individual_operation"
		}
		s := protocol.Sample{Index: i, Warmup: i < r.Warmup, Operations: r.Operations, SampleType: kind, ElapsedNS: elapsed, Result: append([]uint64(nil), results[len(results)-1]...), Verified: true}
		if err := a.barrier(protocol.PhaseEvent{SampleIndex: i, Stage: "steady_batch_verified"}); err != nil {
			return out, err
		}
		out = append(out, s)
	}
	return out, nil
}

// Each sample invokes a fresh initialized instance exactly once. The engine and
// compiled module are retained; setup, verification and release are not counted.
func (a *adapter) firstCallCounters(r *protocol.RunRequest) ([]protocol.Sample, error) {
	if err := protocol.ValidateCounterRun(a.prep, r); err != nil {
		return nil, err
	}
	if r.Scenario != "first-call" || a.barrier == nil {
		return nil, fmt.Errorf("unsupported first-call counter request")
	}
	if err := a.setupCompiled(); err != nil {
		return nil, err
	}
	out := make([]protocol.Sample, 0, r.Samples)
	for i := 0; i < r.Samples; i++ {
		s, err := func() (protocol.Sample, error) {
			m, err := a.instantiate()
			if err != nil {
				return protocol.Sample{}, err
			}
			defer m.Close(ctx)
			if err = a.initialize(m); err != nil {
				return protocol.Sample{}, err
			}
			fn := m.ExportedFunction(a.prep.Workload.Export)
			if fn == nil {
				return protocol.Sample{}, fmt.Errorf("missing workload export")
			}
			if err = a.barrier(protocol.PhaseEvent{SampleIndex: i, Stage: "before_first_call"}); err != nil {
				return protocol.Sample{}, err
			}
			start := time.Now()
			result, callErr := fn.Call(ctx, a.prep.Workload.Args...)
			elapsed := time.Since(start).Nanoseconds()
			if err = a.barrier(protocol.PhaseEvent{SampleIndex: i, Stage: "first_call_returned"}); err != nil {
				return protocol.Sample{}, err
			}
			if callErr != nil {
				return protocol.Sample{}, callErr
			}
			result = append([]uint64(nil), result...)
			if !a.verifyInstance(m, result) {
				return protocol.Sample{}, fmt.Errorf("incorrect result")
			}
			if err = m.Close(ctx); err != nil {
				return protocol.Sample{}, err
			}
			if err = a.barrier(protocol.PhaseEvent{SampleIndex: i, Stage: "first_call_released"}); err != nil {
				return protocol.Sample{}, err
			}
			return protocol.Sample{Index: i, Operations: 1, SampleType: "individual_operation", ElapsedNS: elapsed, Result: result, Verified: true}, nil
		}()
		if err != nil {
			return out, err
		}
		out = append(out, s)
	}
	return out, nil
}
