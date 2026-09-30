package experiment

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/protocol"
)

func TestNativeSnapshotMemoryTrial(t *testing.T) {
	binary := os.Getenv("WASMBENCH_SNAPSHOT_LIVE_ADAPTER")
	if binary == "" || runtime.GOOS != "linux" {
		t.Skip("requires native Linux snapshot adapter")
	}
	for _, backend := range []string{"cranelift", "winch"} {
		t.Run(backend, func(t *testing.T) {
			root, b := snapshotEvidence(t)
			if err := os.Mkdir(filepath.Join(root, "logs"), 0755); err != nil {
				t.Fatal(err)
			}
			r := b.Manifest.Lock.Runtimes[0]
			r.Command = []string{binary, "--adapter=" + backend}
			c, err := agent.Start(context.Background(), r.Command, filepath.Join(root, "logs", "describe.log"), 30*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			response, err := c.Call(protocol.Request{Method: "describe"})
			closeErr := c.Close()
			if err != nil {
				t.Fatal(err)
			}
			if closeErr != nil {
				t.Fatal(closeErr)
			}
			r.Description = response.Description
			b.Manifest.Lock.Runtimes[0] = r
			b.Manifest.Host.Arch = runtime.GOARCH
			b.Manifest.Lock.Options.Profile = "memory"
			b.Manifest.Lock.Options.PhaseBarriers = true
			b.Manifest.Lock.Options.Timeout = 30 * time.Second
			w := b.Manifest.Lock.Workloads[0]
			w.Artifact = filepath.Join("artifacts", w.SHA256+".wasm")
			b.Manifest.Lock.Workloads[0] = w
			b.Trials = nil
			for i, stage := range append([]string{"process-snapshot-restore"}, protocol.ProcessSnapshotScenarios()...) {
				block := 0
				if i == 0 {
					block = -1
				}
				trial := runTrial(context.Background(), root, b.Manifest.Lock.Options, r, w, stage, block, fmt.Sprintf("native-%d", i))
				if trial.Status != "ok" {
					t.Fatalf("%s: %s %s", stage, trial.Status, trial.Reason)
				}
				if block >= 0 && (trial.SnapshotMemory == nil || len(trial.Samples) != 0 || len(trial.Observations) != 0) {
					t.Fatal("missing memory or root-only evidence")
				}
				if block < 0 && (trial.SnapshotMemory != nil || len(trial.Samples) != 2) {
					t.Fatal("sacrificial admission profile changed")
				}
				b.Trials = append(b.Trials, trial)
			}
			if err := ValidateProcessSnapshotEvidence(root, b); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func snapshotMemoryFixture(t *testing.T) (string, Bundle) {
	root, b := snapshotEvidence(t)
	b.Manifest.Lock.Options.Profile = "memory"
	b.Manifest.Lock.Options.PhaseBarriers = true
	b.Manifest.Lock.Runtimes[0].Description.Capabilities["can_inspect_linux_snapshot_lineage"] = true
	trial := &b.Trials[0]
	trial.Profile = "memory"
	e := &SnapshotMemoryEvidence{Version: SnapshotMemoryVersion, ControllerPID: 100,
		Diagnostics: protocol.SnapshotDiagnostics{Version: protocol.SnapshotInspectionVersion, Profile: "memory", Samples: trial.Samples}}
	trial.Samples = nil
	trial.SnapshotMemory = e
	proof := e.Diagnostics.Samples[0].ProcessSnapshotResult
	var clock int64
	for i, stage := range []string{"template_after_source_release", "restore_ready", "first_write_done", "execution_done", "restore_ready", "first_write_done", "execution_done"} {
		boundary := protocol.SnapshotBoundary{Version: protocol.SnapshotLiveBoundaryVersion, Stage: stage, Restoration: -1, Source: proof.Source, Template: proof.Template}
		if i > 0 {
			ref := proof.Restored
			boundary.Restoration = 0
			if i > 3 {
				ref = proof.AlternateRestored
				boundary.Restoration = 1
			}
			boundary.Restored = &ref
		}
		record := SnapshotMemoryRecord{Boundary: boundary}
		for _, item := range snapshotMemoryProcesses(boundary, e.ControllerPID) {
			fields := make([]string, 20)
			for j := range fields {
				fields[j] = "0"
			}
			fields[0], fields[1], fields[19] = "S", fmt.Sprint(item.parent), item.ref.StartTimeTicks
			stat := fmt.Sprintf("%d (snapshot) %s\n", item.ref.PID, strings.Join(fields, " "))
			r := collectors.SnapshotProcessReading{Version: collectors.SnapshotProcessCollectorVersion, Process: item.ref, ParentPID: item.parent, StartNS: clock, EndNS: clock + 1, StatBefore: stat, StatAfter: stat,
				Status: fmt.Sprintf("Pid: %d\nPPid: %d\nState: S (sleeping)\nThreads: 1\nVmRSS: 0 kB\nVmSize: 8 kB\n", item.ref.PID, item.parent), SmapsStatus: "permission_denied", SmapsReason: "test denied"}
			clock += 2
			record.Readings = append(record.Readings, r)
		}
		e.Records = append(e.Records, record)
	}
	return root, b
}

func TestSnapshotMemorySealedContract(t *testing.T) {
	root, b := snapshotMemoryFixture(t)
	if err := ValidateProcessSnapshotEvidence(root, b); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*Bundle){
		"no evidence":      func(b *Bundle) { b.Trials[0].SnapshotMemory = nil },
		"foreign workload": func(b *Bundle) { b.Trials[0].Workload = "other" },
		"timing profile":   func(b *Bundle) { b.Manifest.Lock.Options.Profile = "timing"; b.Trials[0].Profile = "timing" },
		"no barrier":       func(b *Bundle) { b.Manifest.Lock.Options.PhaseBarriers = false },
		"no capability": func(b *Bundle) {
			b.Manifest.Lock.Runtimes[0].Description.Capabilities["can_inspect_linux_snapshot_lineage"] = false
		},
		"promoted samples":     func(b *Bundle) { b.Trials[0].Samples = b.Trials[0].SnapshotMemory.Diagnostics.Samples },
		"parent rss":           func(b *Bundle) { b.Trials[0].Observations = []protocol.Observation{{Metric: "process.peak_rss"}} },
		"version":              func(b *Bundle) { b.Trials[0].SnapshotMemory.Version = "wrong" },
		"controller":           func(b *Bundle) { b.Trials[0].SnapshotMemory.ControllerPID = 0 },
		"aliased controller":   func(b *Bundle) { b.Trials[0].SnapshotMemory.ControllerPID = 101 },
		"headline eligibility": func(b *Bundle) { b.Trials[0].SnapshotMemory.Diagnostics.LatencyEligible = true },
		"missing child":        func(b *Bundle) { e := b.Trials[0].SnapshotMemory; e.Records[1].Readings = e.Records[1].Readings[:2] },
		"extra reading": func(b *Bundle) {
			e := b.Trials[0].SnapshotMemory
			e.Records[0].Readings = append(e.Records[0].Readings, e.Records[0].Readings[0])
		},
		"missing event": func(b *Bundle) { e := b.Trials[0].SnapshotMemory; e.Records = e.Records[:6] },
		"reordered": func(b *Bundle) {
			e := b.Trials[0].SnapshotMemory
			e.Records[1], e.Records[2] = e.Records[2], e.Records[1]
		},
		"wrong parent":      func(b *Bundle) { b.Trials[0].SnapshotMemory.Records[1].Readings[2].ParentPID = 100 },
		"wrong birth":       func(b *Bundle) { b.Trials[0].SnapshotMemory.Records[1].Readings[2].Process.StartTimeTicks = "999" },
		"overlapping clock": func(b *Bundle) { b.Trials[0].SnapshotMemory.Records[1].Readings[2].StartNS = 0 },
		"threads": func(b *Bundle) {
			r := &b.Trials[0].SnapshotMemory.Records[1].Readings[2]
			r.Status = strings.Replace(r.Status, "Threads: 1", "Threads: 2", 1)
		},
		"invented pss": func(b *Bundle) {
			r := &b.Trials[0].SnapshotMemory.Records[1].Readings[2]
			s := "Pss: 0 kB"
			r.SmapsRollup = &s
		},
	} {
		t.Run(name, func(t *testing.T) {
			root, b := snapshotMemoryFixture(t)
			change(&b)
			if ValidateProcessSnapshotEvidence(root, b) == nil {
				t.Fatal("forged memory admitted")
			}
		})
	}
}

func TestSnapshotMemoryCgroupDomain(t *testing.T) {
	o := protocol.Observation{Metric: "cgroup.memory.peak", DefinitionVersion: 1, Unit: "bytes", Scope: "adapter_cgroup", Phase: "process_lifetime/response_end", Collector: "cgroup_v2", CollectorVersion: "1", Quality: "kernel_accounted_peak", Profile: "memory", Denominator: "cgroup", Status: "available", Value: protocol.Value(0)}
	root, b := snapshotMemoryFixture(t)
	b.Trials[0].Observations = []protocol.Observation{o}
	if err := ValidateProcessSnapshotEvidence(root, b); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*protocol.Observation){
		"wrong scope":      func(o *protocol.Observation) { o.Scope = "adapter_process" },
		"wrong quality":    func(o *protocol.Observation) { o.Quality = "sampled_observed_peak" },
		"no value":         func(o *protocol.Observation) { o.Value = nil },
		"negative":         func(o *protocol.Observation) { o.Value = protocol.Value(-1) },
		"fraction":         func(o *protocol.Observation) { o.Value = protocol.Value(0.5) },
		"unavailable zero": func(o *protocol.Observation) { o.Status = "unavailable"; o.Reason = "denied" },
	} {
		t.Run(name, func(t *testing.T) {
			bad := o
			change(&bad)
			if validateSnapshotCgroupObservations([]protocol.Observation{bad}) == nil {
				t.Fatal("forged cgroup admitted")
			}
		})
	}
	if validateSnapshotCgroupObservations([]protocol.Observation{o, o}) == nil {
		t.Fatal("duplicate cgroup admitted")
	}
	o.Status, o.Value, o.Reason = "unavailable", nil, "permission denied"
	if err := validateSnapshotCgroupObservations([]protocol.Observation{o}); err != nil {
		t.Fatal(err)
	}
}
