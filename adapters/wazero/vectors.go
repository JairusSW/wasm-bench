package main

import (
	"fmt"
	"os"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/protocol"
)

func (a *adapter) checkVectors(m api.Module) error {
	p, err := protocol.PrepareWorkloadVectors(a.prep.Workload)
	if err != nil {
		return err
	}
	f := m.ExportedFunction(a.prep.Workload.Export)
	if f == nil || !hasMemory(m) {
		return fmt.Errorf("missing vector export/memory")
	}
	return p.Run(protocol.VectorInstance{Pointer: func(name string) ([]uint64, error) {
		f := m.ExportedFunction(name)
		if f == nil {
			return nil, fmt.Errorf("missing vector pointer")
		}
		return f.Call(ctx)
	}, Write: m.Memory().Write, Read: m.Memory().Read, Invoke: func(in, n, out uint32) error { _, err := f.Call(ctx, uint64(in), uint64(n), uint64(out)); return err }})
}

func (a *adapter) runVectors(r *protocol.RunRequest) ([]protocol.Sample, error) {
	if r.PhaseBarriers && (a.prep.Profile != "memory" || a.barrier == nil) {
		return nil, fmt.Errorf("unsupported vector barriers")
	}
	if a.prep.Workload.HostProfile != "" {
		return nil, fmt.Errorf("unsupported vector host profile")
	}
	a.close()
	if r.Scenario != "teardown" {
		a.engine = a.newEngine()
	}
	defer a.close()
	if r.Scenario != "compile" && r.Scenario != "teardown" {
		var err error
		a.compiled, err = a.engine.CompileModule(ctx, a.wasm)
		if err != nil {
			return nil, err
		}
	}
	sampleIndex := 0
	return protocol.RunVectorSamples(a.prep, r, func(scenario string) (protocol.VectorInstance, int64, func(), error) {
		var elapsed int64
		compiled := a.compiled
		if scenario == "teardown" {
			a.engine = a.newEngine()
		}
		if scenario == "compile" || scenario == "teardown" {
			start := time.Now()
			var err error
			compiled, err = a.engine.CompileModule(ctx, a.wasm)
			elapsed = time.Since(start).Nanoseconds()
			if err != nil {
				return protocol.VectorInstance{}, 0, nil, err
			}
		}
		config := wazero.NewModuleConfig().WithName("").WithStartFunctions().WithStdout(os.Stderr).WithStderr(os.Stderr)
		var apiObservations []protocol.Observation
		finish := func() []protocol.Observation { return nil }
		if r.PhaseBarriers && scenario == "instantiate" {
			if err := a.barrier(protocol.PhaseEvent{SampleIndex: sampleIndex, Stage: "before_instantiate"}); err != nil {
				return protocol.VectorInstance{}, 0, nil, err
			}
			finish = collectors.GoMemoryWindow("instantiate/api_window", "instantiation_including_start_excluding_initialization_verification_release")
		}
		start := time.Now()
		m, err := a.engine.InstantiateModule(ctx, compiled, config)
		if scenario == "instantiate" {
			elapsed = time.Since(start).Nanoseconds()
		}
		apiObservations = finish()
		close := func() {
			if scenario == "teardown" {
				compiled.Close(ctx)
				a.close()
				return
			}
			if m != nil {
				m.Close(ctx)
			}
			if scenario == "compile" {
				compiled.Close(ctx)
			}
		}
		if err != nil {
			close()
			return protocol.VectorInstance{}, 0, nil, err
		}
		if r.PhaseBarriers && scenario == "instantiate" {
			if err := a.barrier(protocol.PhaseEvent{SampleIndex: sampleIndex, Stage: "instantiated"}); err != nil {
				close()
				return protocol.VectorInstance{}, 0, nil, err
			}
			sampleIndex++
		}
		if err = a.initialize(m); err != nil {
			close()
			return protocol.VectorInstance{}, 0, nil, err
		}
		f := m.ExportedFunction(a.prep.Workload.Export)
		if f == nil || !hasMemory(m) {
			close()
			return protocol.VectorInstance{}, 0, nil, fmt.Errorf("missing vector export/memory")
		}
		instance := protocol.VectorInstance{
			ReleaseBarrier: func(i int, stage string) error { return a.barrier(protocol.PhaseEvent{SampleIndex: i, Stage: stage}) },
			Snapshot: func() []protocol.Observation {
				return append(apiObservations, protocol.Observation{Metric: "guest.memory.logical", DefinitionVersion: 1, Value: protocol.Value(float64(m.Memory().Size())), Unit: "bytes", Scope: "guest_linear_memory", Phase: scenario + "/vector_verified", Collector: "wazero.Memory.Size", CollectorVersion: "1.12.0", Quality: "engine_reported", Profile: "memory", Status: "available", Denominator: "instance"})
			},
			Pointer: func(name string) ([]uint64, error) {
				f := m.ExportedFunction(name)
				if f == nil {
					return nil, fmt.Errorf("missing vector pointer %s", name)
				}
				return f.Call(ctx)
			},
			Write: m.Memory().Write, Read: m.Memory().Read,
			Invoke: func(in, n, out uint32) error { _, err := f.Call(ctx, uint64(in), uint64(n), uint64(out)); return err },
		}
		return instance, elapsed, close, nil
	}, func() func() []protocol.Observation {
		if r.PhaseBarriers && r.Scenario == "first-call" {
			return collectors.GoMemoryWindow("first-call/vector_sequence_call_window", protocol.VectorFirstCallWindowDenominator)
		}
		if r.PhaseBarriers && r.Scenario == "instantiate" {
			return func() []protocol.Observation { return nil }
		}
		if r.Scenario == "teardown" {
			return collectors.GoMemoryWindow("teardown/api_window", "release_excluding_setup_verification")
		}
		return collectors.GoMemoryWindow(r.Scenario+"/vector_lifecycle", "sequence_including_instance_setup_verification_release")
	})
}
