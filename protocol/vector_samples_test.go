package protocol

import (
	"fmt"
	"slices"
	"testing"
)

func TestVectorDiagnosticWindow(t *testing.T) {
	for _, profile := range []string{"timing", "memory"} {
		var events []string
		p := &Preparation{Profile: profile, Workload: Workload{Reset: "fresh_instance_per_sample", Oracle: Oracle{Kind: "exact_vectors"}, VectorByteBudget: 1, Vectors: &VectorContract{OutputLen: 1, Cases: []VectorCase{{Out: "ab"}}}}}
		_, err := RunVectorSamples(p, &RunRequest{Scenario: "first-call", Samples: 1, Operations: 1}, func(string) (VectorInstance, int64, func(), error) {
			events = append(events, "create")
			return VectorInstance{Write: func(uint32, []byte) bool { return true }, Invoke: func(uint32, uint32, uint32) error { events = append(events, "invoke"); return nil }, Read: func(uint32, uint32) ([]byte, bool) { events = append(events, "verify"); return []byte{0xab}, true }, Snapshot: func() []Observation { events = append(events, "snapshot"); return nil }}, 0, func() { events = append(events, "close") }, nil
		}, func() func() []Observation {
			events = append(events, "begin")
			return func() []Observation { events = append(events, "end"); return nil }
		})
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"create", "invoke", "verify", "close"}
		if profile == "memory" {
			want = []string{"begin", "create", "invoke", "verify", "snapshot", "close", "end"}
		}
		if !slices.Equal(events, want) {
			t.Fatal(profile, events)
		}
	}
}

func TestVectorReleaseBarrierFailureClosesInstance(t *testing.T) {
	for _, fail := range []string{"", "before_teardown", "torn_down"} {
		closed := 0
		var events []string
		p := &Preparation{Profile: "memory", Workload: Workload{Reset: "fresh_instance_per_sample", Oracle: Oracle{Kind: "exact_vectors"}, VectorByteBudget: 1, Vectors: &VectorContract{OutputLen: 1, Cases: []VectorCase{{Out: "ab"}}}}}
		samples, err := RunVectorSamples(p, &RunRequest{Scenario: "teardown", Samples: 1, Operations: 1, PhaseBarriers: true}, func(string) (VectorInstance, int64, func(), error) {
			return VectorInstance{
				Write:  func(uint32, []byte) bool { return true },
				Invoke: func(uint32, uint32, uint32) error { events = append(events, "invoke"); return nil },
				Read:   func(uint32, uint32) ([]byte, bool) { events = append(events, "verify"); return []byte{0xab}, true },
				ReleaseBarrier: func(i int, stage string) error {
					events = append(events, stage)
					if stage == fail {
						return fmt.Errorf("barrier failed")
					}
					return nil
				},
			}, 0, func() { closed++; events = append(events, "close") }, nil
		})
		if closed != 1 {
			t.Fatal(closed, events)
		}
		if fail == "" {
			if err != nil || len(samples) != 1 || !slices.Equal(events, []string{"invoke", "verify", "before_teardown", "close", "torn_down"}) {
				t.Fatal(samples, err, events)
			}
		} else if err == nil || len(samples) != 0 {
			t.Fatal("accepted failed handshake", samples, err)
		}
	}
}

