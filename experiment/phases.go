package experiment

import (
	"fmt"

	"github.com/wasmbench/wasmbench/protocol"
)

type phaseTracker struct {
	scenario     string
	allocator    bool
	records      []PhaseRecord
	samples      int
	next         int
	observations map[int][]protocol.Observation
	collect      func(string) []protocol.Observation
}

func (p *phaseTracker) handle(event protocol.PhaseEvent) error {
	stages := p.stages()
	if len(stages) == 0 || p.next >= p.samples*len(stages) || event.SampleIndex != p.next/len(stages) || event.Stage != stages[p.next%len(stages)] {
		return fmt.Errorf("invalid phase order: sample %d stage %s", event.SampleIndex, event.Stage)
	}
	if p.observations == nil {
		p.observations = map[int][]protocol.Observation{}
	}
	observations := p.collect(event.Stage)
	p.records = append(p.records, PhaseRecord{Event: event, Observations: observations})
	p.observations[event.SampleIndex] = append(p.observations[event.SampleIndex], observations...)
	p.next++
	return nil
}

func (p *phaseTracker) attach(samples []protocol.Sample) error {
	if len(p.stages()) == 0 || p.next != p.samples*len(p.stages()) || len(samples) != p.samples {
		return fmt.Errorf("incomplete phase sequence")
	}
	for i := range samples {
		if samples[i].Index != i || samples[i].Warmup || samples[i].Operations != 1 {
			return fmt.Errorf("phase sample identity mismatch")
		}
		samples[i].Observations = append(samples[i].Observations, p.observations[i]...)
	}
	return nil
}

func (p *phaseTracker) stages() []string {
	if p.allocator {
		return protocol.RustAllocatorPhaseStages(p.scenario)
	}
	scenario := p.scenario
	if scenario == "" {
		scenario = "compile"
	}
	return protocol.PhaseStages(scenario)
}
