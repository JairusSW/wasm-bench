package analysis

import (
	"testing"

	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

func densityFootprintFixture() experiment.Bundle {
	b := memoryTimelineFixture()
	for i := range b.Trials[0].Samples {
		s := &b.Trials[0].Samples[i]
		o := s.Observations[0]
		s.Observations = nil
		for j, phase := range protocol.PhaseStages("density-cycle") {
			o.Phase = "density-cycle/" + phase
			o.Value = protocol.Value([]float64{100, 500, 80}[j] + float64(i))
			s.Observations = append(s.Observations, o)
		}
	}
	return b
}

func TestDensityFootprintSignedBoundaryChanges(t *testing.T) {
	b := densityFootprintFixture()
	got := DensityFootprints(MemoryTimelines(b))
	if len(got) != 1 || len(got[0].Points) != 3 {
		t.Fatal(got)
	}
	for i, p := range got[0].Points {
		if p.Status != "available" || *p.ProvisionChange != 400 || *p.ReleaseChange != -420 || *p.CycleChange != -20 || p.Sample != i || p.Warmup != (i == 0) {
			t.Fatal(p)
		}
	}
	*got[0].Points[0].CycleChange = 10
	if *b.Trials[0].Samples[0].Observations[0].Value != 100 {
		t.Fatal("mutated evidence")
	}
}

func TestGuestDensityFootprintBoundaries(t *testing.T) {
	b := densityFootprintFixture()
	b.Trials[0].Scenario = "guest-density"
	for i := range b.Trials[0].Samples {
		for j, stage := range protocol.PhaseStages("guest-density") {
			b.Trials[0].Samples[i].Observations[j].Phase = "guest-density/" + stage
		}
	}
	got := DensityFootprints(MemoryTimelines(b))
	if len(got) != 1 || len(got[0].Points) != 3 || *got[0].Points[0].ProvisionChange != 400 || *got[0].Points[0].ReleaseChange != -420 {
		t.Fatal("lost guest density signed footprint changes", got)
	}
}

func TestDensityFootprintRequiresMatchedDomainsAndVerifiedBoundaries(t *testing.T) {
	for name, mutate := range map[string]func(*experiment.Bundle){
		"collector": func(b *experiment.Bundle) {
			for i := range b.Trials[0].Samples {
				b.Trials[0].Samples[i].Observations[1].Collector = "different"
			}
		},
		"scope": func(b *experiment.Bundle) {
			for i := range b.Trials[0].Samples {
				b.Trials[0].Samples[i].Observations[1].Scope = "different"
			}
		},
		"quality": func(b *experiment.Bundle) {
			for i := range b.Trials[0].Samples {
				b.Trials[0].Samples[i].Observations[1].Quality = "sampled_observed_peak"
			}
		},
		"missing": func(b *experiment.Bundle) {
			b.Trials[0].Samples[1].Observations = b.Trials[0].Samples[1].Observations[:2]
		},
		"unavailable": func(b *experiment.Bundle) { b.Trials[0].Samples[1].Observations[1].Status = "permission_denied" },
		"unverified":  func(b *experiment.Bundle) { b.Trials[0].Samples[1].Verified = false },
		"duplicate": func(b *experiment.Bundle) {
			s := &b.Trials[0].Samples[1]
			s.Observations = append(s.Observations, s.Observations[1])
		},
		"unordered": func(b *experiment.Bundle) { b.Trials[0].Samples[1].Index = 0 },
		"failed":    func(b *experiment.Bundle) { b.Trials[0].Status = "timeout" },
	} {
		t.Run(name, func(t *testing.T) {
			b := densityFootprintFixture()
			mutate(&b)
			for _, g := range DensityFootprints(MemoryTimelines(b)) {
				p := g.Points[1]
				if p.Status == "available" || p.ProvisionChange != nil || p.ReleaseChange != nil || p.CycleChange != nil {
					t.Fatal(p)
				}
			}
		})
	}
}

func TestDensityFootprintLaunchIsolationAndSingleSample(t *testing.T) {
	b := densityFootprintFixture()
	b.Trials[0].Samples = b.Trials[0].Samples[:1]
	b.Trials = append(b.Trials, b.Trials[0])
	b.Trials[1].ID = "another-launch"
	g := DensityFootprints(MemoryTimelines(b))
	if len(g) != 2 || g[0].Points[0].Status != "available" || g[0].Trial == g[1].Trial {
		t.Fatal(g)
	}
	// No joining the before phase from one process to ready/released from another.
	b.Trials[0].Samples = append([]protocol.Sample(nil), b.Trials[0].Samples...)
	b.Trials[0].Samples[0].Observations = b.Trials[0].Samples[0].Observations[:1]
	b.Trials[1].Samples[0].Observations = b.Trials[1].Samples[0].Observations[1:]
	for _, g := range DensityFootprints(MemoryTimelines(b)) {
		if g.Points[0].Status == "available" {
			t.Fatal(g)
		}
	}
}

func TestDensityFootprintZeroAndGrowthDuringRelease(t *testing.T) {
	b := densityFootprintFixture()
	b.Trials[0].Scenario = "density"
	for i := range b.Trials[0].Samples {
		for j, phase := range protocol.PhaseStages("density") {
			o := &b.Trials[0].Samples[i].Observations[j]
			o.Phase = "density/" + phase
			o.Value = protocol.Value([]float64{0, 0, 50}[j])
		}
	}
	for _, p := range DensityFootprints(MemoryTimelines(b))[0].Points {
		if p.Status != "available" || *p.ProvisionChange != 0 || *p.ReleaseChange != 50 || *p.CycleChange != 50 {
			t.Fatal("zero or positive release change lost", p)
		}
	}
}
