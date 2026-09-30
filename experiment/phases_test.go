package experiment

import (
	"github.com/wasmbench/wasmbench/protocol"
	"testing"
)

func TestPhaseOrderAndAttachment(t *testing.T) {
	p := phaseTracker{samples: 1, collect: func(s string) []protocol.Observation { return []protocol.Observation{{Phase: s}} }}
	if err := p.handle(protocol.PhaseEvent{Stage: "compiled"}); err == nil {
		t.Fatal("accepted out of order phase")
	}
	for _, stage := range []string{"before_compile", "compiled", "released"} {
		if err := p.handle(protocol.PhaseEvent{Stage: stage}); err != nil {
			t.Fatal(err)
		}
	}
	samples := []protocol.Sample{{Index: 0, Operations: 1}}
	if err := p.attach(samples); err != nil {
		t.Fatal(err)
	}
	if len(samples[0].Observations) != 3 {
		t.Fatal(samples)
	}
	if err := p.handle(protocol.PhaseEvent{SampleIndex: 1, Stage: "before_compile"}); err == nil {
		t.Fatal("accepted excess barrier")
	}
	samples[0].Operations = 2
	if err := p.attach(samples); err == nil {
		t.Fatal("accepted mismatched operation count")
	}
	p.next = 1
	if err := p.attach(samples); err == nil {
		t.Fatal("accepted incomplete phases")
	}
}

func TestTeardownPhaseOrder(t *testing.T) {
	p := phaseTracker{scenario: "teardown", samples: 2, collect: func(s string) []protocol.Observation { return []protocol.Observation{{Phase: "teardown/" + s}} }}
	for _, stage := range []string{"compiled", "torn_down", "released"} {
		if p.handle(protocol.PhaseEvent{Stage: stage}) == nil {
			t.Fatal("accepted invalid first stage", stage)
		}
	}
	for i := 0; i < 2; i++ {
		for _, stage := range protocol.PhaseStages("teardown") {
			if err := p.handle(protocol.PhaseEvent{SampleIndex: i, Stage: stage}); err != nil {
				t.Fatal(err)
			}
		}
	}
	s := []protocol.Sample{{Index: 0, Operations: 1}, {Index: 1, Operations: 1}}
	if err := p.attach(s); err != nil {
		t.Fatal(err)
	}
	for _, sample := range s {
		if len(sample.Observations) != 2 {
			t.Fatal(s)
		}
	}
	if p.handle(protocol.PhaseEvent{SampleIndex: 2, Stage: "before_teardown"}) == nil {
		t.Fatal("accepted excess stage")
	}
	p.next--
	if p.attach(s) == nil {
		t.Fatal("accepted incomplete release")
	}
}

func TestAppInitPhaseOrder(t *testing.T) {
	for _, scenario := range []string{"app-init", "instantiate"} {
		p := phaseTracker{scenario: scenario, samples: 1, collect: func(stage string) []protocol.Observation { return nil }}
		if p.handle(protocol.PhaseEvent{Stage: "before_compile"}) == nil {
			t.Fatal("accepted compile boundary")
		}
		for _, stage := range protocol.PhaseStages(scenario) {
			if err := p.handle(protocol.PhaseEvent{Stage: stage}); err != nil {
				t.Fatal(err)
			}
		}
		if err := p.attach([]protocol.Sample{{Operations: 1}}); err != nil {
			t.Fatal(err)
		}
		p.next--
		if p.attach([]protocol.Sample{{Operations: 1}}) == nil {
			t.Fatal("accepted missing release")
		}
	}
}

func TestRustAllocatorBoundariesDoNotReuseLegacyReleaseLabels(t *testing.T) {
	for _, scenario := range []string{"compile", "instantiate", "first-call", "steady"} {
		p := phaseTracker{scenario: scenario, allocator: true, samples: 2, collect: func(stage string) []protocol.Observation {
			return []protocol.Observation{{Phase: scenario + "/" + stage}}
		}}
		stages := protocol.RustAllocatorPhaseStages(scenario)
		for index := 0; index < 2; index++ {
			for j, stage := range stages {
				if j == 2 && p.handle(protocol.PhaseEvent{SampleIndex: index, Stage: protocol.PhaseStages(scenario)[2]}) == nil {
					t.Fatal("legacy release/verified stage accepted as release entry", scenario)
				}
				if err := p.handle(protocol.PhaseEvent{SampleIndex: index, Stage: stage}); err != nil {
					t.Fatal(err)
				}
			}
		}
		samples := []protocol.Sample{{Operations: 1}, {Index: 1, Operations: 1}}
		if err := p.attach(samples); err != nil || len(samples[0].Observations) != 4 {
			t.Fatal(samples, err)
		}
		p.next--
		if p.attach(samples) == nil {
			t.Fatal("omitted final release decision accepted")
		}
	}
}
