package analysis

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

func pilotFixture() experiment.Bundle {
	b := experiment.Bundle{Manifest: experiment.Manifest{Kind: "measurement", Lock: experiment.Lock{Options: experiment.Options{Profile: "timing", Launches: 6, Scenarios: []string{"compile"}}, Runtimes: []experiment.Runtime{{ID: "a"}, {ID: "b"}}, Workloads: []protocol.Workload{{ID: "w"}}}}}
	for _, r := range []string{"a", "b"} {
		for i := 0; i < 6; i++ {
			value := int64(100)
			if r == "b" {
				value += int64(i * 10)
			}
			b.Trials = append(b.Trials, experiment.Trial{Runtime: r, Workload: "w", Scenario: "compile", Profile: "timing", Block: i, Status: "ok", Samples: []protocol.Sample{{Verified: true, ElapsedNS: value, Operations: 1}}})
		}
	}
	return b
}
func TestPilotCommonBudgetAndImmutableEvidence(t *testing.T) {
	b := pilotFixture()
	before, _ := json.Marshal(b)
	p, err := PlanPilot(b, .05, 6, 1000)
	if err != nil || p.Status != "ready" || len(p.Cells) != 2 || p.Launches <= 6 || p.Launches != p.Cells[1].Launches || p.Cells[0].Launches != 6 {
		t.Fatal(p, err)
	}
	after, _ := json.Marshal(b)
	if string(before) != string(after) {
		t.Fatal("pilot mutated")
	}
	p2, _ := PlanPilot(b, .05, 6, 1000)
	a, _ := json.Marshal(p)
	c, _ := json.Marshal(p2)
	if string(a) != string(c) {
		t.Fatal("nondeterministic plan")
	}
}
func TestPilotNeverDropsCellsOrClipsBudgets(t *testing.T) {
	for _, mode := range []string{"failed", "missing", "unsupported", "duplicate", "invalid-sample", "zero", "cap", "short"} {
		t.Run(mode, func(t *testing.T) {
			b := pilotFixture()
			cap := 1000
			switch mode {
			case "failed":
				b.Trials[0].Status = "error"
			case "unsupported":
				b.Trials[0].Status = "unsupported"
			case "missing":
				b.Trials = b.Trials[6:]
			case "duplicate":
				b.Trials[0].Block = 1
			case "invalid-sample":
				b.Trials[0].Samples = append(b.Trials[0].Samples, protocol.Sample{ElapsedNS: 50, Operations: 1})
			case "zero":
				for i := range b.Trials {
					b.Trials[i].Samples[0].ElapsedNS = 0
				}
			case "cap":
				cap = 6
			case "short":
				b.Manifest.Lock.Options.Launches = 5
				b.Trials = append(b.Trials[:5], b.Trials[6:11]...)
			}
			p, err := PlanPilot(b, .05, 6, cap)
			if err != nil || p.Status != "unresolved" || p.Launches != 0 || len(p.Cells) != 2 {
				t.Fatal(p, err)
			}
		})
	}
}
func TestPilotInputContract(t *testing.T) {
	b0 := pilotFixture()
	b0.Manifest.Lock.PilotPlan = json.RawMessage(`{}`)
	if _, err := PlanPilot(b0, .05, 6, 100); err == nil {
		t.Fatal("confirmation reused as pilot")
	}
	b0 = pilotFixture()
	b0.Trials = append(b0.Trials, experiment.Trial{Runtime: "a", Workload: "w", Scenario: "compile", Block: -1, Status: "error"})
	if p, err := PlanPilot(b0, .05, 6, 1000); err != nil || p.Status != "unresolved" {
		t.Fatal("failed admission ignored", p, err)
	}
	for _, target := range []float64{0, -1, 1, math.NaN(), math.Inf(1)} {
		if _, err := PlanPilot(pilotFixture(), target, 6, 100); err == nil {
			t.Fatal(target)
		}
	}
	for _, profile := range []string{"profiling", "memory", "counters", "code"} {
		b := pilotFixture()
		b.Manifest.Lock.Options.Profile = profile
		if _, err := PlanPilot(b, .05, 6, 100); err == nil {
			t.Fatal(profile)
		}
	}
	b := pilotFixture()
	b.Manifest.Lock.Options.Check = true
	if _, err := PlanPilot(b, .05, 6, 100); err == nil {
		t.Fatal("correctness accepted")
	}
}
