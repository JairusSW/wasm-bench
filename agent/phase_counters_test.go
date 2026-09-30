package agent

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/protocol"
)

type testCounterWindow struct {
	log                 *[]string
	startErr, finishErr error
	readings            []collectors.PerfReading
	closed              bool
}

func (w *testCounterWindow) Start() error { *w.log = append(*w.log, "start"); return w.startErr }
func (w *testCounterWindow) Finish() ([]collectors.PerfReading, error) {
	*w.log = append(*w.log, "finish")
	return w.readings, w.finishErr
}
func (w *testCounterWindow) Close() error {
	if !w.closed {
		*w.log = append(*w.log, "close")
		w.closed = true
	}
	return nil
}

type counterInput struct{ bytes.Buffer }

func (*counterInput) Close() error { return nil }

func counterRequest(scenario string) protocol.Request {
	return protocol.Request{Method: "run", Run: &protocol.RunRequest{Scenario: scenario, Samples: 2, Operations: 1, PhaseBarriers: true}}
}

func counterResponses(scenario string, truncate bool) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	for i := 0; i < 2; i++ {
		for _, stage := range protocol.PhaseStages(scenario) {
			enc.Encode(protocol.Response{Version: protocol.Version, ID: 1, Status: "phase", Phase: &protocol.PhaseEvent{SampleIndex: i, Stage: stage}})
			if truncate {
				return b.String()
			}
		}
	}
	enc.Encode(protocol.Response{Version: protocol.Version, ID: 1, Status: "ok", Samples: []protocol.Sample{{Index: 0, Operations: 1, Verified: true}, {Index: 1, Operations: 1, Verified: true}}})
	return b.String()
}

func TestCounterPhasesHandshakeAndCleanup(t *testing.T) {
	for _, scenario := range []string{"compile", "instantiate", "first-call", "teardown", "app-init", "density", "density-cycle"} {
		t.Run(scenario, func(t *testing.T) {
			var log []string
			input := &counterInput{}
			c := &Client{in: input, out: bufio.NewScanner(strings.NewReader(counterResponses(scenario, false)))}
			_, records, err := c.callCounterPhases(counterRequest(scenario), func() (counterWindow, error) {
				log = append(log, "open")
				return &testCounterWindow{log: &log, readings: []collectors.PerfReading{{Status: "available"}}}, nil
			})
			if err != nil || len(records) != 2 {
				t.Fatal(records, err)
			}
			if strings.Join(log, ",") != "open,start,finish,close,open,start,finish,close" {
				t.Fatal(log)
			}
			for i, r := range records {
				if r.Sample != i || r.Phase != scenario+"/barrier_window" || r.Status != "available" {
					t.Fatal(r)
				}
			}
			dec := json.NewDecoder(bytes.NewReader(input.Bytes()))
			var req protocol.Request
			if err = dec.Decode(&req); err != nil || req.Method != "run" {
				t.Fatal(req, err)
			}
			for i := 0; i < 2*len(protocol.PhaseStages(scenario)); i++ {
				if err = dec.Decode(&req); err != nil || req.Method != "continue" || req.ID != 1 {
					t.Fatal(req, err)
				}
			}
		})
	}
}

func TestCounterPhasesFailureAndUnavailable(t *testing.T) {
	for _, mode := range []string{"open", "start", "finish", "eof", "partial", "denied"} {
		t.Run(mode, func(t *testing.T) {
			var log []string
			c := &Client{in: &counterInput{}, out: bufio.NewScanner(strings.NewReader(counterResponses("compile", mode == "eof")))}
			_, records, err := c.callCounterPhases(counterRequest("compile"), func() (counterWindow, error) {
				if mode == "open" {
					return nil, fmt.Errorf("no cgroup")
				}
				w := &testCounterWindow{log: &log, readings: []collectors.PerfReading{{Status: "available"}}}
				if mode == "start" {
					w.startErr = fmt.Errorf("coverage changed")
				}
				if mode == "finish" {
					w.finishErr = fmt.Errorf("coverage changed")
				}
				if mode == "partial" {
					w.readings = append(w.readings, collectors.PerfReading{Status: "multiplexed"})
				}
				if mode == "denied" {
					w.readings = []collectors.PerfReading{{Status: "permission_denied", Reason: "denied"}}
				}
				return w, nil
			})
			if len(records) == 0 {
				t.Fatal("missing failure evidence")
			}
			switch mode {
			case "start", "finish", "eof":
				if err == nil || log[len(log)-1] != "close" {
					t.Fatal(log, err)
				}
			case "open", "denied":
				if err != nil || records[0].Status != "unavailable" {
					t.Fatal(records, err)
				}
			case "partial":
				if err != nil || records[0].Status != "partial" {
					t.Fatal(records, err)
				}
			}
		})
	}
}

