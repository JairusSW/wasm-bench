package analysis

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

func regressionFixture(id string, values []int64) experiment.Bundle {
	w := protocol.Workload{Schema: 1, ID: "work", SHA256: "same-wasm", ABI: "core", Export: "run", Reset: "stateless", Units: 1, Oracle: protocol.Oracle{Kind: "exact_u64", Expected: protocol.Values{7}}}
	b := experiment.Bundle{Manifest: experiment.Manifest{ID: id, Kind: "measurement", Lock: experiment.Lock{Protocol: 1, RunnerSHA256: "runner", Options: experiment.Options{Profile: "timing", Scenarios: []string{"steady"}, Launches: len(values), Samples: 1, Operations: 1, Timeout: time.Second}, Workloads: []protocol.Workload{w}, Runtimes: []experiment.Runtime{{ID: "runtime"}}}}}
	for block, n := range values {
		b.Trials = append(b.Trials, experiment.Trial{Runtime: "runtime", Workload: "work", Scenario: "steady", Profile: "timing", Block: block, Status: "ok", Samples: []protocol.Sample{{ElapsedNS: n, Operations: 1, Verified: true}}})
	}
	return b
}
func TestCrossRunDoesNotPairUnrelatedBlocks(t *testing.T) {
	a := regressionFixture("a", []int64{1, 2, 100})
	b := regressionFixture("b", []int64{2, 100, 1})
	r, e := CompareRuns(a, b, "runtime", "runtime")
	if e != nil {
		t.Fatal(e)
	}
	if len(r.Results) != 1 || r.Results[0].Ratio == nil || *r.Results[0].Ratio != 1 {
		t.Fatalf("used paired ratios instead of independent medians: %+v", r)
	}
	if r.Results[0].Low == nil || r.Results[0].High == nil || r.Results[0].Status != "uncertain" {
		t.Fatal(r.Results[0])
	}
}
func TestCrossRunRejectsChangedProtocolAndHost(t *testing.T) {
	for _, field := range []string{"profile", "warmup", "operations", "timeout", "host", "resources", "irq", "partition", "baseline"} {
		t.Run(field, func(t *testing.T) {
			a := regressionFixture("a", []int64{10, 10, 10})
			b := regressionFixture("b", []int64{20, 20, 20})
			switch field {
			case "irq":
				b.Manifest.Lock.RequireIRQAffinity = true
			case "partition":
				b.Manifest.Lock.RequireIsolatedCPUPartition = true
			case "baseline":
				b.Manifest.Lock.HostPolicy = &agent.HostPolicy{Version: agent.HostPolicyVersion}
			case "resources":
				b.Manifest.Lock.Options.Resources.MemoryMaxBytes = 1024
			case "profile":
				b.Manifest.Lock.Options.Profile = "memory"
			case "warmup":
				b.Manifest.Lock.Options.Warmup++
			case "operations":
				b.Manifest.Lock.Options.Operations++
			case "timeout":
				b.Manifest.Lock.Options.Timeout++
			case "host":
				b.Manifest.Host.Arch = "different"
			}
			if _, e := CompareRuns(a, b, "runtime", "runtime"); e == nil {
				t.Fatal("accepted mismatched " + field)
			}
		})
	}
}

func TestCrossRunRejectsChangedHostFacts(t *testing.T) {
	for _, field := range []string{"microcode", "governor", "topology", "numa", "availability", "version", "legacy"} {
		t.Run(field, func(t *testing.T) {
			a, b := regressionFixture("a", []int64{1, 2, 3}), regressionFixture("b", []int64{1, 2, 3})
			for _, bundle := range []*experiment.Bundle{&a, &b} {
				v := "observed"
				bundle.Manifest.Host.Fingerprint = &agent.HostFingerprint{Version: agent.HostFingerprintVersion, Facts: map[string]agent.HostFact{field: {Source: "test", Status: "available", Value: &v}}}
			}
			switch field {
			case "version":
				b.Manifest.Host.Fingerprint.Version = "different"
			case "legacy":
				b.Manifest.Host.Fingerprint = nil
			case "availability":
				b.Manifest.Host.Fingerprint.Facts[field] = agent.HostFact{Source: "test", Status: "permission_denied", Reason: "denied"}
			default:
				v := "changed"
				b.Manifest.Host.Fingerprint.Facts[field] = agent.HostFact{Source: "test", Status: "available", Value: &v}
			}
			if _, err := CompareRuns(a, b, "runtime", "runtime"); err == nil {
				t.Fatal("accepted changed", field)
			}
		})
	}
}
func TestCrossRunKeepsMissingAndChangedWorkloads(t *testing.T) {
	a := regressionFixture("a", []int64{10, 10, 10})
	b := regressionFixture("b", []int64{20, 20, 20})
	extra := a.Manifest.Lock.Workloads[0]
	extra.ID = "missing"
	a.Manifest.Lock.Workloads = append(a.Manifest.Lock.Workloads, extra)
	b.Manifest.Lock.Workloads[0].SHA256 = "different"
	r, e := CompareRuns(a, b, "runtime", "runtime")
	if e != nil {
		t.Fatal(e)
	}
	if len(r.Results) != 2 || len(r.CommonSubset) != 0 {
		t.Fatal(r)
	}
	for _, cell := range r.Results {
		if cell.Ratio != nil || cell.Low != nil || cell.High != nil {
			t.Fatal("fabricated a missing value")
		}
	}
	encoded, _ := json.Marshal(r)
	if !strings.Contains(string(encoded), `"candidate_over_baseline":null`) {
		t.Fatal(string(encoded))
	}
}
func TestCrossRunRegressionAndSmallSample(t *testing.T) {
	a := regressionFixture("a", []int64{10, 10, 10})
	b := regressionFixture("b", []int64{20, 20, 20})
	r, e := CompareRuns(a, b, "runtime", "runtime")
	if e != nil {
		t.Fatal(e)
	}
	if r.Results[0].Status != "observed_slower" || *r.Results[0].Ratio != 2 {
		t.Fatal(r.Results)
	}
	b.Trials = b.Trials[:1]
	r, e = CompareRuns(a, b, "runtime", "runtime")
	if e != nil {
		t.Fatal(e)
	}
	if r.Results[0].Status != "insufficient_launches" || r.Results[0].Low != nil {
		t.Fatal(r.Results)
	}
}
