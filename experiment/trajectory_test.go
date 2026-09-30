package experiment

import (
	"github.com/wasmbench/wasmbench/protocol"
	"testing"
)

func TestTrajectorySampleSequence(t *testing.T) {
	r := protocol.RunRequest{Scenario: "trajectory", Samples: 1, Warmup: 1, Operations: 99}
	good := []protocol.Sample{{Index: 0, Warmup: true, Operations: 1, SampleType: "individual_operation"}, {Index: 1, Operations: 1, SampleType: "individual_operation"}}
	if err := validateSampleSequence(r, good); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func([]protocol.Sample){func(s []protocol.Sample) { s[0].Warmup = false }, func(s []protocol.Sample) { s[1].Operations = 99 }, func(s []protocol.Sample) { s[0].SampleType = "batch_average" }, func(s []protocol.Sample) { s[1].Index = 0 }} {
		bad := append([]protocol.Sample{}, good...)
		change(bad)
		if validateSampleSequence(r, bad) == nil {
			t.Fatal(bad)
		}
	}
}
