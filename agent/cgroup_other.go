//go:build !linux

package agent

import (
	"fmt"
	"github.com/wasmbench/wasmbench/protocol"
	"os/exec"
)

type cgroup struct{}

func (*cgroup) verifyResources(string) *ResourceVerification { return nil }

func prepareCgroup(_ *exec.Cmd, p ResourcePolicy) (*cgroup, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if p.CgroupParent != "" {
		return nil, fmt.Errorf("cgroup isolation requires Linux")
	}
	return nil, nil
}
func (*cgroup) kill() error                                  { return nil }
func (*cgroup) close() error                                 { return nil }
func (*cgroup) isolation() *Isolation                        { return &Isolation{Mode: "uncontrolled"} }
func (*cgroup) observations() ([]protocol.Observation, bool) { return nil, false }

func (*cgroup) phaseMemory(stage string) []protocol.Observation {
	return unavailablePhaseCollection(stage, "Linux cgroup v2 required")
}
