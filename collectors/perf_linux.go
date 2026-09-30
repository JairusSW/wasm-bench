//go:build linux

package collectors

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

type linuxPerfHandle struct{ fd int }

// OpenPerfCgroupAll discovers the online effective cpuset, then revalidates it
// while opening. A changed set fails rather than silently dropping CPUs.
func OpenPerfCgroupAll(dir *os.File) (*PerfWindow, error) {
	if dir == nil {
		return nil, fmt.Errorf("perf requires an open cgroup v2 directory")
	}
	e, o, err := readPerfCPUState(dir)
	if err != nil {
		return nil, err
	}
	cpus, err := perfCoveredCPUs(e, o)
	if err != nil {
		return nil, err
	}
	return OpenPerfCgroup(dir, cpus)
}

func (h *linuxPerfHandle) Enable() error {
	return unix.IoctlSetInt(h.fd, unix.PERF_EVENT_IOC_ENABLE, 0)
}
func (h *linuxPerfHandle) Disable() error {
	return unix.IoctlSetInt(h.fd, unix.PERF_EVENT_IOC_DISABLE, 0)
}
func (h *linuxPerfHandle) Read(b []byte) (int, error) { return unix.Read(h.fd, b) }
func (h *linuxPerfHandle) Close() error               { return unix.Close(h.fd) }

// OpenPerfCgroup counts all threads in the supplied cgroup on each explicitly
// selected CPU. Selection must cover the online effective cpuset exactly.
// The caller must keep the cgroup directory open through Finish. Boundary checks
// detect changed CPU sets, but do not establish continuous topology stability.
// The caller retains ownership of dir; event fds are owned by the returned window.
func OpenPerfCgroup(dir *os.File, cpus []int) (*PerfWindow, error) {
	if dir == nil {
		return nil, fmt.Errorf("perf requires an open cgroup v2 directory")
	}
	var st unix.Statfs_t
	if err := unix.Fstatfs(int(dir.Fd()), &st); err != nil {
		return nil, err
	}
	if uint64(st.Type) != unix.CGROUP2_SUPER_MAGIC {
		return nil, fmt.Errorf("perf target is not cgroup v2")
	}
	info, err := dir.Stat()
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("perf target is not a directory")
	}
	effective, online, err := readPerfCPUState(dir)
	if err != nil {
		return nil, err
	}
	if err = validatePerfCPUs(cpus, effective, online); err != nil {
		return nil, err
	}
	w, err := newPerfWindow(cpus, func(event PerfEvent, cpu int) (perfHandle, error) {
		attr := unix.PerfEventAttr{Type: event.Type, Size: unix.PERF_ATTR_SIZE_VER0, Config: event.Config, Read_format: unix.PERF_FORMAT_TOTAL_TIME_ENABLED | unix.PERF_FORMAT_TOTAL_TIME_RUNNING, Bits: unix.PerfBitDisabled | unix.PerfBitExcludeHv}
		fd, err := unix.PerfEventOpen(&attr, int(dir.Fd()), cpu, -1, unix.PERF_FLAG_PID_CGROUP|unix.PERF_FLAG_FD_CLOEXEC)
		if err != nil {
			return nil, err
		}
		return &linuxPerfHandle{fd}, nil
	})
	if err != nil {
		return nil, err
	}
	w.checkCoverage = func() error {
		e, o, err := readPerfCPUState(dir)
		if err != nil {
			return err
		}
		if e != effective || o != online {
			return fmt.Errorf("effective cpuset or online CPU set changed since perf open")
		}
		return nil
	}
	return w, nil
}

func readPerfCPUState(dir *os.File) (effective, online string, err error) {
	fd, err := unix.Openat(int(dir.Fd()), "cpuset.cpus.effective", unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return "", "", fmt.Errorf("read perf effective cpuset: %w", err)
	}
	f := os.NewFile(uintptr(fd), "cpuset.cpus.effective")
	b, err := io.ReadAll(io.LimitReader(f, 65537))
	closeErr := f.Close()
	if err != nil {
		return "", "", err
	}
	if closeErr != nil {
		return "", "", closeErr
	}
	o, err := os.ReadFile("/sys/devices/system/cpu/online")
	if err != nil {
		return "", "", fmt.Errorf("read perf online CPUs: %w", err)
	}
	if _, err = perfCoveredCPUs(string(b), string(o)); err != nil {
		return "", "", err
	}
	return string(b), string(o), nil
}
