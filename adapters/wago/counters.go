package main

import (
	"fmt"
	"time"

	wago "github.com/wago-org/wago"
	"github.com/wasmbench/wasmbench/protocol"
)

func (a *adapter) steadyCounters(r *protocol.RunRequest) ([]protocol.Sample, error) {
	if err := protocol.ValidateCounterRun(a.prep, r); err != nil {
		return nil, err
	}
	if r.Scenario != "steady" || a.barrier == nil {
		return nil, fmt.Errorf("unsupported steady counter request")
	}
	a.close()
	if err := a.setup(); err != nil {
		return nil, err
	}
	defer a.close()
	out := make([]protocol.Sample, 0, r.Samples+r.Warmup)
	for i := 0; i < r.Samples+r.Warmup; i++ {
		results := make([][]uint64, r.Operations)
		for j := range results {
			results[j] = make([]uint64, 0, len(a.prep.Workload.Oracle.Expected))
		}
		if err := a.barrier(protocol.PhaseEvent{SampleIndex: i, Stage: "before_steady_batch"}); err != nil {
			return out, err
		}
		var callErr error
		start := time.Now()
		for j := range results {
			v, err := a.fn.Invoke(a.prep.Workload.Args...)
			if err != nil {
				callErr = err
				break
			}
			// Invoke returns instance-owned storage, invalidated by the next call.
			results[j] = append(results[j], v...)
		}
		elapsed := time.Since(start).Nanoseconds()
		if err := a.barrier(protocol.PhaseEvent{SampleIndex: i, Stage: "steady_batch_returned"}); err != nil {
			return out, err
		}
		if callErr != nil {
			return out, callErr
		}
		for _, result := range results {
			if err := a.verify(a.instance, result); err != nil {
				return out, err
			}
		}
		kind := "batch_average"
		if r.Operations == 1 {
			kind = "individual_operation"
		}
		s := protocol.Sample{Index: i, Warmup: i < r.Warmup, Operations: r.Operations, SampleType: kind, ElapsedNS: elapsed, Result: results[len(results)-1], Verified: true}
		if err := a.barrier(protocol.PhaseEvent{SampleIndex: i, Stage: "steady_batch_verified"}); err != nil {
			return out, err
		}
		out = append(out, s)
	}
	return out, nil
}

func (a *adapter) firstCallCounters(r *protocol.RunRequest) ([]protocol.Sample, error) {
	if err := protocol.ValidateCounterRun(a.prep, r); err != nil {
		return nil, err
	}
	if r.Scenario != "first-call" || a.barrier == nil {
		return nil, fmt.Errorf("unsupported first-call counter request")
	}
	if a.compiled == nil {
		var err error
		a.compiled, err = wago.Compile(a.compileConfig, a.wasm)
		if err != nil {
			return nil, err
		}
	}
	out := make([]protocol.Sample, 0, r.Samples)
	for i := 0; i < r.Samples; i++ {
		s, err := func() (protocol.Sample, error) {
			m, err := a.instantiate(a.compiled)
			if err != nil {
				return protocol.Sample{}, err
			}
			defer m.Close()
			if err = a.initialize(m); err != nil {
				return protocol.Sample{}, err
			}
			fn, err := m.WasmFunc(a.prep.Workload.Export)
			if err != nil {
				return protocol.Sample{}, err
			}
			if err = a.barrier(protocol.PhaseEvent{SampleIndex: i, Stage: "before_first_call"}); err != nil {
				return protocol.Sample{}, err
			}
			start := time.Now()
			result, callErr := fn.Invoke(a.prep.Workload.Args...)
			elapsed := time.Since(start).Nanoseconds()
			if err = a.barrier(protocol.PhaseEvent{SampleIndex: i, Stage: "first_call_returned"}); err != nil {
				return protocol.Sample{}, err
			}
			if callErr != nil {
				return protocol.Sample{}, callErr
			}
			result = append([]uint64(nil), result...)
			if err = a.verify(m, result); err != nil {
				return protocol.Sample{}, err
			}
			m.Close()
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
