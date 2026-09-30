package collectors

import (
	"github.com/wasmbench/wasmbench/metrics"
	"runtime"
	"testing"
)

func TestGoMemoryWindow(t *testing.T) {
	finish := GoMemoryWindow("steady/vector_lifecycle", "sequence_including_instance_setup_verification_release")
	buffer := make([]byte, 1<<20)
	buffer[0] = 1
	observations := finish()
	runtime.KeepAlive(buffer)
	if len(observations) != 7 {
		t.Fatal(observations)
	}
	for _, o := range observations {
		if o.Value == nil || o.Status != "available" || o.Scope != "adapter_process_go_heap" || o.Profile != "memory" || o.Denominator != "sequence_including_instance_setup_verification_release" {
			t.Fatal(o)
		}
		if o.Metric == "host.alloc.bytes" && *o.Value < 1<<20 {
			t.Fatal("lost allocator volume", o)
		}
	}
}

func TestGoMemorySnapshotSemantics(t *testing.T) {
	before := runtime.MemStats{TotalAlloc: 100, Mallocs: 10, HeapAlloc: 80, NumGC: 3, NumForcedGC: 1, PauseTotalNs: 1000}
	after := runtime.MemStats{TotalAlloc: 150, Mallocs: 15, HeapAlloc: 20, NumGC: 5, NumForcedGC: 2, PauseTotalNs: 1300}
	want := map[string]float64{"host.alloc.bytes": 50, "host.alloc.count": 5, "host.heap.start": 80, "host.heap.end": 20, "host.gc.cycles": 2, "host.gc.forced_cycles": 1, "host.gc.pause_time": 300}
	for _, o := range GoMemoryObservations(before, after, "compile/api_window", "test_window") {
		v, ok := want[o.Metric]
		if !ok || o.Value == nil || *o.Value != v || o.Phase != "compile/api_window" || o.Denominator != "test_window" || o.Quality != "engine_reported" {
			t.Fatal(o)
		}
		registered := false
		for _, d := range metrics.Registry {
			if d.Name == o.Metric {
				registered = d.Unit == o.Unit && d.Scope == o.Scope && d.Version == o.DefinitionVersion
			}
		}
		if !registered {
			t.Fatal("unregistered observation", o)
		}
		delete(want, o.Metric)
	}
	if len(want) != 0 {
		t.Fatal("missing metrics", want)
	}
}

func TestGoMemoryWindowObservesGC(t *testing.T) {
	finish := GoMemoryWindow("diagnostic", "test_forced_gc")
	// Force collection only in this test, never in the production collector.
	runtime.GC()
	seen := map[string]bool{}
	for _, o := range finish() {
		if o.Metric == "host.gc.cycles" || o.Metric == "host.gc.forced_cycles" || o.Metric == "host.gc.pause_time" {
			if o.Value == nil || *o.Value <= 0 {
				t.Fatal("lost GC accounting", o)
			}
			seen[o.Metric] = true
		}
	}
	if len(seen) != 3 {
		t.Fatal(seen)
	}
}
