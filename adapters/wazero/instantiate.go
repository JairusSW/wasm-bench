package main

import (
	"fmt"
	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/protocol"
	"time"
)

func (a *adapter) instantiatePhases(r *protocol.RunRequest) ([]protocol.Sample, error) {
	if err := protocol.ValidateInstantiatePhases(a.prep.Workload); err != nil {
		return nil, err
	}
	if (a.prep.Profile != "memory" && a.prep.Profile != "counters") || a.barrier == nil || r.Samples > 100000 {
		return nil, fmt.Errorf("unsupported instantiation barriers")
	}
	if err := a.setupCompiled(); err != nil {
		return nil, err
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
			m, err := a.instantiate()
			elapsed := time.Since(start).Nanoseconds()
			obs := finish()
			if err != nil {
				return protocol.Sample{}, err
			}
			defer m.Close(ctx)
			if err := a.barrier(protocol.PhaseEvent{SampleIndex: i, Stage: "instantiated"}); err != nil {
				return protocol.Sample{}, err
			}
			if err := a.initialize(m); err != nil {
				return protocol.Sample{}, err
			}
			fn := m.ExportedFunction(a.prep.Workload.Export)
			if fn == nil {
				return protocol.Sample{}, fmt.Errorf("missing workload export")
			}
			result, err := fn.Call(ctx, a.prep.Workload.Args...)
			if err != nil {
				return protocol.Sample{}, err
			}
			result = append([]uint64(nil), result...)
			if !a.verifyInstance(m, result) {
				return protocol.Sample{}, fmt.Errorf("incorrect result")
			}
			if err := m.Close(ctx); err != nil {
				return protocol.Sample{}, err
			}
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
