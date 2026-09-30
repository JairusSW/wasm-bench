//go:build darwin || linux

package agent

import (
	"os"
	"syscall"
)

// wait4 reports the maximum resident set for this child process over its whole
// lifetime. Darwin reports bytes; Linux reports KiB. It is not a phase peak.
func processPeakRSS(state *os.ProcessState) (float64, bool) {
	if state == nil {
		return 0, false
	}
	usage, ok := state.SysUsage().(*syscall.Rusage)
	if !ok || usage.Maxrss <= 0 {
		return 0, false
	}
	bytes := float64(usage.Maxrss)
	if peakRSSKilobytes {
		bytes *= 1024
	}
	return bytes, true
}
