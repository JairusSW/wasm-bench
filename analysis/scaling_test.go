package analysis

import (
	"fmt"
	"github.com/wasmbench/wasmbench/corpus"
	"math"
	"testing"

	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

func scalingFixture() experiment.Bundle {
	b := timingFixture(experiment.Bundle{})
	for _, size := range []int{1, 10, 100} {
		id := fmt.Sprint(size)
		b.Manifest.Lock.Workloads = append(b.Manifest.Lock.Workloads, protocol.Workload{ID: id, Generator: "test-v1", Dimension: "functions", Size: size})
		for launch := 0; launch < 3; launch++ {
			b.Trials = append(b.Trials, experiment.Trial{ID: fmt.Sprintf("%s-%d", id, launch), Runtime: "r", Workload: id, Scenario: "compile", Profile: "timing", Block: launch, Status: "ok", Samples: []protocol.Sample{{ElapsedNS: int64(size * size * 2), Operations: 2, Verified: true}, {ElapsedNS: 99999999, Operations: 1, Verified: true, Warmup: true}}})
		}
	}
	return b
}

func TestScalingFitAndMissing(t *testing.T) {
	b := scalingFixture()
	curves := Scaling(b)
	if len(curves) != 1 || curves[0].LogLogSlope == nil || math.Abs(*curves[0].LogLogSlope-2) > 1e-10 {
		t.Fatal(curves)
	}
	p := curves[0].Points[0]
	if p.Launches != 3 || p.Median == nil || *p.Median != 1 || p.Low == nil {
		t.Fatal(p)
	}
	for i := range b.Trials {
		if b.Trials[i].Workload == "10" {
			b.Trials[i].Status = "timeout"
		}
	}
	c := Scaling(b)[0]
	if c.LogLogSlope != nil || c.Points[1].Median != nil || c.Points[1].Low != nil || c.Points[1].Outcomes["timeout"] != 3 {
		t.Fatal(c)
	}
}

func TestGuestDensityCurvesDoNotMergeStatePolicies(t *testing.T) {
	ws, err := corpus.Generate(t.TempDir(), "guest-density")
	if err != nil {
		t.Fatal(err)
	}
	b := timingFixture(experiment.Bundle{})
	b.Manifest.Lock.Workloads = ws
	for _, w := range ws {
		for block := 0; block < 3; block++ {
			b.Trials = append(b.Trials, experiment.Trial{ID: fmt.Sprintf("%s/%d", w.ID, block), Runtime: "r", Workload: w.ID, Scenario: "guest-density", Profile: "timing", Status: "ok", Block: block, Samples: []protocol.Sample{{ElapsedNS: int64(w.Size) * 10, Operations: 1, Verified: true}}})
		}
	}
	curves := Scaling(b)
	if len(curves) != 8 {
		t.Fatal("merged policies/state/memory sizes", len(curves))
	}
	for _, c := range curves {
		if len(c.Points) != 4 || len(c.Marginal) != 3 {
			t.Fatal("missing full or marginal curve", c)
		}
		for _, p := range c.Points {
			if p.Launches != 3 || p.Median == nil || *p.Median != float64(p.Size*10) {
				t.Fatal(p)
			}
		}
	}
}

func TestScalingCollectorIsolationAndLaunchWeight(t *testing.T) {
	b := scalingFixture()
	for i := range b.Trials {
		t := &b.Trials[i]
		t.Profile = "memory"
		o := protocol.Observation{Metric: "host.alloc.bytes", DefinitionVersion: 1, Value: protocol.Value(10), Status: "available", Scope: "go_heap", Quality: "engine_reported", Denominator: "batch", Collector: "go"}
		t.Samples[0].Observations = []protocol.Observation{o}
		o.Scope = "native_allocator"
		o.Value = protocol.Value(30)
		t.Samples[0].Observations = append(t.Samples[0].Observations, o)
	}
	// More samples in one launch must not give that launch more weight.
	for j := 0; j < 100; j++ {
		s := b.Trials[0].Samples[0]
		s.Observations = append([]protocol.Observation(nil), s.Observations...)
		s.Observations[0].Value = protocol.Value(1000)
		b.Trials[0].Samples = append(b.Trials[0].Samples, s)
	}
	cs := Scaling(b)
	if len(cs) != 2 {
		t.Fatal(cs)
	}
	for _, c := range cs {
		want := 10.0
		if c.Measurement.Scope == "native_allocator" {
			want = 30
		}
		if *c.Points[0].Median != want || c.BatchOperations != 2 {
			t.Fatal(c)
		}
	}
	// An unavailable observation with a numeric field is still unavailable.
	for i := range b.Trials {
		b.Trials[i].Samples = b.Trials[i].Samples[:1]
		for j := range b.Trials[i].Samples[0].Observations {
			b.Trials[i].Samples[0].Observations[j].Status = "unsupported"
		}
	}
	for _, c := range Scaling(b) {
		if c.Points[0].Median != nil {
			t.Fatal(c)
		}
	}
}

func TestScalingInsufficientLaunchesAndZero(t *testing.T) {
	b := scalingFixture()
	b.Trials = b.Trials[:1]
	b.Trials[0].Samples[0].ElapsedNS = 0
	c := Scaling(b)[0]
	if c.Points[0].Median == nil || *c.Points[0].Median != 0 || c.Points[0].Low != nil || c.LogLogSlope != nil || c.Points[1].Median != nil {
		t.Fatal(c)
	}
}
