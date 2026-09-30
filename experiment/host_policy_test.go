package experiment

import (
	"github.com/wasmbench/wasmbench/agent"
	"testing"
)

func TestHostEvidence(t *testing.T) {
	h := agent.Host{OS: "linux", Arch: "arm64", Hostname: "worker", CPUs: 4, PageSize: 4096, Environment: map[string]string{}, Policy: map[string]string{}}
	p := &agent.HostPolicy{Version: agent.HostPolicyVersion, Expected: h}
	start := agent.CheckHostPolicy(p, h, "before_run_preparation")
	end := agent.CheckHostPolicy(p, h, "after_trials_before_seal")
	m := Manifest{Lock: Lock{HostPolicy: p}, Host: h, HostEnd: &h, HostStartCheck: &start, HostEndCheck: &end}
	if !HostBaselineAllowsMeasurements(m) {
		t.Fatal("matched baseline rejected")
	}
	changed := h
	changed.CPUs++
	m.HostEnd = &changed
	if ValidateHostEvidence(m) == nil {
		t.Fatal("forged end check accepted")
	}
	end = agent.CheckHostPolicy(p, changed, "after_trials_before_seal")
	if ValidateHostEvidence(m) == nil {
		t.Fatal("missing prohibition accepted")
	}
	m.Publication = "prohibited_host_baseline_mismatch"
	if err := ValidateHostEvidence(m); err != nil {
		t.Fatal(err)
	}
	if HostBaselineAllowsMeasurements(m) {
		t.Fatal("mismatched baseline eligible")
	}
	m.HostEnd = nil
	if ValidateHostEvidence(m) == nil {
		t.Fatal("missing boundary accepted")
	}
	if !HostBaselineAllowsMeasurements(Manifest{}) {
		t.Fatal("legacy bundle rejected")
	}
}
