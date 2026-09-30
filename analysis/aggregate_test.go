package analysis

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"

	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

func aggregateFixture() (experiment.Bundle, AggregateSet) {
	b := experiment.Bundle{}
	b.Manifest.Kind = "measurement"
	b.Manifest.Lock.Options = experiment.Options{Profile: "timing", Scenarios: []string{"steady"}, Launches: 3}
	b.Manifest.Lock.Runtimes = []experiment.Runtime{{ID: "a"}, {ID: "b"}}
	for i, ratio := range []int{2, 8, 1} {
		id := fmt.Sprint(i)
		family := "algorithms"
		if i == 2 {
			family = "applications"
		}
		b.Manifest.Lock.Workloads = append(b.Manifest.Lock.Workloads, protocol.Workload{ID: id, Family: family, Artifact: "/original/fixture.wasm", SHA256: "artifact", Reset: "stateless", Oracle: protocol.Oracle{Kind: "exact_u64", Expected: protocol.Values{7}}})
		for block := 0; block < 3; block++ {
			for _, rt := range []string{"a", "b"} {
				ns := int64(100)
				if rt == "b" {
					ns *= int64(ratio)
				}
				b.Trials = append(b.Trials, experiment.Trial{ID: fmt.Sprintf("%d/%s/%d", i, rt, block), Runtime: rt, Workload: id, Block: block, Scenario: "steady", Profile: "timing", Status: "ok", Samples: []protocol.Sample{{ElapsedNS: ns, Operations: 1, Verified: true}, {Warmup: true, ElapsedNS: 999999, Operations: 1, Verified: true}}})
			}
		}
	}
	s, _ := NewAggregateSet(b, "fixture-v1", "steady")
	return b, s
}

func TestAggregateWeightsAndPinnedContracts(t *testing.T) {
	b, s := aggregateFixture()
	// Category medians are 4 and 1; equal category weights yield 2, not
	// the unweighted workload geometric mean cubert(16).
	r, e := Aggregate(b, s, "a", "b")
	if e != nil {
		t.Fatal(e)
	}
	if r.Overall.Status != "available" || math.Abs(*r.Overall.Ratio-2) > 1e-12 || r.Overall.Low == nil || r.Overall.Required != 3 || len(r.Overall.Blocks) != 3 {
		t.Fatal(r)
	}
	s.Categories[0].Weight = 3
	r, e = Aggregate(b, s, "a", "b")
	if e != nil || math.Abs(*r.Overall.Ratio-math.Pow(4, .75)) > 1e-12 {
		t.Fatal(r, e)
	}
	old := r.SetSHA256
	b.Manifest.Lock.Workloads[0].Artifact = "/bundled/fixture.wasm"
	r, e = Aggregate(b, s, "a", "b")
	if e != nil || r.Overall.Ratio == nil || r.SetSHA256 != old {
		t.Fatal(r, e)
	}
	b.Manifest.Lock.Workloads[0].Reset = "fresh_instance_per_sample"
	r, e = Aggregate(b, s, "a", "b")
	if e != nil || r.Overall.Ratio != nil || r.Coverage[0].Status != "contract_mismatch" {
		t.Fatal(r, e)
	}
	if _, e = json.Marshal(r); e != nil {
		t.Fatal(e)
	}
}

