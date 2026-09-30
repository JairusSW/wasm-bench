package analysis

import (
	"encoding/json"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
	"math"
	"testing"
)

func throughputFixture() experiment.Bundle {
	b := timingFixture(experiment.Bundle{})
	b.Manifest.Lock.Workloads = []protocol.Workload{{ID: "w", WorkUnit: "byte", Units: 10}}
	for block := 0; block < 3; block++ {
		b.Trials = append(b.Trials, experiment.Trial{Block: block, Runtime: "r", Workload: "w", Scenario: "steady", Profile: "timing", Status: "ok", Samples: []protocol.Sample{
			{ElapsedNS: 9999999999, Operations: 1, Verified: true, Warmup: true, SampleType: "batch_average"},
			{ElapsedNS: 1_000_000_000, Operations: 1, Verified: true, SampleType: "batch_average"},
			{ElapsedNS: 1_000_000_000, Operations: 3, Verified: true, SampleType: "batch_average"},
		}})
	}
	return b
}

func TestThroughputIndependentLaunchRates(t *testing.T) {
	b := throughputFixture()
	// Unequal sample counts must not weight the process more heavily.
	b.Trials[2].Samples = b.Trials[2].Samples[:1]
	for i := 0; i < 1000; i++ {
		b.Trials[2].Samples = append(b.Trials[2].Samples, protocol.Sample{ElapsedNS: 1_000_000_000, Operations: 100, Verified: true, SampleType: "batch_average"})
	}
	before, _ := json.Marshal(b)
	s := Throughput(b)[0]
	if s.Median == nil || *s.Median != 20 || s.Rates[0] != 20 || s.Rates[2] != 1000 || s.Launches != 3 || s.Low == nil || s.High == nil || s.WorkUnit != "byte" || s.UnitsPerInvocation != "10" {
		t.Fatalf("%+v", s)
	}
	after, _ := json.Marshal(b)
	if string(before) != string(after) {
		t.Fatal("mutated evidence")
	}
}

func TestThroughputExclusions(t *testing.T) {
	for _, mode := range []string{"memory", "barriers", "check", "compile", "missing_units", "failed", "invalid", "unknown_type", "zero_time", "duplicate"} {
		t.Run(mode, func(t *testing.T) {
			b := throughputFixture()
			switch mode {
			case "memory":
				b.Manifest.Lock.Options.Profile = "memory"
			case "barriers":
				b.Manifest.Lock.Options.PhaseBarriers = true
			case "check":
				b.Manifest.Lock.Options.Check = true
			case "missing_units":
				b.Manifest.Lock.Workloads[0].Units = 0
			}
			for i := range b.Trials {
				switch mode {
				case "compile":
					b.Trials[i].Scenario = "compile"
				case "failed":
					b.Trials[i].Status = "incorrect"
				case "invalid":
					b.Trials[i].Samples[2].Verified = false
				case "unknown_type":
					b.Trials[i].Samples[2].SampleType = "unknown"
				case "zero_time":
					b.Trials[i].Samples[1].ElapsedNS = 0
					b.Trials[i].Samples[2].ElapsedNS = 0
				}
			}
			if mode == "duplicate" {
				b.Trials = append(b.Trials, b.Trials...)
			}
			s := Throughput(b)[0]
			if len(s.Evidence) != len(b.Trials) {
				t.Fatal("lost excluded evidence")
			}
			for _, e := range s.Evidence {
				if e.Rate != nil || e.WorkUnits != nil || e.ElapsedNS != nil || e.Operations != nil {
					t.Fatal("fabricated excluded evidence", e)
				}
			}
			if s.Median != nil || s.Low != nil || s.High != nil || s.Launches != 0 {
				t.Fatalf("fabricated throughput: %+v", s)
			}
		})
	}
}

func TestThroughputLargeIntegerTotalsAndShortRuns(t *testing.T) {
	b := throughputFixture()
	b.Trials = b.Trials[:1]
	b.Manifest.Lock.Workloads[0].Units = math.MaxUint64
	for i := 1; i < 3; i++ {
		b.Trials[0].Samples[i].ElapsedNS = math.MaxInt64
	}
	s := Throughput(b)[0]
	if s.Median == nil || math.Abs(*s.Median-4e9) > 1 || s.Low != nil || s.High != nil || s.UnitsPerInvocation != "18446744073709551615" {
		t.Fatalf("overflow or fabricated interval: %+v", s)
	}
	if _, err := json.Marshal(s); err != nil {
		t.Fatal(err)
	}
}
