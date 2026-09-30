package main

import (
	"fmt"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/protocol"
	"time"
)

type densityGroup struct {
	engines []wazero.Runtime
	modules []api.Module
	results [][]uint64
}

func (g *densityGroup) close() error {
	var first error
	for _, e := range g.engines {
		if err := e.Close(ctx); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// All modules remain live until the caller explicitly closes the group.
func (a *adapter) buildDensityGroup(g *densityGroup) error {
	d := a.prep.Workload.Density
	var engine wazero.Runtime
	var compiled wazero.CompiledModule
	for i := 0; i < d.Instances; i++ {
		if i == 0 || d.Sharing == "separate_engines" {
			engine = a.newEngine()
			g.engines = append(g.engines, engine)
			var err error
			compiled, err = engine.CompileModule(ctx, a.wasm)
			if err != nil {
				return err
			}
			if len(compiled.ImportedFunctions()) != 0 || len(compiled.ImportedMemories()) != 0 {
				return fmt.Errorf("density requires import-free modules")
			}
		}
		m, err := engine.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName("").WithStartFunctions())
		if err != nil {
			return err
		}
		g.modules = append(g.modules, m)
		if err := a.initialize(m); err != nil {
			return err
		}
		f := m.ExportedFunction(a.prep.Workload.Export)
		if f == nil {
			return fmt.Errorf("missing density export")
		}
		result, err := f.Call(ctx, a.prep.Workload.Args...)
		if err != nil {
			return err
		}
		g.results = append(g.results, append([]uint64(nil), result...))
	}
	return nil
}

func (a *adapter) runDensity(r *protocol.RunRequest) ([]protocol.Sample, error) {
	if err := protocol.ValidateDensity(a.prep, r); err != nil {
		return nil, err
	}
	if r.PhaseBarriers && a.barrier == nil {
		return nil, fmt.Errorf("density barrier handler unavailable")
	}
	a.close()
	out := make([]protocol.Sample, 0, r.Samples)
	for i := 0; i < r.Samples; i++ {
		sample, err := func() (protocol.Sample, error) {
			g := &densityGroup{}
			defer g.close()
			barrier := func(stage string) error {
				if r.PhaseBarriers {
					return a.barrier(protocol.PhaseEvent{SampleIndex: i, Stage: stage})
				}
				return nil
			}
			if err := barrier("before_density"); err != nil {
				return protocol.Sample{}, err
			}
			var finish func() []protocol.Observation
			if a.prep.Profile == "memory" {
				finish = collectors.GoMemoryWindow("density/provision_window", "engine_compile_instantiate_initialize_and_invoke_group_excluding_verification_release")
			}
			start := time.Now()
			err := a.buildDensityGroup(g)
			elapsed := time.Since(start).Nanoseconds()
			var obs []protocol.Observation
			if finish != nil {
				obs = finish()
			}
			if err != nil {
				return protocol.Sample{}, err
			}
			for j, m := range g.modules {
				if !a.verifyInstance(m, g.results[j]) {
					return protocol.Sample{}, fmt.Errorf("incorrect density result at instance %d", j)
				}
			}
			if a.prep.Profile == "memory" {
				var logical uint64
				for _, m := range g.modules {
					if hasMemory(m) {
						logical += uint64(m.Memory().Size())
					}
				}
				obs = append(obs, protocol.Observation{Metric: "density.guest_memory.logical", DefinitionVersion: 1, Value: protocol.Value(float64(logical)), Unit: "bytes", Scope: "instance_group_linear_memory", Phase: "density_ready", Collector: "wazero.Memory.Size", CollectorVersion: "1.12.0", Quality: "engine_reported", Profile: "memory", Status: "available", Denominator: "instance_group"})
			}
			if err := barrier("density_ready"); err != nil {
				return protocol.Sample{}, err
			}
			if err := g.close(); err != nil {
				return protocol.Sample{}, err
			}
			if err := barrier("density_released"); err != nil {
				return protocol.Sample{}, err
			}
			return protocol.Sample{Index: i, ElapsedNS: elapsed, Operations: 1, SampleType: "individual_operation", Verified: true, Result: g.results[0], Observations: obs}, nil
		}()
		if err != nil {
			return out, err
		}
		out = append(out, sample)
	}
	return out, nil
}
