package sourcebuild

import (
	"encoding/json"
	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/corpus"
	"testing"
	"time"
)

func sourceIRQBoundary() agent.IRQAffinityProbe {
	p := agent.IRQAffinityProbe{Version: agent.IRQAffinityVersion, Scope: agent.IRQAffinityScope, At: time.Now().UTC(), CPUs: "2", Facts: map[string]agent.HostFact{}}
	for _, boundary := range []string{"before", "after"} {
		for key, raw := range map[string]string{"online": "0-2", "default": "3", "inventory": `["44"]`, "irq/44/requested": "0-1", "irq/44/effective": "1"} {
			value := raw
			p.Facts[boundary+"/"+key] = agent.HostFact{Status: "available", Value: &value}
		}
	}
	p.Status, p.Reason = agent.CheckIRQAffinity(p)
	return p
}

func TestSourceIRQBoundaryEvidence(t *testing.T) {
	for _, mode := range []string{"ready", "missing", "unlocked", "mismatched", "forged", "failed_end", "priority_label", "missing_label"} {
		t.Run(mode, func(t *testing.T) {
			start, end := sourceIRQBoundary(), sourceIRQBoundary()
			b := BuildBenchmark{Config: BenchmarkConfig{RequireIRQAffinity: true, Resources: &agent.ResourcePolicy{CgroupParent: "/cg", CPUs: "2"}}, IRQAffinityStart: &start, IRQAffinityEnd: &end, Publication: "local_exploratory"}
			switch mode {
			case "missing":
				b.IRQAffinityEnd = nil
			case "unlocked":
				b.Config.RequireIRQAffinity = false
			case "mismatched":
				b.IRQAffinityEnd.CPUs = "1"
			case "forged":
				b.IRQAffinityEnd.Status = "ready"
			case "failed_end", "priority_label", "missing_label":
				v := "7"
				f := end.Facts["after/default"]
				f.Value = &v
				end.Facts["after/default"] = f
				end.Status, end.Reason = agent.CheckIRQAffinity(end)
				if mode != "missing_label" {
					b.Publication = "prohibited_irq_affinity_mismatch"
				}
				if mode == "priority_label" {
					// A higher-priority host failure may occupy the label; the IRQ
					// validator still independently withholds the measurement.
					b.Publication = "prohibited_cpu_partition_mismatch"
				}
			}
			valid := mode == "ready" || mode == "failed_end"
			if (b.ValidateHostEvidence() == nil) != valid {
				t.Fatal("incorrect validity", b.ValidateHostEvidence())
			}
			if b.HostBaselineAllowsMeasurements() != (mode == "ready") {
				t.Fatal("incorrect eligibility")
			}
		})
	}
}

func TestSourceIRQConfigIdentity(t *testing.T) {
	c := BenchmarkConfig{Resources: &agent.ResourcePolicy{CgroupParent: "/cg", CPUs: "2"}}
	legacy, _ := json.Marshal(c)
	c.RequireIRQAffinity = true
	locked, _ := json.Marshal(c)
	if corpus.Hash(legacy) == corpus.Hash(locked) {
		t.Fatal("requirement not part of configuration identity")
	}
	var copy BenchmarkConfig
	if err := json.Unmarshal(locked, &copy); err != nil {
		t.Fatal(err)
	}
	if !copy.RequireIRQAffinity || copy.Resources.CPUs != "2" {
		t.Fatal("replay configuration lost requirement")
	}
	for _, resources := range []*agent.ResourcePolicy{nil, {}, {CgroupParent: "/cg"}, {CgroupParent: "relative", CPUs: "2"}} {
		c.Resources = resources
		if err := c.validate(); err == nil {
			t.Fatal("unbudgeted IRQ policy accepted")
		}
	}
	if !(BuildBenchmark{}).HostBaselineAllowsMeasurements() {
		t.Fatal("legacy evidence became unreadable")
	}
}
