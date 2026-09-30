package collectors

import (
	"github.com/wasmbench/wasmbench/protocol"
	"runtime"
)

// GoMemoryWindow snapshots the Go allocator only. The caller supplies the
// boundary and denominator; this is not native allocation or process RSS.
func GoMemoryWindow(phase, denominator string) func() []protocol.Observation {
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	return func() []protocol.Observation {
		var after runtime.MemStats
		runtime.ReadMemStats(&after)
		return GoMemoryObservations(before, after, phase, denominator)
	}
}

// GoMemoryObservations normalizes already captured snapshots without moving the
// caller's collection boundaries. Pause time is cumulative GC stop-the-world
// accounting, not elapsed API time or concurrent GC CPU work. No GC is forced.
func GoMemoryObservations(before, after runtime.MemStats, phase, denominator string) []protocol.Observation {
	values := []struct {
		name, unit string
		value      uint64
	}{
		{"host.alloc.bytes", "bytes", after.TotalAlloc - before.TotalAlloc},
		{"host.alloc.count", "count", after.Mallocs - before.Mallocs},
		{"host.heap.start", "bytes", before.HeapAlloc},
		{"host.heap.end", "bytes", after.HeapAlloc},
		{"host.gc.cycles", "count", uint64(after.NumGC - before.NumGC)},
		{"host.gc.forced_cycles", "count", uint64(after.NumForcedGC - before.NumForcedGC)},
		{"host.gc.pause_time", "ns", after.PauseTotalNs - before.PauseTotalNs},
	}
	out := make([]protocol.Observation, 0, len(values))
	for _, v := range values {
		out = append(out, protocol.Observation{Metric: v.name, DefinitionVersion: 1, Value: protocol.Value(float64(v.value)), Unit: v.unit, Scope: "adapter_process_go_heap", Phase: phase, Collector: "runtime.ReadMemStats", CollectorVersion: runtime.Version(), Quality: "engine_reported", Profile: "memory", Status: "available", Denominator: denominator})
	}
	return out
}
