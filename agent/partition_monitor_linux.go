//go:build linux

package agent

import (
	"os"
	"time"

	"golang.org/x/sys/unix"
)

func ProbeActiveCPUPartition(parent, worker, cpus string, pid int) ActivePartitionSample {
	at := time.Now().UTC()
	s := ActivePartitionSample{Version: ActivePartitionVersion, Scope: ActivePartitionScope, At: at, Parent: parent, Worker: worker, CPUs: cpus, PID: pid, Facts: map[string]HostFact{}}
	if err := activePartitionRequest(parent, worker, cpus, pid); err != nil {
		s.Status, s.Reason = "invalid_request", err.Error()
		return s
	}
	var st unix.Statfs_t
	if err := unix.Statfs(parent, &st); err != nil {
		s.Status, s.Reason = "unavailable", "cannot inspect partition filesystem: "+err.Error()
		return s
	}
	if uint64(st.Type) != unix.CGROUP2_SUPER_MAGIC {
		s.Status, s.Reason = "invalid_request", "partition must reside on a cgroup v2 filesystem"
		return s
	}
	return readActivePartition(os.DirFS("/"), parent, worker, cpus, pid, at)
}
