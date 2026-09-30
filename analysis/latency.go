package analysis

import "github.com/wasmbench/wasmbench/experiment"

const LatencyPolicyVersion = "timing-pass-eligibility-v1"

// LatencyPolicy separates eligibility from execution success. Instrumented
// timings remain raw evidence, but never become headline latency estimates.
type LatencyPolicy struct {
	Version string `json:"version"`
	Profile string `json:"locked_profile"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
}

func HeadlineLatencyPolicy(m experiment.Manifest) LatencyPolicy {
	p := LatencyPolicy{Version: LatencyPolicyVersion, Profile: m.Lock.Options.Profile, Status: "timing_pass", Reason: "Minimally instrumented timing pass; only successful, verified, non-warmup samples with valid operation counts contribute."}
	switch {
	case m.Kind != "measurement" || m.Lock.Options.Check:
		p.Status, p.Reason = "not_measurement", "Correctness-only or unspecified run kinds do not establish performance measurements."
	case m.Lock.Options.Profile != "timing":
		p.Status, p.Reason = "not_timing_pass", "Diagnostic pass: raw timers are retained, but headline latency estimates and performance comparisons are withheld."
	case m.Lock.Options.PhaseBarriers:
		p.Status, p.Reason = "phase_instrumented", "Phase-handshake instrumentation is not a minimally instrumented timing pass."
	case !experiment.HostBaselineAllowsMeasurements(m):
		p.Status, p.Reason = "host_policy_mismatch", "Locked host baseline or CPU partition requirements did not pass at both boundaries."
	}
	return p
}

func (p LatencyPolicy) ForProfile(profile string) LatencyPolicy {
	if profile != p.Profile {
		p.Status, p.Reason = "profile_mismatch", "Trial profile differs from the locked collection profile; its timers are not headline latency evidence."
	}
	return p
}

func (p LatencyPolicy) ForScenario(scenario string) LatencyPolicy {
	if scenario == "harness-calibration" && p.Status == "timing_pass" {
		p.Status, p.Reason = "calibration_only", "Empty adapter-local harness iterations are diagnostic evidence, not workload latency; no automatic subtraction or performance scoring."
	}
	return p
}
