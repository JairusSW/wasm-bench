package publish

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/analysis"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

// Everything below is synthetic, including kernel and analyzer evidence. This
// tests the archive contract, not real machine qualification or performance.
func publicationFixture(t *testing.T) (string, string, analysis.DedicatedQualification, ed25519.PublicKey) {
	t.Helper()
	now := time.Unix(1700000000, 0).UTC()
	fact := func(value string) agent.HostFact {
		return agent.HostFact{Source: "synthetic fixture", Status: "available", Value: &value}
	}
	resources := agent.ResourcePolicy{CgroupParent: "/cg", CPUs: "2", Mems: "0", MemoryMaxBytes: 1024}
	host := agent.Host{OS: "linux", Arch: "arm64", Hostname: "synthetic-not-certified", CPUs: 4, PageSize: 4096, Environment: map[string]string{}, Policy: map[string]string{}}
	partition := agent.CPUPartitionProbe{Version: agent.CPUPartitionVersion, Scope: agent.CPUPartitionScope, Path: "/cg", CPUs: "2", Facts: map[string]agent.HostFact{}}
	irq := agent.IRQAffinityProbe{Version: agent.IRQAffinityVersion, Scope: agent.IRQAffinityScope, At: now, CPUs: "2", Facts: map[string]agent.HostFact{}}
	for _, boundary := range []string{"before", "after"} {
		for k, v := range map[string]string{"cpuset.cpus.partition": "isolated", "cpuset.cpus.effective": "2", "cpuset.cpus.exclusive.effective": "2", "cgroup.events": "populated 0", "online": "0-3", "controller_threads": "10", "thread/10": "0-1,3"} {
			partition.Facts[boundary+"/"+k] = fact(v)
		}
		for k, v := range map[string]string{"online": "0-3", "default": "3", "inventory": `["44"]`, "irq/44/requested": "0-1", "irq/44/effective": "1"} {
			irq.Facts[boundary+"/"+k] = fact(v)
		}
	}
	partition.Facts["siblings/2"] = fact("2")
	partition.Status, partition.Reason = agent.CheckCPUPartition(partition)
	irq.Status, irq.Reason = agent.CheckIRQAffinity(irq)
	digest := strings.Repeat("a", 64)
	b := experiment.Bundle{Manifest: experiment.Manifest{ID: "synthetic-pilot", Created: now, Kind: "measurement", LockSHA256: strings.Repeat("b", 64), Host: host, HostEnd: &host, CPUPartitionStart: &partition, CPUPartitionEnd: &partition, IRQAffinityStart: &irq, IRQAffinityEnd: &irq, Lock: experiment.Lock{RequireIsolatedCPUPartition: true, RequireIRQAffinity: true, HostPolicy: &agent.HostPolicy{Version: agent.HostPolicyVersion, Expected: host}, Options: experiment.Options{Profile: "timing", Scenarios: []string{"compile"}, Launches: 6, Samples: 3, Operations: 1, Resources: resources}, Analyzer: &experiment.AnalyzerLock{Executable: "/synthetic/analyzer", SHA256: digest, Profile: "wasm1", Name: "wasmparser", Version: "0.251.0", AnalysisVersion: "core-structure-v2"}, Runtimes: []experiment.Runtime{{ID: "synthetic-runtime"}}, Workloads: []protocol.Workload{{ID: "synthetic-workload", SHA256: digest, ABI: "core", Family: "mechanisms"}}}}}
	start := agent.CheckHostPolicy(b.Manifest.Lock.HostPolicy, host, "before_run_preparation")
	end := agent.CheckHostPolicy(b.Manifest.Lock.HostPolicy, host, "after_trials_before_seal")
	b.Manifest.HostStartCheck, b.Manifest.HostEndCheck = &start, &end
	for block := -1; block < 6; block++ {
		values := map[string]string{"cpuset.cpus": "2", "cpuset.cpus.effective": "2", "cpuset.mems": "0", "cpuset.mems.effective": "0", "memory.max": "1024", "memory.oom.group": "1"}
		a := agent.CheckResourceReadback(resources, values, "before_spawn")
		z := agent.CheckResourceReadback(resources, values, "response_end_before_cleanup")
		worker := "/cg/wasmbench-123"
		s := agent.ActivePartitionSample{Version: agent.ActivePartitionVersion, Scope: agent.ActivePartitionScope, At: now.Add(time.Second), Parent: "/cg", Worker: worker, CPUs: "2", PID: 123, Facts: map[string]agent.HostFact{}}
		for k, v := range map[string]string{"partition": "isolated", "effective_cpus": "2", "exclusive_cpus": "2", "worker_cpus": "2", "parent_events": "populated 1", "worker_events": "populated 1", "parent_procs": "", "worker_procs": "123", "online": "0-3", "children": `["wasmbench-123"]`} {
			s.Facts[k] = fact(v)
		}
		for _, stage := range []string{"before", "after"} {
			s.Facts["controller/"+stage+"/threads"] = fact("10")
			s.Facts["controller/"+stage+"/thread/10"] = fact("0-1,3")
		}
		s.Status, s.Reason = agent.CheckActivePartition(s)
		trial := experiment.Trial{ID: fmt.Sprintf("trial-%02d", block+1), Runtime: "synthetic-runtime", Workload: "synthetic-workload", Scenario: "compile", Profile: "timing", Block: block, Status: "ok", Started: now.Add(time.Duration(block+2) * time.Second), DurationNS: int64(time.Second), Isolation: &agent.Isolation{Mode: "cgroup_v2_at_spawn", Path: worker, Effective: values, Verification: &a, FinalVerification: &z}, PartitionMonitor: &agent.PartitionMonitor{Version: s.Version, IntervalNS: int64(250 * time.Millisecond), Status: "ready_at_samples", Reason: "all recorded samples show the single adapter worker; unsampled intervals remain unqualified", Samples: []agent.ActivePartitionSample{s}}}
		if block < 0 {
			trial.Scenario = "first-call"
		}
		for i := 0; i < 3; i++ {
			trial.Samples = append(trial.Samples, protocol.Sample{Index: i, Verified: true, Operations: 1, ElapsedNS: 100, SampleType: "individual_operation"})
		}
		b.Trials = append(b.Trials, trial)
	}
	write := func(b experiment.Bundle) string {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, "trials"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(root, "validation"), 0755); err != nil {
			t.Fatal(err)
		}
		evidence := map[string]any{"schema": 2, "analyzer": "wasmparser", "analyzer_version": "0.251.0", "analysis_version": "core-structure-v2", "sha256": digest, "validation_profile": "wasm1", "encoding": "core-module", "validated": true, "validation_features": []string{"MVP"}}
		if err := experiment.WriteJSON(filepath.Join(root, "validation", digest+".json"), evidence); err != nil {
			t.Fatal(err)
		}
		if err := experiment.WriteJSON(filepath.Join(root, "manifest.json"), b.Manifest); err != nil {
			t.Fatal(err)
		}
		for _, tr := range b.Trials {
			if err := experiment.WriteJSON(filepath.Join(root, "trials", tr.ID+".json"), tr); err != nil {
				t.Fatal(err)
			}
		}
		if err := experiment.Seal(root); err != nil {
			t.Fatal(err)
		}
		return root
	}
	pilot := write(b)
	loaded, err := experiment.Load(pilot)
	if err != nil {
		t.Fatal(err)
	}
	p, err := analysis.PlanPilot(loaded, .5, 6, 100)
	if err != nil || p.Status != "ready" {
		t.Fatal(p, err)
	}
	p.SourceBundle = pilot
	p.SourceChecksumsSHA256, _ = experiment.DigestFile(filepath.Join(pilot, "checksums.json"))
	b.Manifest.ID = "synthetic-confirmation"
	b.Manifest.Created = now.Add(100 * time.Second)
	b.Manifest.Lock.Options.Launches = p.Launches
	b.Manifest.Lock.PilotPlan, _ = json.Marshal(p)
	for i := range b.Trials {
		b.Trials[i].Started = b.Trials[i].Started.Add(100 * time.Second)
		b.Trials[i].PartitionMonitor.Samples[0].At = b.Trials[i].Started
	}
	run := write(b)
	loaded, err = experiment.Load(run)
	if err != nil {
		t.Fatal(err)
	}
	checksum, _ := experiment.DigestFile(filepath.Join(run, "checksums.json"))
	statement, err := analysis.DedicatedQualificationDraft(loaded, checksum)
	if err != nil {
		t.Fatal(err)
	}
	statement.Operator = "synthetic operator; tests only"
	statement.IssuedAt = now.Add(200 * time.Second)
	statement.Assertions = map[string]string{"measurement_cpus": "exclusively_reserved_for_this_run", "controller_collectors": "outside_measurement_cpus", "unrelated_work": "excluded_from_measurement_cpus", "builds_and_uploads": "outside_measurement_cpus", "machine_policy": "maintained_throughout_validity_interval"}
	for k := range statement.MachinePolicy {
		statement.MachinePolicy[k] = "synthetic maintained " + k + " policy"
	}
	statement.OperationalLog = "All kernel facts, timing samples, admission and operator assertions are synthetic fixtures; this does not qualify a real host."
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	q, err := analysis.SignDedicatedQualification(statement, key)
	if err != nil {
		t.Fatal(err)
	}
	return run, pilot, q, pub
}

