//go:build linux

package agent

import (
	"golang.org/x/sys/unix"
	"os"
)

func probeCPUPartitionLive(p CPUPartitionProbe) CPUPartitionProbe {
	var st unix.Statfs_t
	if err := unix.Statfs(p.Path, &st); err != nil {
		p.Status = "unavailable"
		p.Reason = "cannot inspect partition filesystem: " + err.Error()
		return p
	}
	if uint64(st.Type) != unix.CGROUP2_SUPER_MAGIC {
		p.Status = "invalid_request"
		p.Reason = "partition must reside on a cgroup v2 filesystem"
		return p
	}
	return readCPUPartition(os.DirFS("/"), p)
}
