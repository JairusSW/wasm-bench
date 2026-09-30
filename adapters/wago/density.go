package main

import (
	"context"
	"errors"
	"fmt"
	wago "github.com/wago-org/wago"
	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/protocol"
	"time"
)

type densityGroup struct {
	engines   []*wago.Runtime
	modules   []*wago.Module
	instances []*wago.Instance
	results   [][]uint64
}

func (g *densityGroup) close() error {
	var result error
	for _, e := range g.engines {
		result = errors.Join(result, e.CloseContext(context.Background()))
	}
	for _, m := range g.modules {
		result = errors.Join(result, m.Close())
	}
	return result
}

func (a *adapter) buildDensityGroup(g *densityGroup) error {
	d := a.prep.Workload.Density
	var engine *wago.Runtime
	var module *wago.Module
	for i := 0; i < d.Instances; i++ {
		if i == 0 || d.Sharing == "separate_engines" {
			engine = wago.NewRuntime()
			g.engines = append(g.engines, engine)
			var err error
			module, err = engine.Compile(a.wasm)
			if err != nil {
				return err
			}
			g.modules = append(g.modules, module)
			if len(module.Imports()) != 0 {
				return fmt.Errorf("density requires import-free modules")
			}
		}
		m, err := engine.Instantiate(context.Background(), module)
		if err != nil {
			return err
		}
		g.instances = append(g.instances, m)
		if err := a.initialize(m); err != nil {
			return err
		}
		f, err := m.WasmFunc(a.prep.Workload.Export)
		if err != nil {
			return err
		}
		result, err := f.Invoke(a.prep.Workload.Args...)
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
	a.closeAll()
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
			for j, m := range g.instances {
				if err := a.verifyCompiled(nil, m, g.results[j]); err != nil {
					return protocol.Sample{}, fmt.Errorf("density instance %d: %w", j, err)
				}
			}
			if a.prep.Profile == "memory" {
				var logical uint64
				for _, m := range g.instances {
					if m.Memory() != nil {
						logical += uint64(len(m.Memory().UnsafeBytes()))
					}
				}
				obs = append(obs, protocol.Observation{Metric: "density.guest_memory.logical", DefinitionVersion: 1, Value: protocol.Value(float64(logical)), Unit: "bytes", Scope: "instance_group_linear_memory", Phase: "density_ready", Collector: "wago.Memory", CollectorVersion: sourceRevision, Quality: "engine_reported", Profile: "memory", Status: "available", Denominator: "instance_group"})
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
