package experiment

import (
	"github.com/wasmbench/wasmbench/protocol"
	"testing"
)

func TestTimingPeakRSSRequiresTrialProvenance(t *testing.T) {
	peak := protocol.Observation{Metric: "process.peak_rss", DefinitionVersion: 1, Unit: "bytes", Scope: "adapter_process", Phase: "compile/process_lifetime", Collector: "wait4_rusage", CollectorVersion: "1", Quality: "kernel_accounted_peak", Profile: "timing", Status: "available", Denominator: "process", Value: protocol.Value(123456)}
	b := Bundle{Manifest: Manifest{Lock: Lock{Options: Options{Profile: "timing", TimingPeakRSS: true}}}, Trials: []Trial{{ID: "one", Scenario: "compile", Profile: "timing", Status: "ok", Block: 0, Observations: []protocol.Observation{peak}}}}
	if err := ValidateTimingPeakRSS(b); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*Bundle){func(b *Bundle) { b.Trials[0].Observations = nil }, func(b *Bundle) { b.Trials[0].Observations[0].Phase = "instantiate/process_lifetime" }, func(b *Bundle) { b.Trials[0].Observations[0].Profile = "memory" }, func(b *Bundle) { b.Trials[0].Observations = append(b.Trials[0].Observations, peak) }} {
		copy := b
		copy.Trials = append([]Trial{}, b.Trials...)
		copy.Trials[0].Observations = append([]protocol.Observation{}, b.Trials[0].Observations...)
		change(&copy)
		if err := ValidateTimingPeakRSS(copy); err == nil {
			t.Fatal("accepted altered or missing peak")
		}
	}
}
