package main

import (
	"context"
	"fmt"
	wago "github.com/wago-org/wago"
	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/protocol"
	"time"
)

func (a *adapter) instantiatePhases(r *protocol.RunRequest) ([]protocol.Sample, error) {
	if err := protocol.ValidateInstantiatePhases(a.prep.Workload); err != nil {
		return nil, err
	}
	if (a.prep.Profile != "memory" && a.prep.Profile != "counters") || a.barrier == nil {
		return nil, fmt.Errorf("unsupported instantiation barriers")
	}
	if a.compiled == nil {
		var err error
		a.compiled, err = wago.Compile(a.compileConfig, a.wasm)
		if err != nil {
			return nil, err
		}
	}
	instantiate := func() (*wago.Instance, error) {
		return wago.Instantiate(a.compiled, wago.InstantiateOptions{Imports: a.imports})
	}
	if a.hostRuntime != nil {
		module, err := a.hostRuntime.Module(a.compiled)
		if err != nil {
			return nil, err
		}
		defer module.Close()
		instantiate = func() (*wago.Instance, error) {
			return a.hostRuntime.Instantiate(context.Background(), module, wago.WithImports(a.imports))
		}
	}
	out := make([]protocol.Sample, 0, r.Samples)
	for i := 0; i < r.Samples; i++ {
		s, err := func() (protocol.Sample, error) {
			if err := a.barrier(protocol.PhaseEvent{SampleIndex: i, Stage: "before_instantiate"}); err != nil {
				return protocol.Sample{}, err
			}
			finish := func() []protocol.Observation { return nil }
			if a.prep.Profile == "memory" {
				finish = collectors.GoMemoryWindow("instantiate/api_window", "instantiation_including_start_excluding_initialization_verification_release")
			}
			start := time.Now()
			m, err := instantiate()
			elapsed := time.Since(start).Nanoseconds()
			obs := finish()
			if err != nil {
				return protocol.Sample{}, err
			}
			defer m.Close()
			if err := a.barrier(protocol.PhaseEvent{SampleIndex: i, Stage: "instantiated"}); err != nil {
				return protocol.Sample{}, err
			}
			if err := a.initialize(m); err != nil {
				return protocol.Sample{}, err
			}
			fn, err := m.WasmFunc(a.prep.Workload.Export)
			if err != nil {
				return protocol.Sample{}, err
			}
			result, err := fn.Invoke(a.prep.Workload.Args...)
			if err != nil {
				return protocol.Sample{}, err
			}
			result = append([]uint64(nil), result...)
			if err := a.verify(m, result); err != nil {
				return protocol.Sample{}, err
			}
			m.Close()
			if err := a.barrier(protocol.PhaseEvent{SampleIndex: i, Stage: "instance_released"}); err != nil {
				return protocol.Sample{}, err
			}
			return protocol.Sample{Index: i, ElapsedNS: elapsed, Operations: 1, SampleType: "individual_operation", Verified: true, Result: result, Observations: obs}, nil
		}()
		if err != nil {
			return out, err
		}
		out = append(out, s)
	}
	return out, nil
}
