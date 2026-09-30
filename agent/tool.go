package agent

import (
	"errors"
	"fmt"
	"os/exec"
	"time"

	"github.com/wasmbench/wasmbench/protocol"
)

// ToolExecution describes one fresh tool cgroup, never an adapter phase or a
// complete multi-step build. The controller remains outside this cgroup.
type ToolExecution struct {
	WallNS       int64                  `json:"process_wall_ns"`
	Isolation    *Isolation             `json:"isolation"`
	Observations []protocol.Observation `json:"observations,omitempty"`
	OOM          bool                   `json:"oom_kill"`
	CleanupError string                 `json:"cleanup_error,omitempty"`
}

// RunTool applies requested limits before spawn, without unisolated fallback.
// Memory is read after Cmd.Run returns and before descendant cleanup. A fresh
// cgroup makes the lifetime peak meaningful without resetting memory.peak.
// Cleanup is outside the command wall timer and covers remaining descendants.
// cmd must be created with exec.CommandContext when requesting a cgroup;
// RunTool replaces its cancellation hook to kill the entire tool cgroup.
func RunTool(cmd *exec.Cmd, policy ResourcePolicy, memory bool) (ToolExecution, error) {
	var r ToolExecution
	if memory && policy.CgroupParent == "" {
		return r, fmt.Errorf("source memory collection requires a delegated cgroup")
	}
	g, err := prepareCgroup(cmd, policy)
	if err != nil {
		return r, err
	}
	r.Isolation = g.isolation()
	if g != nil {
		cmd.Cancel = func() error {
			if e := g.kill(); e != nil {
				return errors.Join(e, cmd.Process.Kill())
			}
			return nil
		}
	}
	start := time.Now()
	runErr := cmd.Run()
	r.WallNS = time.Since(start).Nanoseconds()
	r.Isolation.FinalVerification = g.verifyResources("tool_exit_before_cleanup")
	if v := r.Isolation.FinalVerification; v != nil {
		runErr = errors.Join(runErr, v.Err())
	}
	observations, oom := g.observations()
	r.OOM = oom
	if memory {
		for _, o := range observations {
			switch o.Metric {
			case "cgroup.memory.current":
				o.Metric = "source.build.cgroup.current"
			case "cgroup.memory.peak":
				o.Metric = "source.build.cgroup.peak"
			default:
				continue
			}
			o.Scope = "tool_step_cgroup"
			o.Phase = "tool_step/post_wait"
			o.Denominator = "tool_step_cgroup"
			r.Observations = append(r.Observations, o)
		}
	}
	killErr := g.kill()
	closeErr := g.close()
	if cleanupErr := errors.Join(killErr, closeErr); cleanupErr != nil {
		r.CleanupError = cleanupErr.Error()
	}
	if oom {
		runErr = errors.Join(runErr, fmt.Errorf("tool cgroup recorded an OOM kill"))
	}
	return r, errors.Join(runErr, killErr, closeErr)
}
