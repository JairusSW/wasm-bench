package experiment

import (
	"github.com/wasmbench/wasmbench/agent"
	"testing"
)

func TestMatchHostMeasurementPolicy(t *testing.T) {
	for _, change := range []func(*Lock){
		func(l *Lock) { l.Options.Resources.CPUs = "2" },
		func(l *Lock) { l.Options.Resources.CgroupParent = "/cg" },
		func(l *Lock) { l.Options.Resources.Mems = "1" },
		func(l *Lock) { l.Options.Resources.CPUQuotaUS = 10000 },
		func(l *Lock) { l.Options.Resources.MemoryMaxBytes = 1024 },
		func(l *Lock) { l.Options.Resources.DisableSwap = true },
		func(l *Lock) { l.Options.Resources.PidsMax = 16 },
		func(l *Lock) { l.RequireIRQAffinity = true },
		func(l *Lock) { l.RequireIsolatedCPUPartition = true },
		func(l *Lock) { l.HostPolicy = &agent.HostPolicy{Version: agent.HostPolicyVersion} },
	} {
		a, b := Lock{}, Lock{}
		change(&b)
		if MatchHostMeasurementPolicy(a, b) == nil || MatchHostMeasurementPolicy(b, a) == nil {
			t.Fatal("policy mismatch accepted")
		}
		if err := MatchHostMeasurementPolicy(b, b); err != nil {
			t.Fatal(err)
		}
	}
	a, b := Lock{}, Lock{}
	a.HostPolicy = &agent.HostPolicy{Version: agent.HostPolicyVersion, Expected: agent.Host{Environment: map[string]string{"GOGC": "100"}}}
	b.HostPolicy = &agent.HostPolicy{Version: agent.HostPolicyVersion, Expected: agent.Host{Environment: map[string]string{"GOGC": "100"}}}
	a.RequireIRQAffinity, b.RequireIRQAffinity = true, true
	if err := MatchHostMeasurementPolicy(a, b); err != nil {
		t.Fatal("same values with independent maps rejected", err)
	}
	b.HostPolicy.Expected.Environment["GOGC"] = "50"
	if MatchHostMeasurementPolicy(a, b) == nil {
		t.Fatal("changed baseline accepted")
	}
	if MatchHostMeasurementPolicy(Lock{}, Lock{}) != nil {
		t.Fatal("legacy policy rejected")
	}
}
