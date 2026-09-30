package experiment

import (
	"fmt"
	"github.com/wasmbench/wasmbench/protocol"
	"strings"
)

func ValidateTierEvidence(b Bundle) error {
	for _, t := range b.Trials {
		var w protocol.Workload
		for _, candidate := range b.Manifest.Lock.Workloads {
			if candidate.ID == t.Workload {
				w = candidate
				break
			}
		}
		required := false
		version := ""
		for _, r := range b.Manifest.Lock.Runtimes {
			if r.ID == t.Runtime && r.Description != nil {
				required = r.Description.Capabilities["can_profile_tier_trajectory"]
				version = r.Description.Version
			}
		}
		if err := validateTierTrial(w, version, required, b.Manifest.Lock.Options, t); err != nil {
			return fmt.Errorf("trial %s: %w", t.ID, err)
		}
	}
	return nil
}

// Only structurally validated tier records belong in Samples. Untrusted adapter
// payloads stay in AdapterSamples for raw inspection and never enter plots.
func validateTierTrial(w protocol.Workload, version string, required bool, o Options, t Trial) error {
	active := required && t.Profile == "profiling" && t.Block >= 0
	hasWindow := false
	for _, s := range t.Samples {
		hasWindow = hasWindow || s.TierWindow != nil
	}
	if hasWindow && !active {
		return fmt.Errorf("tier observation outside declared diagnostic trial")
	}
	if !active || (t.Status != "ok" && len(t.Samples) == 0) {
		return nil
	}
	request := protocol.RunRequest{Scenario: t.Scenario, Samples: o.Samples, Warmup: o.Warmup, Operations: o.Operations, PhaseBarriers: o.PhaseBarriers}
	if o.Profile != t.Profile {
		return fmt.Errorf("tier profile differs from locked request")
	}
	if err := protocol.ValidateTierRun(&protocol.Preparation{Workload: w, Profile: t.Profile}, &request); err != nil {
		return err
	}
	if t.Status == "ok" {
		if err := validateSampleSequence(request, t.Samples); err != nil {
			return err
		}
	}
	if len(t.Samples) > o.Samples+o.Warmup {
		return fmt.Errorf("tier prefix exceeds locked invocation budget")
	}
	var previous int64
	for i, s := range t.Samples {
		x := s.TierWindow
		if x == nil {
			return fmt.Errorf("omitted tier observation")
		}
		if err := x.Validate(w, s); err != nil {
			return err
		}
		if x.CollectorVersion != version || s.Index != i || s.Warmup != (i < o.Warmup) {
			return fmt.Errorf("tier collector version or sequence differs from lock")
		}
		matches := len(s.Result) == len(w.Oracle.Expected)
		if matches {
			for j, v := range s.Result {
				matches = matches && v == w.Oracle.Expected[j]
			}
		}
		switch x.InvocationOutcome {
		case "", "returned":
			if !s.Verified || !matches {
				return fmt.Errorf("incorrect tier trajectory result")
			}
		case "oracle_mismatch", "guest_trap":
			if s.Verified || matches || i != len(t.Samples)-1 || t.Status == "ok" || !strings.Contains(t.Reason, x.FailureReason) {
				return fmt.Errorf("tier failure is not the terminal failed invocation")
			}
			if (x.InvocationOutcome == "oracle_mismatch" && t.Status != "incorrect_result") || (x.InvocationOutcome == "guest_trap" && t.Status != "error") {
				return fmt.Errorf("tier failure differs from trial outcome")
			}
		}
		if x.Before.StartNS < previous {
			return fmt.Errorf("tier observation clocks overlap across samples")
		}
		previous = x.After.EndNS
	}
	return nil
}