func TestQualifiedPublicationArchiveAndReaderTrust(t *testing.T) {
	run, pilot, q, key := publicationFixture(t)
	out := filepath.Join(t.TempDir(), "publication")
	if requested := os.Getenv("WASMBENCH_SYNTHETIC_PUBLICATION_EVIDENCE_DIR"); requested != "" {
		out = requested
	}
	if err := ReportQualified(run, pilot, q, key, out); err != nil {
		t.Fatal(err)
	}
	if err := VerifyReport(out); err != nil {
		t.Fatal(err)
	}
	if err := VerifyPublicationReport(out, key); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("WASMBENCH_SYNTHETIC_PUBLICATION_EVIDENCE_DIR") != "" {
		if err := experiment.WriteJSON(out+"-test-public-key.json", fmt.Sprintf("%x", key)); err != nil {
			t.Fatal(err)
		}
	}
	if VerifyPublicationReport(out, nil) == nil || VerifyPublicationReport(out, make([]byte, 32)) == nil {
		t.Fatal("bundled key automatically trusted")
	}
	if err := ReportQualified(run, pilot, q, key, out); err == nil {
		t.Fatal("overwrote archive")
	}
	for _, source := range []string{run, pilot} {
		if err := experiment.Verify(source); err != nil {
			t.Fatal("modified source", err)
		}
	}
	for _, mode := range []string{"qualification", "audit", "pilot", "receipt", "unrecorded"} {
		t.Run(mode, func(t *testing.T) {
			copy := filepath.Join(t.TempDir(), "copy")
			if err := os.CopyFS(copy, os.DirFS(out)); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "qualification":
				if err := os.WriteFile(filepath.Join(copy, "qualification.json"), []byte(`{"signature_hex":"wrong"}`), 0644); err != nil {
					t.Fatal(err)
				}
			case "audit":
				if err := os.WriteFile(filepath.Join(copy, "publication-audit.json"), []byte(`{}`), 0644); err != nil {
					t.Fatal(err)
				}
			case "pilot":
				if err := os.WriteFile(filepath.Join(copy, "raw-pilot", "manifest.json"), []byte(`{}`), 0644); err != nil {
					t.Fatal(err)
				}
			case "receipt", "unrecorded":
				var d Dataset
				if err := experiment.ReadJSON(filepath.Join(copy, "data.json"), &d); err != nil {
					t.Fatal(err)
				}
				if mode == "receipt" {
					d.Publication.Audit.Status = "fake"
				} else {
					d.Publication = nil
				}
				encoded, _ := json.Marshal(d)
				if err := os.WriteFile(filepath.Join(copy, "data.json"), encoded, 0644); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Remove(filepath.Join(copy, "checksums.json")); err != nil {
				t.Fatal(err)
			}
			if err := experiment.Seal(copy); err != nil {
				t.Fatal(err)
			}
			if VerifyReport(copy) == nil || VerifyPublicationReport(copy, key) == nil {
				t.Fatal("resealed invalid evidence accepted")
			}
		})
	}
}

