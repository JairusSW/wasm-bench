package experiment

import (
	"github.com/wasmbench/wasmbench/protocol"
	"testing"
)

func TestTeardownSampleBoundary(t *testing.T) {
	r := protocol.RunRequest{Scenario: "teardown", Samples: 1, Operations: 9, Warmup: 4}
	s := protocol.Sample{Operations: 1, SampleType: "individual_operation"}
	if err := validateSampleSequence(r, []protocol.Sample{s}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []protocol.Sample{
		{Operations: 9, SampleType: "batch_average"},
		{Operations: 1, SampleType: "batch_average"},
		{Operations: 1, SampleType: "individual_operation", Warmup: true},
	} {
		if validateSampleSequence(r, []protocol.Sample{bad}) == nil {
			t.Fatal("accepted invalid release boundary", bad)
		}
	}
}
