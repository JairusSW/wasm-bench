package analysis

import (
	"encoding/json"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
	"testing"
)

func TestSummaryMissingIsNullAndZeroIsMeasured(t *testing.T) {
	for _, tc := range []struct {
		name, status string
		samples      []protocol.Sample
		launches     int
	}{
		{"failed", "timeout", nil, 0},
		{"negative", "ok", []protocol.Sample{{ElapsedNS: -1, Operations: 1, Verified: true}}, 0},
		{"unverified", "ok", []protocol.Sample{{ElapsedNS: 10, Operations: 1}}, 0},
		{"zero", "ok", []protocol.Sample{{ElapsedNS: 0, Operations: 1, Verified: true}}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := experiment.Bundle{Trials: []experiment.Trial{{Runtime: "r", Workload: "w", Status: tc.status, Samples: tc.samples}}}
			s := Summarize(timingFixture(b))[0]
			if s.Launches != tc.launches || s.CILow != nil || s.CIHigh != nil || s.StdDev != nil {
				t.Fatal(s)
			}
			if tc.launches == 0 && (s.Median != nil || s.Mean != nil) {
				t.Fatal("fabricated measurement", s)
			}
			if tc.launches == 1 && (s.Median == nil || *s.Median != 0) {
				t.Fatal("lost real zero", s)
			}
			encoded, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			var raw map[string]any
			if err = json.Unmarshal(encoded, &raw); err != nil {
				t.Fatal(err)
			}
			if raw["ci95_low"] != nil || raw["ci95_high"] != nil {
				t.Fatal(string(encoded))
			}
		})
	}
}

func TestPairedMissingAndShortIntervalAreNull(t *testing.T) {
	b := experiment.Bundle{Manifest: experiment.Manifest{Lock: experiment.Lock{Workloads: []protocol.Workload{{ID: "w"}}, Options: experiment.Options{Scenarios: []string{"steady"}, Launches: 1}}}}
	c := CompareWithin(timingFixture(b), "a", "b")[0]
	if c.Ratio != nil || c.Low != nil || c.High != nil {
		t.Fatal(c)
	}
	for _, runtime := range []string{"a", "b"} {
		b.Trials = append(b.Trials, experiment.Trial{Runtime: runtime, Workload: "w", Scenario: "steady", Status: "ok", Samples: []protocol.Sample{{ElapsedNS: 10, Operations: 1, Verified: true}}})
	}
	c = CompareWithin(timingFixture(b), "a", "b")[0]
	if c.Ratio == nil || *c.Ratio != 1 || c.Low != nil || c.High != nil || c.Status != "insufficient_pairs" {
		t.Fatal(c)
	}
}

func TestIndependentLaunchWeightAndWarmup(t *testing.T) {
	b := experiment.Bundle{}
	b.Trials = append(b.Trials, experiment.Trial{Runtime: "r", Workload: "w", Scenario: "steady", Block: 0, Status: "ok", Samples: []protocol.Sample{{Warmup: true, ElapsedNS: 999999, Operations: 1, Verified: true}, {ElapsedNS: 10, Operations: 1, Verified: true}}})
	many := make([]protocol.Sample, 1000)
	for i := range many {
		many[i] = protocol.Sample{ElapsedNS: 100, Operations: 1, Verified: true}
	}
	b.Trials = append(b.Trials, experiment.Trial{Runtime: "r", Workload: "w", Scenario: "steady", Block: 1, Status: "ok", Samples: many}, experiment.Trial{Runtime: "r", Workload: "w", Scenario: "steady", Block: 2, Status: "timeout"})
	s := Summarize(timingFixture(b))
	if len(s) != 1 || s[0].Median == nil || *s[0].Median != 55 || s[0].Launches != 2 || s[0].Attempted != 3 {
		t.Fatalf("inner loops incorrectly weighted: %+v", s)
	}
}
func TestPairedComparisonRetainsMissingWorkload(t *testing.T) {
	b := experiment.Bundle{Manifest: experiment.Manifest{Lock: experiment.Lock{Workloads: []protocol.Workload{{ID: "w"}, {ID: "missing"}}, Options: experiment.Options{Scenarios: []string{"steady"}, Launches: 3}}}}
	for block := 0; block < 3; block++ {
		for _, r := range []string{"a", "b"} {
			ns := int64(100)
			if r == "b" {
				ns = 50
			}
			b.Trials = append(b.Trials, experiment.Trial{Runtime: r, Workload: "w", Scenario: "steady", Block: block, Status: "ok", Samples: []protocol.Sample{{ElapsedNS: ns, Operations: 1, Verified: true}}})
		}
	}
	c := CompareWithin(timingFixture(b), "a", "b")
	if len(c) != 2 || c[0].Ratio == nil || *c[0].Ratio != 0.5 || c[0].Pairs != 3 || c[1].Status != "no_common_successful_blocks" {
		t.Fatal(c)
	}
}
func TestCheckCannotPublish(t *testing.T) {
	if e := ValidatePublication(experiment.Bundle{Manifest: experiment.Manifest{Kind: "correctness_only", Publication: "official"}}); e == nil {
		t.Fatal("published check")
	}
}
