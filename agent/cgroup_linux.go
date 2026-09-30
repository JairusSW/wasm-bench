//go:build linux

package agent

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/wasmbench/wasmbench/protocol"
)

type cgroup struct {
	policy       ResourcePolicy
	verification ResourceVerification
	dir          string
	fd           *os.File
	effective    map[string]string
	peakFD       *os.File
	peakReason   string
	cpuBefore    map[string]uint64
	cpuReason    string
}

func prepareCgroup(cmd *exec.Cmd, p ResourcePolicy) (*cgroup, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if p.CgroupParent == "" {
		return nil, nil
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(p.CgroupParent, &stat); err != nil {
		return nil, err
	}
	if uint64(stat.Type) != 0x63677270 {
		return nil, fmt.Errorf("cgroup parent is not a cgroup v2 filesystem")
	}
	dir, err := os.MkdirTemp(p.CgroupParent, "wasmbench-")
	if err != nil {
		return nil, err
	}
	g := &cgroup{dir: dir, policy: p, effective: map[string]string{}}
	g.effective["interpretation"] = "local controller settings; ancestor limits still apply; cpus are not exclusive"
	ok := false
	defer func() {
		if !ok {
			g.close()
		}
	}()
	write := func(name, value string) error { return os.WriteFile(filepath.Join(dir, name), []byte(value), 0600) }
	// Require cgroup.kill before starting: timeout cleanup must cover descendants.
	if _, err = os.Stat(filepath.Join(dir, "cgroup.kill")); err != nil {
		return nil, fmt.Errorf("cgroup.kill required: %w", err)
	}
	settings := map[string]string{}
	if p.DisableSwap {
		settings["memory.swap.max"] = "0"
	}
	if p.MemoryMaxBytes > 0 {
		settings["memory.max"] = strconv.FormatUint(p.MemoryMaxBytes, 10)
		settings["memory.oom.group"] = "1"
	}
	if p.CPUQuotaUS > 0 {
		settings["cpu.max"] = fmt.Sprintf("%d 100000", p.CPUQuotaUS)
	}
	if p.PidsMax > 0 {
		settings["pids.max"] = strconv.FormatUint(p.PidsMax, 10)
	}
	if p.CPUs != "" {
		settings["cpuset.cpus"] = p.CPUs
	}
	if p.Mems != "" {
		settings["cpuset.mems"] = p.Mems
	}
	for name, value := range settings {
		if err = write(name, value); err != nil {
			return nil, fmt.Errorf("configure %s: %w", name, err)
		}
	}
	for name, value := range readResourceSettings(dir) {
		g.effective[name] = value
	}
	g.verification = CheckResourceReadback(p, g.effective, "before_spawn")
	if err = g.verification.Err(); err != nil {
		return nil, err
	}
	g.fd, err = os.Open(dir)
	if err != nil {
		return nil, err
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.UseCgroupFD = true
	cmd.SysProcAttr.CgroupFD = int(g.fd.Fd())
	ok = true
	return g, nil
}

func readResourceSettings(dir string) map[string]string {
	effective := map[string]string{}
	for _, name := range []string{"memory.max", "memory.swap.max", "memory.oom.group", "cpu.max", "cpuset.cpus", "cpuset.cpus.effective", "cpuset.mems", "cpuset.mems.effective", "pids.max"} {
		b, e := os.ReadFile(filepath.Join(dir, name))
		if e == nil {
			effective[name] = strings.TrimSpace(string(b))
		} else {
			effective[name] = "unavailable: " + e.Error()
		}
	}
	return effective
}

func (g *cgroup) verifyResources(stage string) *ResourceVerification {
	if g == nil {
		return nil
	}
	v := CheckResourceReadback(g.policy, readResourceSettings(g.dir), stage)
	return &v
}

func (g *cgroup) kill() error {
	if g == nil {
		return nil
	}
	return os.WriteFile(filepath.Join(g.dir, "cgroup.kill"), []byte("1"), 0600)
}
func (g *cgroup) close() error {
	if g == nil {
		return nil
	}
	var errs []error
	if g.fd != nil {
		errs = append(errs, g.fd.Close())
	}
	if g.peakFD != nil {
		errs = append(errs, g.peakFD.Close())
	}
	// Only remove our freshly created leaf, never a parent or recursive tree.
	var removeErr error
	for attempt := 0; attempt < 100; attempt++ {
		removeErr = os.Remove(g.dir)
		if !errors.Is(removeErr, syscall.EBUSY) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	errs = append(errs, removeErr)
	return errors.Join(errs...)
}
func (g *cgroup) isolation() *Isolation {
	if g == nil {
		return &Isolation{Mode: "uncontrolled"}
	}
	return &Isolation{Mode: "cgroup_v2_at_spawn", Path: g.dir, Effective: g.effective, Verification: &g.verification}
}
func (g *cgroup) observations() ([]protocol.Observation, bool) {
	if g == nil {
		return nil, false
	}
	var out []protocol.Observation
	for _, spec := range []struct{ file, metric, quality string }{{"memory.current", "cgroup.memory.current", "boundary_snapshot_only"}, {"memory.peak", "cgroup.memory.peak", "kernel_accounted_peak"}} {
		o := protocol.Observation{Metric: spec.metric, DefinitionVersion: 1, Unit: "bytes", Scope: "adapter_cgroup", Phase: "process_lifetime/response_end", Collector: "cgroup_v2", CollectorVersion: "1", Quality: spec.quality, Profile: "memory", Denominator: "cgroup", Status: "unavailable"}
		b, err := os.ReadFile(filepath.Join(g.dir, spec.file))
		if err == nil {
			var n uint64
			n, err = strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
			if err == nil {
				o.Value = protocol.Value(float64(n))
				o.Status = "available"
			}
		}
		if err != nil {
			o.Reason = err.Error()
		}
		out = append(out, o)
	}
	oom := false
	if b, err := os.ReadFile(filepath.Join(g.dir, "memory.events")); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 2 && fields[0] == "oom_kill" {
				n, _ := strconv.ParseUint(fields[1], 10, 64)
				oom = n > 0
			}
		}
	}
	return out, oom
}

func readPeakFD(f *os.File) (uint64, error) {
	if _, err := f.Seek(0, 0); err != nil {
		return 0, err
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
}

func (g *cgroup) armPeak() error {
	if g.peakFD != nil {
		g.peakFD.Close()
		g.peakFD = nil
	}
	f, err := os.OpenFile(filepath.Join(g.dir, "memory.peak"), os.O_RDWR, 0)
	if err != nil {
		return err
	}
	if _, err = f.Write([]byte("1")); err != nil {
		f.Close()
		return err
	}
	if _, err = readPeakFD(f); err != nil {
		f.Close()
		return err
	}
	g.peakFD = f
	return nil
}

func (g *cgroup) phaseMemory(stage string) []protocol.Observation {
	scenario, start, end := phaseWindow(stage)
	if g == nil {
		return unavailablePhaseCollection(stage, "adapter not isolated in a cgroup")
	}
	if start {
		g.peakReason = ""
		if err := g.armPeak(); err != nil {
			g.peakReason = err.Error()
		}
	}
	all, _ := g.observations()
	var out []protocol.Observation
	for _, o := range all {
		if o.Metric == "cgroup.memory.current" {
			o.Phase = scenario + "/" + stage
			out = append(out, o)
		}
	}
	values, err := g.readAccounting("memory.stat")
	reason := ""
	if err != nil {
		reason = err.Error()
	}
	out = append(out, accountingObservations(memoryAccountingMetrics, nil, values, false, scenario+"/"+stage, "cgroup_v2_memory.stat", reason)...)
	if end {
		o := phasePeakObservation(g.peakReason)
		o.Phase = scenario + "/barrier_window"
		if g.peakFD != nil {
			if n, err := readPeakFD(g.peakFD); err == nil {
				o.Value = protocol.Value(float64(n))
				o.Status = "available"
				o.Reason = ""
			} else {
				o.Reason = err.Error()
			}
		}
		out = append(out, o)
		after, err := g.readAccounting("cpu.stat")
		reason := g.cpuReason
		if err != nil {
			reason = err.Error()
		}
		out = append(out, accountingObservations(cpuAccountingMetrics, g.cpuBefore, after, true, scenario+"/barrier_window", "cgroup_v2_cpu.stat", reason)...)
	}
	if start {
		g.cpuBefore, err = g.readAccounting("cpu.stat")
		g.cpuReason = ""
		if err != nil {
			g.cpuReason = err.Error()
		}
	}
	return out
}

func (g *cgroup) readAccounting(name string) (map[string]uint64, error) {
	b, err := os.ReadFile(filepath.Join(g.dir, name))
	if err != nil {
		return nil, err
	}
	return parseAccounting(b)
}
