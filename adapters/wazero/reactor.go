package main

import (
	"bytes"
	"fmt"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/protocol"
)

type reactorOutput struct{ attempted bool }

func (w *reactorOutput) Write(p []byte) (int, error) {
	if len(p) > 0 {
		w.attempted = true
		return 0, fmt.Errorf("reactor no-I/O profile forbids stream output")
	}
	return 0, nil
}

type reactorInstance struct {
	module api.Module
	output *reactorOutput
	fn     api.Function
}

func (a *adapter) newReactorEngine() (wazero.Runtime, error) {
	e := a.newEngine()
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, e); err != nil {
		e.Close(ctx)
		return nil, err
	}
	return e, nil
}
func instantiateReactor(e wazero.Runtime, c wazero.CompiledModule) (*reactorInstance, error) {
	out := &reactorOutput{}
	cfg := wazero.NewModuleConfig().WithName("").WithStartFunctions().WithStdin(bytes.NewReader(nil)).WithStdout(out).WithStderr(out).WithRandSource(zeroRandom{})
	m, err := e.InstantiateModule(ctx, c, cfg)
	if err != nil {
		return nil, err
	}
	return &reactorInstance{module: m, output: out}, nil
}
func reactorInitializer(m api.Module) (api.Function, error) {
	if m.ExportedFunction("_start") != nil {
		return nil, fmt.Errorf("reactor cannot export command _start")
	}
	f := m.ExportedFunction("_initialize")
	if f == nil || len(f.Definition().ParamTypes()) != 0 || len(f.Definition().ResultTypes()) != 0 {
		return nil, fmt.Errorf("reactor _initialize must be () -> ()")
	}
	return f, nil
}
func (a *adapter) readyReactor(instance *reactorInstance) error {
	if instance.output.attempted {
		return fmt.Errorf("reactor no-I/O profile observed forbidden stream output")
	}
	if err := a.applyInput(instance.module); err != nil {
		return err
	}
	instance.fn = instance.module.ExportedFunction(a.prep.Workload.Export)
	if instance.fn == nil {
		return fmt.Errorf("missing reactor workload export")
	}
	return nil
}
func (a *adapter) verifyReactor(instance *reactorInstance, result []uint64) error {
	if instance.output.attempted {
		return fmt.Errorf("reactor no-I/O profile observed forbidden stream output")
	}
	if !a.verifyInstance(instance.module, result) {
		return fmt.Errorf("incorrect reactor result")
	}
	if instance.output.attempted {
		return fmt.Errorf("reactor no-I/O profile observed forbidden stream output during oracle verification")
	}
	return nil
}

