//go:build linux

package agent

import (
	"fmt"
	"github.com/wasmbench/wasmbench/collectors"
)

func (g *cgroup) openPerf() (counterWindow, error) {
	if g == nil || g.fd == nil {
		return nil, fmt.Errorf("perf counters require an isolated cgroup from process start")
	}
	return collectors.OpenPerfCgroupAll(g.fd)
}
