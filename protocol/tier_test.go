package protocol

import (
	"strings"
	"testing"
)

func tierFixture() (Workload, Sample) {
	w := Workload{SHA256: strings.Repeat("a", 64), ABI: "core", Reset: "stateless", Export: "run", Oracle: Oracle{Kind: "exact_u64", Expected: Values{7}}}
	s := Sample{Index: 0, Operations: 1, SampleType: "individual_operation", ElapsedNS: 10, Verified: true, Result: Values{7}}
	s.TierWindow = &TierWindow{Version: 1, ModuleSHA256: w.SHA256, Export: w.Export, Collector: "V8/testing-code-tier-intrinsics", CollectorVersion: "test-v8", Scope: "exported_entry_code_nonatomic_boundary_snapshots", Quality: "engine_reported", Invocation: 1, Before: TierReading{StartNS: 0, EndNS: 2, State: "uncompiled"}, OperationStartNS: 3, OperationEndNS: 13, After: TierReading{StartNS: 14, EndNS: 15, State: "liftoff"}}
	return w, s
}

func TestTierWindowValidation(t *testing.T) {
	w, s := tierFixture()
	if err := s.TierWindow.Validate(w, s); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*TierWindow){
		"module":           func(x *TierWindow) { x.ModuleSHA256 = strings.Repeat("b", 64) },
		"export":           func(x *TierWindow) { x.Export = "other" },
		"scope":            func(x *TierWindow) { x.Scope = "executed_tier" },
		"version":          func(x *TierWindow) { x.Version = 3 },
		"collector":        func(x *TierWindow) { x.Collector = "inferred" },
		"quality":          func(x *TierWindow) { x.Quality = "exact" },
		"invocation":       func(x *TierWindow) { x.Invocation = 0 },
		"negative":         func(x *TierWindow) { x.Before.StartNS = -1 },
		"reversed":         func(x *TierWindow) { x.After.EndNS = 13 },
		"overlap":          func(x *TierWindow) { x.Before.EndNS = 4 },
		"elapsed":          func(x *TierWindow) { x.OperationEndNS = 12 },
		"state":            func(x *TierWindow) { x.After.State = "fully_materialized" },
		"reason":           func(x *TierWindow) { x.After.State = "unavailable" },
		"available_reason": func(x *TierWindow) { x.After.Reason = "contradictory" },
	} {
		t.Run(name, func(t *testing.T) {
			w, s := tierFixture()
			change(s.TierWindow)
			if s.TierWindow.Validate(w, s) == nil {
				t.Fatal("accepted malformed reading")
			}
		})
	}
	s.TierWindow.After = TierReading{StartNS: 14, EndNS: 16, State: "unavailable", Reason: "non-atomic queries disagreed"}
	if err := s.TierWindow.Validate(w, s); err != nil {
		t.Fatal("lost unavailable evidence", err)
	}
}

func TestTierRunRequiresDedicatedProfile(t *testing.T) {
	w, _ := tierFixture()
	p := Preparation{Workload: w, Profile: "profiling"}
	r := RunRequest{Scenario: "trajectory", Samples: 3, Warmup: 2, Operations: 1}
	if err := ValidateTierRun(&p, &r); err != nil {
		t.Fatal(err)
	}
	for _, profile := range []string{"timing", "memory", "code", "counters"} {
		p.Profile = profile
		if ValidateTierRun(&p, &r) == nil {
			t.Fatal("accepted", profile)
		}
	}
	p.Profile = "profiling"
	r.PhaseBarriers = true
	if ValidateTierRun(&p, &r) == nil {
		t.Fatal("accepted barriers")
	}
}

func TestTierInvocationFailureValidation(t *testing.T) {
	for _, outcome := range []string{"returned", "oracle_mismatch", "guest_trap"} {
		w, s := tierFixture()
		s.TierWindow.Version = 2
		s.TierWindow.InvocationOutcome = outcome
		if outcome != "returned" {
			s.Verified = false
			s.TierWindow.FailureReason = "failure"
			if outcome == "guest_trap" {
				s.Result = nil
			}
		}
		if err := s.TierWindow.Validate(w, s); err != nil {
			t.Fatal(outcome, err)
		}
		s.Verified = !s.Verified
		if s.TierWindow.Validate(w, s) == nil {
			t.Fatal("accepted contradictory verification", outcome)
		}
	}
	w, s := tierFixture()
	s.TierWindow.Version = 2
	if s.TierWindow.Validate(w, s) == nil {
		t.Fatal("missing v2 outcome")
	}
	s.TierWindow.Version = 1
	s.TierWindow.InvocationOutcome = "returned"
	if s.TierWindow.Validate(w, s) == nil {
		t.Fatal("v1 declaration contains v2 outcome")
	}
	s.TierWindow.Version = 2
	s.TierWindow.InvocationOutcome = "oracle_mismatch"
	s.TierWindow.FailureReason = "mismatch"
	s.Verified = false
	s.Result = Values{1 << 32}
	if s.TierWindow.Validate(w, s) == nil {
		t.Fatal("accepted out-of-range i32 bits")
	}
}
