//go:build linux

package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestToolCgroupLifecycle(t *testing.T) {
	parent := os.Getenv("WASMBENCH_TEST_CGROUP_PARENT")
	if parent == "" {
		t.Skip("delegated cgroup required")
	}
	for _, tc := range []struct {
		name, script string
		limit        uint64
		timeout      time.Duration
		wantErr, oom bool
	}{
		{"memory", "cat /proc/self/cgroup; dd if=/dev/zero of=/dev/null bs=8M count=1", 128 << 20, 5 * time.Second, false, false},
		{"timeout", "sleep 30 & wait", 128 << 20, 150 * time.Millisecond, true, false},
		{"oom", "dd if=/dev/zero of=/dev/null bs=64M count=1", 16 << 20, 5 * time.Second, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), tc.timeout)
			defer cancel()
			cmd := exec.CommandContext(ctx, "/bin/sh", "-c", tc.script)
			cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			var output strings.Builder
			cmd.Stdout, cmd.Stderr = &output, &output
			cmd.WaitDelay = time.Second
			r, err := RunTool(cmd, ResourcePolicy{CgroupParent: parent, MemoryMaxBytes: tc.limit, DisableSwap: true, CPUQuotaUS: 100000, PidsMax: 32}, true)
			if (err != nil) != tc.wantErr || r.OOM != tc.oom || r.CleanupError != "" {
				t.Fatalf("execution=%+v err=%v output=%s", r, err, output.String())
			}
			if !cmd.SysProcAttr.Setpgid || r.Isolation == nil || r.Isolation.Mode != "cgroup_v2_at_spawn" || r.WallNS <= 0 {
				t.Fatal("missing spawn isolation", r)
			}
			if r.Isolation.Verification == nil || r.Isolation.Verification.Status != "verified" || r.Isolation.FinalVerification == nil || r.Isolation.FinalVerification.Status != "verified" || r.Isolation.FinalVerification.Stage != "tool_exit_before_cleanup" {
				t.Fatal("missing boundary readback", r.Isolation)
			}
			if _, err := os.Stat(r.Isolation.Path); !os.IsNotExist(err) {
				t.Fatalf("tool cgroup remains: %v", err)
			}
			if tc.name == "memory" && !strings.Contains(output.String(), filepath.Base(r.Isolation.Path)) {
				t.Fatal("tool did not observe its cgroup", output.String())
			}
			if len(r.Observations) != 2 {
				t.Fatal(r.Observations)
			}
			for _, o := range r.Observations {
				if o.Status != "available" || o.Value == nil || o.Scope != "tool_step_cgroup" || o.Phase != "tool_step/post_wait" {
					t.Fatal(o)
				}
				if tc.name == "memory" && o.Metric == "source.build.cgroup.peak" && *o.Value < 8<<20 {
					t.Fatal("allocation missing from peak", o)
				}
			}
		})
	}
}
