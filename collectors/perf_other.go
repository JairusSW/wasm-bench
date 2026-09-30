//go:build !linux

package collectors

import (
	"fmt"
	"os"
)

func OpenPerfCgroup(_ *os.File, _ []int) (*PerfWindow, error) {
	return nil, fmt.Errorf("perf cgroup counters require Linux")
}

func OpenPerfCgroupAll(_ *os.File) (*PerfWindow, error) {
	return nil, fmt.Errorf("perf cgroup counters require Linux")
}
