package agent

import (
	"context"
	"os/exec"
	"testing"
)

func TestToolFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		policy ResourcePolicy
		memory bool
	}{
		{ResourcePolicy{}, true},
		{ResourcePolicy{CgroupParent: t.TempDir()}, false},
		{ResourcePolicy{MemoryMaxBytes: 1}, false},
	} {
		cmd := exec.CommandContext(context.Background(), "must-not-launch")
		r, err := RunTool(cmd, tc.policy, tc.memory)
		if err == nil || cmd.Process != nil || r.WallNS != 0 {
			t.Fatalf("invalid policy launched a tool: %+v, %v", r, err)
		}
	}
}
