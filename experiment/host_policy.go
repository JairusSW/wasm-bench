package experiment

import (
	"fmt"
	"github.com/wasmbench/wasmbench/agent"
	"reflect"
)

// MatchHostMeasurementPolicy compares requested controls, not observation
// timestamps or labels. Matching policy does not establish readiness; each
// bundle must independently validate its evidence before yielding estimates.
func MatchHostMeasurementPolicy(a, b Lock) error {
	if !reflect.DeepEqual(a.Options.Resources, b.Options.Resources) {
		return fmt.Errorf("resource policies differ")
	}
	if !reflect.DeepEqual(a.HostPolicy, b.HostPolicy) {
		return fmt.Errorf("locked host baselines differ")
	}
	if a.RequireIsolatedCPUPartition != b.RequireIsolatedCPUPartition {
		return fmt.Errorf("isolated CPU partition requirements differ")
	}
	if a.RequireIRQAffinity != b.RequireIRQAffinity {
		return fmt.Errorf("IRQ affinity requirements differ")
	}
	return nil
}

func ValidateHostEvidence(m Manifest) error {
	if err := ValidateIRQAffinityEvidence(m); err != nil {
		return err
	}
	if err := ValidateCPUPartitionEvidence(m); err != nil {
		return err
	}
	if m.Lock.HostPolicy == nil {
		if m.Publication == "prohibited_host_baseline_mismatch" {
			return fmt.Errorf("host baseline prohibition without a locked baseline")
		}
		if m.HostEnd != nil || m.HostStartCheck != nil || m.HostEndCheck != nil {
			return fmt.Errorf("host baseline evidence without a policy")
		}
		return nil
	}
	if err := m.Lock.HostPolicy.Validate(); err != nil {
		return err
	}
	if m.HostEnd == nil || m.HostStartCheck == nil || m.HostEndCheck == nil {
		return fmt.Errorf("host baseline requires both boundary observations")
	}
	start := agent.CheckHostPolicy(m.Lock.HostPolicy, m.Host, "before_run_preparation")
	end := agent.CheckHostPolicy(m.Lock.HostPolicy, *m.HostEnd, "after_trials_before_seal")
	if start.Err() != nil || !reflect.DeepEqual(start, *m.HostStartCheck) || !reflect.DeepEqual(end, *m.HostEndCheck) {
		return fmt.Errorf("inconsistent host baseline evidence")
	}
	if end.Err() != nil && m.Publication != "prohibited_host_baseline_mismatch" {
		return fmt.Errorf("host mismatch publication status missing")
	}
	if end.Err() == nil && m.Publication == "prohibited_host_baseline_mismatch" {
		return fmt.Errorf("host mismatch publication status contradicts boundary evidence")
	}
	return nil
}

func HostBaselineAllowsMeasurements(m Manifest) bool {
	return ValidateHostEvidence(m) == nil && irqAffinityAllowsMeasurements(m) && cpuPartitionAllowsMeasurements(m) && (m.Lock.HostPolicy == nil || m.HostEndCheck.Status == "matched_observed_baseline")
}
