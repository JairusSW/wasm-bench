package experiment

import (
	"context"
	"github.com/wasmbench/wasmbench/protocol"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func componentU64Fixture(t *testing.T) (protocol.Workload, []Runtime, *AnalyzerLock) {
	t.Helper()
	artifact, adapter, analyzer := os.Getenv("WASMBENCH_COMPONENT_U64_ARTIFACT"), os.Getenv("WASMBENCH_COMPONENT_ADAPTER"), os.Getenv("WASMBENCH_COMPONENT_ANALYZER")
	if artifact == "" || adapter == "" || analyzer == "" {
		t.Skip("set component u64 artifact, adapter and analyzer paths")
	}
	artifact, _ = filepath.Abs(artifact)
	adapter, _ = filepath.Abs(adapter)
	artifactHash, err := DigestFile(artifact)
	if err != nil {
		t.Fatal(err)
	}
	adapterHash, err := DigestFile(adapter)
	if err != nil {
		t.Fatal(err)
	}
	a, err := PinAnalyzer(analyzer, "default")
	if err != nil {
		t.Fatal(err)
	}
	w := protocol.Workload{Schema: 1, ID: "components/u64-state-reset", Family: "mechanisms", Artifact: artifact, SHA256: artifactHash, ABI: "component", HostProfile: protocol.ComponentU64Policy, Features: []string{"component-model"}, Export: "benchmark", Args: protocol.Values{7}, WorkUnit: "invocation", Units: 1, Reset: "fresh_instance_per_sample", Oracle: protocol.Oracle{Kind: "exact_u64", Expected: protocol.Values{8}}, License: "MIT", Source: "recipes/fixtures/component-u64.wat", Generator: "wat 1.251.0"}
	runtimes := []Runtime{{ID: "wasmtime", Command: []string{adapter}, Files: map[string]string{adapter: adapterHash}}, {ID: "wasmtime-winch", Command: []string{adapter, "--winch"}, Files: map[string]string{adapter: adapterHash}}}
	return w, runtimes, a
}

func TestComponentU64Bundle(t *testing.T) {
	w, runtimes, a := componentU64Fixture(t)
	artifact, adapter := w.Artifact, runtimes[0].Command[0]
	lock, err := NewLock(Options{Suite: "component-u64", Profile: "timing", Scenarios: []string{"compile", "instantiate", "first-call"}, Launches: 1, Samples: 3, Operations: 1, Timeout: 30 * time.Second}, runtimes, []protocol.Workload{w})
	if err != nil {
		t.Fatal(err)
	}
	lock.Analyzer, lock.ArchiveTools = a, true
	base := os.Getenv("WASMBENCH_COMPONENT_U64_EVIDENCE_DIR")
	if base == "" {
		base = filepath.Join(t.TempDir(), "component-u64")
	}
	ctx := context.Background()
	original, err := Run(ctx, lock, filepath.Dir(artifact), base, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	restored, err := RestoreTools(original, base+"-tools")
	if err != nil {
		t.Fatal(err)
	}
	var replayLock Lock
	if err := ReadJSON(restored.Lock, &replayLock); err != nil {
		t.Fatal(err)
	}
	if replayLock.Runtimes[0].Command[0] == adapter {
		t.Fatal("replay used original adapter")
	}
	replayed, err := Run(ctx, replayLock, filepath.Dir(restored.Lock), base+"-replayed", func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{original, replayed} {
		b, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(b.Trials) != 8 {
			t.Fatal("incomplete backend/stage coverage", len(b.Trials))
		}
		for _, trial := range b.Trials {
			wantSamples := 3
			if trial.Block < 0 {
				wantSamples = 1
			}
			if trial.Status != "ok" || len(trial.Samples) != wantSamples {
				t.Fatal(trial.ID, trial.Status, trial.Reason)
			}
			for _, s := range trial.Samples {
				if !s.Verified || len(s.Result) != 1 || s.Result[0] != 8 || s.Operations != 1 || s.Warmup || s.SampleType != "individual_operation" {
					t.Fatal("invalid typed component evidence", s)
				}
			}
			if len(trial.PhaseEvents) != 0 || len(trial.Observations) != 0 {
				t.Fatal("timing instrumented")
			}
		}
	}
	for _, name := range []string{"wrong-type", "missing", "trap", "wrong-oracle", "profiling"} {
		bad := lock
		bad.ArchiveTools = false
		bad.Workloads = append([]protocol.Workload(nil), lock.Workloads...)
		bad.Options.Scenarios = []string{"first-call"}
		if name == "wrong-oracle" {
			bad.Workloads[0].Oracle.Expected = protocol.Values{9}
		} else if name == "profiling" {
			bad.Options.Profile = "profiling"
		} else {
			bad.Workloads[0].Export = name
		}
		path, err := Run(ctx, bad, filepath.Dir(artifact), base+"-"+name, func(string) {})
		if err != nil {
			t.Fatal(err)
		}
		b, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, trial := range b.Trials {
			if trial.Status == "ok" || len(trial.Samples) != 0 {
				t.Fatal("invalid typed component call measured", name, trial)
			}
		}
	}
}

func TestComponentU64MemoryBundle(t *testing.T) {
	w, runtimes, analyzer := componentU64Fixture(t)
	lock, err := NewLock(Options{Suite: "component-u64-memory", Profile: "memory", Scenarios: []string{"compile", "instantiate", "first-call"}, Launches: 1, Samples: 3, Operations: 1, PhaseBarriers: true, Timeout: 30 * time.Second}, runtimes, []protocol.Workload{w})
	if err != nil {
		t.Fatal(err)
	}
	lock.Analyzer, lock.ArchiveTools = analyzer, true
	base := os.Getenv("WASMBENCH_COMPONENT_U64_EVIDENCE_DIR")
	if base == "" {
		base = filepath.Join(t.TempDir(), "component-u64")
	}
	base += "-memory"
	original, err := Run(context.Background(), lock, filepath.Dir(w.Artifact), base, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	restored, err := RestoreTools(original, base+"-tools")
	if err != nil {
		t.Fatal(err)
	}
	var replayLock Lock
	if err := ReadJSON(restored.Lock, &replayLock); err != nil {
		t.Fatal(err)
	}
	for i, r := range replayLock.Runtimes {
		if r.Command[0] == runtimes[i].Command[0] {
			t.Fatal("replay used original adapter")
		}
	}
	replayed, err := Run(context.Background(), replayLock, filepath.Dir(restored.Lock), base+"-replayed", func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{original, replayed} {
		b, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(b.Trials) != 8 {
			t.Fatal("missing backend/stage/check coverage", len(b.Trials))
		}
		cells := map[string]bool{}
		for _, trial := range b.Trials {
			wantSamples, wantPhases := 3, 9
			if trial.Block < 0 {
				wantSamples, wantPhases = 1, 0
			} else {
				cells[trial.Runtime+"/"+trial.Scenario] = true
			}
			if trial.Status != "ok" || len(trial.Samples) != wantSamples || len(trial.PhaseEvents) != wantPhases {
				t.Fatal("incomplete memory trial", trial.ID, trial.Status, trial.Reason, len(trial.PhaseEvents))
			}
			stages := protocol.PhaseStages(trial.Scenario)
			for i, record := range trial.PhaseEvents {
				if record.Event.SampleIndex != i/3 || record.Event.Stage != stages[i%3] || len(record.Observations) == 0 {
					t.Fatal("missing ordered external boundary", record)
				}
			}
			for _, sample := range trial.Samples {
				if !sample.Verified || len(sample.Result) != 1 || sample.Result[0] != 8 || sample.Operations != 1 || sample.Warmup || sample.SampleType != "individual_operation" {
					t.Fatal("invalid typed memory sample", sample)
				}
				if trial.Block >= 0 {
					for _, stage := range stages {
						phase := trial.Scenario + "/" + stage
						if !slices.ContainsFunc(sample.Observations, func(o protocol.Observation) bool { return o.Phase == phase }) {
							t.Fatal("boundary observations not attached to sample", sample.Index, phase)
						}
					}
				} else if len(sample.Observations) != 0 {
					t.Fatal("sacrificial check sample instrumented")
				}
			}
			if trial.Block >= 0 && !slices.ContainsFunc(trial.Observations, func(o protocol.Observation) bool { return o.Metric == "process.peak_rss" }) {
				t.Fatal("missing whole-process high-water mark")
			}
			if trial.Block < 0 && len(trial.Observations) != 0 {
				t.Fatal("sacrificial timing check instrumented")
			}
		}
		if len(cells) != 6 {
			t.Fatal("missing measured cells", cells)
		}
	}
}

func TestComponentU64ControllerMemoryCapabilityGate(t *testing.T) {
	w := protocol.Workload{ABI: "component", HostProfile: protocol.ComponentU64Policy, Export: "benchmark", Args: protocol.Values{7}, Oracle: protocol.Oracle{Kind: "exact_u64", Expected: protocol.Values{8}}, Reset: "fresh_instance_per_sample", WorkUnit: "invocation", Units: 1}
	for _, mode := range []string{"missing-memory", "missing-calls", "timing-barriers", "missing-phase", "profiling", "operations", "steady"} {
		t.Run(mode, func(t *testing.T) {
			o := Options{Profile: "memory", PhaseBarriers: true, Operations: 1, Samples: 1, Timeout: time.Second}
			scenario := "first-call"
			caps := map[string]bool{"can_component_u64_calls_v1": true, "can_component_u64_memory_v1": true}
			phases := []string{scenario}
			switch mode {
			case "missing-memory":
				delete(caps, "can_component_u64_memory_v1")
			case "missing-calls":
				delete(caps, "can_component_u64_calls_v1")
			case "timing-barriers":
				o.Profile = "timing"
			case "missing-phase":
				phases = nil
			case "profiling":
				o.Profile = "profiling"
			case "operations":
				o.Operations = 2
			case "steady":
				scenario = "steady"
			}
			r := Runtime{ID: "refused", Description: &protocol.Description{ABIs: []string{"component"}, Scenarios: []string{scenario}, Capabilities: caps, PhaseBarrierScenarios: phases}}
			trial := runTrial(context.Background(), t.TempDir(), o, r, w, scenario, 0, "refused")
			if trial.Status != "unsupported" || len(trial.Samples) != 0 || len(trial.PhaseEvents) != 0 {
				t.Fatal("unsupported contract reached execution", trial)
			}
		})
	}
}
