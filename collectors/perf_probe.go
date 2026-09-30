package collectors

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// PerfProbe checks descriptor creation only. It never enables events, moves
// tasks, creates cgroups or changes host permissions. Open success is not proof
// that events can be scheduled or that useful workload counts can be obtained.
type PerfProbe struct {
	Version        string        `json:"version"`
	Status         string        `json:"status"`
	Reason         string        `json:"reason,omitempty"`
	Target         string        `json:"target,omitempty"`
	Paranoid       *string       `json:"perf_event_paranoid"`
	Interpretation string        `json:"interpretation"`
	Events         []PerfReading `json:"events"`
}

func ProbePerfCgroup(path string) PerfProbe {
	p := PerfProbe{Version: PerfVersion, Status: "not_requested", Target: path, Interpretation: "Disabled descriptor-open probe only; no events enabled or counted. Open success does not establish scheduling, useful counts, isolated workload placement, or publication readiness. perf_event_paranoid is context, not a permission verdict."}
	if runtime.GOOS != "linux" {
		p.Status = "unsupported"
		p.Reason = "perf cgroup counters require Linux"
		return p
	}
	if b, err := os.ReadFile("/proc/sys/kernel/perf_event_paranoid"); err == nil {
		v := strings.TrimSpace(string(b))
		p.Paranoid = &v
	}
	if path == "" {
		p.Reason = "pass --perf-cgroup with an existing cgroup v2 directory to probe disabled event opens"
		return p
	}
	if !filepath.IsAbs(path) {
		p.Status = "invalid_target"
		p.Reason = "perf probe requires an absolute cgroup path"
		return p
	}
	dir, err := os.Open(path)
	if err != nil {
		p.Status = perfErrorStatus(err)
		p.Reason = err.Error()
		return p
	}
	defer dir.Close()
	return probePerfWindow(p, func() (*PerfWindow, error) { return OpenPerfCgroupAll(dir) })
}

func probePerfWindow(p PerfProbe, open func() (*PerfWindow, error)) PerfProbe {
	w, err := open()
	if err != nil {
		p.Status = perfErrorStatus(err)
		p.Reason = err.Error()
		return p
	}
	opened := 0
	for _, entry := range w.entries {
		r := entry.reading
		if entry.handle != nil && r.Status == "not_started" {
			r.Status = "open_succeeded"
			opened++
		}
		p.Events = append(p.Events, r)
	}
	switch {
	case opened == len(p.Events) && opened > 0:
		p.Status = "open_succeeded"
	case opened > 0:
		p.Status = "partial_open"
	default:
		p.Status = "unavailable"
	}
	if err = w.Close(); err != nil {
		p.Status = "cleanup_error"
		p.Reason = err.Error()
	}
	return p
}
