package main

import (
	"bytes"
	"fmt"
	"runtime"
	"time"

	"github.com/tetratelabs/wazero/api"
	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
)

func (a *adapter) runGuestDensity(r *protocol.RunRequest) ([]protocol.Sample, error) {
	if err := protocol.ValidateGuestDensity(a.prep, r); err != nil {
		return nil, err
	}
	d := a.prep.Workload.GuestDensity
	canonical, err := corpus.CheckpointModule(d.Pages)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(a.wasm, canonical) {
		return nil, fmt.Errorf("guest density requires exact fixed-memory scalar fixture")
	}
	if r.PhaseBarriers && a.barrier == nil {
		return nil, fmt.Errorf("guest density barrier unavailable")
	}
	if err := a.setupCompiled(); err != nil {
		return nil, err
	}
	var saved guestCheckpoint
	if d.Provisioning == "eager_guest_restore" {
		saved, err = a.guestDensityPayload(d.Pages)
		if err != nil {
			return nil, err
		}
	}
	out := make([]protocol.Sample, 0, r.Samples)
	for index := 0; index < r.Samples; index++ {
		s, err := a.guestDensitySample(r, index, saved)
		if err != nil {
			return out, err
		}
		out = append(out, s)
	}
	runtime.KeepAlive(saved)
	return out, nil
}

// Source references leave scope before any measurement sample. Logical close
// and reference drop do not imply that the Go allocator has reclaimed pages.
func (a *adapter) guestDensityPayload(pages uint32) (guestCheckpoint, error) {
	source, err := a.instantiate()
	if err != nil {
		return guestCheckpoint{}, err
	}
	defer source.Close(ctx)
	if _, err := source.ExportedFunction("initialize").Call(ctx); err != nil {
		return guestCheckpoint{}, err
	}
	data, g, err := checkpointState(source, pages*65536)
	if err != nil {
		return guestCheckpoint{}, err
	}
	expected := protocol.CheckpointMemory(pages, false)
	if !bytes.Equal(data, expected) || g.Get() != 42 {
		return guestCheckpoint{}, fmt.Errorf("incorrect guest density source state")
	}
	saved := saveGuestCheckpoint(data, g)
	if _, err := source.ExportedFunction("first_write").Call(ctx); err != nil {
		return guestCheckpoint{}, err
	}
	if !bytes.Equal(data, protocol.CheckpointMemory(pages, true)) || g.Get() != 43 || !bytes.Equal(saved.memory, expected) || saved.state != 42 {
		return guestCheckpoint{}, fmt.Errorf("guest density source copy aliases or mutation failed")
	}
	if err := source.Close(ctx); err != nil {
		return guestCheckpoint{}, err
	}
	return saved, nil
}

func (a *adapter) guestDensitySample(r *protocol.RunRequest, index int, saved guestCheckpoint) (protocol.Sample, error) {
	w := a.prep.Workload
	d := w.GuestDensity
	instances := make([]api.Module, d.Instances)
	defer func() {
		for _, m := range instances {
			if m != nil {
				_ = m.Close(ctx)
			}
		}
	}()
	evidence := &protocol.GuestDensityResult{Provisioning: d.Provisioning, Instances: make([]protocol.GuestDensityInstance, d.Instances)}
	stages := protocol.PhaseStages(r.Scenario)
	barrier := func(stage string) error {
		if r.PhaseBarriers {
			return a.barrier(protocol.PhaseEvent{SampleIndex: index, Stage: stage})
		}
		return nil
	}
	if err := barrier(stages[0]); err != nil {
		return protocol.Sample{}, err
	}
	var finish func() []protocol.Observation
	if a.prep.Profile == "memory" {
		finish = collectors.GoMemoryWindow(protocol.GuestDensityPhase, protocol.GuestDensityDenominator)
	}
	start := time.Now()
	for i := range instances {
		m, err := a.instantiate()
		if err != nil {
			return protocol.Sample{}, err
		}
		instances[i] = m
		if d.Provisioning == "fresh_initialize" {
			_, err = m.ExportedFunction("initialize").Call(ctx)
		} else {
			var g api.MutableGlobal
			_, g, err = checkpointState(m, d.Pages*65536)
			if err == nil {
				err = restoreGuestCheckpoint(m, g, saved)
			}
		}
		if err != nil {
			return protocol.Sample{}, err
		}
		if d.FirstWrite {
			if _, err := m.ExportedFunction("first_write").Call(ctx); err != nil {
				return protocol.Sample{}, err
			}
		}
	}
	elapsed := time.Since(start).Nanoseconds()
	var obs []protocol.Observation
	if finish != nil {
		obs = finish()
	}
	// Observe the simultaneous group before verification allocations. Source,
	// compilation and checkpoint creation are outside the provisioning window.
	if err := barrier(stages[1]); err != nil {
		return protocol.Sample{}, err
	}
	expected := protocol.CheckpointMemory(d.Pages, d.FirstWrite)
	global := uint64(42)
	if d.FirstWrite {
		global = 43
	}
	for i, m := range instances {
		data, g, err := checkpointState(m, d.Pages*65536)
		if err != nil {
			return protocol.Sample{}, err
		}
		if !bytes.Equal(data, expected) || g.Get() != global {
			return protocol.Sample{}, fmt.Errorf("incorrect guest density instance %d state", i)
		}
		result, err := m.ExportedFunction("benchmark").Call(ctx)
		if err != nil {
			return protocol.Sample{}, err
		}
		if len(result) != 1 || result[0] != w.Oracle.Expected[0] {
			return protocol.Sample{}, fmt.Errorf("incorrect guest density instance %d checksum", i)
		}
		evidence.Instances[i] = protocol.GuestDensityInstance{MemorySHA256: checkpointDigest(data), Global: uint32(g.Get()), Checksum: result[0]}
	}
	if d.Provisioning == "eager_guest_restore" {
		evidence.PayloadBytes = uint64(len(saved.memory)) + 4
		evidence.SourceIndependent = true
		evidence.PayloadUnchanged = bytes.Equal(saved.memory, protocol.CheckpointMemory(d.Pages, false)) && saved.state == 42
	}
	if a.prep.Profile == "memory" {
		obs = append(obs, protocol.Observation{Metric: "density.guest_memory.logical", DefinitionVersion: 1, Value: protocol.Value(float64(uint64(d.Pages) * 65536 * uint64(d.Instances))), Unit: "bytes", Scope: "instance_group_linear_memory", Phase: stages[1], Collector: "wazero.Memory.Size", CollectorVersion: "1.12.0", Quality: "exact", Profile: "memory", Status: "available", Denominator: "instance_group"})
	}
	s := protocol.Sample{Index: index, ElapsedNS: elapsed, Operations: 1, SampleType: "individual_operation", Verified: true, Result: w.Oracle.Expected, GuestDensityResult: evidence, Observations: obs}
	if err := protocol.VerifyGuestDensitySample(w, s); err != nil {
		return protocol.Sample{}, err
	}
	for i, m := range instances {
		if err := m.Close(ctx); err != nil {
			return protocol.Sample{}, err
		}
		instances[i] = nil
	}
	if err := barrier(stages[2]); err != nil {
		return protocol.Sample{}, err
	}
	runtime.KeepAlive(saved)
	return s, nil
}
