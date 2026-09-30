package experiment

import (
	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestContinuationOfflineEvidence(t *testing.T) {
	root := t.TempDir()
	ws, err := corpus.Generate(root, "continuations")
	if err != nil {
		t.Fatal(err)
	}
	w := ws[2]
	if err := os.Mkdir(filepath.Join(root, "artifacts"), 0755); err != nil {
		t.Fatal(err)
	}
	canonical, _ := corpus.ContinuationModule(w.Continuation.Depth)
	path := filepath.Join(root, "artifacts", w.SHA256+".wasm")
	if err := os.WriteFile(path, canonical, 0644); err != nil {
		t.Fatal(err)
	}
	proof := &protocol.ContinuationResult{Mode: protocol.NativeContinuationMode, Depth: 8, StackResult: 43, GuestResumed: true, MemoryAfterRestoreSHA256: protocol.ContinuationMemorySHA256(false), MemoryAfterWriteSHA256: protocol.ContinuationMemorySHA256(true), GlobalAfterRestore: 99, GlobalAfterWrite: 100}
	s := protocol.Sample{Verified: true, Operations: 1, SampleType: "individual_operation", Result: protocol.Values{133}, ContinuationResult: proof}
	d := &protocol.Description{Runtime: "wazero", Version: "1.12.0", Backend: "compiler", Capabilities: map[string]bool{"can_native_continuation": true}, Configuration: map[string]string{"native_continuation_protocol": protocol.NativeContinuationMode}}
	b := Bundle{Manifest: Manifest{Lock: Lock{Workloads: []protocol.Workload{w}, Runtimes: []Runtime{{ID: "r", Description: d}}, Options: Options{Profile: "timing", Samples: 1, Operations: 1}}}, Trials: []Trial{{ID: "t", Runtime: "r", Workload: w.ID, Scenario: "continuation-resume", Profile: "timing", Block: 0, Status: "ok", Samples: []protocol.Sample{s}}}}
	verify := func() {
		t.Helper()
		if err := ValidateContinuationEvidence(root, b); err != nil {
			t.Fatal(err)
		}
	}
	reject := func() {
		t.Helper()
		if ValidateContinuationEvidence(root, b) == nil {
			t.Fatal("forged continuation evidence admitted")
		}
	}
	verify()
	proof.GuestResumed = false
	reject()
	proof.GuestResumed = true
	d.Backend = "interpreter"
	reject()
	d.Backend = "compiler"
	d.Configuration["native_continuation_protocol"] = "whole-instance"
	reject()
	d.Configuration["native_continuation_protocol"] = protocol.NativeContinuationMode
	b.Trials[0].Scenario = "checkpoint-restore"
	reject()
	b.Trials[0].Scenario = "continuation-resume"
	b.Trials[0].Samples[0].Operations = 2
	reject()
	b.Trials[0].Samples[0].Operations = 1
	b.Trials[0].Workload = "unrelated"
	reject()
	b.Trials[0].Workload = w.ID
	b.Trials[0].Block = -1
	second := s
	second.Index = 1
	b.Trials[0].Samples = append(b.Trials[0].Samples, second)
	verify()
	b.Trials[0].Block = 0
	b.Trials[0].Samples = b.Trials[0].Samples[:1]
	b.Manifest.Lock.Options.Profile = "memory"
	b.Trials[0].Profile = "memory"
	obs := collectors.GoMemoryObservations(runtime.MemStats{}, runtime.MemStats{}, "continuation-resume/operation_window", protocol.ContinuationDenominator)
	b.Trials[0].Samples[0].Observations = obs
	verify()
	b.Trials[0].Samples[0].Observations[0].Denominator = "one_whole_instance"
	reject()
	b.Trials[0].Samples[0].Observations[0].Denominator = protocol.ContinuationDenominator
	b.Manifest.Lock.Options.PhaseBarriers = true
	for _, stage := range protocol.PhaseStages("continuation-resume") {
		b.Trials[0].PhaseEvents = append(b.Trials[0].PhaseEvents, PhaseRecord{Event: protocol.PhaseEvent{Stage: stage}, Observations: collectors.Snapshot(0, "continuation-resume/"+stage)})
	}
	for _, e := range b.Trials[0].PhaseEvents {
		b.Trials[0].Samples[0].Observations = append(b.Trials[0].Samples[0].Observations, e.Observations...)
	}
	verify()
	b.Trials[0].PhaseEvents[1].Event.Stage = "returned"
	reject()
	b.Trials[0].PhaseEvents[1].Event.Stage = protocol.PhaseStages("continuation-resume")[1]
	savedReason := b.Trials[0].Samples[0].Observations[7].Reason
	b.Trials[0].Samples[0].Observations[7].Reason = "detached"
	reject()
	b.Trials[0].Samples[0].Observations[7].Reason = savedReason
	verify()
	// A kernel byte snapshot is integral and must stay exactly representable.
	original := b.Trials[0].PhaseEvents[0].Observations[0]
	for _, value := range []float64{0.5, 9007199254740992} {
		bad := original
		bad.Status, bad.Reason, bad.Value = "available", "", protocol.Value(value)
		b.Trials[0].PhaseEvents[0].Observations[0] = bad
		b.Trials[0].Samples[0].Observations[7] = bad
		reject()
	}
	b.Trials[0].PhaseEvents[0].Observations[0] = original
	b.Trials[0].Samples[0].Observations[7] = original
	verify()
	if err := os.WriteFile(path, append(canonical, 0, 1, 0), 0644); err != nil {
		t.Fatal(err)
	}
	reject()
}

func TestContinuationSealedEvidence(t *testing.T) {
	root := os.Getenv("WASMBENCH_CONTINUATION_BUNDLE")
	if root == "" {
		t.Skip("set WASMBENCH_CONTINUATION_BUNDLE to paired sealed continuation run directory")
	}
	for _, profile := range []string{"timing", "memory"} {
		b, err := Load(filepath.Join(root, profile))
		if err != nil {
			t.Fatal(err)
		}
		if b.Manifest.Lock.Options.Check || b.Manifest.Lock.Options.Profile != profile || len(b.Manifest.Lock.Workloads) != 5 {
			t.Fatal("wrong measured evidence family")
		}
		ok, unsupported, samples, barriers := 0, 0, 0, 0
		for _, trial := range b.Trials {
			if trial.Block < 0 {
				continue
			}
			if trial.Runtime == "wazero-interpreter" {
				if trial.Status != "unsupported" || len(trial.Samples) != 0 {
					t.Fatal("unqualified interpreter contributed measurements")
				}
				unsupported++
				continue
			}
			if trial.Runtime != "wazero" || trial.Status != "ok" || !protocol.IsContinuationScenario(trial.Scenario) {
				t.Fatal("failed native continuation measured cell")
			}
			ok++
			samples += len(trial.Samples)
			barriers += len(trial.PhaseEvents)
		}
		if ok != 60 || unsupported != 60 || samples != 120 {
			t.Fatalf("missing coverage: ok=%d unsupported=%d samples=%d", ok, unsupported, samples)
		}
		if profile == "memory" && barriers != 360 {
			t.Fatal("missing selected-stage boundaries")
		}
		if profile == "timing" && barriers != 0 {
			t.Fatal("timing has memory barriers")
		}
	}
}

func TestContinuationLinuxResidencyEvidence(t *testing.T) {
	root := os.Getenv("WASMBENCH_CONTINUATION_LINUX_BUNDLE")
	if root == "" {
		t.Skip("set WASMBENCH_CONTINUATION_LINUX_BUNDLE to a sealed paired Linux continuation run")
	}
	b, err := Load(filepath.Join(root, "memory"))
	if err != nil {
		t.Fatal(err)
	}
	if b.Manifest.Host.OS != "linux" || (b.Manifest.Host.Arch != "arm64" && b.Manifest.Host.Arch != "amd64") || b.Manifest.Lock.Options.Profile != "memory" || !b.Manifest.Lock.Options.PhaseBarriers || b.Manifest.Lock.Options.Check {
		t.Fatal("expected measured Linux memory-boundary evidence")
	}
	measured, snapshots := 0, 0
	for _, trial := range b.Trials {
		if trial.Block < 0 || trial.Runtime == "wazero-interpreter" {
			continue
		}
		if trial.Runtime != "wazero" || trial.Status != "ok" {
			t.Fatal("incomplete Linux native compiler qualification", trial.ID)
		}
		measured++
		for _, record := range trial.PhaseEvents {
			available := 0
			for _, o := range record.Observations {
				if o.Metric == "process.rss" || o.Metric == "process.pss" || o.Metric == "process.private" || o.Metric == "process.virtual" {
					if o.Status != "available" || o.Value == nil {
						t.Fatal("Linux residency was not collected", trial.ID, o)
					}
					available++
					snapshots++
				}
				if o.Metric == "cgroup.memory.phase_peak" && o.Status != "available" && o.Value != nil {
					t.Fatal("unavailable cgroup peak became numeric")
				}
			}
			if available != 4 {
				t.Fatal("incomplete Linux residency domain coverage", trial.ID)
			}
		}
	}
	if measured != 60 || snapshots != 1440 {
		t.Fatalf("Linux cell/residency coverage incomplete: %d/%d", measured, snapshots)
	}
}
