package experiment

import (
	"fmt"
	"github.com/wasmbench/wasmbench/agent"
)

func validateIRQPolicy(l Lock) error {
	if !l.RequireIRQAffinity {
		return nil
	}
	return agent.ValidateIRQAffinityPolicy(l.Options.Resources)
}

func ValidateIRQAffinityEvidence(m Manifest) error {
	if !m.Lock.RequireIRQAffinity {
		if m.IRQAffinityStart != nil || m.IRQAffinityEnd != nil || m.Publication == "prohibited_irq_affinity_mismatch" {
			return fmt.Errorf("IRQ evidence without locked requirement")
		}
		return nil
	}
	if err := validateIRQPolicy(m.Lock); err != nil {
		return err
	}
	if m.IRQAffinityStart == nil || m.IRQAffinityEnd == nil {
		return fmt.Errorf("IRQ affinity requires both run boundaries")
	}
	for _, p := range []*agent.IRQAffinityProbe{m.IRQAffinityStart, m.IRQAffinityEnd} {
		if p.CPUs != m.Lock.Options.Resources.CPUs {
			return fmt.Errorf("IRQ observation differs from locked CPUs")
		}
		if err := agent.ValidateIRQAffinityProbe(*p); err != nil {
			return err
		}
	}
	if err := m.IRQAffinityStart.Err(); err != nil {
		return fmt.Errorf("IRQ start was not ready: %w", err)
	}
	if m.IRQAffinityEnd.At.Before(m.IRQAffinityStart.At) {
		return fmt.Errorf("IRQ boundary timestamps reversed")
	}
	if m.IRQAffinityEnd.Err() != nil {
		switch m.Publication {
		case "prohibited_irq_affinity_mismatch", "prohibited_cpu_partition_mismatch", "prohibited_cpu_partition_sample_mismatch", "prohibited_host_baseline_mismatch":
		default:
			return fmt.Errorf("failed IRQ boundary requires prohibited publication status")
		}
	} else if m.Publication == "prohibited_irq_affinity_mismatch" {
		return fmt.Errorf("IRQ prohibition contradicts boundary evidence")
	}
	return nil
}

func irqAffinityAllowsMeasurements(m Manifest) bool {
	return ValidateIRQAffinityEvidence(m) == nil && (!m.Lock.RequireIRQAffinity || m.IRQAffinityEnd.Err() == nil)
}
