package experiment

import (
	"github.com/wasmbench/wasmbench/protocol"
	"strings"
	"testing"
)

func tierBundleFixture() Bundle {
	w := protocol.Workload{ID: "w", SHA256: strings.Repeat("a", 64), ABI: "core", Reset: "stateless", Export: "run", Oracle: protocol.Oracle{Kind: "exact_u64", Expected: protocol.Values{7}}}
	b := Bundle{Manifest: Manifest{Lock: Lock{Workloads: []protocol.Workload{w}, Options: Options{Profile: "profiling", Samples: 1, Warmup: 1, Operations: 1}, Runtimes: []Runtime{{ID: "r", Description: &protocol.Description{Version: "test-v8", Capabilities: map[string]bool{"can_profile_tier_trajectory": true}}}}}}, Trials: []Trial{{Runtime: "r", Workload: "w", Scenario: "trajectory", Profile: "profiling", Status: "ok"}}}
	for i := 0; i < 2; i++ {
		start := int64(i * 20)
		b.Trials[0].Samples = append(b.Trials[0].Samples, protocol.Sample{Index: i, Warmup: i == 0, Operations: 1, SampleType: "individual_operation", ElapsedNS: 10, Verified: true, Result: protocol.Values{7}, TierWindow: &protocol.TierWindow{Version: 1, ModuleSHA256: w.SHA256, Export: w.Export, Collector: "V8/testing-code-tier-intrinsics", CollectorVersion: "test-v8", Scope: "exported_entry_code_nonatomic_boundary_snapshots", Quality: "engine_reported", Invocation: i + 1, Before: protocol.TierReading{StartNS: start, EndNS: start + 2, State: "liftoff"}, OperationStartNS: start + 3, OperationEndNS: start + 13, After: protocol.TierReading{StartNS: start + 14, EndNS: start + 15, State: "liftoff"}}})
	}
	return b
}

func TestTierEvidenceBoundToLock(t *testing.T) {
	if err := ValidateTierEvidence(tierBundleFixture()); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*Bundle){
		"missing":    func(b *Bundle) { b.Trials[0].Samples[0].TierWindow = nil },
		"empty":      func(b *Bundle) { b.Trials[0].Samples = nil },
		"count":      func(b *Bundle) { b.Manifest.Lock.Options.Samples = 2 },
		"profile":    func(b *Bundle) { b.Manifest.Lock.Options.Profile = "timing" },
		"scope":      func(b *Bundle) { b.Trials[0].Profile = "timing" },
		"version":    func(b *Bundle) { b.Trials[0].Samples[0].TierWindow.CollectorVersion = "different" },
		"warmup":     func(b *Bundle) { b.Trials[0].Samples[0].Warmup = false },
		"oracle":     func(b *Bundle) { b.Trials[0].Samples[0].Result[0] = 8 },
		"unverified": func(b *Bundle) { b.Trials[0].Samples[0].Verified = false },
		"capability": func(b *Bundle) {
			b.Manifest.Lock.Runtimes[0].Description.Capabilities["can_profile_tier_trajectory"] = false
		},
		"ordering": func(b *Bundle) { b.Trials[0].Samples[1].TierWindow.Before.StartNS = 0 },
		"workload": func(b *Bundle) { b.Manifest.Lock.Workloads[0].Reset = "fresh_instance" },
	} {
		t.Run(name, func(t *testing.T) {
			b := tierBundleFixture()
			change(&b)
			if ValidateTierEvidence(b) == nil {
				t.Fatal("accepted forged trace")
			}
		})
	}
	b := tierBundleFixture()
	b.Trials[0].Samples = nil
	b.Trials[0].Status = "unsupported"
	if err := ValidateTierEvidence(b); err != nil {
		t.Fatal("unsupported is not a missing successful trace", err)
	}
}

func tierFailureFixture(outcome string) Bundle {
	b := tierBundleFixture()
	trial := &b.Trials[0]
	for i := range trial.Samples {
		trial.Samples[i].TierWindow.Version = 2
		trial.Samples[i].TierWindow.InvocationOutcome = "returned"
	}
	s := &trial.Samples[1]
	s.Verified = false
	s.TierWindow.InvocationOutcome = outcome
	s.TierWindow.FailureReason = "failed call"
	trial.Reason = "error: failed call"
	if outcome == "guest_trap" {
		s.Result = nil
		trial.Status = "error"
	} else {
		s.Result = protocol.Values{8}
		trial.Status = "incorrect_result"
	}
	return b
}

func TestTierFailurePrefixValidation(t *testing.T) {
	for _, outcome := range []string{"guest_trap", "oracle_mismatch"} {
		b := tierFailureFixture(outcome)
		if err := ValidateTierEvidence(b); err != nil {
			t.Fatal(outcome, err)
		}
		for name, change := range map[string]func(*Bundle){
			"successful-trial": func(b *Bundle) { b.Trials[0].Status = "ok" },
			"verified-failure": func(b *Bundle) { b.Trials[0].Samples[1].Verified = true },
			"missing-reason":   func(b *Bundle) { b.Trials[0].Samples[1].TierWindow.FailureReason = "" },
			"unbound-reason":   func(b *Bundle) { b.Trials[0].Reason = "different failure" },
			"more-calls":       func(b *Bundle) { b.Trials[0].Samples = append(b.Trials[0].Samples, b.Trials[0].Samples[0]) },
			"false-outcome":    func(b *Bundle) { b.Trials[0].Samples[1].TierWindow.InvocationOutcome = "returned" },
			"false-legacy":     func(b *Bundle) { b.Trials[0].Samples[1].TierWindow.Version = 1 },
			"not-terminal":     func(b *Bundle) { b.Trials[0].Samples[0] = b.Trials[0].Samples[1] },
		} {
			t.Run(outcome+"/"+name, func(t *testing.T) {
				b := tierFailureFixture(outcome)
				change(&b)
				if ValidateTierEvidence(b) == nil {
					t.Fatal("forged failure accepted")
				}
			})
		}
	}
}
