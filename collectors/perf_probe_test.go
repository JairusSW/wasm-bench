package collectors

import (
	"os"
	"runtime"
	"testing"
)

type failingProbeClose struct{ fakePerf }

func (h *failingProbeClose) Close() error { h.fakePerf.Close(); return os.ErrInvalid }

func TestPerfProbeCloseFailure(t *testing.T) {
	var handles []*failingProbeClose
	p := probePerfWindow(PerfProbe{}, func() (*PerfWindow, error) {
		return newPerfWindow([]int{0}, func(PerfEvent, int) (perfHandle, error) {
			h := &failingProbeClose{}
			handles = append(handles, h)
			return h, nil
		})
	})
	if p.Status != "cleanup_error" || p.Reason == "" {
		t.Fatal(p)
	}
	for _, h := range handles {
		if h.closed != 1 || h.enabled != 0 {
			t.Fatal(h)
		}
	}
}

func TestPerfProbeNeverEnablesCounters(t *testing.T) {
	for _, mode := range []string{"all", "partial", "denied"} {
		var handles []*fakePerf
		got := probePerfWindow(PerfProbe{Version: PerfVersion}, func() (*PerfWindow, error) {
			return newPerfWindow([]int{0, 1}, func(event PerfEvent, cpu int) (perfHandle, error) {
				if mode == "denied" || (mode == "partial" && event.Name == "cycles") {
					return nil, os.ErrPermission
				}
				h := &fakePerf{}
				handles = append(handles, h)
				return h, nil
			})
		})
		want := map[string]string{"all": "open_succeeded", "partial": "partial_open", "denied": "unavailable"}[mode]
		if got.Status != want || len(got.Events) != 14 {
			t.Fatal(got)
		}
		for _, h := range handles {
			if h.enabled != 0 || h.disabled != 0 || h.closed != 1 {
				t.Fatal("probe enabled or leaked counter", h)
			}
		}
		for _, r := range got.Events {
			if r.Count != nil || r.EnabledNS != nil || r.RunningNS != nil || (r.Status != "open_succeeded" && r.Status != "permission_denied") {
				t.Fatal(r)
			}
		}
	}
	got := probePerfWindow(PerfProbe{}, func() (*PerfWindow, error) { return nil, os.ErrPermission })
	if got.Status != "permission_denied" || got.Reason == "" || len(got.Events) != 0 {
		t.Fatal(got)
	}
}

func TestPerfProbeDefaultDoesNotAssumeAvailability(t *testing.T) {
	p := ProbePerfCgroup("")
	want := "unsupported"
	if runtime.GOOS == "linux" {
		want = "not_requested"
	}
	if p.Status != want || p.Interpretation == "" || len(p.Events) != 0 {
		t.Fatal(p)
	}
	if runtime.GOOS == "linux" {
		p = ProbePerfCgroup("relative")
		if p.Status != "invalid_target" {
			t.Fatal(p)
		}
	}
}

func TestPerfProbeKernel(t *testing.T) {
	if runtime.GOOS != "linux" || os.Getenv("WASMBENCH_PERF_KERNEL_TEST") != "1" {
		t.Skip("opt-in Linux probe")
	}
	p := ProbePerfCgroup("/sys/fs/cgroup")
	if p.Status != "open_succeeded" && p.Status != "partial_open" && p.Status != "unavailable" {
		t.Fatal(p)
	}
	if len(p.Events) == 0 {
		t.Fatal("expected event-open evidence", p)
	}
	for _, r := range p.Events {
		if r.Count != nil || r.EnabledNS != nil || r.RunningNS != nil {
			t.Fatal("probe counted", r)
		}
	}
	t.Logf("disabled-open probe: %s; %d event/CPU records", p.Status, len(p.Events))
}
