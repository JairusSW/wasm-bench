package main

import (
	"errors"
	wago "github.com/wago-org/wago"
	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/protocol"
)

func classifyTrap(err error) *protocol.TrapResult {
	var trap *wago.TrapError
	if !errors.As(err, &trap) {
		return nil
	}
	code := map[wago.TrapCode]string{wago.TrapUnreachable: "unreachable", wago.TrapLinMemOutOfBounds: "memory_out_of_bounds", wago.TrapDivZero: "integer_divide_by_zero", wago.TrapDivOverflow: "integer_overflow"}[trap.Code]
	if code == "" {
		return nil
	}
	return &protocol.TrapResult{Code: code, Source: "wago.TrapError.Code/v1", Message: err.Error()}
}

func (a *adapter) runTraps(r *protocol.RunRequest) ([]protocol.Sample, error) {
	if err := protocol.ValidateTrap(a.prep.Workload); err != nil {
		return nil, err
	}
	if a.compiled == nil {
		var err error
		a.compiled, err = wago.Compile(a.compileConfig, a.wasm)
		if err != nil {
			return nil, err
		}
	}
	return protocol.RunTrapSamples(a.prep, r, func() (protocol.TrapInstance, error) {
		m, err := a.instantiate(a.compiled)
		if err != nil {
			return protocol.TrapInstance{}, err
		}
		f, err := m.WasmFunc(a.prep.Workload.Export)
		if err != nil {
			m.Close()
			return protocol.TrapInstance{}, err
		}
		return protocol.TrapInstance{
			Invoke: func() error { _, err := f.Invoke(); return err },
			Close:  func() { m.Close() },
			BeginMemory: func() func() []protocol.Observation {
				finish := collectors.GoMemoryWindow(r.Scenario+"/trap_api_window", "trapping_invocation_excluding_classification_release")
				return func() []protocol.Observation {
					observations := finish()
					if m.Memory() != nil {
						observations = append(observations, protocol.Observation{Metric: "guest.memory.logical", DefinitionVersion: 1, Value: protocol.Value(float64(len(m.Memory().UnsafeBytes()))), Unit: "bytes", Scope: "guest_linear_memory", Phase: r.Scenario + "/after_trap", Collector: "wago.Memory", CollectorVersion: sourceRevision, Quality: "engine_reported", Profile: "memory", Status: "available", Denominator: "instance"})
					}
					return observations
				}
			},
		}, nil
	}, classifyTrap)
}
