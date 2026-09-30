package main

import (
	"errors"
	"fmt"
	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/protocol"
	"reflect"
)

func classifyTrap(err error) *protocol.TrapResult {
	if err == nil {
		return nil
	}
	message := err.Error()
	for depth := 0; err != nil && depth < 64; depth++ {
		t := reflect.TypeOf(err)
		// wazero has no public trap enum. Require its pinned internal error
		// type AND an exact diagnostic, never a substring of arbitrary errors.
		if t.Kind() == reflect.Pointer && t.Elem().PkgPath() == "github.com/tetratelabs/wazero/internal/wasmruntime" && t.Elem().Name() == "Error" {
			code := map[string]string{"unreachable": "unreachable", "out of bounds memory access": "memory_out_of_bounds", "integer divide by zero": "integer_divide_by_zero", "integer overflow": "integer_overflow"}[err.Error()]
			if code != "" {
				return &protocol.TrapResult{Code: code, Source: "wazero/1.12.0/internal-error-type-and-exact-message-v1", Message: message}
			}
		}
		err = errors.Unwrap(err)
	}
	return nil
}

func (a *adapter) runTraps(r *protocol.RunRequest) ([]protocol.Sample, error) {
	if err := protocol.ValidateTrap(a.prep.Workload); err != nil {
		return nil, err
	}
	if err := a.setupCompiled(); err != nil {
		return nil, err
	}
	return protocol.RunTrapSamples(a.prep, r, func() (protocol.TrapInstance, error) {
		m, err := a.instantiate()
		if err != nil {
			return protocol.TrapInstance{}, err
		}
		f := m.ExportedFunction(a.prep.Workload.Export)
		if f == nil {
			m.Close(ctx)
			return protocol.TrapInstance{}, fmt.Errorf("missing trap export")
		}
		if len(f.Definition().ParamTypes()) != 0 {
			m.Close(ctx)
			return protocol.TrapInstance{}, fmt.Errorf("trap export must have no parameters")
		}
		return protocol.TrapInstance{
			Invoke: func() error { _, err := f.Call(ctx); return err },
			Close:  func() { m.Close(ctx) },
			BeginMemory: func() func() []protocol.Observation {
				finish := collectors.GoMemoryWindow(r.Scenario+"/trap_api_window", "trapping_invocation_excluding_classification_release")
				return func() []protocol.Observation {
					observations := finish()
					if hasMemory(m) {
						observations = append(observations, protocol.Observation{Metric: "guest.memory.logical", DefinitionVersion: 1, Value: protocol.Value(float64(m.Memory().Size())), Unit: "bytes", Scope: "guest_linear_memory", Phase: r.Scenario + "/after_trap", Collector: "wazero.Memory", CollectorVersion: "1.12.0", Quality: "engine_reported", Profile: "memory", Status: "available", Denominator: "instance"})
					}
					return observations
				}
			},
		}, nil
	}, classifyTrap)
}
