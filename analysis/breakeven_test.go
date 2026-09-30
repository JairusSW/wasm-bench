package analysis

import (
	"encoding/json"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
	"reflect"
	"testing"
)

func breakEvenFixture() experiment.Bundle {
	b := experiment.Bundle{Manifest: experiment.Manifest{Kind: "measurement", Lock: experiment.Lock{Options: experiment.Options{Profile: "timing", Scenarios: []string{"compile", "instantiate", "trajectory"}, Launches: 3, Samples: 3, Warmup: 2, Operations: 1}, Runtimes: []experiment.Runtime{{ID: "a"}, {ID: "b"}}, Workloads: []protocol.Workload{{ID: "w", ABI: "core", Export: "run", Reset: "stateless", Oracle: protocol.Oracle{Kind: "exact_u64"}}}}}}
	for _, rt := range []string{"a", "b"} {
		for block := 0; block < 3; block++ {
			for _, scenario := range []string{"compile", "instantiate", "trajectory"} {
				n := 3
				if scenario == "trajectory" {
					n = 5
				}
				tr := experiment.Trial{ID: rt + scenario + string(rune('0'+block)), Runtime: rt, Workload: "w", Scenario: scenario, Profile: "timing", Block: block, Status: "ok"}
				for i := 0; i < n; i++ {
					elapsed := int64(10)
					if scenario == "compile" {
						elapsed = 100
						if rt == "b" {
							elapsed = 200
						}
					} else if scenario == "trajectory" {
						elapsed = 100
						if rt == "b" {
							elapsed = 20
						}
						if i == 0 {
							elapsed *= 2
						}
					}
					tr.Samples = append(tr.Samples, protocol.Sample{Index: i, Warmup: scenario == "trajectory" && i < 2, ElapsedNS: elapsed, Operations: 1, SampleType: "individual_operation", Verified: true})
				}
				b.Trials = append(b.Trials, tr)
			}
		}
	}
	return b
}
func TestBreakEvenObservedPrefixes(t *testing.T) {
	b := breakEvenFixture()
	r, err := BreakEven(b, nil, "a", "b")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Curves) != 2 || len(r.Comparisons) != 1 {
		t.Fatal(r)
	}
	a, c := r.Curves[0], r.Curves[1]
	if !reflect.DeepEqual(a.Setup, []string{"compile", "instantiate"}) || len(a.Blocks) != 3 || len(a.Trials) != 9 {
		t.Fatal(a)
	}
	for i, want := range []float64{110, 310, 410, 710} {
		if *a.Points[i].Median != want || *a.Points[i].Low != want || *a.Points[i].High != want {
			t.Fatal(a.Points)
		}
	}
	if *c.Points[3].Median != 330 {
		t.Fatal("warmup omitted or trajectory flattened", c)
	}
	p := r.Comparisons[0].Points
	if p[0].Conclusion != "baseline_faster" || p[1].Conclusion != "candidate_faster" || p[3].Median != -380 {
		t.Fatal(p)
	}
	encoded, _ := json.Marshal(r)
	again, _ := BreakEven(b, nil, "a", "b")
	encodedAgain, _ := json.Marshal(again)
	if string(encoded) != string(encodedAgain) {
		t.Fatal("nondeterministic")
	}
}
func TestBreakEvenCoverageAndValidation(t *testing.T) {
	for _, mode := range []string{"failure", "missing", "duplicate", "unverified", "batch", "order", "warmup", "profile", "negative", "truncated", "reset", "initializer"} {
		t.Run(mode, func(t *testing.T) {
			b := breakEvenFixture()
			switch mode {
			case "failure":
				b.Trials[2].Status = "timeout"
			case "missing":
				b.Trials = append(b.Trials[:2], b.Trials[3:]...)
			case "duplicate":
				b.Trials = append(b.Trials, b.Trials[2])
			case "unverified":
				b.Trials[2].Samples[0].Verified = false
			case "batch":
				b.Trials[2].Samples[0].Operations = 2
			case "order":
				b.Trials[2].Samples[0].Index = 1
			case "warmup":
				b.Trials[2].Samples[0].Warmup = false
			case "profile":
				b.Trials[2].Profile = "memory"
			case "negative":
				b.Trials[2].Samples[0].ElapsedNS = -1
			case "truncated":
				b.Trials[2].Samples = b.Trials[2].Samples[:4]
			case "reset":
				b.Manifest.Lock.Workloads[0].Reset = "fresh_instance_per_sample"
			case "initializer":
				b.Manifest.Lock.Workloads[0].Initialize = "init"
			}
			r, err := BreakEven(b, nil, "a", "b")
			if err != nil {
				t.Fatal(err)
			}
			want := 2
			if mode == "reset" || mode == "initializer" {
				want = 0
			}
			if len(r.Curves[0].Blocks) != want || len(r.Comparisons[0].Blocks) != want {
				t.Fatal(r)
			}
			for _, p := range r.Curves[0].Points {
				if p.Low != nil || p.High != nil {
					t.Fatal("false certainty", p)
				}
			}
		})
	}
	b := breakEvenFixture()
	for _, setup := range [][]string{{}, {"compile", "compile"}, {"cold-process"}, {"first-call"}, {"steady"}} {
		if _, err := BreakEven(b, setup, "", ""); err == nil {
			t.Fatal(setup)
		}
	}
	for _, mutate := range []func(*experiment.Bundle){func(b *experiment.Bundle) { b.Manifest.Kind = "check" }, func(b *experiment.Bundle) { b.Manifest.Lock.Options.Profile = "memory" }, func(b *experiment.Bundle) { b.Manifest.Lock.Options.Scenarios = []string{"steady"} }} {
		b := breakEvenFixture()
		mutate(&b)
		if _, err := BreakEven(b, nil, "", ""); err == nil {
			t.Fatal(b)
		}
	}
	if _, err := BreakEven(b, nil, "a", "absent"); err == nil {
		t.Fatal("unknown runtime")
	}
	r, err := BreakEven(b, []string{"compile"}, "", "")
	if err != nil || *r.Curves[0].Points[0].Median != 100 {
		t.Fatal(r, err)
	}
}
