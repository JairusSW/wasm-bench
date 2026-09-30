//go:build linux

package collectors

import (
	"os"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestPerfLinuxEventDefinitions(t *testing.T) {
	want := []PerfEvent{
		{"cycles", unix.PERF_TYPE_HARDWARE, unix.PERF_COUNT_HW_CPU_CYCLES},
		{"instructions", unix.PERF_TYPE_HARDWARE, unix.PERF_COUNT_HW_INSTRUCTIONS},
		{"branches", unix.PERF_TYPE_HARDWARE, unix.PERF_COUNT_HW_BRANCH_INSTRUCTIONS},
		{"branch-misses", unix.PERF_TYPE_HARDWARE, unix.PERF_COUNT_HW_BRANCH_MISSES},
		{"page-faults", unix.PERF_TYPE_SOFTWARE, unix.PERF_COUNT_SW_PAGE_FAULTS},
		{"context-switches", unix.PERF_TYPE_SOFTWARE, unix.PERF_COUNT_SW_CONTEXT_SWITCHES},
		{"cpu-migrations", unix.PERF_TYPE_SOFTWARE, unix.PERF_COUNT_SW_CPU_MIGRATIONS},
	}
	for i, event := range PerfEvents() {
		if event != want[i] {
			t.Fatal(event, want[i])
		}
	}
}

func TestPerfLinuxRejectsNonCgroup(t *testing.T) {
	if _, err := OpenPerfCgroup(nil, []int{0}); err == nil {
		t.Fatal("nil target")
	}
	dir, err := os.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	if _, err := OpenPerfCgroup(dir, []int{0}); err == nil {
		t.Fatal("ordinary directory accepted")
	}
}

// This opt-in kernel smoke test neither requests elevated privileges nor moves
// processes/changes host settings. A denied syscall is valid unavailable evidence,
// not a successful hardware-count qualification.
func TestPerfLinuxKernelSmoke(t *testing.T) {
	if os.Getenv("WASMBENCH_PERF_KERNEL_TEST") != "1" {
		t.Skip("set WASMBENCH_PERF_KERNEL_TEST=1 on a Linux cgroup-v2 host")
	}
	dir, err := os.Open("/sys/fs/cgroup")
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	effective, online, err := readPerfCPUState(dir)
	if err != nil {
		t.Fatal(err)
	}
	cpus, err := perfCoveredCPUs(effective, online)
	if err != nil {
		t.Fatal(err)
	}
	w, err := OpenPerfCgroup(dir, cpus)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if err = w.Start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	readings, err := w.Finish()
	if err != nil {
		t.Fatal(err)
	}
	if len(readings) != 7*len(cpus) {
		t.Fatal(readings)
	}
	for _, r := range readings {
		t.Logf("%s CPU %d: %s (%s)", r.Event.Name, r.CPU, r.Status, r.Reason)
		switch r.Status {
		case "available", "multiplexed", "not_running":
			if r.Count == nil || r.EnabledNS == nil || r.RunningNS == nil {
				t.Fatal(r)
			}
		case "permission_denied", "unavailable":
			if r.Count != nil || r.Reason == "" {
				t.Fatal(r)
			}
		default:
			t.Fatal(r)
		}
	}
}
