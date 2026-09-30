package main

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/protocol"
)

// compilePhases holds the newly compiled module at the compiled barrier and
// releases it after measured-result verification. Diagnostic samples deliberately
// use one operation, irrespective of the requested timing batch size.
func (a *adapter) compilePhases(r *protocol.RunRequest) ([]protocol.Sample, error) {
	if (a.prep.Profile != "memory" && a.prep.Profile != "counters") || r.Scenario != "compile" || a.barrier == nil {
		return nil, fmt.Errorf("unsupported phase barrier request")
	}
	a.close()
	a.engine = a.newEngine()
	if err := a.registerHostImports(); err != nil {
		return nil, err
	}
	out := make([]protocol.Sample, 0, r.Samples)
	for i := 0; i < r.Samples; i++ {
		if err := a.barrier(protocol.PhaseEvent{SampleIndex: i, Stage: "before_compile"}); err != nil {
			return nil, err
		}
		var before, after runtime.MemStats
		if a.prep.Profile == "memory" {
			runtime.ReadMemStats(&before)
		}
		start := time.Now()
		m, err := a.engine.CompileModule(ctx, a.wasm)
		elapsed := time.Since(start).Nanoseconds()
		if a.prep.Profile == "memory" {
			runtime.ReadMemStats(&after)
		}
		if err != nil {
			return nil, err
		}
		if err = a.barrier(protocol.PhaseEvent{SampleIndex: i, Stage: "compiled"}); err != nil {
			m.Close(ctx)
			return nil, err
		}
		// Verification happens after retained-state collection, outside the timer.
		instance, err := a.engine.InstantiateModule(ctx, m, wazero.NewModuleConfig().WithName("").WithStartFunctions().WithStdout(os.Stderr).WithStderr(os.Stderr))
		if err != nil {
			m.Close(ctx)
			return nil, err
		}
		a.instance = instance
		if err = a.initialize(instance); err != nil {
			instance.Close(ctx)
			m.Close(ctx)
			return nil, err
		}
		fn := instance.ExportedFunction(a.prep.Workload.Export)
		if fn == nil {
			instance.Close(ctx)
			m.Close(ctx)
			return nil, fmt.Errorf("missing export")
		}
		var v []uint64
		verified := false
		if a.prep.Workload.Vectors != nil {
			err = a.checkVectors(instance)
			verified = err == nil
		} else {
			v, err = fn.Call(ctx, a.prep.Workload.Args...)
			verified = err == nil && a.verify(v)
		}
		instance.Close(ctx)
		a.instance = nil
		m.Close(ctx)
		if err != nil {
			return nil, err
		}
		if !verified {
			return nil, fmt.Errorf("incorrect result")
		}
		if err = a.barrier(protocol.PhaseEvent{SampleIndex: i, Stage: "released"}); err != nil {
			return nil, err
		}
		s := protocol.Sample{Index: i, ElapsedNS: elapsed, Operations: 1, SampleType: "individual_operation", Verified: true, Result: v}
		if a.prep.Profile == "memory" {
			s.Observations = collectors.GoMemoryObservations(before, after, "compile/api_window", "operation_excluding_verification_release_and_barriers")
		}
		out = append(out, s)
	}
	return out, nil
}
