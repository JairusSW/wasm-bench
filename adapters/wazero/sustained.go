package main

import (
	"fmt"
	"runtime"
	"time"

	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/protocol"
)

func (a *adapter) runSustained(r *protocol.RunRequest) ([]protocol.Sample, error) {
	if err := protocol.ValidateSustained(a.prep, r); err != nil {
		return nil, err
	}
	a.close()
	if err := a.setup(); err != nil {
		return nil, err
	}
	// No untimed verification call: invocation one is retained as a sample.
	out := make([]protocol.Sample, 0, r.Samples+r.Warmup)
	results := make([][]uint64, r.Operations)
	epoch := time.Now()
	for i := 0; i < r.Samples+r.Warmup; i++ {
		p := &protocol.SustainedWindow{StartNS: time.Since(epoch).Nanoseconds()}
		var before, after runtime.MemStats
		if a.prep.Profile == "memory" {
			runtime.ReadMemStats(&before)
		}
		start := time.Now()
		p.OperationStartNS = start.Sub(epoch).Nanoseconds()
		for j := 0; j < r.Operations; j++ {
			v, err := a.fn.Call(ctx, a.prep.Workload.Args...)
			if err != nil {
				return out, err
			}
			results[j] = v
		}
		end := time.Now()
		p.OperationEndNS = end.Sub(epoch).Nanoseconds()
		if a.prep.Profile == "memory" {
			runtime.ReadMemStats(&after)
		}
		p.EndNS = time.Since(epoch).Nanoseconds()
		for _, v := range results {
			if !a.verify(v) {
				return out, fmt.Errorf("incorrect result in sustained batch %d", i)
			}
		}
		s := protocol.Sample{Index: i, Warmup: i < r.Warmup, ElapsedNS: end.Sub(start).Nanoseconds(), Operations: r.Operations, SampleType: "batch_average", Verified: true, Result: append([]uint64(nil), results[len(results)-1]...), SustainedWindow: p}
		if r.Operations == 1 {
			s.SampleType = "individual_operation"
		}
		if a.prep.Profile == "memory" {
			s.Observations = collectors.GoMemoryObservations(before, after, "sustained", "batch_operation_window_including_allocator_snapshots_excluding_verification")
			if hasMemory(a.instance) {
				s.Observations = append(s.Observations, protocol.Observation{Metric: "guest.memory.logical", DefinitionVersion: 1, Value: protocol.Value(float64(a.instance.Memory().Size())), Unit: "bytes", Scope: "guest_linear_memory", Phase: "sustained", Collector: "wazero.Memory.Size", CollectorVersion: "1.12.0", Quality: "engine_reported", Profile: "memory", Status: "available", Denominator: "instance"})
			}
		}
		out = append(out, s)
	}
	var before, after runtime.MemStats
	if a.prep.Profile == "memory" {
		runtime.ReadMemStats(&before)
	}
	start := time.Now()
	instance := a.instance
	if err := a.engine.Close(ctx); err != nil {
		return out, err
	}
	end := time.Now()
	if !instance.IsClosed() {
		return out, fmt.Errorf("sustained instance not closed")
	}
	a.engine = nil
	a.compiled = nil
	a.instance = nil
	a.fn = nil
	instance = nil
	results = nil
	last := &out[len(out)-1]
	last.SustainedRelease = &protocol.SustainedRelease{StartNS: start.Sub(epoch).Nanoseconds(), EndNS: end.Sub(epoch).Nanoseconds(), Closed: true}
	if a.prep.Profile == "memory" {
		runtime.ReadMemStats(&after)
		last.Observations = append(last.Observations, collectors.GoMemoryObservations(before, after, "sustained/release_window", "logical_engine_module_instance_release_without_forced_gc")...)
	}
	if r.SustainedPostCollection {
		collection := &protocol.SustainedCollection{Policy: "one_forced_go_gc"}
		collection.StartNS = time.Since(epoch).Nanoseconds()
		runtime.ReadMemStats(&before)
		runtime.GC()
		runtime.ReadMemStats(&after)
		collection.EndNS = time.Since(epoch).Nanoseconds()
		collection.Observations = collectors.GoMemoryObservations(before, after, "sustained/post_collection", protocol.SustainedCollectionDenominator)
		last.SustainedRelease.PostCollection = collection
	}
	if _, err := protocol.VerifySustainedSequence(a.prep.Workload, *r, out); err != nil {
		return out, err
	}
	return out, nil
}
