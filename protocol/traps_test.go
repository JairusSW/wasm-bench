package protocol

import (
	"errors"
	"reflect"
	"testing"
)

func TestTrapMemoryWindow(t *testing.T) {
	for _, profile := range []string{"timing", "memory"} {
		t.Run(profile, func(t *testing.T) {
			var events []string
			mark := func(event string) { events = append(events, event) }
			factory := func() (TrapInstance, error) {
				mark("setup")
				return TrapInstance{
					Invoke: func() error { mark("invoke"); return errors.New("trap") },
					Close:  func() { mark("close") },
					BeginMemory: func() func() []Observation {
						mark("begin")
						return func() []Observation { mark("finish"); return []Observation{{Metric: "test"}} }
					},
				}, nil
			}
			classify := func(err error) *TrapResult {
				mark("classify")
				return &TrapResult{Code: "unreachable", Source: "test", Message: err.Error()}
			}
			samples, err := RunTrapSamples(&Preparation{Workload: trapWorkload(), Profile: profile}, &RunRequest{Scenario: "first-call", Samples: 1, Operations: 1}, factory, classify)
			want := []string{"setup", "invoke", "classify", "close"}
			count := 0
			if profile == "memory" {
				want = []string{"setup", "begin", "invoke", "finish", "classify", "close"}
				count = 1
			}
			if err != nil || !reflect.DeepEqual(events, want) || len(samples) != 1 || len(samples[0].Observations) != count {
				t.Fatal(events, samples, err)
			}
		})
	}
}

func trapWorkload() Workload {
	return Workload{ABI: "core", Export: "trap", Reset: "fresh_instance_per_sample", Oracle: Oracle{Kind: "expected_trap", ExpectedTrap: "unreachable"}}
}

func TestTrapContract(t *testing.T) {
	if err := ValidateTrap(trapWorkload()); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Workload){func(w *Workload) { w.Oracle.ExpectedTrap = "any" }, func(w *Workload) { w.Initialize = "init" }, func(w *Workload) { w.HostProfile = "host" }, func(w *Workload) { w.Args = Values{0} }, func(w *Workload) { w.Reset = "stateless" }, func(w *Workload) { w.Oracle.Expected = Values{0} }, func(w *Workload) { w.Input = &MemoryInput{} }} {
		w := trapWorkload()
		mutate(&w)
		if ValidateTrap(w) == nil {
			t.Fatal("accepted ambiguous contract", w)
		}
	}
	for _, result := range []*TrapResult{nil, {Code: "unreachable"}, {Code: "integer_overflow", Source: "test", Message: "overflow"}} {
		if VerifyTrap(trapWorkload(), result) == nil {
			t.Fatal("accepted wrong evidence", result)
		}
	}
}

func TestTrapMemoryMissingCollector(t *testing.T) {
	for _, nilBegin := range []bool{true, false} {
		closed, invoked := false, false
		factory := func() (TrapInstance, error) {
			i := TrapInstance{Invoke: func() error { invoked = true; return errors.New("trap") }, Close: func() { closed = true }}
			if !nilBegin {
				i.BeginMemory = func() func() []Observation { return nil }
			}
			return i, nil
		}
		_, err := RunTrapSamples(&Preparation{Workload: trapWorkload(), Profile: "memory"}, &RunRequest{Scenario: "first-call", Samples: 1, Operations: 1}, factory, func(error) *TrapResult { t.Fatal("classified without collector"); return nil })
		if err == nil || !closed || invoked {
			t.Fatal(err, closed, invoked)
		}
	}
}

func TestTrapSampleBoundaries(t *testing.T) {
	for _, mode := range []string{"correct", "returns", "wrong-trap", "setup-trap"} {
		t.Run(mode, func(t *testing.T) {
			created, called, closed := 0, 0, 0
			p := &Preparation{Workload: trapWorkload(), Profile: "timing"}
			r := &RunRequest{Scenario: "steady", Samples: 3, Warmup: 2, Operations: 99}
			factory := func() (TrapInstance, error) {
				created++
				if mode == "setup-trap" {
					return TrapInstance{}, errors.New("unreachable")
				}
				return TrapInstance{Invoke: func() error {
					called++
					if mode == "returns" {
						return nil
					}
					return errors.New("unreachable")
				}, Close: func() { closed++ }}, nil
			}
			classifier := func(err error) *TrapResult {
				if err == nil {
					return nil
				}
				code := "unreachable"
				if mode == "wrong-trap" {
					code = "integer_overflow"
				}
				return &TrapResult{Code: code, Source: "test", Message: err.Error()}
			}
			samples, err := RunTrapSamples(p, r, factory, classifier)
			if mode != "correct" {
				if err == nil {
					t.Fatal("accepted failure")
				}
				if mode == "setup-trap" && called != 0 {
					t.Fatal("invoked after setup failure")
				}
				if closed != called {
					t.Fatal("failed to release instance")
				}
				return
			}
			if err != nil || len(samples) != 5 || created != 5 || called != 5 || closed != 5 {
				t.Fatal(samples, err, created, called, closed)
			}
			for i, s := range samples {
				if s.Index != i || s.Warmup != (i < 2) || s.Operations != 1 || !s.Verified || VerifyTrap(p.Workload, s.TrapResult) != nil {
					t.Fatal(s)
				}
			}
		})
	}
}
