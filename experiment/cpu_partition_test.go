package experiment

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/agent"
)

func partitionEvidence() agent.CPUPartitionProbe {
	p := agent.CPUPartitionProbe{Version: agent.CPUPartitionVersion, Path: "/cg", CPUs: "2", Scope: agent.CPUPartitionScope, Facts: map[string]agent.HostFact{}}
	for _, stage := range []string{"before", "after"} {
		for name, value := range map[string]string{"cpuset.cpus.partition": "isolated", "cpuset.cpus.effective": "2", "cpuset.cpus.exclusive.effective": "2", "cgroup.events": "populated 0", "online": "0-2", "controller_threads": "1", "thread/1": "0-1"} {
			v := value
			p.Facts[stage+"/"+name] = agent.HostFact{Source: "fixture", Status: "available", Value: &v}
		}
	}
	siblings := "2"
	p.Facts["siblings/2"] = agent.HostFact{Source: "fixture", Status: "available", Value: &siblings}
	p.Status, p.Reason = agent.CheckCPUPartition(p)
	return p
}

func activeTrialMonitor() *agent.PartitionMonitor {
	facts := map[string]agent.HostFact{}
	for key, value := range map[string]string{
		"partition": "isolated", "effective_cpus": "2", "exclusive_cpus": "2", "worker_cpus": "2",
		"parent_events": "populated 1", "worker_events": "populated 1", "parent_procs": "", "worker_procs": "123",
		"online": "0-2", "children": `["wasmbench-123"]`, "child/wasmbench-123": "populated 1",
		"controller/before/threads": "10", "controller/after/threads": "10",
		"controller/before/thread/10": "0-1", "controller/after/thread/10": "0-1",
	} {
		v := value
		facts[key] = agent.HostFact{Source: "fixture", Status: "available", Value: &v}
	}
	s := agent.ActivePartitionSample{Version: agent.ActivePartitionVersion, Scope: agent.ActivePartitionScope, At: time.Now().UTC(), Parent: "/cg", Worker: "/cg/wasmbench-123", CPUs: "2", PID: 123, Facts: facts}
	s.Status, s.Reason = agent.CheckActivePartition(s)
	return &agent.PartitionMonitor{Version: agent.ActivePartitionVersion, IntervalNS: int64(250 * time.Millisecond), Status: "ready_at_samples", Reason: "all recorded samples show the single adapter worker; unsampled intervals remain unqualified", Samples: []agent.ActivePartitionSample{s}}
}

func TestPartitionTrialEvidenceAndSealedFailure(t *testing.T) {
	start, end := partitionEvidence(), partitionEvidence()
	m := Manifest{Lock: Lock{RequireIsolatedCPUPartition: true, Options: Options{Resources: agent.ResourcePolicy{CgroupParent: "/cg", CPUs: "2"}}}, CPUPartitionStart: &start, CPUPartitionEnd: &end, Publication: "local_exploratory"}
	trial := Trial{ID: "trial-000000", Status: "error", Isolation: &agent.Isolation{Mode: "cgroup_v2_at_spawn", Path: "/cg/wasmbench-123"}, PartitionMonitor: activeTrialMonitor()}
	b := Bundle{Manifest: m, Trials: []Trial{trial}}
	if err := ValidatePartitionTrialEvidence(b); err != nil {
		t.Fatal(err)
	}
	b.Trials = append(b.Trials, Trial{ID: "trial-000001", Status: "error", Isolation: &agent.Isolation{Mode: "cgroup_v2_at_spawn", Path: "/cg/wasmbench-456"}})
	if err := ValidatePartitionTrialEvidence(b); err == nil {
		t.Fatal("partially monitored run accepted")
	}
	b.Trials = b.Trials[:1]
	s := &b.Trials[0].PartitionMonitor.Samples[0]
	f := s.Facts["worker_procs"]
	v := "123\n456"
	f.Value = &v
	s.Facts["worker_procs"] = f
	s.Status, s.Reason = agent.CheckActivePartition(*s)
	b.Trials[0].PartitionMonitor.Status = "not_ready"
	b.Trials[0].PartitionMonitor.Reason = "one or more active partition samples were not ready"
	if err := ValidatePartitionTrialEvidence(b); err == nil {
		t.Fatal("failed monitor without publication prohibition accepted")
	}
	b.Manifest.Publication = "prohibited_cpu_partition_sample_mismatch"
	if err := ValidatePartitionTrialEvidence(b); err != nil {
		t.Fatal(err)
	}
	if HostBaselineAllowsMeasurements(b.Manifest) {
		t.Fatal("sample failure remained performance eligible")
	}
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "trials"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSON(filepath.Join(dir, "manifest.json"), b.Manifest); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSON(filepath.Join(dir, "trials", trial.ID+".json"), b.Trials[0]); err != nil {
		t.Fatal(err)
	}
	if err := Seal(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err != nil {
		t.Fatal("sealed failure evidence was not readable:", err)
	}
}

func TestLockedCPUPartitionEvidence(t *testing.T) {
	for _, mode := range []string{"ready", "end-failure", "unreadable-end", "missing-end", "forged-end", "wrong-path", "wrong-cpus", "missing-requirement", "unready-start", "missing-prohibition"} {
		t.Run(mode, func(t *testing.T) {
			start, end := partitionEvidence(), partitionEvidence()
			m := Manifest{Lock: Lock{RequireIsolatedCPUPartition: true, Options: Options{Resources: agent.ResourcePolicy{CgroupParent: "/cg", CPUs: "2"}}}, CPUPartitionStart: &start, CPUPartitionEnd: &end, Publication: "local_exploratory"}
			switch mode {
			case "end-failure", "forged-end", "missing-prohibition":
				v := "member"
				f := end.Facts["after/cpuset.cpus.partition"]
				f.Value = &v
				end.Facts["after/cpuset.cpus.partition"] = f
				if mode != "forged-end" {
					end.Status, end.Reason = agent.CheckCPUPartition(end)
				}
				if mode != "missing-prohibition" {
					m.Publication = "prohibited_cpu_partition_mismatch"
				}
			case "unreadable-end":
				end.Facts = nil
				end.Status = "unavailable"
				end.Reason = "partition filesystem disappeared"
				m.Publication = "prohibited_cpu_partition_mismatch"
			case "missing-end":
				m.CPUPartitionEnd = nil
			case "wrong-path":
				end.Path = "/other"
			case "wrong-cpus":
				end.CPUs = "1"
			case "missing-requirement":
				m.Lock.RequireIsolatedCPUPartition = false
			case "unready-start":
				start.Facts = nil
				start.Status = "unavailable"
				start.Reason = "missing partition"
			}
			dir := t.TempDir()
			if err := os.Mkdir(filepath.Join(dir, "trials"), 0755); err != nil {
				t.Fatal(err)
			}
			if err := WriteJSON(filepath.Join(dir, "manifest.json"), m); err != nil {
				t.Fatal(err)
			}
			if err := Seal(dir); err != nil {
				t.Fatal(err)
			}
			b, err := Load(dir)
			valid := mode == "ready" || mode == "end-failure" || mode == "unreadable-end"
			if (err == nil) != valid {
				t.Fatal(mode, err)
			}
			if valid && HostBaselineAllowsMeasurements(b.Manifest) != (mode == "ready") {
				t.Fatal("incorrect measurement eligibility")
			}
		})
	}
}
