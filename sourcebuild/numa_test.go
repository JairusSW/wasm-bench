package sourcebuild

import (
	"github.com/wasmbench/wasmbench/agent"
	"testing"
)

func TestNUMAReceiptBoundariesRequired(t *testing.T) {
	p := &agent.ResourcePolicy{CgroupParent: "/delegated", Mems: "0,1"}
	s := memoryStep(12)
	iso := s.Resources.Isolation
	iso.Effective["cpuset.mems"] = "0-1"
	iso.Effective["cpuset.mems.effective"] = "0-1"
	first := agent.CheckResourceReadback(*p, iso.Effective, "before_spawn")
	last := agent.CheckResourceReadback(*p, iso.Effective, "tool_exit_before_cleanup")
	iso.Verification = &first
	iso.FinalVerification = &last
	if err := validateStepResources(s, p, "memory"); err != nil {
		t.Fatal(err)
	}
	iso.FinalVerification = nil
	if validateStepResources(s, p, "memory") == nil {
		t.Fatal("missing NUMA endpoint accepted")
	}
	iso.FinalVerification = &last
	last.Effective["cpuset.mems.effective"] = "0"
	if validateStepResources(s, p, "memory") == nil {
		t.Fatal("NUMA mismatch accepted")
	}
}
