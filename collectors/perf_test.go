package collectors

import (
	"encoding/binary"
	"errors"
	"os"
	"testing"
)

type fakePerf struct {
	data                           []byte
	enableErr, disableErr, readErr error
	enabled, disabled, closed      int
	log                            *[]string
}

func (f *fakePerf) Enable() error { f.enabled++; return f.enableErr }
func (f *fakePerf) Disable() error {
	f.disabled++
	if f.log != nil {
		*f.log = append(*f.log, "disable")
	}
	return f.disableErr
}
func (f *fakePerf) Read(b []byte) (int, error) {
	if f.log != nil {
		*f.log = append(*f.log, "read")
	}
	return copy(b, f.data), f.readErr
}
func (f *fakePerf) Close() error { f.closed++; return nil }
func perfBytes(count, enabled, running uint64) []byte {
	b := make([]byte, 24)
	binary.NativeEndian.PutUint64(b, count)
	binary.NativeEndian.PutUint64(b[8:], enabled)
	binary.NativeEndian.PutUint64(b[16:], running)
	return b
}

func TestPerfReadingExactAndMultiplexed(t *testing.T) {
	for _, tc := range []struct {
		count, enabled, running uint64
		status                  string
	}{{0, 100, 100, "available"}, {50, 100, 100, "available"}, {50, 100, 20, "multiplexed"}, {0, 100, 0, "not_running"}, {0, 0, 0, "not_running"}, {1, 0, 0, "invalid_reading"}, {1, 5, 6, "invalid_reading"}, {1<<63 + 1, 100, 100, "available"}} {
		var handles []*fakePerf
		var log []string
		w, err := newPerfWindow([]int{3, 1}, func(PerfEvent, int) (perfHandle, error) {
			h := &fakePerf{data: perfBytes(tc.count, tc.enabled, tc.running), log: &log}
			handles = append(handles, h)
			return h, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Finish(); err == nil {
			t.Fatal("finish before start")
		}
		if err = w.Start(); err != nil {
			t.Fatal(err)
		}
		if err = w.Start(); err == nil {
			t.Fatal("double start")
		}
		got, err := w.Finish()
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 14 {
			t.Fatal(len(got))
		}
		for i, r := range got {
			if r.Status != tc.status || r.CPU != []int{1, 3}[i%2] || r.PrivilegeScope != "user_and_kernel_excluding_hypervisor" {
				t.Fatal(r)
			}
			if tc.status == "invalid_reading" {
				if r.Count != nil || r.EnabledNS != nil || r.RunningNS != nil {
					t.Fatal(r)
				}
			} else if *r.Count != tc.count || *r.EnabledNS != tc.enabled || *r.RunningNS != tc.running {
				t.Fatal("scaled or lost raw integers", r)
			}
		}
		for i, s := range log {
			want := "read"
			if i < 14 {
				want = "disable"
			}
			if s != want {
				t.Fatal(log)
			}
		}
		if _, err = w.Finish(); err == nil {
			t.Fatal("double finish")
		}
		if err = w.Close(); err != nil {
			t.Fatal(err)
		}
		for _, h := range handles {
			if h.closed != 1 || h.enabled != 1 || h.disabled != 1 {
				t.Fatal(h)
			}
		}
	}
}

func TestPerfErrorsStayUnavailable(t *testing.T) {
	for _, stage := range []string{"open", "enable", "disable", "read", "short"} {
		t.Run(stage, func(t *testing.T) {
			var handles []*fakePerf
			w, err := newPerfWindow([]int{0}, func(e PerfEvent, _ int) (perfHandle, error) {
				if e.Name == "cycles" && stage == "open" {
					return nil, os.ErrPermission
				}
				h := &fakePerf{data: perfBytes(1, 10, 10)}
				handles = append(handles, h)
				if e.Name == "cycles" {
					switch stage {
					case "enable":
						h.enableErr = os.ErrPermission
					case "disable":
						h.disableErr = os.ErrPermission
					case "read":
						h.readErr = os.ErrPermission
					case "short":
						h.data = h.data[:8]
					}
				}
				return h, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if err = w.Start(); err != nil {
				t.Fatal(err)
			}
			got, err := w.Finish()
			if err != nil {
				t.Fatal(err)
			}
			want := "permission_denied"
			if stage == "short" {
				want = "unavailable"
			}
			if got[0].Status != want || got[0].Count != nil || got[0].Reason == "" {
				t.Fatal(got[0])
			}
			for _, r := range got[1:] {
				if r.Status != "available" {
					t.Fatal("unrelated event suppressed", r)
				}
			}
			for _, h := range handles {
				if h.closed != 1 {
					t.Fatal(h)
				}
			}
		})
	}
}

func TestPerfRejectsAmbiguousCPUsAndClosesUnstarted(t *testing.T) {
	for _, cpus := range [][]int{nil, {}, {-1}, {0, 0}, make([]int, 4097)} {
		if _, err := newPerfWindow(cpus, func(PerfEvent, int) (perfHandle, error) { t.Fatal("opened invalid request"); return nil, nil }); err == nil {
			t.Fatal(cpus)
		}
	}
	var hs []*fakePerf
	w, err := newPerfWindow([]int{0}, func(PerfEvent, int) (perfHandle, error) { h := &fakePerf{}; hs = append(hs, h); return h, nil })
	if err != nil {
		t.Fatal(err)
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	if err = w.Start(); err == nil {
		t.Fatal("start after close")
	}
	for _, h := range hs {
		if h.closed != 1 || h.enabled != 0 {
			t.Fatal(h)
		}
	}
	if perfErrorStatus(errors.New("PMU absent")) != "unavailable" {
		t.Fatal("invented permission status")
	}
}

func TestPerfDecodeByteOrders(t *testing.T) {
	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		b := make([]byte, 24)
		order.PutUint64(b, 123)
		order.PutUint64(b[8:], 456)
		order.PutUint64(b[16:], 456)
		c, e, r, err := decodePerfReading(b, order)
		if err != nil || c != 123 || e != 456 || r != 456 {
			t.Fatal(c, e, r, err)
		}
		for _, n := range []int{0, 8, 16, 23, 25} {
			if _, _, _, err := decodePerfReading(make([]byte, n), order); err == nil {
				t.Fatal(n)
			}
		}
	}
}