func TestVectorInstantiationReleaseRequiresEveryOracle(t *testing.T) {
	for _, mode := range []string{"correct", "wrong-second", "release-fails"} {
		t.Run(mode, func(t *testing.T) {
			p := &Preparation{Profile: "memory", Workload: Workload{Reset: "fresh_instance_per_sample", Oracle: Oracle{Kind: "exact_vectors"}, VectorByteBudget: 2, Vectors: &VectorContract{OutputLen: 1, Cases: []VectorCase{{Out: "ab"}, {Out: "ab"}}}}}
			closed, calls := 0, 0
			var events []string
			samples, err := RunVectorSamples(p, &RunRequest{Scenario: "instantiate", Samples: 1, Operations: 99, PhaseBarriers: true}, func(string) (VectorInstance, int64, func(), error) {
				events = append(events, "before_instantiate", "instantiated", "initialized")
				return VectorInstance{
					Write:  func(uint32, []byte) bool { return true },
					Invoke: func(uint32, uint32, uint32) error { calls++; return nil },
					Read: func(uint32, uint32) ([]byte, bool) {
						if mode == "wrong-second" && calls == 2 {
							return []byte{0xac}, true
						}
						return []byte{0xab}, true
					},
					ReleaseBarrier: func(index int, stage string) error {
						if index != 0 {
							t.Fatal(index)
						}
						events = append(events, stage)
						if mode == "release-fails" {
							return fmt.Errorf("release handshake failed")
						}
						return nil
					},
				}, 37, func() { closed++; events = append(events, "close") }, nil
			})
			if closed != 1 || calls != 2 {
				t.Fatal(closed, calls)
			}
			want := []string{"before_instantiate", "instantiated", "initialized", "close"}
			if mode != "wrong-second" {
				want = append(want, "instance_released")
			}
			if !slices.Equal(events, want) {
				t.Fatal(events)
			}
			if mode == "correct" {
				if err != nil || len(samples) != 1 || !samples[0].Verified || samples[0].Operations != 1 || samples[0].ElapsedNS != 37 || samples[0].SampleType != "individual_operation" {
					t.Fatal(samples, err)
				}
			} else if err == nil || len(samples) != 0 {
				t.Fatal("qualified failed sequence", samples, err)
			}
		})
	}
}

func TestVectorFirstCallMemoryWindow(t *testing.T) {
	for _, mode := range []string{"correct", "wrong-first", "wrong-last", "before_first_call", "first_call_returned", "first_call_released"} {
		t.Run(mode, func(t *testing.T) {
			var events []string
			calls, closed := 0, 0
			p := &Preparation{Profile: "memory", Workload: Workload{Reset: "fresh_instance_per_sample", Oracle: Oracle{Kind: "exact_vectors"}, VectorByteBudget: 2, Vectors: &VectorContract{OutputLen: 1, Cases: []VectorCase{{Out: "ab"}, {Out: "ab"}}}}}
			samples, err := RunVectorSamples(p, &RunRequest{Scenario: "first-call", Samples: 1, Operations: 99, PhaseBarriers: true}, func(string) (VectorInstance, int64, func(), error) {
				events = append(events, "create")
				return VectorInstance{
					Write:  func(uint32, []byte) bool { events = append(events, "write"); return true },
					Invoke: func(uint32, uint32, uint32) error { calls++; events = append(events, "invoke"); return nil },
					Read: func(uint32, uint32) ([]byte, bool) {
						events = append(events, "verify")
						if (mode == "wrong-first" && calls == 1) || (mode == "wrong-last" && calls == 2) {
							return []byte{0xac}, true
						}
						return []byte{0xab}, true
					},
					Snapshot: func() []Observation { events = append(events, "snapshot"); return nil },
					ReleaseBarrier: func(index int, stage string) error {
						if index != 0 {
							t.Fatal(index)
						}
						events = append(events, stage)
						if stage == mode {
							return fmt.Errorf("handshake failed")
						}
						return nil
					},
				}, 37, func() { closed++; events = append(events, "close") }, nil
			}, func() func() []Observation {
				events = append(events, "begin")
				return func() []Observation {
					events = append(events, "end")
					return []Observation{{Metric: "fixture.window", Value: Value(0)}}
				}
			})
			want := []string{"create", "write", "before_first_call"}
			if mode != "before_first_call" {
				want = append(want, "begin", "invoke", "verify")
				if mode != "wrong-first" {
					want = append(want, "write", "invoke", "end", "first_call_returned")
					if mode != "first_call_returned" {
						want = append(want, "verify")
					}
					if mode == "correct" || mode == "first_call_released" {
						want = append(want, "snapshot")
					}
				}
			}
			want = append(want, "close")
			if mode == "correct" || mode == "first_call_released" {
				want = append(want, "first_call_released")
			}
			if closed != 1 || !slices.Equal(events, want) {
				t.Fatal(events, want, closed)
			}
			if mode == "correct" {
				if err != nil || len(samples) != 1 || !samples[0].Verified || samples[0].Operations != 1 || samples[0].SampleType != "sequence_call_sum" || len(samples[0].Observations) != 1 || *samples[0].Observations[0].Value != 0 {
					t.Fatal(samples, err)
				}
			} else if err == nil || len(samples) != 0 {
				t.Fatal("qualified a failed handshake/oracle", samples, err)
			}
		})
	}
}

