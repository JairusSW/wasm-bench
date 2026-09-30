package experiment

import (
	"github.com/wasmbench/wasmbench/agent"
	"testing"
	"time"
)

func irqBoundary() agent.IRQAffinityProbe {
	p := agent.IRQAffinityProbe{Version: agent.IRQAffinityVersion, Scope: agent.IRQAffinityScope, At: time.Now().UTC(), CPUs: "2-3", Facts: map[string]agent.HostFact{}}
	for _, boundary := range []string{"before", "after"} {
		for key, raw := range map[string]string{"online": "0-3", "default": "3", "inventory": `["44"]`, "irq/44/requested": "0-1", "irq/44/effective": "1"} {
			value := raw
			p.Facts[boundary+"/"+key] = agent.HostFact{Status: "available", Value: &value}
		}
	}
	p.Status, p.Reason = agent.CheckIRQAffinity(p)
	return p
}

func irqManifest() Manifest {
	start, end := irqBoundary(), irqBoundary()
	return Manifest{Lock: Lock{RequireIRQAffinity: true, Options: Options{Resources: agent.ResourcePolicy{CPUs: "2-3", CgroupParent: "/cg"}}}, IRQAffinityStart: &start, IRQAffinityEnd: &end, Publication: "local_exploratory"}
}

func TestLockedIRQAffinityEvidence(t *testing.T) {
	for _, tc := range []struct {
		name             string
		change           func(*Manifest)
		valid, estimates bool
	}{
		{"ready", func(m *Manifest) {}, true, true},
		{"legacy", func(m *Manifest) { *m = Manifest{} }, true, true},
		{"missing end", func(m *Manifest) { m.IRQAffinityEnd = nil }, false, false},
		{"unlocked", func(m *Manifest) { m.Lock.RequireIRQAffinity = false }, false, false},
		{"different cpus", func(m *Manifest) { m.IRQAffinityEnd.CPUs = "2" }, false, false},
		{"forged status", func(m *Manifest) { m.IRQAffinityEnd.Status = "ready" }, false, false},
		{"missing fact", func(m *Manifest) { delete(m.IRQAffinityEnd.Facts, "after/default") }, false, false},
		{"reversed", func(m *Manifest) { m.IRQAffinityEnd.At = m.IRQAffinityStart.At.Add(-time.Second) }, false, false},
		{"false prohibition", func(m *Manifest) { m.Publication = "prohibited_irq_affinity_mismatch" }, false, false},
		{"sealed end failure", func(m *Manifest) {
			v := "f"
			f := m.IRQAffinityEnd.Facts["after/default"]
			f.Value = &v
			m.IRQAffinityEnd.Facts["after/default"] = f
			m.IRQAffinityEnd.Status, m.IRQAffinityEnd.Reason = agent.CheckIRQAffinity(*m.IRQAffinityEnd)
			m.Publication = "prohibited_irq_affinity_mismatch"
		}, true, false},
		{"failure without label", func(m *Manifest) {
			v := "f"
			f := m.IRQAffinityEnd.Facts["after/default"]
			f.Value = &v
			m.IRQAffinityEnd.Facts["after/default"] = f
			m.IRQAffinityEnd.Status, m.IRQAffinityEnd.Reason = agent.CheckIRQAffinity(*m.IRQAffinityEnd)
		}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := irqManifest()
			tc.change(&m)
			err := ValidateHostEvidence(m)
			if (err == nil) != tc.valid {
				t.Fatal(err)
			}
			if HostBaselineAllowsMeasurements(m) != tc.estimates {
				t.Fatal("incorrect estimate eligibility")
			}
		})
	}
}

func TestIRQAffinityPolicy(t *testing.T) {
	for _, cpus := range []string{"", "3-2"} {
		l := irqManifest().Lock
		l.Options.Resources.CPUs = cpus
		if validateIRQPolicy(l) == nil {
			t.Fatal("accepted invalid CPUs")
		}
	}
	l := irqManifest().Lock
	l.Options.Resources.CgroupParent = ""
	if validateIRQPolicy(l) == nil {
		t.Fatal("accepted unbudgeted CPUs")
	}
}
