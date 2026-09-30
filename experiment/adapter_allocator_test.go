package experiment_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/parquet-go/parquet-go"
	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
	"github.com/wasmbench/wasmbench/publish"
)

func TestWasmtimeRustAllocatorControllerRoundTrip(t *testing.T) {
	testRustAllocatorRoundTrip(t, false)
}

func TestWasmtimeRustAllocatorPhaseControllerRoundTrip(t *testing.T) {
	testRustAllocatorRoundTrip(t, true)
}

func testRustAllocatorRoundTrip(t *testing.T, phased bool) {
	t.Helper()
	if os.Getenv("WASMBENCH_RUST_ALLOCATOR_TEST") != "1" {
		t.Skip("build wasmtime-allocator and wasmtime-winch-allocator, then set WASMBENCH_RUST_ALLOCATOR_TEST=1")
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	runtimes, err := experiment.ResolveRuntimes(root, []string{"wasmtime-allocator", "wasmtime-winch-allocator"})
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	workloads, err := corpus.Generate(filepath.Join(tmp, "corpus"), "core")
	if err != nil {
		t.Fatal(err)
	}
	lock, err := experiment.NewLock(experiment.Options{Suite: "core", Profile: "memory", Scenarios: []string{"compile", "instantiate", "first-call", "steady"}, Launches: 1, Samples: 3, Operations: 1, PhaseBarriers: phased, Timeout: 15 * time.Second}, runtimes, workloads)
	if err != nil {
		t.Fatal(err)
	}
	lock.Analyzer, err = experiment.PinAnalyzer(filepath.Join(root, "adapters/wasmtime/target/release/wasm-analyze"), "default")
	if err != nil {
		t.Fatal(err)
	}
	out, err := experiment.Run(context.Background(), lock, tmp, filepath.Join(tmp, "run"), func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	b, err := experiment.Load(out)
	if err != nil {
		t.Fatal(err)
	}
	measured := 0
	for i, trial := range b.Trials {
		if trial.Status != "ok" {
			t.Fatal(trial.Runtime, trial.Scenario, trial.Status, trial.Reason)
		}
		for index, sample := range trial.Samples {
			if err := protocol.ValidateRustAllocatorSample(sample, trial.Scenario); err != nil {
				t.Fatal(err)
			}
			if err := protocol.ValidateRustAllocatorRelease(sample, trial.Scenario, index+1 == len(trial.Samples)); err != nil {
				t.Fatal(err)
			}
		}
		if trial.Block < 0 {
			continue
		}
		measured++
		if phased {
			if len(trial.PhaseEvents) != 4*len(trial.Samples) {
				t.Fatal("incomplete allocator process boundaries", trial)
			}
			originalStage := b.Trials[i].PhaseEvents[2].Event.Stage
			b.Trials[i].PhaseEvents[2].Event.Stage = "released"
			if experiment.ValidateRustAllocatorEvidence(b) == nil {
				t.Fatal("verification/release boundary relabel accepted")
			}
			b.Trials[i].PhaseEvents[2].Event.Stage = originalStage
			last := len(b.Trials[i].Samples[0].Observations) - 1
			original := b.Trials[i].Samples[0].Observations[last]
			b.Trials[i].Samples[0].Observations[last].Reason = "changed snapshot"
			if experiment.ValidateRustAllocatorEvidence(b) == nil {
				t.Fatal("detached boundary evidence accepted")
			}
			b.Trials[i].Samples[0].Observations[last] = original
		}
		original := b.Trials[i].Profile
		b.Trials[i].Profile = "timing"
		if experiment.ValidateRustAllocatorEvidence(b) == nil {
			t.Fatal("instrumented timing evidence accepted")
		}
		b.Trials[i].Profile = original
		originalObservation := b.Trials[i].Samples[0].Observations[6]
		b.Trials[i].Samples[0].Observations[6].Phase = trial.Scenario
		if experiment.ValidateRustAllocatorEvidence(b) == nil {
			t.Fatal("release evidence accepted in API window")
		}
		b.Trials[i].Samples[0].Observations[6] = originalObservation
		originalIndex := b.Trials[i].Samples[0].Index
		b.Trials[i].Samples[0].Index = 100
		if experiment.ValidateRustAllocatorEvidence(b) == nil {
			t.Fatal("changed release sequence accepted")
		}
		b.Trials[i].Samples[0].Index = originalIndex
	}
	if measured != 16 {
		t.Fatal("incomplete allocator coverage", measured)
	}
	report := filepath.Join(tmp, "report")
	if err := publish.Report(out, report); err != nil {
		t.Fatal(err)
	}
	if err := publish.VerifyReport(report); err != nil {
		t.Fatal(err)
	}
	// Read the actual report export, not a synthetic observation fixture. Retained
	// Store samples must remain typed nulls, while performed release windows keep
	// their exact values and independent measurement domain.
	rows, err := parquet.ReadFile[publish.ObservationRow](filepath.Join(report, "observations.parquet"))
	if err != nil {
		t.Fatal(err)
	}
	available, retained := 0, 0
	for _, row := range rows {
		if row.Block < 0 || !strings.HasPrefix(row.Metric, "host.rust.release.") {
			continue
		}
		if row.SampleIndex == nil || row.Operations == nil || *row.Operations != 1 || row.Verified == nil || !*row.Verified || row.Phase != row.Scenario+"/logical_release_window" || row.Denominator != "single_logical_release_operation" || row.Profile != "memory" || row.InstrumentationProfile != "memory" || row.Scope != "allocations_routed_through_rust_global_allocator" || row.CollectorVersion != protocol.RustAllocatorVersion {
			t.Fatal("release domain lost in Parquet", row)
		}
		var original *protocol.Observation
		for _, trial := range b.Trials {
			if trial.ID != row.Trial {
				continue
			}
			for _, sample := range trial.Samples {
				if int64(sample.Index) != *row.SampleIndex {
					continue
				}
				for _, observation := range sample.Observations {
					if observation.Metric == row.Metric {
						copy := observation
						original = &copy
					}
				}
			}
		}
		if original == nil || row.Status != original.Status || row.Reason != original.Reason || row.Unit != original.Unit || row.Collector != original.Collector || row.Quality != original.Quality || row.DefinitionVersion != int64(original.DefinitionVersion) {
			t.Fatal("release provenance changed in Parquet", row, original)
		}
		switch row.Status {
		case "available":
			available++
			if row.Value == nil || original.Value == nil || *row.Value != *original.Value {
				t.Fatal("release value changed in Parquet", row, original)
			}
		case "not_applicable":
			retained++
			if row.Value != nil || original.Value != nil || row.Scenario != "steady" || *row.SampleIndex >= 2 {
				t.Fatal("retained Store became a measured release", row)
			}
		default:
			t.Fatal("unexpected release availability", row)
		}
	}
	if available != 280 || retained != 56 {
		t.Fatal("incomplete release export", available, retained)
	}
	// The instrumented build is not usable as a headline timing configuration.
	lock.Options.Profile = "timing"
	lock.Options.PhaseBarriers = false
	timing, err := experiment.Run(context.Background(), lock, tmp, filepath.Join(tmp, "timing-rejected"), func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	rejected, err := experiment.Load(timing)
	if err != nil {
		t.Fatal(err)
	}
	for _, trial := range rejected.Trials {
		if trial.Status != "unsupported" || len(trial.Samples) != 0 {
			t.Fatal("instrumented timing run admitted", trial)
		}
	}
}

func TestWasmtimeRustAllocatorWorkloadContracts(t *testing.T) {
	if os.Getenv("WASMBENCH_RUST_ALLOCATOR_TEST") != "1" {
		t.Skip("build allocator adapters, then set WASMBENCH_RUST_ALLOCATOR_TEST=1")
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	runtimes, err := experiment.ResolveRuntimes(root, []string{"wasmtime-allocator", "wasmtime-winch-allocator"})
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		suite  string
		phased bool
	}{{"floats", false}, {"floats", true}, {"lifecycle", false}, {"lifecycle", true}} {
		suite, name := fixture.suite, fixture.suite
		if fixture.phased {
			name += "-boundaries"
		}
		t.Run(name, func(t *testing.T) {
			tmp := t.TempDir()
			workloads, err := corpus.Generate(filepath.Join(tmp, "corpus"), suite)
			if err != nil {
				t.Fatal(err)
			}
			lock, err := experiment.NewLock(experiment.Options{Suite: suite, Profile: "memory", Scenarios: []string{"compile", "instantiate", "first-call", "steady"}, Launches: 1, Samples: 3, Operations: 1, PhaseBarriers: fixture.phased, Timeout: 15 * time.Second}, runtimes, workloads)
			if err != nil {
				t.Fatal(err)
			}
			lock.Analyzer, err = experiment.PinAnalyzer(filepath.Join(root, "adapters/wasmtime/target/release/wasm-analyze"), "default")
			if err != nil {
				t.Fatal(err)
			}
			out, err := experiment.Run(context.Background(), lock, tmp, filepath.Join(tmp, "run"), func(string) {})
			if err != nil {
				t.Fatal(err)
			}
			bundle, err := experiment.Load(out)
			if err != nil {
				t.Fatal(err)
			}
			measured, unsupported := 0, 0
			for _, trial := range bundle.Trials {
				if suite == "lifecycle" && trial.Scenario == "steady" {
					if trial.Status != "unsupported" || len(trial.Samples) != 0 {
						t.Fatal("fresh-state steady run admitted", trial)
					}
					if trial.Block >= 0 {
						unsupported++
					}
					continue
				}
				if trial.Status != "ok" || len(trial.Samples) == 0 {
					t.Fatal(trial.Runtime, trial.Workload, trial.Scenario, trial.Status, trial.Reason)
				}
				for i, sample := range trial.Samples {
					if err := protocol.ValidateRustAllocatorSample(sample, trial.Scenario); err != nil {
						t.Fatal(err)
					}
					if err := protocol.ValidateRustAllocatorRelease(sample, trial.Scenario, i+1 == len(trial.Samples)); err != nil {
						t.Fatal(err)
					}
				}
				if trial.Block >= 0 {
					measured++
				}
			}
			want := len(workloads) * len(runtimes) * 4
			if suite == "lifecycle" {
				want = len(workloads) * len(runtimes) * 3
				if unsupported != len(workloads)*len(runtimes) {
					t.Fatal("missing explicit reset-policy exclusions", unsupported)
				}
			}
			if measured != want {
				t.Fatal("incomplete workload coverage", measured, want)
			}
			report := filepath.Join(tmp, "report")
			if err := publish.Report(out, report); err != nil {
				t.Fatal(err)
			}
			if err := publish.VerifyReport(report); err != nil {
				t.Fatal(err)
			}
			// Exercise a wrong oracle directly for every supported API boundary,
			// not just the controller's sacrificial admission invocation.
			wrong := workloads[0]
			if suite == "floats" {
				wrong.Oracle.Expected = protocol.Values{0}
			} else {
				wrong.Oracle.Memory = append([]protocol.MemoryCheck(nil), wrong.Oracle.Memory...)
				wrong.Oracle.Memory[0].Hex = "00000000"
			}
			for _, runtime := range runtimes {
				client, err := agent.Start(context.Background(), runtime.Command, filepath.Join(t.TempDir(), "log"), 15*time.Second)
				if err != nil {
					t.Fatal(err)
				}
				for _, scenario := range lock.Options.Scenarios {
					if suite == "lifecycle" && scenario == "steady" {
						continue
					}
					_, err := client.Call(protocol.Request{Method: "prepare", Prepare: &protocol.Preparation{Artifact: wrong.Artifact, ArtifactSHA256: wrong.SHA256, Workload: wrong, Profile: "memory"}})
					if err != nil {
						client.Close()
						t.Fatal(err)
					}
					response, err := client.Call(protocol.Request{Method: "run", Run: &protocol.RunRequest{Scenario: scenario, Samples: 3, Operations: 1}})
					if err == nil || !strings.Contains(err.Error(), "incorrect result") || len(response.Samples) != 0 {
						client.Close()
						t.Fatal("wrong oracle produced allocator evidence", runtime.ID, scenario, response, err)
					}
				}
				client.Close()
			}
			wrongLock := lock
			wrongLock.Workloads = []protocol.Workload{wrong}
			wrongOut, err := experiment.Run(context.Background(), wrongLock, tmp, filepath.Join(tmp, "wrong-oracle"), func(string) {})
			if err != nil {
				t.Fatal(err)
			}
			wrongBundle, err := experiment.Load(wrongOut)
			if err != nil {
				t.Fatal(err)
			}
			for _, trial := range wrongBundle.Trials {
				want := "preflight_failed"
				if trial.Block < 0 {
					want = "incorrect_result"
				}
				if trial.Status != want || len(trial.Samples) != 0 {
					t.Fatal("incorrect oracle classified or admitted incorrectly", trial)
				}
			}
		})
	}
}

// Consume sealed evidence from an actual Linux run. Portable integration tests
// also cover unavailable collectors, but cannot establish Linux residency.
func TestRustAllocatorLinuxBoundaryEvidence(t *testing.T) {
	root := os.Getenv("WASMBENCH_RUST_ALLOCATOR_LINUX_BUNDLE")
	if root == "" {
		t.Skip("set WASMBENCH_RUST_ALLOCATOR_LINUX_BUNDLE to a sealed Linux boundary run")
	}
	b, err := experiment.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	lock := b.Manifest.Lock
	if b.Manifest.Host.OS != "linux" || lock.Options.Profile != "memory" || !lock.Options.PhaseBarriers || lock.Options.Check {
		t.Fatal("expected Linux diagnostic phase run")
	}
	measured := 0
	for _, trial := range b.Trials {
		if trial.Status != "ok" {
			t.Fatal("incomplete Linux coverage", trial.ID, trial.Status, trial.Reason)
		}
		if trial.Block < 0 {
			continue
		}
		measured++
		if len(trial.PhaseEvents) != 4*len(trial.Samples) {
			t.Fatal("incomplete process boundaries", trial.ID)
		}
		for _, record := range trial.PhaseEvents {
			available := 0
			for _, o := range record.Observations {
				if o.Scope == "adapter_process" && strings.HasPrefix(o.Metric, "process.") {
					if o.Status != "available" || o.Value == nil || o.Quality != "boundary_snapshot_only" {
						t.Fatal("Linux residency was not observed", trial.ID, o)
					}
					available++
				}
				if o.Metric == "cgroup.memory.phase_peak" && o.Status != "available" && o.Value != nil {
					t.Fatal("unavailable cgroup peak became numerical", o)
				}
			}
			if available != 4 {
				t.Fatal("RSS/PSS/private/virtual snapshot coverage incomplete", trial.ID, record.Event)
			}
		}
	}
	if measured != len(lock.Workloads)*len(lock.Runtimes)*len(lock.Options.Scenarios)*lock.Options.Launches {
		t.Fatal("Linux boundary cells missing", measured)
	}
}
