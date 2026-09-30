package sourcebuild

import (
	"github.com/wasmbench/wasmbench/agent"
	"testing"
)

func TestSourcePartitionRequirementCannotBeDropped(t *testing.T) {
	b := BuildBenchmark{Config: BenchmarkConfig{RequireIsolatedCPUPartition: true, Resources: &agent.ResourcePolicy{CgroupParent: "/partition", CPUs: "2"}}}
	if b.ValidateHostEvidence() == nil || b.HostBaselineAllowsMeasurements() {
		t.Fatal("missing source partition endpoints accepted")
	}
	p := agent.CPUPartitionProbe{Version: agent.CPUPartitionVersion, Scope: agent.CPUPartitionScope, Path: "/partition", CPUs: "2", Status: "unavailable", Reason: "missing partition"}
	b.CPUPartitionStart = &p
	b.CPUPartitionEnd = &p
	if b.ValidateHostEvidence() == nil {
		t.Fatal("failed start accepted")
	}
	b.Config.RequireIsolatedCPUPartition = false
	if b.ValidateHostEvidence() == nil {
		t.Fatal("receipt accepted without requirement")
	}
	b.CPUPartitionStart = nil
	b.CPUPartitionEnd = nil
	if !b.HostBaselineAllowsMeasurements() {
		t.Fatal("legacy source benchmark rejected")
	}
}
