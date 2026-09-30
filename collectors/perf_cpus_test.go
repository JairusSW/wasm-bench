package collectors

import (
	"fmt"
	"slices"
	"testing"
)

func TestPerfCPUList(t *testing.T) {
	got, err := ParseCPUList(" 4,0-2,8-9\n")
	if err != nil || !slices.Equal(got, []int{0, 1, 2, 4, 8, 9}) {
		t.Fatal(got, err)
	}
	for _, s := range []string{"", "0,", "-1", "+1", "0 -2", "1-0", "0-1,1", "0,0", "0-1-2", "0-4096", "999999999999999999999"} {
		if _, err := ParseCPUList(s); err == nil {
			t.Fatal("accepted", s)
		}
	}
	if got, err := ParseCPUList("0-4095"); err != nil || len(got) != 4096 {
		t.Fatal(len(got), err)
	}
}

func TestPerfCPUExactCoverage(t *testing.T) {
	if err := validatePerfCPUs([]int{3, 1}, "1-3", "0-1,3"); err != nil {
		t.Fatal(err)
	}
	for _, cpus := range [][]int{nil, {1}, {1, 2, 3}, {1, 3, 3}, {-1, 1, 3}} {
		if err := validatePerfCPUs(cpus, "1-3", "0-1,3"); err == nil {
			t.Fatal(cpus)
		}
	}
	for _, tc := range [][2]string{{"", "0"}, {"0", ""}, {"0", "1"}, {"0-2", "bad"}} {
		if _, err := perfCoveredCPUs(tc[0], tc[1]); err == nil {
			t.Fatal(tc)
		}
	}
}

func TestPerfCoverageBoundaryFailure(t *testing.T) {
	for _, failStart := range []bool{true, false} {
		var handles []*fakePerf
		w, err := newPerfWindow([]int{0}, func(PerfEvent, int) (perfHandle, error) {
			h := &fakePerf{data: perfBytes(5, 10, 10)}
			handles = append(handles, h)
			return h, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		checks := 0
		w.checkCoverage = func() error {
			checks++
			if failStart || checks == 2 {
				return fmt.Errorf("topology changed")
			}
			return nil
		}
		err = w.Start()
		if failStart {
			if err == nil {
				t.Fatal("start allowed")
			}
		} else {
			if err != nil {
				t.Fatal(err)
			}
			got, err := w.Finish()
			if err == nil {
				t.Fatal("finish accepted changed CPUs")
			}
			for _, r := range got {
				if r.Status != "coverage_changed" || r.Count == nil || *r.Count != 5 {
					t.Fatal("raw evidence discarded or silently usable", r)
				}
			}
		}
		for _, h := range handles {
			if h.closed != 1 {
				t.Fatal("handle leaked", h)
			}
			if failStart && h.enabled != 0 {
				t.Fatal("enabled before coverage check")
			}
		}
	}
}
