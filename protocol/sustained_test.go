package protocol

import (
	"math"
	"testing"
)

func sustainedFixture() (Workload, RunRequest, []Sample) {
	w := Workload{ABI: "core", Reset: "stateless", Export: "benchmark", Oracle: Oracle{Kind: "exact_u64", Expected: Values{7}}}
	r := RunRequest{Scenario: "sustained", Samples: 2, Operations: 2, Warmup: 1, SustainedDurationNS: 1000000}
	var out []Sample
	for i := 0; i < 3; i++ {
		start := int64(i) * 2000000
		out = append(out, Sample{Index: i, Warmup: i == 0, ElapsedNS: 1000000, Operations: 2, SampleType: "batch_average", Verified: true, Result: Values{7}, SustainedWindow: &SustainedWindow{StartNS: start, OperationStartNS: start + 100, OperationEndNS: start + 1000100, EndNS: start + 1000200}})
	}
	out[2].SustainedRelease = &SustainedRelease{StartNS: 6000000, EndNS: 6000100, Closed: true}
	return w, r, out
}

func collectionFixture() (Workload, RunRequest, []Sample) {
	w, r, s := sustainedFixture()
	r.SustainedPostCollection = true
	c := &SustainedCollection{StartNS: 7000000, EndNS: 8000000, Policy: "one_forced_go_gc"}
	for _, name := range []string{"host.alloc.bytes", "host.alloc.count", "host.heap.start", "host.heap.end", "host.gc.cycles", "host.gc.forced_cycles", "host.gc.pause_time"} {
		unit := "bytes"
		if name == "host.alloc.count" || name == "host.gc.cycles" || name == "host.gc.forced_cycles" {
			unit = "count"
		}
		if name == "host.gc.pause_time" {
			unit = "ns"
		}
		c.Observations = append(c.Observations, Observation{Metric: name, DefinitionVersion: 1, Unit: unit, Value: Value(1), Phase: "sustained/post_collection", Scope: "adapter_process_go_heap", Collector: "runtime.ReadMemStats", CollectorVersion: "test", Profile: "memory", Quality: "engine_reported", Denominator: SustainedCollectionDenominator, Status: "available"})
	}
	s[len(s)-1].SustainedRelease.PostCollection = c
	return w, r, s
}

func TestPostCollectionEvidence(t *testing.T) {
	w, r, s := collectionFixture()
	if _, err := VerifySustainedSequence(w, r, s); err != nil {
		t.Fatal(err)
	}
	if ValidateSustained(&Preparation{Workload: w, Profile: "timing"}, &r) == nil {
		t.Fatal("collection in headline timing")
	}
	r.SustainedPostCollection = false
	if _, err := VerifySustainedSequence(w, r, s); err == nil {
		t.Fatal("unrequested collection")
	}
	for name, change := range map[string]func(*SustainedCollection){
		"clock":      func(c *SustainedCollection) { c.StartNS = 0 },
		"policy":     func(c *SustainedCollection) { c.Policy = "natural" },
		"missing":    func(c *SustainedCollection) { c.Observations = c.Observations[:6] },
		"duplicate":  func(c *SustainedCollection) { c.Observations[1] = c.Observations[0] },
		"domain":     func(c *SustainedCollection) { c.Observations[0].Scope = "process_rss" },
		"phase":      func(c *SustainedCollection) { c.Observations[0].Phase = "sustained" },
		"nonfinite":  func(c *SustainedCollection) { c.Observations[0].Value = Value(math.Inf(1)) },
		"not forced": func(c *SustainedCollection) { c.Observations[5].Value = Value(0) },
		"fractional": func(c *SustainedCollection) { c.Observations[5].Value = Value(1.5) },
	} {
		t.Run(name, func(t *testing.T) {
			w, r, s := collectionFixture()
			change(s[len(s)-1].SustainedRelease.PostCollection)
			if _, err := VerifySustainedSequence(w, r, s); err == nil {
				t.Fatal("forged collection accepted")
			}
		})
	}
}

func TestSustainedJSReferenceRelease(t *testing.T) {
	w, r, s := sustainedFixture()
	release := s[len(s)-1].SustainedRelease
	release.Policy = "js_references_dropped"
	release.Closed = false
	if _, err := VerifySustainedSequence(w, r, s); err != nil {
		t.Fatal(err)
	}
	release.Closed = true
	if _, err := VerifySustainedSequence(w, r, s); err == nil {
		t.Fatal("JS references claimed engine disposal")
	}
	release.Closed = false
	release.Policy = "unknown"
	if _, err := VerifySustainedSequence(w, r, s); err == nil {
		t.Fatal("unknown release policy")
	}
	release.Policy = "js_references_dropped"
	r.SustainedPostCollection = true
	if _, err := VerifySustainedSequence(w, r, s); err == nil {
		t.Fatal("Go collection applied to JS engine")
	}
}

func TestSustainedClocksAndState(t *testing.T) {
	w, r, s := sustainedFixture()
	if err := ValidateSustained(&Preparation{Workload: w, Profile: "memory"}, &r); err != nil {
		t.Fatal(err)
	}
	if total, err := VerifySustainedSequence(w, r, s); err != nil || total != 2000000 {
		t.Fatal(total, err)
	}
	for _, mutate := range []func([]Sample){func(s []Sample) { s[1].ElapsedNS++ }, func(s []Sample) { s[1].SustainedWindow.StartNS = 0 }, func(s []Sample) { s[1].Result = Values{8} }, func(s []Sample) { s[1].Warmup = true }, func(s []Sample) { s[1].Operations = 1 }, func(s []Sample) { s[0].SustainedRelease = s[2].SustainedRelease }, func(s []Sample) { s[2].SustainedRelease.Closed = false }} {
		w, r, s := sustainedFixture()
		mutate(s)
		if _, err := VerifySustainedSequence(w, r, s); err == nil {
			t.Fatal("accepted forged sequence")
		}
	}
	for _, mutate := range []func(*RunRequest){func(r *RunRequest) { r.PhaseBarriers = true }, func(r *RunRequest) { r.SustainedDurationNS = 0 }, func(r *RunRequest) { r.Samples = 1 }} {
		_, r, _ := sustainedFixture()
		mutate(&r)
		if ValidateSustained(&Preparation{Workload: w, Profile: "timing"}, &r) == nil {
			t.Fatal("invalid sustained request")
		}
	}
}