func TestVectorSamples(t *testing.T) {
	for _, scenario := range []string{"compile", "instantiate", "first-call", "steady", "teardown"} {
		t.Run(scenario, func(t *testing.T) {
			p := &Preparation{Profile: "timing", Workload: Workload{Reset: "fresh_instance_per_sample", Oracle: Oracle{Kind: "exact_vectors"}, VectorByteBudget: 16, Vectors: &VectorContract{OutputLen: 1, Cases: []VectorCase{{Out: "ab"}, {Len: 1, Out: "ab"}}}}}
			created, closed, calls := 0, 0, 0
			samples, err := RunVectorSamples(p, &RunRequest{Scenario: scenario, Samples: 2, Operations: 99, Warmup: 1}, func(string) (VectorInstance, int64, func(), error) {
				created++
				return VectorInstance{Write: func(uint32, []byte) bool { return true }, Invoke: func(uint32, uint32, uint32) error { calls++; return nil }, Read: func(uint32, uint32) ([]byte, bool) { return []byte{0xab}, true }}, 37, func() { closed++ }, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			want := 2
			if scenario == "steady" {
				want = 3
			}
			if len(samples) != want || created != want || closed != want || calls != want*2 {
				t.Fatal(samples, created, closed, calls)
			}
			for i, s := range samples {
				if s.Index != i || s.Operations != 1 || !s.Verified || s.Warmup != (scenario == "steady" && i == 0) {
					t.Fatal(s)
				}
				if scenario == "compile" || scenario == "instantiate" {
					if s.ElapsedNS != 37 || s.SampleType != "individual_operation" {
						t.Fatal(s)
					}
				} else if scenario == "teardown" {
					if s.SampleType != "individual_operation" {
						t.Fatal(s)
					}
				} else if s.SampleType != "sequence_call_sum" {
					t.Fatal(s)
				}
			}
		})
	}
}

func TestVectorTeardownDiagnosticWindow(t *testing.T) {
	var events []string
	p := &Preparation{Profile: "memory", Workload: Workload{Reset: "fresh_instance_per_sample", Oracle: Oracle{Kind: "exact_vectors"}, VectorByteBudget: 1, Vectors: &VectorContract{OutputLen: 1, Cases: []VectorCase{{Out: "ab"}}}}}
	_, err := RunVectorSamples(p, &RunRequest{Scenario: "teardown", Samples: 1, Operations: 99, Warmup: 4}, func(string) (VectorInstance, int64, func(), error) {
		events = append(events, "create")
		return VectorInstance{
			Write:    func(uint32, []byte) bool { return true },
			Invoke:   func(uint32, uint32, uint32) error { events = append(events, "invoke"); return nil },
			Read:     func(uint32, uint32) ([]byte, bool) { events = append(events, "verify"); return []byte{0xab}, true },
			Snapshot: func() []Observation { events = append(events, "snapshot"); return nil },
		}, 37, func() { events = append(events, "close") }, nil
	}, func() func() []Observation {
		events = append(events, "begin")
		return func() []Observation { events = append(events, "end"); return nil }
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(events, []string{"create", "invoke", "verify", "snapshot", "begin", "close", "end"}) {
		t.Fatal(events)
	}
}
