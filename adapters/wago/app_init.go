package main

import (
	"fmt"
	wago "github.com/wago-org/wago"
	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/protocol"
	"time"
)

func (a *adapter) runAppInit(r *protocol.RunRequest) ([]protocol.Sample, error) {
	if err := protocol.ValidateAppInit(a.prep.Workload); err != nil {
		return nil, err
	}
	if a.prep.Profile != "timing" && a.prep.Profile != "memory" {
		return nil, fmt.Errorf("unsupported app-init profile")
	}
	if r.PhaseBarriers && (a.prep.Profile != "memory" || a.barrier == nil) {
		return nil, fmt.Errorf("unsupported app-init barriers")
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
			init, err := m.WasmFunc(a.prep.Workload.Initialize)
			if err != nil {
				return protocol.Sample{}, err
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
			v, err := init.Invoke()
			elapsed := time.Since(start).Nanoseconds()
			var obs []protocol.Observation
			if finish != nil {
				obs = finish()
			}
			if err != nil {
				return protocol.Sample{}, err
			}
			if len(v) != 0 {
				return protocol.Sample{}, fmt.Errorf("initializer must be () -> ()")
			}
			if r.PhaseBarriers {
				if err := a.barrier(protocol.PhaseEvent{SampleIndex: i, Stage: "app_initialized"}); err != nil {
					return protocol.Sample{}, err
				}
			}
			if err = a.applyInput(m); err != nil {
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
			if err = a.verify(m, result); err != nil {
				return protocol.Sample{}, err
			}
			m.Close()
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
