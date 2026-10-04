package publish

import (
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
	"testing"
)

func TestSameTrialTimingRSSIsOnePeakAcrossThreeSamples(t *testing.T) {
	peak := protocol.Observation{Metric: "process.peak_rss", DefinitionVersion: 1, Unit: "bytes", Scope: "adapter_process", Phase: "compile/process_lifetime", Collector: "wait4_rusage", CollectorVersion: "1", Quality: "kernel_accounted_peak", Profile: "timing", Status: "available", Denominator: "process", Value: protocol.Value(123456)}
	b := experiment.Bundle{Manifest: experiment.Manifest{Lock: experiment.Lock{Options: experiment.Options{Profile: "timing", TimingPeakRSS: true}}}, Trials: []experiment.Trial{{ID: "compile-one", Runtime: "r", Workload: "w", Scenario: "compile", Profile: "timing", Block: 0, Status: "ok", Observations: []protocol.Observation{peak}, Samples: []protocol.Sample{{Verified: true}, {Verified: true}, {Verified: true}}}}}
	matched := map[string]bool{"r\x00w": true}
	rows := memoryStages(b, matched)
	if len(rows) != 1 || rows[0].Launches != 1 || *rows[0].Median != 123456 || rows[0].Trials[0] != "compile-one" {
		t.Fatalf("same-trial peak changed: %+v", rows)
	}
	for _, change := range []func(*experiment.Bundle){func(b *experiment.Bundle) { b.Manifest.Lock.Options.TimingPeakRSS = false }, func(b *experiment.Bundle) { b.Trials[0].Status = "timeout" }, func(b *experiment.Bundle) { b.Trials[0].Observations[0].Profile = "memory" }, func(b *experiment.Bundle) { b.Trials[0].Observations = append(b.Trials[0].Observations, peak) }} {
		copy := b
		copy.Trials = append([]experiment.Trial{}, b.Trials...)
		copy.Trials[0].Observations = append([]protocol.Observation{}, b.Trials[0].Observations...)
		change(&copy)
		if got := memoryStages(copy, matched); len(got) != 0 {
			t.Fatal("accepted invalid same-trial memory provenance")
		}
	}
}
