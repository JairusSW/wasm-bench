package analysis

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

func TestPublicationControllerSeparationRequiresV2(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		b, _, _ := confirmationFixture(t)
		b.Manifest.Lock.RequireIsolatedCPUPartition = true
		b.Manifest.Lock.Options.Resources = agent.ResourcePolicy{CgroupParent: "/cg", CPUs: "2"}
		fact := func(value string) agent.HostFact {
			return agent.HostFact{Source: "fixture", Status: "available", Value: &value}
		}
		p := agent.CPUPartitionProbe{Version: agent.CPUPartitionVersion, Scope: agent.CPUPartitionScope, Path: "/cg", CPUs: "2", Facts: map[string]agent.HostFact{}}
		for _, stage := range []string{"before", "after"} {
			for key, value := range map[string]string{"cpuset.cpus.partition": "isolated", "cpuset.cpus.effective": "2", "cpuset.cpus.exclusive.effective": "2", "cgroup.events": "populated 0", "online": "0-2", "controller_threads": "10", "thread/10": "0-1"} {
				p.Facts[stage+"/"+key] = fact(value)
			}
		}
		p.Facts["siblings/2"] = fact("2")
		p.Status, p.Reason = agent.CheckCPUPartition(p)
		b.Manifest.CPUPartitionStart, b.Manifest.CPUPartitionEnd = &p, &p
		for i := range b.Trials {
			worker := "/cg/wasmbench-123"
			s := agent.ActivePartitionSample{Version: agent.ActivePartitionVersion, Scope: agent.ActivePartitionScope, At: time.Now().UTC(), Parent: "/cg", Worker: worker, CPUs: "2", PID: 123, Facts: map[string]agent.HostFact{}}
			for key, value := range map[string]string{"partition": "isolated", "effective_cpus": "2", "exclusive_cpus": "2", "worker_cpus": "2", "parent_events": "populated 1", "worker_events": "populated 1", "parent_procs": "", "worker_procs": "123", "online": "0-2", "children": `["wasmbench-123"]`} {
				s.Facts[key] = fact(value)
			}
			if legacy {
				s.Version, s.Scope = agent.LegacyActivePartitionVersion, agent.LegacyActivePartitionScope
			} else {
				for _, stage := range []string{"before", "after"} {
					s.Facts["controller/"+stage+"/threads"] = fact("10")
					s.Facts["controller/"+stage+"/thread/10"] = fact("0-1")
				}
			}
			s.Status, s.Reason = agent.CheckActivePartition(s)
			b.Trials[i].Isolation = &agent.Isolation{Mode: "cgroup_v2_at_spawn", Path: worker}
			b.Trials[i].PartitionMonitor = &agent.PartitionMonitor{Version: s.Version, IntervalNS: int64(250 * time.Millisecond), Status: "ready_at_samples", Reason: "all recorded samples show the single adapter worker; unsampled intervals remain unqualified", Samples: []agent.ActivePartitionSample{s}}
		}
		a := AuditPublication(b, PilotEvidence{})
		checks := map[string]string{}
		for _, requirement := range a.Requirements {
			checks[requirement.ID] = requirement.Status
		}
		if checks["sampled_partition_workers"] != "passed" || (checks["sampled_controller_separation"] == "passed") == legacy || checks["dedicated_machine_qualification"] != "unmet" {
			t.Fatal("controller audit confused legacy worker observations with v2 separation", checks)
		}
	}
}

func TestPublicationIRQBoundaryRequirement(t *testing.T) {
	b, _, _ := confirmationFixture(t)
	b.Manifest.Lock.RequireIRQAffinity = true
	b.Manifest.Lock.Options.Resources = agent.ResourcePolicy{CgroupParent: "/cg", CPUs: "2"}
	p := agent.IRQAffinityProbe{Version: agent.IRQAffinityVersion, Scope: agent.IRQAffinityScope, At: time.Now().UTC(), CPUs: "2", Facts: map[string]agent.HostFact{}}
	for _, boundary := range []string{"before", "after"} {
		for key, raw := range map[string]string{"online": "0-2", "default": "3", "inventory": `["44"]`, "irq/44/requested": "0-1", "irq/44/effective": "1"} {
			value := raw
			p.Facts[boundary+"/"+key] = agent.HostFact{Status: "available", Value: &value}
		}
	}
	p.Status, p.Reason = agent.CheckIRQAffinity(p)
	b.Manifest.IRQAffinityStart, b.Manifest.IRQAffinityEnd = &p, &p
	for _, valid := range []bool{true, false} {
		if !valid {
			b.Manifest.IRQAffinityEnd = nil
		}
		a := AuditPublication(b, PilotEvidence{})
		found := false
		for _, requirement := range a.Requirements {
			if requirement.ID == "device_irq_affinity_boundaries" {
				found = true
				if (requirement.Status == "passed") != valid {
					t.Fatal(requirement)
				}
			}
			if requirement.ID == "dedicated_machine_qualification" && requirement.Status != "unmet" {
				t.Fatal("IRQ evidence waived host qualification")
			}
		}
		if !found || a.Status != "blocked" {
			t.Fatal(a)
		}
	}
}

