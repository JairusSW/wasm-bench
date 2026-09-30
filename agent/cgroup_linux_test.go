//go:build linux

package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestCgroupCPUPhaseIncludesChildWork(t *testing.T) {
	parent := os.Getenv("WASMBENCH_TEST_CGROUP_PARENT")
	if parent == "" {
		t.Skip("delegated cgroup required")
	}
	cmd := exec.Command("/bin/sh", "-c", "dd if=/dev/zero of=/dev/null bs=1M count=200 & wait")
	g, err := prepareCgroup(cmd, ResourcePolicy{CgroupParent: parent, MemoryMaxBytes: 128 << 20, CPUQuotaUS: 100000, DisableSwap: true})
	if err != nil {
		t.Fatal(err)
	}
	defer g.close()
	g.phaseMemory("before_compile")
	if err = cmd.Run(); err != nil {
		t.Fatal(err)
	}
	observations := g.phaseMemory("compiled")
	cpu, breakdown := 0, 0
	for _, o := range observations {
		if strings.HasPrefix(o.Metric, "time.cpu.") {
			cpu++
			if o.Status != "available" || o.Value == nil {
				t.Fatal(o)
			}
			if o.Metric == "time.cpu.total" && *o.Value <= 0 {
				t.Fatal("child CPU was not accounted", o)
			}
		}
		if o.Collector == "cgroup_v2_memory.stat" {
			breakdown++
			if o.Status != "available" || o.Value == nil {
				t.Fatal(o)
			}
		}
	}
	if cpu != 3 || breakdown != 6 {
		t.Fatal(observations)
	}
}

func TestCgroupPeakResetUsesSameDescriptor(t *testing.T) {
	parent := os.Getenv("WASMBENCH_TEST_CGROUP_PARENT")
	if parent == "" {
		t.Skip("delegated cgroup required")
	}
	cmd := exec.Command("dd", "if=/dev/zero", "of=/dev/null", "bs=32M", "count=1")
	g, err := prepareCgroup(cmd, ResourcePolicy{CgroupParent: parent, MemoryMaxBytes: 128 << 20, DisableSwap: true})
	if err != nil {
		t.Fatal(err)
	}
	defer g.close()
	if err = cmd.Run(); err != nil {
		t.Fatal(err)
	}
	global, err := os.Open(filepath.Join(g.dir, "memory.peak"))
	if err != nil {
		t.Fatal(err)
	}
	defer global.Close()
	historical, err := readPeakFD(global)
	if err != nil {
		t.Fatal(err)
	}
	if historical < 32<<20 {
		t.Fatalf("allocation probe did not establish a historical peak: %d", historical)
	}
	if err = g.armPeak(); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command("dd", "if=/dev/zero", "of=/dev/null", "bs=1M", "count=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{UseCgroupFD: true, CgroupFD: int(g.fd.Fd())}
	if err = cmd.Run(); err != nil {
		t.Fatal(err)
	}
	reset, err := readPeakFD(g.peakFD)
	if err != nil {
		t.Fatal(err)
	}
	other, err := readPeakFD(global)
	if err != nil {
		t.Fatal(err)
	}
	if reset >= historical || other < historical {
		t.Fatalf("same-FD reset semantics failed: old=%d reset=%d other=%d", historical, reset, other)
	}
}

func TestCgroupDeadlineKillsDescendants(t *testing.T) {
	parent := os.Getenv("WASMBENCH_TEST_CGROUP_PARENT")
	if parent == "" {
		t.Skip("delegated cgroup required")
	}
	start := time.Now()
	c, err := StartIsolated(context.Background(), []string{"/bin/sh", "-c", "sleep 20 & wait"}, filepath.Join(t.TempDir(), "log"), 100*time.Millisecond, ResourcePolicy{CgroupParent: parent})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if c.out.Scan() {
		t.Fatal("unexpected output")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("descendant kept pipe open after deadline")
	}
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestCgroupOOMHelper(t *testing.T) {
	if os.Getenv("WASMBENCH_OOM_HELPER") != "1" {
		t.Skip("subprocess helper")
	}
	b := make([]byte, 512<<20)
	for i := 0; i < len(b); i += 4096 {
		b[i] = 1
	}
	runtime.KeepAlive(b)
}

func TestCgroupOOMIsAccounted(t *testing.T) {
	parent := os.Getenv("WASMBENCH_TEST_CGROUP_PARENT")
	if parent == "" {
		t.Skip("delegated cgroup required")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	c, err := StartIsolated(context.Background(), []string{"/usr/bin/env", "WASMBENCH_OOM_HELPER=1", exe, "-test.run=^TestCgroupOOMHelper$"}, filepath.Join(t.TempDir(), "log"), 5*time.Second, ResourcePolicy{CgroupParent: parent, MemoryMaxBytes: 64 << 20, DisableSwap: true})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for c.out.Scan() {
	}
	_, oom := c.ResourceObservations()
	if !oom {
		t.Fatal("allocation did not produce a cgroup OOM event")
	}
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestCgroupSpawnAndCleanup(t *testing.T) {
	parent := os.Getenv("WASMBENCH_TEST_CGROUP_PARENT")
	if parent == "" {
		t.Skip("set WASMBENCH_TEST_CGROUP_PARENT to a delegated cgroup v2 directory")
	}
	policy := ResourcePolicy{CgroupParent: parent, MemoryMaxBytes: 128 << 20, CPUQuotaUS: 100000, PidsMax: 64}
	allowed, err := os.ReadFile(filepath.Join(parent, "cpuset.cpus.effective"))
	if err != nil {
		t.Fatal(err)
	}
	policy.CPUs = strings.FieldsFunc(strings.TrimSpace(string(allowed)), func(r rune) bool { return r == ',' || r == '-' })[0]
	c, err := StartIsolated(context.Background(), []string{"/bin/sh", "-c", "cat /proc/self/cgroup; sleep 20"}, filepath.Join(t.TempDir(), "log"), 2*time.Second, policy)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	isolation := c.Isolation()
	if isolation.Effective["cpuset.cpus.effective"] != policy.CPUs {
		t.Fatal("CPU selection was not applied", isolation)
	}
	if isolation.Mode != "cgroup_v2_at_spawn" || isolation.Effective["memory.max"] != "134217728" || isolation.Effective["cpu.max"] != "100000 100000" || isolation.Effective["pids.max"] != "64" {
		t.Fatal(isolation)
	}
	if !c.out.Scan() || !strings.Contains(c.out.Text(), filepath.Base(isolation.Path)) {
		t.Fatalf("child not born in isolated cgroup: %s", c.out.Text())
	}
	self, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(self), filepath.Base(isolation.Path)) {
		t.Fatal("controller is in adapter cgroup")
	}
	observations, oom := c.ResourceObservations()
	if oom || len(observations) != 2 {
		t.Fatal(observations, oom)
	}
	for _, o := range observations {
		if o.Status != "available" || o.Value == nil {
			t.Fatal(o)
		}
	}
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(isolation.Path); !os.IsNotExist(err) {
		t.Fatalf("cgroup was not removed: %v", err)
	}
}