func TestCounterPhasesRejectsMalformedSequencesAndTiming(t *testing.T) {
	for _, profile := range []string{"", "memory", "timing", "code"} {
		c := &Client{preparedProfile: profile}
		if _, _, err := c.CallCounterPhases(counterRequest("compile")); err == nil {
			t.Fatal(profile)
		}
	}
	for _, mutate := range []func(*protocol.RunRequest){func(r *protocol.RunRequest) { r.Warmup = 1 }, func(r *protocol.RunRequest) { r.Operations = 2 }, func(r *protocol.RunRequest) { r.PhaseBarriers = false }, func(r *protocol.RunRequest) { r.Scenario = "unknown" }, func(r *protocol.RunRequest) { r.Samples = 0 }} {
		r := counterRequest("compile")
		mutate(r.Run)
		if _, err := newCounterPhases(r, nil); err == nil {
			t.Fatal(r)
		}
	}
	p, _ := newCounterPhases(counterRequest("compile"), func() (counterWindow, error) { return nil, fmt.Errorf("no cgroup") })
	if err := p.handle(protocol.PhaseEvent{SampleIndex: 0, Stage: "compiled"}); err == nil {
		t.Fatal("out-of-order accepted")
	}
	if err := p.handle(protocol.PhaseEvent{SampleIndex: 0, Stage: "before_compile"}); err != nil {
		t.Fatal(err)
	}
	if err := p.handle(protocol.PhaseEvent{SampleIndex: 1, Stage: "compiled"}); err == nil {
		t.Fatal("wrong sample accepted")
	}
	if err := p.complete(protocol.Response{}); err == nil {
		t.Fatal("missing evidence accepted")
	}
}

func TestCounterSteadyWarmupSequence(t *testing.T) {
	r := counterRequest("steady")
	r.Run.Operations, r.Run.Warmup = 3, 1
	p, err := newCounterPhases(r, func() (counterWindow, error) { return nil, fmt.Errorf("unavailable") })
	if err != nil {
		t.Fatal(err)
	}
	resp := protocol.Response{}
	for i := 0; i < 3; i++ {
		for _, stage := range protocol.PhaseStages("steady") {
			if err = p.handle(protocol.PhaseEvent{SampleIndex: i, Stage: stage}); err != nil {
				t.Fatal(err)
			}
		}
		resp.Samples = append(resp.Samples, protocol.Sample{Index: i, Operations: 3, Warmup: i == 0, Verified: true})
	}
	if len(p.records) != 3 || p.complete(resp) != nil {
		t.Fatal(p.records, resp)
	}
	resp.Samples[0].Warmup = false
	if p.complete(resp) == nil {
		t.Fatal("lost warmup accepted")
	}
	resp.Samples[0].Warmup = true
	resp.Samples[1].Operations = 1
	if p.complete(resp) == nil {
		t.Fatal("wrong denominator accepted")
	}
}

func TestCounterPreparationProfileTracksOnlySuccess(t *testing.T) {
	for _, status := range []string{"ok", "error"} {
		var b bytes.Buffer
		json.NewEncoder(&b).Encode(protocol.Response{Version: protocol.Version, ID: 1, Status: status, Reason: "test"})
		c := &Client{preparedProfile: "counters", in: &counterInput{}, out: bufio.NewScanner(&b)}
		_, err := c.Call(protocol.Request{Method: "prepare", Prepare: &protocol.Preparation{Profile: "memory"}})
		want := "memory"
		if status == "error" {
			want = ""
			if err == nil {
				t.Fatal("failed prepare accepted")
			}
		}
		if c.preparedProfile != want {
			t.Fatal(c.preparedProfile, want)
		}
		if _, _, err = c.CallCounterPhases(counterRequest("compile")); err == nil {
			t.Fatal("stale counters preparation reused")
		}
	}
}

func TestCounterPhasesRefusesUnverifiedFinalSamples(t *testing.T) {
	r := counterRequest("teardown")
	r.Run.Samples = 1
	p, err := newCounterPhases(r, func() (counterWindow, error) { return nil, fmt.Errorf("not configured") })
	if err != nil {
		t.Fatal(err)
	}
	for _, stage := range protocol.PhaseStages("teardown") {
		if err = p.handle(protocol.PhaseEvent{SampleIndex: 0, Stage: stage}); err != nil {
			t.Fatal(err)
		}
	}
	resp := protocol.Response{Samples: []protocol.Sample{{Index: 0, Operations: 1, Verified: false}}}
	if err = p.complete(resp); err == nil {
		t.Fatal("unverified result accepted")
	}
	resp.Samples[0].Verified = true
	if err = p.complete(resp); err != nil {
		t.Fatal(err)
	}
}