func confirmationFixture(t *testing.T) (experiment.Bundle, experiment.Bundle, string) {
	t.Helper()
	source := pilotFixture()
	source.Manifest.ID = "pilot"
	source.Manifest.Created = time.Unix(100, 0)
	source.Manifest.LockSHA256 = strings.Repeat("a", 64)
	checksum := strings.Repeat("b", 64)
	p, err := PlanPilot(source, .5, 6, 100)
	if err != nil || p.Status != "ready" {
		t.Fatal(p, err)
	}
	p.SourceBundle = "/original/pilot"
	p.SourceChecksumsSHA256 = checksum
	raw, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	var confirmation experiment.Bundle
	if err := json.Unmarshal(raw, &confirmation); err != nil {
		t.Fatal(err)
	}
	confirmation.Manifest.ID = "confirmation"
	confirmation.Manifest.Created = time.Unix(200, 0)
	confirmation.Manifest.Lock.Options.Launches = p.Launches
	confirmation.Manifest.Lock.PilotPlan, err = json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	return confirmation, source, checksum
}

func TestPilotConfirmationContract(t *testing.T) {
	b, source, checksum := confirmationFixture(t)
	before, _ := json.Marshal(b)
	if err := VerifyPilotConfirmation(b, source, checksum); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(b)
	if string(before) != string(after) {
		t.Fatal("mutated confirmation")
	}
	for _, mode := range []string{"digest", "operations", "budget", "runtime", "workload", "resource", "host", "same-run", "earlier", "unknown-plan-field", "cell"} {
		t.Run(mode, func(t *testing.T) {
			b, source, checksum := confirmationFixture(t)
			switch mode {
			case "digest":
				checksum = strings.Repeat("c", 64)
			case "operations":
				b.Manifest.Lock.Options.Operations++
			case "budget":
				b.Manifest.Lock.Options.Launches++
			case "runtime":
				b.Manifest.Lock.Runtimes[0].ID = "different"
			case "workload":
				b.Manifest.Lock.Workloads[0].ID = "different"
			case "resource":
				b.Manifest.Lock.Options.Resources.CPUs = "0"
			case "host":
				b.Manifest.Host.CPU = "different"
			case "same-run":
				b.Manifest.ID = source.Manifest.ID
			case "earlier":
				b.Manifest.Created = source.Manifest.Created
			case "unknown-plan-field":
				b.Manifest.Lock.PilotPlan = append([]byte(`{"future":1,`), b.Manifest.Lock.PilotPlan[1:]...)
			case "cell":
				p, err := DecodePilotPlan(b.Manifest.Lock.PilotPlan)
				if err != nil {
					t.Fatal(err)
				}
				p.Cells[0].Launches++
				b.Manifest.Lock.PilotPlan, _ = json.Marshal(p)
			}
			if VerifyPilotConfirmation(b, source, checksum) == nil {
				t.Fatal("changed evidence accepted")
			}
		})
	}
}

func TestOfficialLabelDoesNotGrantPublication(t *testing.T) {
	b, source, checksum := confirmationFixture(t)
	b.Manifest.Publication = "official"
	a := AuditPublication(b, PilotEvidence{Bundle: &source, ChecksumsSHA256: checksum})
	if a.Status != "blocked" || a.Err() == nil || ValidatePublication(b) == nil {
		t.Fatal("label bypassed qualification")
	}
	checks := map[string]PublicationRequirement{}
	for _, c := range a.Requirements {
		if _, exists := checks[c.ID]; exists {
			t.Fatal("duplicate requirement")
		}
		checks[c.ID] = c
	}
	if checks["pilot_confirmation"].Status != "passed" || checks["timing_measurement"].Status != "passed" || checks["complete_stable_measurements"].Status != "passed" {
		t.Fatal(checks)
	}
	if checks["matched_host_baseline"].Status != "unmet" || checks["resource_boundary_evidence"].Status != "unmet" || checks["artifact_admission"].Status != "unmet" || checks["sampled_partition_workers"].Status != "unmet" || checks["dedicated_machine_qualification"].Status != "unmet" {
		t.Fatal(checks)
	}
	if checks["sampled_controller_separation"].Status != "unmet" {
		t.Fatal("missing controller samples granted separation", checks)
	}
	b.Trials[0].Status = "unsupported"
	a = AuditPublication(b, PilotEvidence{Bundle: &source, ChecksumsSHA256: checksum})
	for _, r := range a.Requirements {
		if r.ID == "complete_stable_measurements" && r.Status == "passed" {
			t.Fatal("dropped unsupported cell")
		}
	}
	if b.Trials[0].Status != "unsupported" || len(b.Manifest.Lock.PilotPlan) == 0 {
		t.Fatal("audit mutated evidence")
	}
}

func TestPublicationSacrificialChecks(t *testing.T) {
	for _, mode := range []string{"valid", "missing", "duplicate", "incorrect", "unknown"} {
		t.Run(mode, func(t *testing.T) {
			b, _, _ := confirmationFixture(t)
			for _, runtime := range b.Manifest.Lock.Runtimes {
				b.Trials = append(b.Trials, experiment.Trial{Runtime: runtime.ID, Workload: "w", Scenario: "first-call", Profile: "timing", Block: -1, Status: "ok", Samples: []protocol.Sample{{Verified: true, Operations: 1, ElapsedNS: 100}}})
			}
			last := len(b.Trials) - 1
			switch mode {
			case "missing":
				b.Trials = b.Trials[:last]
			case "duplicate":
				b.Trials = append(b.Trials, b.Trials[last])
			case "incorrect":
				b.Trials[last].Samples[0].Verified = false
			case "unknown":
				b.Trials[last].Runtime = "undeclared"
			}
			a := AuditPublication(b, PilotEvidence{})
			for _, requirement := range a.Requirements {
				if requirement.ID == "sacrificial_correctness" && (requirement.Status == "passed") != (mode == "valid") {
					t.Fatal(requirement)
				}
			}
		})
	}
}
