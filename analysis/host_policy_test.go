package analysis

import (
	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/experiment"
	"testing"
)

func TestHostEligibilityDoesNotMutateEvidence(t *testing.T) {
	b := experiment.Bundle{Manifest: experiment.Manifest{Lock: experiment.Lock{HostPolicy: &agent.HostPolicy{}}}, Trials: []experiment.Trial{{Status: "ok"}}}
	derived := hostEligibleBundle(b)
	if derived.Trials[0].Status != "host_policy_mismatch" {
		t.Fatal("invalid host evidence eligible")
	}
	if b.Trials[0].Status != "ok" {
		t.Fatal("raw trial mutated")
	}
	if ValidatePublication(b) == nil {
		t.Fatal("invalid host evidence publishable")
	}
}

func TestPartitionEligibilityDoesNotMutateEvidence(t *testing.T) {
	b := experiment.Bundle{Manifest: experiment.Manifest{Lock: experiment.Lock{RequireIsolatedCPUPartition: true, Options: experiment.Options{Resources: agent.ResourcePolicy{CgroupParent: "/partition", CPUs: "2"}}}}, Trials: []experiment.Trial{{Status: "ok"}}}
	derived := hostEligibleBundle(b)
	if derived.Trials[0].Status != "host_policy_mismatch" || b.Trials[0].Status != "ok" {
		t.Fatal("partition eligibility not separated from raw outcome")
	}
}
