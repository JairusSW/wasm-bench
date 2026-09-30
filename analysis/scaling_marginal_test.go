package analysis

import "testing"

func TestScalingMarginalPairedBlocks(t *testing.T) {
	b := scalingFixture()
	c := Scaling(b)[0]
	if len(c.Marginal) != 2 {
		t.Fatal(c)
	}
	for i, want := range []float64{11, 110} {
		m := c.Marginal[i]
		if m.Status != "available" || m.PairedBlocks != 3 || m.Median == nil || *m.Median != want || m.Low == nil || *m.Low != want || *m.High != want {
			t.Fatal(m)
		}
	}
	// Shared block shifts cancel before aggregation, even with an outlier.
	for i := range b.Trials {
		b.Trials[i].Samples[0].ElapsedNS += int64(b.Trials[i].Block * 10000)
	}
	if got := Scaling(b)[0].Marginal[0]; *got.Median != 11 || *got.Low != 11 {
		t.Fatal(got)
	}
	// No connecting across a missing middle point.
	for i := range b.Trials {
		if b.Trials[i].Workload == "10" {
			b.Trials[i].Status = "timeout"
		}
	}
	for _, m := range Scaling(b)[0].Marginal {
		if m.Median != nil || m.Status != "no_common_blocks" {
			t.Fatal(m)
		}
	}
}

func TestScalingMarginalNegativeAndMissingPairs(t *testing.T) {
	b := scalingFixture()
	for i := range b.Trials {
		b.Trials[i].Samples[0].ElapsedNS = int64(1000 - 2*b.Manifest.Lock.Workloads[i/3].Size)
	}
	c := Scaling(b)[0]
	for _, m := range c.Marginal {
		if *m.Median != -1 || *m.Low != -1 {
			t.Fatal(m)
		}
	}
	// Three launches at each endpoint but none in the same blocks is not paired.
	for i := range b.Trials {
		if b.Trials[i].Workload == "10" {
			b.Trials[i].Block += 3
		}
	}
	for _, m := range Scaling(b)[0].Marginal {
		if m.Median != nil || m.PairedBlocks != 0 {
			t.Fatal(m)
		}
	}
	b = scalingFixture()
	b.Trials = append(b.Trials, b.Trials[0])
	if m := Scaling(b)[0].Marginal[0]; m.Status != "duplicate_trial_cell" || m.Median != nil {
		t.Fatal(m)
	}
	b = scalingFixture()
	b.Trials = append(b.Trials[:1], b.Trials[3:]...)
	if m := Scaling(b)[0].Marginal[0]; m.Status != "insufficient_blocks" || m.PairedBlocks != 1 || m.Low != nil || m.Median == nil {
		t.Fatal(m)
	}
}
