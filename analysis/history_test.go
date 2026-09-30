package analysis

import (
	"reflect"
	"testing"

	"github.com/wasmbench/wasmbench/experiment"
)

func TestHistoryFixedBaselineAndCoverage(t *testing.T) {
	a := regressionFixture("a", []int64{10, 10, 10})
	b := regressionFixture("b", []int64{20, 20, 20})
	c := regressionFixture("c", []int64{30, 30, 30})
	bad := regressionFixture("other-host", []int64{1, 1, 1})
	bad.Manifest.Host.Arch = "other"
	changed := regressionFixture("changed-artifact", []int64{1, 1, 1})
	changed.Manifest.Lock.Workloads[0].SHA256 = "different"
	bundles := []experiment.Bundle{a, b, bad, c, changed}
	r, err := History(bundles, "runtime")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Points) != 5 || r.Points[0].Comparison != nil || r.Points[0].Status != "baseline" {
		t.Fatal(r)
	}
	if len(r.Points[0].Summaries) != 1 || len(r.Points[2].Summaries) != 1 {
		t.Fatal("absolute timing history missing")
	}
	if *r.Points[1].Comparison.Results[0].Ratio != 2 || *r.Points[3].Comparison.Results[0].Ratio != 3 {
		t.Fatal("not fixed baseline", r)
	}
	if r.Points[2].Status != "incomparable" || r.Points[2].Reason == "" || r.Points[2].Comparison != nil {
		t.Fatal("incompatible run dropped", r)
	}
	if r.Points[4].Comparison.Results[0].Ratio != nil || r.Points[4].Comparison.Results[0].Status != "incomparable" {
		t.Fatal("changed artifact compared", r)
	}
	again, _ := History(bundles, "runtime")
	if !reflect.DeepEqual(r, again) {
		t.Fatal("nondeterministic history")
	}
}

func TestHistoryRejectsAmbiguousBaseline(t *testing.T) {
	a := regressionFixture("a", []int64{10, 10, 10})
	b := regressionFixture("b", []int64{10, 10, 10})
	for _, runs := range [][]experiment.Bundle{nil, {a}, {a, a}} {
		if _, err := History(runs, "runtime"); err == nil {
			t.Fatal("invalid history accepted")
		}
	}
	if _, err := History([]experiment.Bundle{a, b}, "missing"); err == nil {
		t.Fatal("missing baseline runtime accepted")
	}
	a.Manifest.Kind = "correctness_only"
	if _, err := History([]experiment.Bundle{a, b}, "runtime"); err == nil {
		t.Fatal("correctness baseline accepted")
	}
}
