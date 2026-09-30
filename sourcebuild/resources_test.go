package sourcebuild

import (
	"testing"

	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/protocol"
)

func memoryStep(value float64) StepResult {
	r := &agent.ToolExecution{WallNS: 10, Isolation: &agent.Isolation{Mode: "cgroup_v2_at_spawn", Path: "/delegated/tool", Effective: map[string]string{"cpuset.cpus": "1-3"}}}
	for _, metric := range []string{"current", "peak"} {
		quality := "boundary_snapshot_only"
		if metric == "peak" {
			quality = "kernel_accounted_peak"
		}
		r.Observations = append(r.Observations, protocol.Observation{Metric: "source.build.cgroup." + metric, DefinitionVersion: 1, Unit: "bytes", Scope: "tool_step_cgroup", Phase: "tool_step/post_wait", Collector: "cgroup_v2", CollectorVersion: "1", Quality: quality, Profile: "memory", Denominator: "tool_step_cgroup", Status: "available", Value: protocol.Value(value)})
	}
	return StepResult{ElapsedNS: 10, Resources: r}
}

func TestSourceMemoryEvidence(t *testing.T) {
	p := &agent.ResourcePolicy{CgroupParent: "/delegated", CPUs: "3,1,2"}
	if err := validateStepResources(memoryStep(12), p, "memory"); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*StepResult){
		func(s *StepResult) { s.Resources.OOM = true },
		func(s *StepResult) { s.Resources.CleanupError = "failed" },
		func(s *StepResult) { s.Resources.WallNS++ },
		func(s *StepResult) { s.Resources.Isolation.Path = "/other/tool" },
		func(s *StepResult) { s.Resources.Isolation.Effective["cpuset.cpus"] = "1-2" },
		func(s *StepResult) { s.Resources.Observations[0].Unit = "ns" },
		func(s *StepResult) { s.Resources.Observations[0].Value = protocol.Value(-1) },
		func(s *StepResult) { s.Resources.Observations[0].Value = protocol.Value(1.5) },
		func(s *StepResult) { s.Resources.Observations[0] = s.Resources.Observations[1] },
	} {
		s := memoryStep(12)
		mutate(&s)
		if validateStepResources(s, p, "memory") == nil {
			t.Fatal("accepted invalid evidence", s)
		}
	}
	if validateStepResources(memoryStep(12), p, "timing") == nil {
		t.Fatal("memory leaked into timing")
	}
	if got := maximumStepPeak([]StepResult{memoryStep(12), memoryStep(20)}); got == nil || *got != 20 {
		t.Fatal("peaks summed", got)
	}
	if got := maximumStepPeak([]StepResult{memoryStep(0)}); got == nil || *got != 0 {
		t.Fatal("lost zero")
	}
	s := memoryStep(12)
	s.Resources.Observations[1].Value = nil
	s.Resources.Observations[1].Status = "unavailable"
	s.Resources.Observations[1].Reason = "missing kernel interface"
	if err := validateStepResources(s, p, "memory"); err != nil {
		t.Fatal(err)
	}
	if maximumStepPeak([]StepResult{memoryStep(20), s}) != nil {
		t.Fatal("missing peak treated as zero")
	}
	s.Resources.OOM = true
	if buildFailureStatus(Result{Steps: []StepResult{s}}) != "oom" {
		t.Fatal("lost OOM classification")
	}
}

func TestCPUListNormalization(t *testing.T) {
	for _, input := range []string{"3,1,2", "1-3", "1-2,2-3"} {
		got, err := normalizedCPUList(input)
		if err != nil || got != "1-3" {
			t.Fatal(input, got, err)
		}
	}
	for _, input := range []string{"", "3-1", "1-2-3", "4294967296", "1,,2"} {
		if _, err := normalizedCPUList(input); err == nil {
			t.Fatal("accepted", input)
		}
	}
	if got, err := normalizedCPUList("0-4294967295"); err != nil || got != "0-4294967295" {
		t.Fatal(got, err)
	}
}
