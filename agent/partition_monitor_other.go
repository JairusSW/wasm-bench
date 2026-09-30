//go:build !linux

package agent

import "time"

func ProbeActiveCPUPartition(parent, worker, cpus string, pid int) ActivePartitionSample {
	return ActivePartitionSample{Version: ActivePartitionVersion, Scope: ActivePartitionScope, At: time.Now().UTC(), Parent: parent, Worker: worker, CPUs: cpus, PID: pid, Facts: map[string]HostFact{}, Status: "unsupported", Reason: "occupied cgroup CPU partition samples require Linux"}
}