func TestQualifiedPublicationReproductionDoesNotTransferTrust(t *testing.T) {
	run, pilot, q, key := publicationFixture(t)
	source := filepath.Join(t.TempDir(), "qualified")
	if err := ReportQualified(run, pilot, q, key, source); err != nil {
		t.Fatal(err)
	}
	before, err := experiment.DigestFile(filepath.Join(source, "checksums.json"))
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "reproduced")
	// Inject a synthetic replay; no runtime executes and no machine is certified.
	err = reproduceReport(context.Background(), source, out, nil,
		func(*reportReplayPass) error { return nil },
		func(_ context.Context, p reportReplayPass, path string, _ func(string)) error {
			return os.CopyFS(path, os.DirFS(p.Source))
		})
	if err != nil {
		t.Fatal(err)
	}
	report := filepath.Join(out, "report")
	if err := VerifyReport(report); err != nil {
		t.Fatal(err)
	}
	var d Dataset
	if err := experiment.ReadJSON(filepath.Join(report, "data.json"), &d); err != nil {
		t.Fatal(err)
	}
	if d.Publication != nil || VerifyPublicationReport(report, key) == nil {
		t.Fatal("historical operator trust transferred to reproduction")
	}
	for _, name := range []string{"qualification.json", "publication-audit.json", "raw-pilot"} {
		if _, err := os.Lstat(filepath.Join(report, name)); !os.IsNotExist(err) {
			t.Fatal("historical publication evidence copied", name, err)
		}
	}
	after, err := experiment.DigestFile(filepath.Join(source, "checksums.json"))
	if err != nil || before != after {
		t.Fatal("source archive changed", err)
	}
}

func TestQualifiedPublicationFailsBeforeOutput(t *testing.T) {
	run, pilot, q, key := publicationFixture(t)
	for _, mode := range []string{"key", "signature", "nested-run", "nested-pilot"} {
		out := filepath.Join(t.TempDir(), "publication")
		candidate := q
		trusted := key
		switch mode {
		case "key":
			trusted = nil
		case "signature":
			candidate.Signature = "wrong"
		case "nested-run":
			out = filepath.Join(run, "new", "report")
		case "nested-pilot":
			out = filepath.Join(pilot, "new", "report")
		}
		if ReportQualified(run, pilot, candidate, trusted, out) == nil {
			t.Fatal("bad publication accepted", mode)
		}
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			t.Fatal("failed admission created output", mode)
		}
	}
}
