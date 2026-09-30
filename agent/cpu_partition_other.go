//go:build !linux

package agent

func probeCPUPartitionLive(p CPUPartitionProbe) CPUPartitionProbe {
	p.Status = "unsupported"
	p.Reason = "isolated cgroup CPU partitions require Linux"
	return p
}
