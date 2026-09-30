package agent

import "testing"

func TestHostPolicyBoundaries(t *testing.T) {
	h := Host{OS: "linux", Arch: "arm64", Hostname: "worker", CPUs: 4, PageSize: 4096, Environment: map[string]string{}, Policy: map[string]string{"frequency": "uncontrolled"}}
	p := &HostPolicy{Version: HostPolicyVersion, Expected: h}
	if err := CheckHostPolicy(p, h, "start").Err(); err != nil {
		t.Fatal(err)
	}
	h.Environment = map[string]string{"GOGC": "50"}
	c := CheckHostPolicy(p, h, "end")
	if c.Status != "mismatch" || len(c.Changed) != 1 || c.Changed[0] != "environment" {
		t.Fatalf("%+v", c)
	}
	if len(p.Expected.Environment) != 0 {
		t.Fatal("baseline mutated")
	}
	h = p.Expected
	h.CPUs++
	if CheckHostPolicy(p, h, "end").Err() == nil {
		t.Fatal("CPU change accepted")
	}
	p.Version = "future"
	if CheckHostPolicy(p, h, "end").Status != "invalid" {
		t.Fatal("invalid version accepted")
	}
	if CheckHostPolicy(nil, h, "end").Err() != nil {
		t.Fatal("legacy policy rejected")
	}
}
