//go:build !darwin

package collectors

import "github.com/wasmbench/wasmbench/protocol"

func Snapshot(pid int, phase string) []protocol.Observation {
	return snapshotProcfs(pid, phase)
}
