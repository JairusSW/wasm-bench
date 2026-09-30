package experiment

import (
	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/protocol"
	"runtime"
	"testing"
	"time"
)

func TestSustainedSavedDurationQualification(t *testing.T) {
	w := protocol.Workload{ID: "w", ABI: "core", Reset: "stateless", Export: "benchmark", Oracle: protocol.Oracle{Kind: "exact_u64", Expected: protocol.Values{7}}}
	s := []protocol.Sample{{Index: 0, ElapsedNS: 1000000, Operations: 1, SampleType: "individual_operation", Verified: true, Result: protocol.Values{7}, SustainedWindow: &protocol.SustainedWindow{StartNS: 0, OperationStartNS: 0, OperationEndNS: 1000000, EndNS: 1000001}}, {Index: 1, ElapsedNS: 1000000, Operations: 1, SampleType: "individual_operation", Verified: true, Result: protocol.Values{7}, SustainedWindow: &protocol.SustainedWindow{StartNS: 2000000, OperationStartNS: 2000000, OperationEndNS: 3000000, EndNS: 3000001}, SustainedRelease: &protocol.SustainedRelease{StartNS: 4000000, EndNS: 5000000, Closed: true}}}
	b := Bundle{Manifest: Manifest{Lock: Lock{Options: Options{Profile: "timing", Samples: 2, Operations: 1, SustainedDuration: time.Millisecond}, Workloads: []protocol.Workload{w}}}, Trials: []Trial{{ID: "t", Workload: "w", Scenario: "sustained", Profile: "timing", Status: "ok", DurationNS: 6000000, Samples: s}}}
	if err := ValidateSustainedEvidence(b); err != nil {
		t.Fatal(err)
	}
	b.Manifest.Lock.Options.SustainedDuration = time.Second
	if ValidateSustainedEvidence(b) == nil {
		t.Fatal("too-short run relabeled successful")
	}
	b.Trials[0].Status = "duration_budget_not_met"
	if err := ValidateSustainedEvidence(b); err != nil {
		t.Fatal(err)
	}
	b.Trials[0].DurationNS = 100
	if ValidateSustainedEvidence(b) == nil {
		t.Fatal("forged session exceeds trial lifetime")
	}
	b.Trials[0].DurationNS = 6000000
	b.Manifest.Lock.Options.Profile = "memory"
	b.Trials[0].Profile = "memory"
	b.Manifest.Lock.Options.SustainedPostCollection = true
	s[1].SustainedRelease.PostCollection = &protocol.SustainedCollection{StartNS: 5100000, EndNS: 6500000, Policy: "one_forced_go_gc", Observations: collectors.GoMemoryObservations(runtime.MemStats{}, runtime.MemStats{NumGC: 1, NumForcedGC: 1}, "sustained/post_collection", protocol.SustainedCollectionDenominator)}
	if ValidateSustainedEvidence(b) == nil {
		t.Fatal("post-collection exceeds trial lifetime")
	}
	s[1].SustainedRelease.PostCollection.EndNS = 5500000
	if err := ValidateSustainedEvidence(b); err != nil {
		t.Fatal(err)
	}
	b.Manifest.Lock.Options.SustainedPostCollection = false
	if ValidateSustainedEvidence(b) == nil {
		t.Fatal("unlocked post-collection accepted")
	}
	s[1].SustainedRelease.PostCollection = nil
	s[1].SustainedRelease.Policy = "js_references_dropped"
	s[1].SustainedRelease.Closed = false
	s[0].Observations = []protocol.Observation{{Metric: "host.js_heap.end", DefinitionVersion: 1, Unit: "bytes", Scope: "adapter_process_v8_heap", Collector: "node:process.memoryUsage", Quality: "engine_reported", Profile: "memory", Phase: "sustained", Denominator: "batch_operation_window_including_heap_snapshots_excluding_verification", Status: "available", Value: protocol.Value(10)}}
	if err := ValidateSustainedEvidence(b); err != nil {
		t.Fatal(err)
	}
	s[0].Observations[0].Phase = "sustained/release_window"
	if ValidateSustainedEvidence(b) == nil {
		t.Fatal("V8 heap release window moved before release")
	}
}