func TestAggregateNoSurvivorScoring(t *testing.T) {
	b, s := aggregateFixture()
	for i := range b.Trials {
		if b.Trials[i].Workload == "1" && b.Trials[i].Runtime == "b" {
			b.Trials[i].Status = "unsupported"
		}
	}
	r, e := Aggregate(b, s, "a", "b")
	if e != nil || r.Overall.Ratio != nil || r.Overall.Status != "incomplete_coverage" || r.Overall.Covered != 2 || r.Coverage[1].CandidateOutcomes["unsupported"] != 3 {
		t.Fatal(r, e)
	}
	// A category with full coverage may still be viewed, but never silently
	// replaces the required whole-set aggregate.
	if r.Categories[1].Ratio == nil {
		t.Fatal(r)
	}
	b, s = aggregateFixture()
	b.Trials = b.Trials[2:]
	r, e = Aggregate(b, s, "a", "b")
	if e != nil || r.Overall.Status != "insufficient_blocks" || len(r.Overall.Blocks) != 2 || r.Overall.Low != nil {
		t.Fatal(r, e)
	}
	b, s = aggregateFixture()
	for i := range b.Trials {
		v := &b.Trials[i]
		if (v.Workload == "0" && v.Block != 0) || (v.Workload == "1" && v.Block == 0) {
			v.Status = "timeout"
		}
	}
	r, e = Aggregate(b, s, "a", "b")
	if e != nil || r.Overall.Ratio != nil || r.Overall.Status != "no_common_complete_blocks" {
		t.Fatal(r, e)
	}
}

func TestAggregateValidation(t *testing.T) {
	for _, mode := range []string{"check", "memory", "same-runtime", "missing-runtime", "duplicate-trial", "duplicate-member", "zero-weight", "nan-weight", "empty-category", "bad-digest", "missing-workload", "zero-timer"} {
		t.Run(mode, func(t *testing.T) {
			b, s := aggregateFixture()
			a, z := "a", "b"
			switch mode {
			case "check":
				b.Manifest.Kind = "correctness_only"
			case "memory":
				b.Manifest.Lock.Options.Profile = "memory"
			case "same-runtime":
				z = a
			case "missing-runtime":
				z = "missing"
			case "duplicate-trial":
				b.Trials = append(b.Trials, b.Trials[0])
			case "duplicate-member":
				s.Categories[1].Workloads = append(s.Categories[1].Workloads, s.Categories[0].Workloads[0])
			case "zero-weight":
				s.Categories[0].Weight = 0
			case "nan-weight":
				s.Categories[0].Weight = math.NaN()
			case "empty-category":
				s.Categories[0].Workloads = nil
			case "bad-digest":
				s.Categories[0].Workloads[0].ContractSHA256 = "bad"
			case "missing-workload":
				b.Manifest.Lock.Workloads = b.Manifest.Lock.Workloads[1:]
			case "zero-timer":
				for i := range b.Trials {
					b.Trials[i].Samples[0].ElapsedNS = 0
				}
			}
			r, e := Aggregate(b, s, a, z)
			if mode == "missing-workload" || mode == "zero-timer" {
				if e != nil || r.Overall.Ratio != nil {
					t.Fatal(r, e)
				}
			} else if e == nil {
				t.Fatal("invalid contract accepted", mode)
			}
		})
	}
}

func TestAggregateLaunchWeightAndMissingOutcomes(t *testing.T) {
	b, s := aggregateFixture()
	// One noisy launch gets many inner samples, not many independent votes.
	for i := range b.Trials {
		tr := &b.Trials[i]
		if tr.Runtime == "b" && tr.Block == 0 {
			tr.Samples = []protocol.Sample{}
			for j := 0; j < 100; j++ {
				tr.Samples = append(tr.Samples, protocol.Sample{ElapsedNS: 1000000, Operations: 1, Verified: true})
			}
		}
	}
	r, e := Aggregate(b, s, "a", "b")
	if e != nil || math.Abs(*r.Overall.Ratio-2) > 1e-12 {
		t.Fatal(r, e)
	}
	b.Trials = b.Trials[2:]
	r, e = Aggregate(b, s, "a", "b")
	if e != nil || r.Coverage[0].BaselineOutcomes["not_attempted"] != 1 || r.Coverage[0].CandidateOutcomes["not_attempted"] != 1 {
		t.Fatal(r, e)
	}
	b.Manifest.Lock.Runtimes = []experiment.Runtime{{ID: "a"}, {ID: "a"}}
	if _, e = Aggregate(b, s, "a", "b"); e == nil {
		t.Fatal("duplicate baseline hid missing candidate")
	}
}
