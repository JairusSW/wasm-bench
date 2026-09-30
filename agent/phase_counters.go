package agent

import (
	"errors"
	"fmt"

	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/protocol"
)

// CounterPhase retains raw per-CPU evidence for one diagnostic barrier window.
// Status describes counter availability, not workload correctness. Records from
// a failed call are diagnostic only, even if an earlier window completed.
type CounterPhase struct {
	CollectorVersion string                   `json:"collector_version"`
	Sample           int                      `json:"sample_index"`
	Phase            string                   `json:"phase"`
	Status           string                   `json:"status"`
	Reason           string                   `json:"reason,omitempty"`
	Readings         []collectors.PerfReading `json:"readings"`
}

type counterWindow interface {
	Start() error
	Finish() ([]collectors.PerfReading, error)
	Close() error
}

type counterPhases struct {
	request protocol.RunRequest
	stages  []string
	samples int
	next    int
	open    func() (counterWindow, error)
	active  counterWindow
	records []CounterPhase
}

func newCounterPhases(req protocol.Request, open func() (counterWindow, error)) (*counterPhases, error) {
	if req.Method != "run" || req.Run == nil {
		return nil, fmt.Errorf("counter collection requires run request")
	}
	r := *req.Run
	stages := protocol.PhaseStages(r.Scenario)
	samples, err := protocol.CounterSampleCount(&r)
	if err != nil {
		return nil, err
	}
	if len(stages) < 2 {
		return nil, fmt.Errorf("counters require declared phase barriers")
	}
	return &counterPhases{request: r, stages: stages, samples: samples, open: open}, nil
}

func (p *counterPhases) close() error {
	if p.active == nil {
		return nil
	}
	w := p.active
	p.active = nil
	return w.Close()
}

func (p *counterPhases) handle(e protocol.PhaseEvent) error {
	if p.next >= p.samples*len(p.stages) || e.SampleIndex != p.next/len(p.stages) || e.Stage != p.stages[p.next%len(p.stages)] {
		return fmt.Errorf("invalid counter phase order: sample %d stage %s", e.SampleIndex, e.Stage)
	}
	_, start, end := phaseWindow(e.Stage)
	if start {
		if p.active != nil {
			return fmt.Errorf("counter window already active")
		}
		r := CounterPhase{CollectorVersion: collectors.PerfVersion, Sample: e.SampleIndex, Phase: p.request.Scenario + "/barrier_window", Status: "incomplete"}
		w, err := p.open()
		if err != nil {
			r.Status = "unavailable"
			r.Reason = err.Error()
		} else {
			p.active = w
			if err = w.Start(); err != nil {
				r.Status = "collection_error"
				r.Reason = err.Error()
				p.records = append(p.records, r)
				return errors.Join(err, p.close())
			}
		}
		p.records = append(p.records, r)
	}
	if end && p.active != nil {
		w := p.active
		readings, err := w.Finish()
		closeErr := p.close()
		r := &p.records[len(p.records)-1]
		r.Readings = readings
		if err = errors.Join(err, closeErr); err != nil {
			r.Status = "collection_error"
			r.Reason = err.Error()
			return err
		}
		r.Status = "available"
		available := 0
		for _, reading := range readings {
			if reading.Status == "available" {
				available++
			}
		}
		if available == 0 {
			r.Status = "unavailable"
		} else if available < len(readings) {
			r.Status = "partial"
		}
	}
	p.next++
	return nil
}

func (p *counterPhases) complete(resp protocol.Response) error {
	if p.next != p.samples*len(p.stages) || len(resp.Samples) != p.samples {
		return fmt.Errorf("incomplete counter phase/sample sequence")
	}
	for i, s := range resp.Samples {
		if s.Index != i || s.Warmup != (i < p.request.Warmup) || s.Operations != p.request.Operations || !s.Verified {
			return fmt.Errorf("counter sample identity or verification mismatch")
		}
	}
	return nil
}

// CallCounterPhases is opt-in and does not change preparation/profile semantics.
// Callers must prepare an adapter that supports counters-only phase handshakes;
// it must not be used to relabel an instrumented memory pass as a counters pass.
// The normal Call/CallPhased methods never open perf descriptors.
func (c *Client) CallCounterPhases(req protocol.Request) (protocol.Response, []CounterPhase, error) {
	if c.preparedProfile != "counters" {
		return protocol.Response{}, nil, fmt.Errorf("counter phases require successful counters-profile preparation")
	}
	return c.callCounterPhases(req, c.cgroup.openPerf)
}

func (c *Client) callCounterPhases(req protocol.Request, open func() (counterWindow, error)) (resp protocol.Response, records []CounterPhase, err error) {
	p, err := newCounterPhases(req, open)
	if err != nil {
		return resp, nil, err
	}
	defer func() { err = errors.Join(err, p.close()); records = p.records }()
	resp, err = c.CallPhased(req, p.handle)
	if err == nil {
		err = p.complete(resp)
	}
	return
}
