package analysis

import (
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
	"testing"
)

func TestTierFailurePrefixesNeverProduceLatencyOrThroughput(t *testing.T) {
	b := experiment.Bundle{Manifest: experiment.Manifest{Kind: "measurement", Lock: experiment.Lock{Options: experiment.Options{Profile: "profiling"}, Workloads: []protocol.Workload{{ID: "w", WorkUnit: "invocation", Units: 1}}}}, Trials: []experiment.Trial{{Runtime: "v8-tier-observed", Workload: "w", Scenario: "trajectory", Profile: "profiling", Status: "error", Samples: []protocol.Sample{{Index: 0, Operations: 1, ElapsedNS: 10, Verified: true}, {Index: 1, Operations: 1, ElapsedNS: 20, Verified: false, TierWindow: &protocol.TierWindow{Version: 2, InvocationOutcome: "guest_trap", FailureReason: "unreachable"}}}}}}
	s := Summarize(b)
	if len(s) != 1 || s[0].Median != nil || s[0].Mean != nil || s[0].Launches != 0 || s[0].SuccessfulLaunches != 0 || s[0].RecordedSamples != 2 || s[0].Attempted != 1 || s[0].Failures["error"] != 1 {
		t.Fatal("partial successes became headline estimates or failures disappeared", s)
	}
	rates := Throughput(b)
	if len(rates) != 1 || rates[0].Median != nil || rates[0].Launches != 0 {
		t.Fatal("failed tier prefix produced throughput", rates)
	}
}
