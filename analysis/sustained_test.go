package analysis

import (
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
	"testing"
	"time"
)

func TestSustainedRatesUseWindowNotHeapChange(t *testing.T) {
	w := protocol.Workload{ID: "w", ABI: "core", Reset: "stateless", Export: "benchmark", Oracle: protocol.Oracle{Kind: "exact_u64", Expected: protocol.Values{7}}}
	obs := func(name, unit string, value float64) protocol.Observation {
		return protocol.Observation{Metric: name, DefinitionVersion: 1, Value: protocol.Value(value), Unit: unit, Scope: "adapter_process_go_heap", Phase: "sustained", Collector: "runtime.ReadMemStats", Quality: "engine_reported", Profile: "memory", Status: "available", Denominator: "batch_operation_window_including_allocator_snapshots_excluding_verification"}
	}
	var samples []protocol.Sample
	for i := 0; i < 2; i++ {
		start := int64(i) * 3000000
		samples = append(samples, protocol.Sample{Index: i, ElapsedNS: 1000000, Operations: 1, SampleType: "individual_operation", Verified: true, Result: protocol.Values{7}, SustainedWindow: &protocol.SustainedWindow{StartNS: start, OperationStartNS: start + 10, OperationEndNS: start + 1000010, EndNS: start + 2000000}, Observations: []protocol.Observation{obs("host.alloc.bytes", "bytes", 100), obs("host.heap.end", "bytes", 1000-float64(i)*500), obs("host.gc.cycles", "count", 0)}})
	}
	samples[1].SustainedRelease = &protocol.SustainedRelease{StartNS: 6000000, EndNS: 6000010, Closed: true}
	b := experiment.Bundle{Manifest: experiment.Manifest{Kind: "measurement", Lock: experiment.Lock{Options: experiment.Options{Profile: "memory", Samples: 2, Operations: 1, SustainedDuration: time.Millisecond}, Workloads: []protocol.Workload{w}}}, Trials: []experiment.Trial{{ID: "t", Workload: "w", Scenario: "sustained", Profile: "memory", Status: "ok", Samples: samples}, {ID: "missing", Workload: "w", Scenario: "sustained", Profile: "memory", Status: "unsupported", Reason: "capability absent"}}}
	got := SustainedSessions(b)
	if len(got) != 2 || len(got[0].Points) != 2 || got[1].Status != "unsupported" {
		t.Fatal("coverage lost", got)
	}
	for _, p := range got[0].Points {
		if p.AllocRate == nil || *p.AllocRate != 50000 || p.GCCycles == nil || *p.GCCycles != 0 {
			t.Fatal("wrong allocation rate or missing zero", p)
		}
	}
	js := protocol.Observation{Metric: "host.js_heap.end", DefinitionVersion: 1, Unit: "bytes", Phase: "sustained", Status: "available", Value: protocol.Value(42), Scope: "adapter_process_v8_heap", Collector: "node:process.memoryUsage", Quality: "engine_reported", Profile: "memory", Denominator: "batch_operation_window_including_heap_snapshots_excluding_verification"}
	samples[0].Observations = append(samples[0].Observations, js)
	if got := SustainedSessions(b); got[0].Points[0].JSHeapEnd == nil || *got[0].Points[0].JSHeapEnd != 42 || *got[0].Points[0].AllocRate != 50000 {
		t.Fatal("V8 heap missing or mixed with Go allocation rate")
	}
	samples[0].Observations = append(samples[0].Observations, js)
	if got := SustainedSessions(b); got[0].Points[0].JSHeapEnd != nil {
		t.Fatal("duplicate V8 heap domain accepted")
	}
	samples[1].Observations = append(samples[1].Observations, obs("host.alloc.bytes", "bytes", 100))
	if got := SustainedSessions(b); got[0].Points[1].AllocRate != nil {
		t.Fatal("duplicate domain treated as exact")
	}
	b.Trials[0].Profile = "timing"
	b.Manifest.Lock.Options.Profile = "timing"
	if got := SustainedSessions(b); got[0].Points[0].AllocRate != nil {
		t.Fatal("memory attributed to timing pass")
	}
	b.Manifest.Lock.Options.SustainedDuration = time.Second
	if got := SustainedSessions(b); got[0].Status != "invalid_duration_qualification" {
		t.Fatal("too-short run qualified")
	}
}
