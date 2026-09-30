package harness

import (
	"github.com/wasmbench/wasmbench/protocol"
	"testing"
)

func TestBookkeepingAndBounds(t *testing.T) {
	p := &protocol.Preparation{Profile: "timing", Workload: protocol.Workload{ABI: "core", Export: "benchmark", Reset: "stateless", Oracle: protocol.Oracle{Kind: "exact_u64"}}}
	for _, ops := range []int{1, 257} {
		s, err := Run(p, &protocol.RunRequest{Scenario: protocol.HarnessCalibrationScenario, Samples: 2, Operations: ops})
		if err != nil || len(s) != 2 {
			t.Fatal(s, err)
		}
		for i, v := range s {
			if v.Index != i || !v.Verified || v.Operations != ops || v.Result[0] != uint64(ops) || len(v.Observations) != 0 {
				t.Fatal(v)
			}
		}
	}
	if verify([]uint64{1, 9, 3}) || !verify([]uint64{1, 2, 3}) {
		t.Fatal("all bookkeeping slots must be checked")
	}
	for _, mutation := range []string{"memory", "warmup", "barriers", "empty", "oversized"} {
		q := *p
		r := protocol.RunRequest{Scenario: protocol.HarnessCalibrationScenario, Samples: 1, Operations: 1}
		switch mutation {
		case "memory":
			q.Profile = "memory"
		case "warmup":
			r.Warmup = 1
		case "barriers":
			r.PhaseBarriers = true
		case "empty":
			r.Operations = 0
		case "oversized":
			r.Operations = 1000001
		}
		if _, err := Run(&q, &r); err == nil {
			t.Fatal("invalid calibration accepted", mutation)
		}
	}
}
