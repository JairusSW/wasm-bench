//go:build !linux

package agent

func probeIRQAffinityLive(p IRQAffinityProbe) IRQAffinityProbe {
	p.Status, p.Reason = "unsupported", "device IRQ affinity probes require Linux procfs"
	return p
}
