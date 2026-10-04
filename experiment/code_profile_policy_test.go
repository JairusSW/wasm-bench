package experiment

import (
	"context"
	"testing"

	"github.com/wasmbench/wasmbench/protocol"
)

func TestExplicitCodeProfilePolicy(t *testing.T) {
	for _, test := range []struct {
		name         string
		capabilities map[string]bool
		status       string
	}{
		{"unavailable", map[string]bool{"can_code_profile": false}, "unsupported"},
		{"available", map[string]bool{"can_code_profile": true}, "error"},
		{"unspecified", map[string]bool{}, "error"},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime := Runtime{ID: "test", Command: []string{"must-not-exist-code-policy-adapter"}, Description: &protocol.Description{ABIs: []string{"core"}, Scenarios: []string{"compile"}, Capabilities: test.capabilities}}
			workload := protocol.Workload{Schema: 1, ID: "test/code-policy", ABI: "core", Export: "run", Reset: "stateless", Oracle: protocol.Oracle{Kind: "exact_u64", Expected: protocol.Values{1}}}
			for _, block := range []int{-1, 0} {
				trial := runTrial(context.Background(), t.TempDir(), Options{Profile: "code", Samples: 1, Operations: 1}, runtime, workload, "compile", block, "code-policy")
				if trial.Status != test.status {
					t.Fatalf("block %d: status %s, reason %s", block, trial.Status, trial.Reason)
				}
			}
		})
	}
}
