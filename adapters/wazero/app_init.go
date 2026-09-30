package main

import (
	"fmt"
	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/protocol"
	"time"
)

func (a *adapter) runAppInit(r *protocol.RunRequest) ([]protocol.Sample, error) {
	if err := protocol.ValidateAppInit(a.prep.Workload); err != nil {
		return nil, err
	}
	if (r.PhaseBarriers && (a.prep.Profile != "memory" || a.barrier == nil)) || r.Samples > 100000 || (a.prep.Profile != "timing" && a.prep.Profile != "memory") {
		return nil, fmt.Errorf("unsupported app-init profile/batch")
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
			init := m.ExportedFunction(a.prep.Workload.Initialize)
			if init == nil || len(init.Definition().ParamTypes()) != 0 || len(init.Definition().ResultTypes()) != 0 {
				return protocol.Sample{}, fmt.Errorf("initializer must be () -> ()")
			}
			var finish func() []protocol.Observation
			if r.PhaseBarriers {
				if err := a.barrier(protocol.PhaseEvent{SampleIndex: i, Stage: "before_app_init"}); err != nil {
					return protocol.Sample{}, err
				}
			}
			if a.prep.Profile == "memory" {
				finish = collectors.GoMemoryWindow("app-init/api_window", "initialization_call_excluding_input_verification_release")
			}
			start := time.Now()
			_, err = init.Call(ctx)
			elapsed := time.Since(start).Nanoseconds()
			var obs []protocol.Observation
			if finish != nil {
				obs = finish()
			}
			if err != nil {
				return protocol.Sample{}, err
			}
			if r.PhaseBarriers {
				if err := a.barrier(protocol.PhaseEvent{SampleIndex: i, Stage: "app_initialized"}); err != nil {
					return protocol.Sample{}, err
				}
			}
			if err = a.applyInput(m); err != nil {
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
			if !a.verifyInstance(m, result) {
				return protocol.Sample{}, fmt.Errorf("incorrect result")
			}
			if err := m.Close(ctx); err != nil {
				return protocol.Sample{}, err
			}
			if r.PhaseBarriers {
				if err := a.barrier(protocol.PhaseEvent{SampleIndex: i, Stage: "app_released"}); err != nil {
					return protocol.Sample{}, err
				}
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
