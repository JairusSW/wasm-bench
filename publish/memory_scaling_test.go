package publish

import (
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
	"testing"
)

func TestPairedMemoryScalingKeepsMatchedContractsAndFailureLinks(t *testing.T) {
	b := experiment.Bundle{Manifest: experiment.Manifest{Kind: "measurement", Lock: experiment.Lock{Options: experiment.Options{Profile: "memory"}, Workloads: []protocol.Workload{{ID: "one", Generator: "g", Dimension: "instances", Size: 1}, {ID: "four", Generator: "g", Dimension: "instances", Size: 4}, {ID: "unmatched", Generator: "g", Dimension: "instances", Size: 16}}}}}
	for _, w := range b.Manifest.Lock.Workloads {
		for _, r := range []string{"r", "different"} {
			b.Trials = append(b.Trials, experiment.Trial{ID: r + "/" + w.ID, Runtime: r, Workload: w.ID, Scenario: "guest-density", Profile: "memory", Status: "ok", Samples: []protocol.Sample{{Operations: 1, Verified: true, Observations: []protocol.Observation{{Metric: "host.alloc.bytes", Value: protocol.Value(float64(w.Size)), Status: "available", Profile: "memory"}}}}})
		}
	}
	b.Trials = append(b.Trials, experiment.Trial{ID: "failed", Runtime: "r", Workload: "four", Scenario: "guest-density", Profile: "memory", Status: "unsupported"})
	cs, links := pairedMemoryScaling(b, map[string]bool{"r\x00one": true, "r\x00four": true})
	if len(cs) != 1 || len(cs[0].Points) != 2 || cs[0].Runtime != "r" || cs[0].Profile != "memory" || len(links) != 3 {
		t.Fatal("unmatched contracts entered curves or failures lost", cs, links)
	}
	if len(b.Manifest.Lock.Workloads) != 3 || len(b.Trials) != 7 {
		t.Fatal("mutated input bundle")
	}
}
