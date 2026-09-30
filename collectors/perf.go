package collectors

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
)

const PerfVersion = "linux-perf-cgroup-v1"

// PerfEvent is a Linux generic perf event, not a vendor-specific raw PMU event.
// Counts include user and kernel work; hypervisor execution is excluded.
type PerfEvent struct {
	Name   string `json:"name"`
	Type   uint32 `json:"type"`
	Config uint64 `json:"config"`
}

func PerfEvents() []PerfEvent {
	return []PerfEvent{{"cycles", 0, 0}, {"instructions", 0, 1}, {"branches", 0, 4}, {"branch-misses", 0, 5}, {"page-faults", 1, 2}, {"context-switches", 1, 3}, {"cpu-migrations", 1, 4}}
}

// PerfReading preserves integer evidence, including unscaled multiplexed counts.
// CPU records must not be treated as independent statistical replications. Their
// sequential enable/disable boundaries are not one atomic cross-CPU snapshot.
type PerfReading struct {
	CollectorVersion string    `json:"collector_version"`
	Scope            string    `json:"scope"`
	PrivilegeScope   string    `json:"privilege_scope"`
	Event            PerfEvent `json:"event"`
	CPU              int       `json:"cpu"`
	Count            *uint64   `json:"raw_count"`
	EnabledNS        *uint64   `json:"time_enabled_ns"`
	RunningNS        *uint64   `json:"time_running_ns"`
	Status           string    `json:"status"`
	Reason           string    `json:"reason,omitempty"`
}

type perfHandle interface {
	Enable() error
	Disable() error
	Read([]byte) (int, error)
	Close() error
}

type perfEntry struct {
	reading PerfReading
	handle  perfHandle
}

// PerfWindow is single-use and not concurrency-safe. Call Start while the
// workload is blocked; call Finish at the ending barrier, or Close on failure.
// Opening counters is separate from enabling them. Each event uses its own fd,
// so unsupported events do not suppress the other event definitions.
type PerfWindow struct {
	entries         []perfEntry
	started, closed bool
	checkCoverage   func() error
}

func newPerfWindow(cpus []int, open func(PerfEvent, int) (perfHandle, error)) (*PerfWindow, error) {
	if len(cpus) == 0 || len(cpus) > 4096 {
		return nil, fmt.Errorf("perf needs 1..4096 explicit CPUs")
	}
	cpus = append([]int(nil), cpus...)
	sort.Ints(cpus)
	for i, cpu := range cpus {
		if cpu < 0 || (i > 0 && cpu == cpus[i-1]) {
			return nil, fmt.Errorf("invalid or duplicate perf CPU")
		}
	}
	w := &PerfWindow{}
	for _, event := range PerfEvents() {
		for _, cpu := range cpus {
			h, err := open(event, cpu)
			r := PerfReading{CollectorVersion: PerfVersion, Scope: "selected_cgroup_threads_on_recorded_cpu", PrivilegeScope: "user_and_kernel_excluding_hypervisor", Event: event, CPU: cpu, Status: "not_started"}
			if err != nil {
				r.Status = perfErrorStatus(err)
				r.Reason = err.Error()
			}
			w.entries = append(w.entries, perfEntry{r, h})
		}
	}
	return w, nil
}

func perfErrorStatus(err error) string {
	if errors.Is(err, os.ErrPermission) {
		return "permission_denied"
	}
	return "unavailable"
}

func (w *PerfWindow) Start() error {
	if w.closed || w.started {
		return fmt.Errorf("perf window already started or closed")
	}
	if w.checkCoverage != nil {
		if err := w.checkCoverage(); err != nil {
			_ = w.Close()
			return err
		}
	}
	w.started = true
	for i := range w.entries {
		e := &w.entries[i]
		if e.handle == nil {
			continue
		}
		if err := e.handle.Enable(); err != nil {
			e.reading.Status = perfErrorStatus(err)
			e.reading.Reason = "enable: " + err.Error()
			_ = e.handle.Close()
			e.handle = nil
		} else {
			e.reading.Status = "enabled"
		}
	}
	return nil
}

func decodePerfReading(data []byte, order binary.ByteOrder) (count, enabled, running uint64, err error) {
	if len(data) != 24 {
		return 0, 0, 0, fmt.Errorf("perf read: expected 24 bytes, got %d", len(data))
	}
	count = order.Uint64(data[:8])
	enabled = order.Uint64(data[8:16])
	running = order.Uint64(data[16:])
	if running > enabled || (running == 0 && count != 0) {
		return 0, 0, 0, fmt.Errorf("inconsistent perf count/running/enabled accounting")
	}
	return
}

func (w *PerfWindow) Finish() ([]PerfReading, error) {
	if w.closed || !w.started {
		return nil, fmt.Errorf("perf window not started or already closed")
	}
	// Disable all events before reading any of them.
	for i := range w.entries {
		e := &w.entries[i]
		if e.handle == nil {
			continue
		}
		if err := e.handle.Disable(); err != nil {
			e.reading.Status = perfErrorStatus(err)
			e.reading.Reason = "disable: " + err.Error()
		}
	}
	out := make([]PerfReading, 0, len(w.entries))
	for i := range w.entries {
		e := &w.entries[i]
		if e.handle != nil && e.reading.Status == "enabled" {
			var buf [24]byte
			n, err := e.handle.Read(buf[:])
			if err == nil && n != len(buf) {
				err = io.ErrUnexpectedEOF
			}
			if err != nil {
				e.reading.Status = perfErrorStatus(err)
				e.reading.Reason = "read: " + err.Error()
			} else {
				count, enabled, running, err := decodePerfReading(buf[:], binary.NativeEndian)
				if err != nil {
					e.reading.Status = "invalid_reading"
					e.reading.Reason = err.Error()
				} else {
					e.reading.Count = &count
					e.reading.EnabledNS = &enabled
					e.reading.RunningNS = &running
					switch {
					case running == 0:
						e.reading.Status = "not_running"
					case running < enabled:
						e.reading.Status = "multiplexed"
					default:
						e.reading.Status = "available"
					}
				}
			}
		}
		out = append(out, e.reading)
	}
	var coverageErr error
	if w.checkCoverage != nil {
		coverageErr = w.checkCoverage()
	}
	if coverageErr != nil {
		for i := range out {
			previous := out[i].Status
			if out[i].Reason != "" {
				previous += ": " + out[i].Reason
			}
			out[i].Status = "coverage_changed"
			out[i].Reason = "CPU coverage invalidated: " + coverageErr.Error() + "; event result: " + previous
		}
	}
	return out, errors.Join(coverageErr, w.Close())
}

func (w *PerfWindow) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	var errs []error
	for i := range w.entries {
		if h := w.entries[i].handle; h != nil {
			errs = append(errs, h.Close())
			w.entries[i].handle = nil
		}
	}
	return errors.Join(errs...)
}
