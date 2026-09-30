package protocol_test

import (
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
	"testing"
)

func TestSnapshotDensityProductContract(t *testing.T) {
	ws, err := corpus.Generate(t.TempDir(), "process-snapshot-density")
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 5 {
		t.Fatal("missing count coverage")
	}
	for _, w := range ws {
		p := protocol.Preparation{Workload: w, Profile: "memory"}
		r := protocol.RunRequest{Scenario: protocol.SnapshotDensityScenario, Samples: 2, Operations: 1, PhaseBarriers: true}
		if err := protocol.ValidateSnapshotDensityRequest(p, r); err != nil {
			t.Fatal(err)
		}
		if protocol.ValidateProcessSnapshotWorkload(w) == nil {
			t.Fatal("density reinterpreted as sequential snapshots")
		}
		for name, mutate := range map[string]func(*protocol.Preparation, *protocol.RunRequest){
			"timing":           func(p *protocol.Preparation, _ *protocol.RunRequest) { p.Profile = "timing" },
			"barriers":         func(_ *protocol.Preparation, r *protocol.RunRequest) { r.PhaseBarriers = false },
			"samples":          func(_ *protocol.Preparation, r *protocol.RunRequest) { r.Samples = 33 },
			"batch":            func(_ *protocol.Preparation, r *protocol.RunRequest) { r.Operations = 2 },
			"warmup":           func(_ *protocol.Preparation, r *protocol.RunRequest) { r.Warmup = 1 },
			"foreign scenario": func(_ *protocol.Preparation, r *protocol.RunRequest) { r.Scenario = "density" },
			"wrong dimension":  func(p *protocol.Preparation, _ *protocol.RunRequest) { p.Workload.Size++ },
			"wrong bytes":      func(p *protocol.Preparation, _ *protocol.RunRequest) { p.Workload.SHA256 = "other" },
			"generic snapshot": func(p *protocol.Preparation, _ *protocol.RunRequest) {
				p.Workload.ProcessSnapshot = &protocol.ProcessSnapshotContract{Mode: protocol.ProcessSnapshotMode}
			},
			"foreign oracle": func(p *protocol.Preparation, _ *protocol.RunRequest) {
				p.Workload.Oracle.Expected = protocol.Values{125}
			},
		} {
			t.Run(w.ID+"/"+name, func(t *testing.T) {
				bad, request := p, r
				mutate(&bad, &request)
				if protocol.ValidateSnapshotDensityRequest(bad, request) == nil {
					t.Fatal("forged density admitted")
				}
			})
		}
	}
}
