package analysis

import (
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
	"math"
	"testing"
)

func memoryTimelineFixture() experiment.Bundle {
	b := experiment.Bundle{}
	t := experiment.Trial{ID: "trial", Runtime: "r", Workload: "w", Scenario: "density-cycle", Profile: "memory", Status: "ok", Block: 0}
	for i, v := range []float64{100, 500, 0} {
		o := protocol.Observation{Metric: "process.rss", DefinitionVersion: 1, Unit: "bytes", Scope: "adapter_process", Phase: "density-cycle/density_cycle_released", Collector: "procfs", CollectorVersion: "1", Quality: "boundary_snapshot_only", Profile: "memory", Status: "available", Value: protocol.Value(v), Denominator: "process"}
		t.Samples = append(t.Samples, protocol.Sample{Index: i, Warmup: i == 0, Verified: true, Operations: 1, SampleType: "individual_operation", Observations: []protocol.Observation{o}})
	}
	b.Trials = []experiment.Trial{t}
	return b
}

func TestMemoryTimelinePreservesSequenceAndSignedChanges(t *testing.T) {
	b := memoryTimelineFixture()
	lines := MemoryTimelines(b)
	if len(lines) != 1 {
		t.Fatal(lines)
	}
	l := lines[0]
	if l.Status != "available" || l.FirstLastChange == nil || *l.FirstLastChange != -100 || len(l.Points) != 3 || *l.Points[1].Value != 500 || !l.Points[0].Warmup || *l.Points[2].Value != 0 {
		t.Fatal(l)
	}
	if l.Measurement.Value != nil || l.Measurement.Status != "" {
		t.Fatal("value leaked into identity")
	}
	// The analysis copies observation values and does not mutate evidence.
	*l.Points[0].Value = 42
	if *b.Trials[0].Samples[0].Observations[0].Value != 100 {
		t.Fatal("evidence mutated")
	}
}

func TestMemoryTimelineUnavailableAndAmbiguous(t *testing.T) {
	for name, mutate := range map[string]func(*experiment.Bundle){
		"missing":     func(b *experiment.Bundle) { b.Trials[0].Samples[1].Observations = nil },
		"unavailable": func(b *experiment.Bundle) { b.Trials[0].Samples[1].Observations[0].Status = "permission_denied" },
		"nil":         func(b *experiment.Bundle) { b.Trials[0].Samples[1].Observations[0].Value = nil },
		"nan":         func(b *experiment.Bundle) { b.Trials[0].Samples[1].Observations[0].Value = protocol.Value(math.NaN()) },
		"negative":    func(b *experiment.Bundle) { b.Trials[0].Samples[1].Observations[0].Value = protocol.Value(-1) },
		"duplicate": func(b *experiment.Bundle) {
			s := &b.Trials[0].Samples[1]
			s.Observations = append(s.Observations, s.Observations[0])
		},
		"unverified":      func(b *experiment.Bundle) { b.Trials[0].Samples[1].Verified = false },
		"failed":          func(b *experiment.Bundle) { b.Trials[0].Status = "oom" },
		"unordered":       func(b *experiment.Bundle) { b.Trials[0].Samples[1].Index = 0 },
		"boundary change": func(b *experiment.Bundle) { b.Trials[0].Samples[1].Operations = 2 },
	} {
		t.Run(name, func(t *testing.T) {
			b := memoryTimelineFixture()
			mutate(&b)
			l := MemoryTimelines(b)[0]
			if l.FirstLastChange != nil || l.Status == "available" || len(l.Points) != 3 {
				t.Fatal(l)
			}
		})
	}
}

func TestMemoryTimelineDomainsAndActivityIsolation(t *testing.T) {
	b := memoryTimelineFixture()
	for i := range b.Trials[0].Samples {
		s := &b.Trials[0].Samples[i]
		o := s.Observations[0]
		o.Phase = "density-cycle/density_cycle_ready"
		s.Observations = append(s.Observations, o)
		o.Metric = "host.alloc.bytes"
		s.Observations = append(s.Observations, o)
		o.Metric = "process.rss"
		o.Quality = "sampled_observed_peak"
		s.Observations = append(s.Observations, o)
	}
	if got := MemoryTimelines(b); len(got) != 2 {
		t.Fatal(got)
	}
	copy := b.Trials[0]
	copy.ID = "other-launch"
	copy.Block = 1
	b.Trials = append(b.Trials, copy)
	if got := MemoryTimelines(b); len(got) != 4 {
		t.Fatal("independent launches joined")
	}
	b.Trials[0].Block = -1
	b.Trials[1].Profile = "timing"
	if len(MemoryTimelines(b)) != 0 {
		t.Fatal("admission or timing included")
	}
}
