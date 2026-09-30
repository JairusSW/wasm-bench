package experiment

import (
	"fmt"
	"github.com/wasmbench/wasmbench/agent"
)

func ValidateCPUPartitionEvidence(m Manifest) error {
	if !m.Lock.RequireIsolatedCPUPartition {
		if m.CPUPartitionStart != nil || m.CPUPartitionEnd != nil || m.Publication == "prohibited_cpu_partition_mismatch" || m.Publication == "prohibited_cpu_partition_sample_mismatch" {
			return fmt.Errorf("CPU partition evidence without locked requirement")
		}
		return nil
	}
	resources := m.Lock.Options.Resources
	if err := agent.ValidateCPUPartitionPolicy(resources); err != nil {
		return err
	}
	if m.CPUPartitionStart == nil || m.CPUPartitionEnd == nil {
		return fmt.Errorf("CPU partition requires both run-boundary observations")
	}
	for _, p := range []*agent.CPUPartitionProbe{m.CPUPartitionStart, m.CPUPartitionEnd} {
		if p.Path != resources.CgroupParent || p.CPUs != resources.CPUs {
			return fmt.Errorf("CPU partition observation differs from locked resource policy")
		}
		if err := agent.ValidateCPUPartitionProbe(*p); err != nil {
			return err
		}
	}
	if err := m.CPUPartitionStart.Err(); err != nil {
		return fmt.Errorf("CPU partition start was not ready: %w", err)
	}
	if m.CPUPartitionEnd.Err() != nil {
		if m.Publication != "prohibited_cpu_partition_mismatch" && m.Publication != "prohibited_host_baseline_mismatch" {
			return fmt.Errorf("CPU partition end failure requires prohibited publication status")
		}
	} else if m.Publication == "prohibited_cpu_partition_mismatch" {
		return fmt.Errorf("CPU partition prohibition contradicts boundary evidence")
	}
	return nil
}

func cpuPartitionAllowsMeasurements(m Manifest) bool {
	return ValidateCPUPartitionEvidence(m) == nil && m.Publication != "prohibited_cpu_partition_sample_mismatch" && (!m.Lock.RequireIsolatedCPUPartition || m.CPUPartitionEnd.Err() == nil)
}

// ValidatePartitionTrialEvidence checks optional in-trial sampled receipts.
// Legacy bundles without this contract remain readable but cannot satisfy the
// publication audit's sampled-partition requirement.
func ValidatePartitionTrialEvidence(b Bundle) error {
	monitored := false
	failed := false
	for _, t := range b.Trials {
		if t.PartitionMonitor == nil {
			continue
		}
		monitored = true
		if !b.Manifest.Lock.RequireIsolatedCPUPartition || t.Isolation == nil || t.Isolation.Mode != "cgroup_v2_at_spawn" {
			return fmt.Errorf("trial %s has partition monitor without locked cgroup isolation", t.ID)
		}
		if err := agent.ValidatePartitionMonitor(t.PartitionMonitor); err != nil {
			return fmt.Errorf("trial %s: %w", t.ID, err)
		}
		for _, sample := range t.PartitionMonitor.Samples {
			if sample.Parent != b.Manifest.Lock.Options.Resources.CgroupParent || sample.CPUs != b.Manifest.Lock.Options.Resources.CPUs || sample.Worker != t.Isolation.Path {
				return fmt.Errorf("trial %s active partition sample differs from locked policy or worker", t.ID)
			}
		}
		failed = failed || t.PartitionMonitor.Status != "ready_at_samples"
	}
	if monitored {
		for _, t := range b.Trials {
			if t.Isolation != nil && t.Isolation.Mode == "cgroup_v2_at_spawn" && t.PartitionMonitor == nil {
				return fmt.Errorf("trial %s omits active partition samples", t.ID)
			}
		}
	}
	if failed && b.Manifest.Publication != "prohibited_cpu_partition_sample_mismatch" && b.Manifest.Publication != "prohibited_cpu_partition_mismatch" && b.Manifest.Publication != "prohibited_host_baseline_mismatch" {
		return fmt.Errorf("failed active partition samples require prohibited publication status")
	}
	if !failed && b.Manifest.Publication == "prohibited_cpu_partition_sample_mismatch" {
		return fmt.Errorf("active partition sample prohibition contradicts trial evidence")
	}
	return nil
}
