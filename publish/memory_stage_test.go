package publish

import (
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

func TestCheckpointStageMemoryUsesOnlyDeclaredDomains(t *testing.T) {
	o := protocol.Observation{Metric: "host.alloc.bytes", DefinitionVersion: 1, Phase: "checkpoint-create/operation_window", Quality: "engine_reported", Scope: "adapter_process_go_heap", Collector: "runtime.ReadMemStats", Denominator: "single_guest_checkpoint_operation_excluding_setup_verification_release", Unit: "bytes", Status: "available", Value: protocol.Value(65536)}
	if v, ok := memoryObservationValue(o, "host.alloc.bytes", "checkpoint-create", 1); !ok || v != 65536 {
		t.Fatal("missing checkpoint allocation evidence")
	}
	o.Denominator = "unrelated"
	if _, ok := memoryObservationValue(o, "host.alloc.bytes", "checkpoint-create", 1); ok {
		t.Fatal("accepted unrelated allocator window")
	}
	o = protocol.Observation{Metric: "checkpoint.payload_bytes", DefinitionVersion: 1, Phase: "checkpoint-create_returned", Quality: "exact", Scope: "guest_state_checkpoint", Collector: "wasmbench.eager_guest_checkpoint", CollectorVersion: "1", Denominator: "one_checkpoint", Unit: "bytes", Status: "available", Value: protocol.Value(65540)}
	if v, ok := memoryObservationValue(o, "checkpoint.payload_bytes", "checkpoint-create", 1); !ok || v != 65540 {
		t.Fatal("missing exact payload domain")
	}
	if _, ok := memoryObservationValue(o, "checkpoint.payload_bytes", "compile", 1); ok {
		t.Fatal("payload relabeled as compile memory")
	}
	if _, ok := memoryObservationValue(o, "checkpoint.payload_bytes", "checkpoint-create", 2); ok {
		t.Fatal("batch payload accepted")
	}
	o.Quality = "sampled"
	if _, ok := memoryObservationValue(o, "checkpoint.payload_bytes", "checkpoint-create", 1); ok {
		t.Fatal("sampled payload accepted as exact")
	}
}

func TestMemoryStagesKeepsMetricDomainsSeparate(t *testing.T) {
	obs := func(metric, phase, quality string, value float64) protocol.Observation {
		o := protocol.Observation{Metric: metric, Phase: phase, Quality: quality, Unit: "bytes", Status: "available", Value: protocol.Value(value)}
		if metric == "process.peak_rss" {
			o.Collector = "wait4_rusage"
		}
		return o
	}
	trial := func(id string, a, b float64) experiment.Trial {
		return experiment.Trial{ID: id, Runtime: "r", Workload: "w", Scenario: "compile", Profile: "memory", Status: "ok", Block: 0,
			Samples: []protocol.Sample{
				{Verified: true, Operations: 2, Observations: []protocol.Observation{obs("host.alloc.bytes", "compile", "engine_reported", a), obs("cgroup.memory.phase_peak", "compile/barrier_window", "kernel_accounted_peak", 4096)}},
				{Verified: true, Operations: 2, Observations: []protocol.Observation{obs("host.alloc.bytes", "compile", "engine_reported", b), obs("cgroup.memory.phase_peak", "compile/barrier_window", "kernel_accounted_peak", 8192)}},
			},
			Observations: []protocol.Observation{obs("process.rss", "compile/after_batch", "boundary_snapshot_only", 16384), obs("process.peak_rss", "compile/process_lifetime", "kernel_accounted_peak", 32768)},
		}
	}
	b := experiment.Bundle{Trials: []experiment.Trial{trial("one", 20, 40), trial("two", 40, 80)}}
	rows := memoryStages(b, map[string]bool{"r\x00w": true})
	values := map[string]float64{}
	for _, row := range rows {
		values[row.Metric] = *row.Median
		if row.Launches != 2 || len(row.Trials) != 2 {
			t.Fatalf("launch provenance lost: %+v", row)
		}
	}
	if values["host.alloc.bytes"] != 22.5 || values["cgroup.memory.phase_peak"] != 6144 || values["process.rss"] != 16384 || values["process.peak_rss"] != 32768 {
		t.Fatalf("metric boundaries or per-operation normalization changed: %+v", values)
	}
}

func TestMemoryPairRequiresSameBinaryAndArtifact(t *testing.T) {
	base := experiment.Bundle{Manifest: experiment.Manifest{Kind: "measurement", Host: agent.Host{OS: "darwin", Arch: "arm64", CPU: "same"}, Lock: experiment.Lock{
		Options:   experiment.Options{Profile: "timing"},
		Runtimes:  []experiment.Runtime{{ID: "r", Files: map[string]string{"adapter": "abc"}}},
		Workloads: []protocol.Workload{{ID: "w", SHA256: "artifact"}},
	}}}
	memory := base
	memory.Manifest.Lock.Options.Profile = "memory"
	matched, err := matchingMemoryCells(base, memory)
	if err != nil || !matched["r\x00w"] {
		t.Fatalf("matching evidence rejected: %v", err)
	}
	memory.Manifest.Lock.Runtimes = []experiment.Runtime{{ID: "r", Files: map[string]string{"adapter": "different"}}}
	if _, err := matchingMemoryCells(base, memory); err == nil {
		t.Fatal("different adapter binary was joined")
	}
	memory.Manifest.Lock.Runtimes = base.Manifest.Lock.Runtimes
	memory.Manifest.Lock.Workloads = []protocol.Workload{{ID: "w", SHA256: "different"}}
	if _, err := matchingMemoryCells(base, memory); err == nil {
		t.Fatal("different workload artifact was joined")
	}
	badTiming := base
	badTiming.Manifest.Lock.Options.Profile = "memory"
	if _, err := matchingMemoryCells(badTiming, memory); err == nil {
		t.Fatal("non-timing primary run was joined")
	}
}

func TestMemoryPairRejectsDifferentExperimentIdentity(t *testing.T) {
	base := experiment.Bundle{Manifest: experiment.Manifest{Kind: "measurement", Host: agent.Host{OS: "linux", Arch: "amd64", CPU: "test", Kernel: "kernel-a", Policy: map[string]string{"smt": "off"}}, Lock: experiment.Lock{
		Protocol:  1,
		Options:   experiment.Options{Profile: "timing", Resources: agent.ResourcePolicy{MemoryMaxBytes: 1024}},
		Runtimes:  []experiment.Runtime{{ID: "r", Files: map[string]string{"adapter": "abc"}, Description: &protocol.Description{Runtime: "r", Configuration: map[string]string{"compiler": "a"}}}},
		Workloads: []protocol.Workload{{ID: "w", SHA256: "artifact", Export: "run", Artifact: "/original.wasm"}},
	}}}
	for name, change := range map[string]func(*experiment.Bundle){
		"IRQ requirement":       func(b *experiment.Bundle) { b.Manifest.Lock.RequireIRQAffinity = true },
		"partition requirement": func(b *experiment.Bundle) { b.Manifest.Lock.RequireIsolatedCPUPartition = true },
		"host baseline": func(b *experiment.Bundle) {
			b.Manifest.Lock.HostPolicy = &agent.HostPolicy{Version: agent.HostPolicyVersion}
		},
		"host kernel":     func(b *experiment.Bundle) { b.Manifest.Host.Kernel = "kernel-b" },
		"resource budget": func(b *experiment.Bundle) { b.Manifest.Lock.Options.Resources.MemoryMaxBytes = 2048 },
		"protocol":        func(b *experiment.Bundle) { b.Manifest.Lock.Protocol = 2 },
		"effective configuration": func(b *experiment.Bundle) {
			b.Manifest.Lock.Runtimes = []experiment.Runtime{{ID: "r", Files: map[string]string{"adapter": "abc"}, Description: &protocol.Description{Runtime: "r", Configuration: map[string]string{"compiler": "b"}}}}
		},
		"input contract": func(b *experiment.Bundle) {
			b.Manifest.Lock.Workloads = []protocol.Workload{{ID: "w", SHA256: "artifact", Export: "other", Artifact: "/restored.wasm"}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			memory := base
			memory.Manifest.Lock.Options.Profile = "memory"
			change(&memory)
			if _, err := matchingMemoryCells(base, memory); err == nil {
				t.Fatal("mismatched timing/memory experiment was joined")
			}
		})
	}
	memory := base
	memory.Manifest.Lock.Options.Profile = "memory"
	memory.Manifest.Lock.Workloads = []protocol.Workload{{ID: "w", SHA256: "artifact", Export: "run", Artifact: "/restored.wasm"}}
	if _, err := matchingMemoryCells(base, memory); err != nil {
		t.Fatalf("same contract at a restored artifact path rejected: %v", err)
	}
}

func TestSustainedPassPairRequiresSameFixedBudgets(t *testing.T) {
	base := experiment.Bundle{Manifest: experiment.Manifest{Kind: "measurement", Lock: experiment.Lock{
		Options:   experiment.Options{Profile: "timing", SustainedDuration: time.Second, Samples: 3000, Operations: 1000, Warmup: 10},
		Runtimes:  []experiment.Runtime{{ID: "r", Files: map[string]string{"adapter": "same"}}},
		Workloads: []protocol.Workload{{ID: "w", SHA256: "same"}},
	}}}
	for name, change := range map[string]func(*experiment.Options){
		"duration":   func(o *experiment.Options) { o.SustainedDuration = 2 * time.Second },
		"samples":    func(o *experiment.Options) { o.Samples++ },
		"operations": func(o *experiment.Options) { o.Operations++ },
		"warmup":     func(o *experiment.Options) { o.Warmup++ },
	} {
		t.Run(name, func(t *testing.T) {
			memory := base
			memory.Manifest.Lock.Options.Profile = "memory"
			if _, err := matchingMemoryCells(base, memory); err != nil {
				t.Fatal(err)
			}
			change(&memory.Manifest.Lock.Options)
			if _, err := matchingMemoryCells(base, memory); err == nil {
				t.Fatal("mismatched sustained budget joined")
			}
		})
	}
}

func TestMemoryStageIntervalAndRawLaunchValues(t *testing.T) {
	values := []float64{30, 10, 20}
	b := experiment.Bundle{}
	for i, value := range values {
		b.Trials = append(b.Trials, experiment.Trial{ID: []string{"third", "first", "second"}[i], Runtime: "r", Workload: "w", Scenario: "steady", Profile: "memory", Status: "ok", Block: i,
			Observations: []protocol.Observation{{Metric: "process.peak_rss", Unit: "bytes", Status: "available", Phase: "steady/process_lifetime", Quality: "kernel_accounted_peak", Collector: "wait4_rusage", Value: protocol.Value(value)}},
		})
	}
	rows := memoryStages(b, map[string]bool{"r\x00w": true})
	if len(rows) != 1 {
		t.Fatalf("expected one metric row, got %+v", rows)
	}
	row := rows[0]
	if *row.Median != 20 || row.Low == nil || row.High == nil || *row.Low < 10 || *row.High > 30 || row.Launches != 3 {
		t.Fatalf("bad memory uncertainty: %+v", row)
	}
	for i, launch := range row.LaunchValues {
		if launch.TrialID != b.Trials[i].ID || launch.Bytes != values[i] {
			t.Fatalf("launch value lost its raw trial identity: %+v", row.LaunchValues)
		}
	}
}

func TestGuestDensityMemoryStageUsesOnlyProvisioningDomain(t *testing.T) {
	o := protocol.Observation{Metric: "host.alloc.bytes", DefinitionVersion: 1, Unit: "bytes", Value: protocol.Value(100), Status: "available", Phase: protocol.GuestDensityPhase, Quality: "engine_reported", Collector: "runtime.ReadMemStats", Scope: "adapter_process_go_heap", Denominator: protocol.GuestDensityDenominator}
	if value, ok := memoryObservationValue(o, o.Metric, "guest-density", 1); !ok || value != 100 {
		t.Fatal("provisioning allocation omitted", value, ok)
	}
	o.Denominator = "batch"
	if _, ok := memoryObservationValue(o, o.Metric, "guest-density", 1); ok {
		t.Fatal("incorrect normalization accepted")
	}
	o.Denominator = protocol.GuestDensityDenominator
	o.Scope = "adapter_process_v8_heap"
	if _, ok := memoryObservationValue(o, o.Metric, "guest-density", 1); ok {
		t.Fatal("allocator scopes merged")
	}
}
