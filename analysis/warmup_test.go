package analysis

import (
	"testing"

	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

func warmupTrial(values ...int64) experiment.Trial {
	t := experiment.Trial{ID: "trial", Status: "ok", Profile: "timing", Scenario: "steady"}
	for i, v := range values {
		t.Samples = append(t.Samples, protocol.Sample{Index: i, ElapsedNS: v, Operations: 1, Verified: true, SampleType: "batch_average"})
	}
	return t
}

func TestWarmupDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name   string
		values []int64
		want   string
	}{
		{"short", []int64{100, 100}, "insufficient_samples"},
		{"flat", []int64{100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100}, "no_drift_detected"},
		{"slowing", []int64{100, 100, 100, 100, 100, 200, 200, 200, 200, 200, 300, 300, 300, 300, 300}, "drift_detected"},
		{"speeding", []int64{300, 300, 300, 300, 300, 200, 200, 200, 200, 200, 100, 100, 100, 100, 100}, "drift_detected"},
		{"returns", []int64{100, 100, 100, 100, 100, 200, 200, 200, 200, 200, 100, 100, 100, 100, 100}, "drift_detected"},
		{"variable", []int64{50, 75, 100, 125, 150, 50, 75, 100, 125, 150, 50, 75, 100, 125, 150}, "high_variability"},
		{"zero", make([]int64, 15), "unresolved_timer_resolution"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := diagnoseWarmup(warmupTrial(tc.values...))
			if d.Status != tc.want {
				t.Fatalf("%+v", d)
			}
		})
	}
}

func TestWarmupMissingAndInvalidEvidence(t *testing.T) {
	for _, mode := range []string{"failed", "unverified", "negative", "zero_operations", "gap", "mixed_operations", "mixed_type", "warmup_after_measurement", "profile", "scenario"} {
		t.Run(mode, func(t *testing.T) {
			trial := warmupTrial(100, 100, 100)
			want := "unavailable"
			switch mode {
			case "failed":
				trial.Status = "timeout"
			case "unverified":
				trial.Samples[1].Verified = false
			case "negative":
				trial.Samples[1].ElapsedNS = -1
			case "zero_operations":
				trial.Samples[1].Operations = 0
			case "gap":
				trial.Samples[1].Index = 3
			case "mixed_operations":
				trial.Samples[1].Operations = 2
			case "mixed_type":
				trial.Samples[1].SampleType = "individual"
			case "warmup_after_measurement":
				trial.Samples[1].Warmup = true
			case "profile":
				trial.Profile = "memory"
				want = "not_applicable"
			case "scenario":
				trial.Scenario = "compile"
				want = "not_applicable"
			}
			d := diagnoseWarmup(trial)
			if d.Status != want || d.RelativeMAD != nil || d.RelativeMedianSpread != nil {
				t.Fatalf("%+v", d)
			}
		})
	}
}

func TestWarmupSummaryDoesNotCherryPick(t *testing.T) {
	b := experiment.Bundle{}
	for block := 0; block < 6; block++ {
		trial := warmupTrial(9999, 100, 100, 100, 100, 100, 200, 200, 200, 200, 200, 300, 300, 300, 300, 300)
		trial.Block = block
		trial.Samples[0].Warmup = true
		b.Trials = append(b.Trials, trial)
	}
	s := Summarize(timingFixture(b))[0]
	if s.Stability != "unstable_within_launch" || *s.Median != 200 || s.Samples != 90 || len(s.WarmupDiagnostics) != 6 {
		t.Fatalf("%+v", s)
	}
	d := s.WarmupDiagnostics[0]
	if d.WarmupSamples != 1 || d.MeasuredSamples != 15 || *d.RelativeMedianSpread != 1 {
		t.Fatalf("%+v", d)
	}
	if b.Trials[0].Samples[0].ElapsedNS != 9999 {
		t.Fatal("modified evidence")
	}
}