func (a *adapter) runReactor(r *protocol.RunRequest) ([]protocol.Sample, error) {
	if err := protocol.ValidateReactorRun(a.prep, r); err != nil {
		return nil, err
	}
	if r.Scenario == "cold-process" {
		return nil, fmt.Errorf("unsupported adapter cold-process request; controller owns process timing")
	}
	var engine wazero.Runtime
	var compiled wazero.CompiledModule
	var err error
	if r.Scenario != "teardown" {
		engine, err = a.newReactorEngine()
		if err != nil {
			return nil, err
		}
		defer engine.Close(ctx)
		if r.Scenario != "compile" {
			compiled, err = engine.CompileModule(ctx, a.wasm)
			if err != nil {
				return nil, err
			}
			defer compiled.Close(ctx)
		}
	}
	// A stateless steady run initializes once and has no hidden workload pre-call.
	var retained *reactorInstance
	if r.Scenario == "steady" && a.prep.Workload.Reset == "stateless" {
		retained, err = instantiateReactor(engine, compiled)
		if err != nil {
			return nil, err
		}
		defer retained.module.Close(ctx)
		init, err := reactorInitializer(retained.module)
		if err != nil {
			return nil, err
		}
		if _, err = init.Call(ctx); err != nil {
			return nil, err
		}
		if err = a.readyReactor(retained); err != nil {
			return nil, err
		}
	}
	warmup := 0
	if r.Scenario == "steady" {
		warmup = r.Warmup
	}
	samples := make([]protocol.Sample, 0, r.Samples+warmup)
	for index := 0; index < r.Samples+warmup; index++ {
		sample, err := func() (protocol.Sample, error) {
			s := protocol.Sample{Index: index, Warmup: index < warmup, Operations: 1, SampleType: "individual_operation", Verified: true}
			measure := func(call func() error) error {
				var finish func() []protocol.Observation
				if a.prep.Profile == "memory" {
					finish = collectors.GoMemoryWindow(r.Scenario+"/reactor_api_window", "declared_reactor_operation_excluding_setup_verification_and_release_unless_teardown")
				}
				start := time.Now()
				err := call()
				s.ElapsedNS = time.Since(start).Nanoseconds()
				if finish != nil {
					s.Observations = finish()
				}
				return err
			}
			e, c := engine, compiled
			if r.Scenario == "teardown" {
				e, err = a.newReactorEngine()
				if err != nil {
					return s, err
				}
				defer e.Close(ctx)
				c, err = e.CompileModule(ctx, a.wasm)
				if err != nil {
					return s, err
				}
				defer c.Close(ctx)
			}
			if r.Scenario == "compile" {
				if err := measure(func() error { var err error; c, err = e.CompileModule(ctx, a.wasm); return err }); err != nil {
					return s, err
				}
				defer c.Close(ctx)
			}
			instance := retained
			if instance == nil {
				instantiate := func() error { var err error; instance, err = instantiateReactor(e, c); return err }
				if r.Scenario == "instantiate" {
					err = measure(instantiate)
				} else {
					err = instantiate()
				}
				if err != nil {
					return s, err
				}
				defer instance.module.Close(ctx)
				init, err := reactorInitializer(instance.module)
				if err != nil {
					return s, err
				}
				initialize := func() error { _, err := init.Call(ctx); return err }
				if r.Scenario == "app-init" {
					err = measure(initialize)
				} else {
					err = initialize()
				}
				if err != nil {
					return s, err
				}
				if err = a.readyReactor(instance); err != nil {
					return s, err
				}
			}
			if r.Scenario == "first-call" || r.Scenario == "steady" {
				if r.Scenario == "steady" && a.prep.Workload.Reset == "stateless" {
					s.Operations = r.Operations
				}
				if s.Operations > 1 {
					s.SampleType = "batch_average"
				}
				results := make([][]uint64, s.Operations)
				if err := measure(func() error {
					for i := range results {
						var err error
						results[i], err = instance.fn.Call(ctx, a.prep.Workload.Args...)
						if err != nil {
							return err
						}
					}
					return nil
				}); err != nil {
					return s, err
				}
				for _, result := range results {
					if err := a.verifyReactor(instance, result); err != nil {
						return s, err
					}
				}
				s.Result = results[len(results)-1]
			} else {
				result, err := instance.fn.Call(ctx, a.prep.Workload.Args...)
				if err != nil {
					return s, err
				}
				if err = a.verifyReactor(instance, result); err != nil {
					return s, err
				}
				s.Result = result
				if r.Scenario == "teardown" {
					if err := measure(func() error {
						if err := instance.module.Close(ctx); err != nil {
							return err
						}
						if err := c.Close(ctx); err != nil {
							return err
						}
						return e.Close(ctx)
					}); err != nil {
						return s, err
					}
				}
			}
			if a.prep.Profile == "memory" && r.Scenario != "teardown" && hasMemory(instance.module) {
				s.Observations = append(s.Observations, protocol.Observation{Metric: "guest.memory.logical", DefinitionVersion: 1, Value: protocol.Value(float64(instance.module.Memory().Size())), Unit: "bytes", Scope: "guest_linear_memory", Phase: r.Scenario + "/post_verification", Collector: "wazero.Memory.Size", CollectorVersion: "1.12.0", Quality: "engine_reported", Profile: "memory", Status: "available", Denominator: "instance"})
			}
			return s, nil
		}()
		if err != nil {
			return samples, err
		}
		samples = append(samples, sample)
	}
	return samples, nil
}
