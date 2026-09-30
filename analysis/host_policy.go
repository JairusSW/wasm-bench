package analysis

import "github.com/wasmbench/wasmbench/experiment"

// Apply host eligibility only to derived views. The sealed raw trial status
// continues to describe execution correctness, with its exact samples intact.
func hostEligibleBundle(b experiment.Bundle) experiment.Bundle {
	if experiment.HostBaselineAllowsMeasurements(b.Manifest) {
		return b
	}
	b.Trials = append([]experiment.Trial(nil), b.Trials...)
	for i := range b.Trials {
		b.Trials[i].Status = "host_policy_mismatch"
		b.Trials[i].Reason = "locked host baseline or CPU partition requirement did not pass at both run boundaries"
	}
	return b
}
