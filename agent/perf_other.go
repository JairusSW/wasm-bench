//go:build !linux

package agent

import "fmt"

func (*cgroup) openPerf() (counterWindow, error) {
	return nil, fmt.Errorf("perf cgroup counters require Linux")
}
