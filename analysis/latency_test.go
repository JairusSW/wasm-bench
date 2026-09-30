package analysis

import (
	"encoding/json"
	"testing"

	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

// Older statistical fixtures omit collection metadata. Declare it explicitly
// without changing the source fixture or relaxing production eligibility.
func timingFixture(b experiment.Bundle) experiment.Bundle {
	b.Manifest.Kind = "measurement"
	b.Manifest.Lock.Options.Profile = "timing"
	b.Trials = append([]experiment.Trial(nil), b.Trials...)
	for i := range b.Trials {
		b.Trials[i].Profile = "timing"
	}
	return b
}

func TestHeadlineLatencyEligibility(t *testing.T) {
	for _, mode := range []string{"timing", "memory", "code", "counters", "profiling", "unknown", "missing_kind", "check", "barriers", "host", "partition", "mismatch"} {
		t.Run(mode, func(t *testing.T) {
			b := timingFixture(experiment.Bundle{Manifest: experiment.Manifest{Lock: experiment.Lock{
				Workloads: []protocol.Workload{{ID: "w"}}, Options: experiment.Options{Scenarios: []string{"steady"}, Launches: 3},
			}}})
			want := "timing_pass"
			switch mode {
			case "memory", "code", "counters", "profiling", "unknown":
				b.Manifest.Lock.Options.Profile, want = mode, "not_timing_pass"
			case "missing_kind":
				b.Manifest.Kind, want = "", "not_measurement"
			case "check":
				b.Manifest.Lock.Options.Check, want = true, "not_measurement"
			case "barriers":
				b.Manifest.Lock.Options.PhaseBarriers, want = true, "phase_instrumented"
			case "host":
				b.Manifest.Lock.HostPolicy, want = &agent.HostPolicy{}, "host_policy_mismatch"
			case "partition":
				b.Manifest.Lock.RequireIsolatedCPUPartition, want = true, "host_policy_mismatch"
			case "mismatch":
				want = "profile_mismatch"
			}
			for block := 0; block < 3; block++ {
				for _, runtime := range []string{"a", "b"} {
					profile := b.Manifest.Lock.Options.Profile
					if mode == "mismatch" {
						profile = "memory"
					}
					b.Trials = append(b.Trials, experiment.Trial{Runtime: runtime, Workload: "w", Scenario: "steady", Profile: profile, Block: block, Status: "ok", Samples: []protocol.Sample{{ElapsedNS: 20, Operations: 2, Verified: true}}})
				}
			}
			before, _ := json.Marshal(b)
			for _, s := range Summarize(b) {
				if s.LatencyStatus != want || s.SuccessfulLaunches != 3 || s.Attempted != 3 || s.RecordedSamples != 3 || s.Failures["ok"] != 3 {
					t.Fatalf("eligibility or execution coverage lost: %+v", s)
				}
				if mode == "timing" {
					if s.Median == nil || *s.Median != 10 || s.Launches != 3 {
						t.Fatal(s)
					}
				} else if s.Median != nil || s.Mean != nil || s.CILow != nil || s.CIHigh != nil || s.StdDev != nil || len(s.LaunchMedians) != 0 || s.Samples != 0 || len(s.WarmupDiagnostics) != 0 {
					t.Fatalf("diagnostic timers became latency: %+v", s)
				}
			}
			c := CompareWithin(b, "a", "b")[0]
			if mode == "timing" {
				if c.Ratio == nil || *c.Ratio != 1 {
					t.Fatal(c)
				}
			} else if c.Ratio != nil || c.Low != nil || c.High != nil || c.Pairs != 0 {
				t.Fatal(c)
			}
			after, _ := json.Marshal(b)
			if string(before) != string(after) {
				t.Fatal("raw evidence mutated")
			}
		})
	}
}

func TestDerivedLatencyUsesManifestEligibility(t *testing.T) {
	for _, mode := range []string{"memory", "check", "barriers", "host"} {
		t.Run(mode, func(t *testing.T) {
			invalidate := func(b *experiment.Bundle) {
				// Leave trial labels untouched: the manifest must also qualify.
				switch mode {
				case "memory":
					b.Manifest.Lock.Options.Profile = "memory"
				case "check":
					b.Manifest.Lock.Options.Check = true
				case "barriers":
					b.Manifest.Lock.Options.PhaseBarriers = true
				case "host":
					b.Manifest.Lock.HostPolicy = &agent.HostPolicy{}
				}
			}
			b := scalingFixture()
			invalidate(&b)
			if curves := Scaling(b); len(curves) != 0 {
				t.Fatalf("ineligible timers produced scaling curves: %+v", curves)
			}
			b = breakEvenFixture()
			invalidate(&b)
			if _, err := BreakEven(b, nil, "a", "b"); err == nil {
				t.Fatal("ineligible timers produced a break-even report")
			}
		})
	}
}
