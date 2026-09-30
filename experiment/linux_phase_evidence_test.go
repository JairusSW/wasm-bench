package experiment_test

import (
	"fmt"
	"os"
	"slices"
	"testing"

	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

// This consumes an actual Linux run, not synthetic snapshots. Load verifies
// checksums; this gate additionally verifies measurement coverage and semantics.
func TestLinuxPhaseEvidence(t *testing.T) {
	root := os.Getenv("WASMBENCH_LINUX_PHASE_BUNDLE")
	if root == "" {
		t.Skip("set WASMBENCH_LINUX_PHASE_BUNDLE to a Linux phase bundle")
	}
	b, err := experiment.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	lock := b.Manifest.Lock
	if !lock.Options.PhaseBarriers || lock.Options.Profile != "memory" || lock.Options.Check || lock.Options.Launches < 1 || lock.Options.Samples < 1 || len(lock.Options.Scenarios) != 1 || len(protocol.PhaseStages(lock.Options.Scenarios[0])) == 0 {
		t.Fatal("expected a nonempty memory phase experiment")
	}
	scenario := lock.Options.Scenarios[0]
	stages := protocol.PhaseStages(scenario)
	expected := map[string]bool{}
	commands := map[string]*protocol.CommandContract{}
	workloads := map[string]protocol.Workload{}
	for _, w := range lock.Workloads {
		workloads[w.ID] = w
		if w.Command != nil {
			commands[w.ID] = w.Command
		}
	}
	for _, runtime := range lock.Runtimes {
		if scenario == "instantiate" && (runtime.Description == nil || runtime.Description.Configuration["instantiate_release_policy"] == "" || !slices.Contains(runtime.Description.PhaseBarrierScenarios, scenario)) {
			t.Fatalf("%s: missing instantiation boundary/release policy", runtime.ID)
		}
		if scenario == "app-init" && (runtime.Description == nil || runtime.Description.Configuration["app_init_release_policy"] == "" || !slices.Contains(runtime.Description.PhaseBarrierScenarios, scenario)) {
			t.Fatalf("%s: missing initialization boundary/release policy", runtime.ID)
		}
		for _, workload := range lock.Workloads {
			for block := 0; block < lock.Options.Launches; block++ {
				expected[fmt.Sprintf("%s/%s/%d", runtime.ID, workload.ID, block)] = true
			}
		}
	}
	measured := 0
	for _, trial := range b.Trials {
		if trial.Status != "ok" {
			t.Fatalf("%s: %s", trial.ID, trial.Status)
		}
		if trial.Block < 0 {
			continue
		}
		measured++
		key := fmt.Sprintf("%s/%s/%d", trial.Runtime, trial.Workload, trial.Block)
		if !expected[key] {
			t.Fatalf("unexpected or duplicate measured cell: %s", key)
		}
		delete(expected, key)
		if len(trial.Samples) != lock.Options.Samples {
			t.Fatalf("%s: sample budget mismatch", trial.ID)
		}
		if trial.Isolation == nil || trial.Isolation.Mode != "cgroup_v2_at_spawn" || trial.Scenario != scenario || len(trial.Samples) == 0 || len(trial.PhaseEvents) != len(stages)*len(trial.Samples) {
			t.Fatalf("%s: missing isolation/samples/barriers", trial.ID)
		}
		for i, sample := range trial.Samples {
			if !sample.Verified || sample.Index != i || sample.Warmup || sample.Operations != 1 || sample.SampleType != "individual_operation" {
				t.Fatalf("%s: invalid phase sample %+v", trial.ID, sample)
			}
			w := workloads[trial.Workload]
			if scenario == "instantiate" {
				for _, o := range sample.Observations {
					if o.Scope == "adapter_process_go_heap" && (o.Phase != "instantiate/api_window" || o.Denominator != "instantiation_including_start_excluding_initialization_verification_release") {
						t.Fatalf("%s: instantiation allocator boundary changed: %+v", trial.ID, o)
					}
				}
			}
			if scenario == "app-init" {
				if len(sample.Observations) == 0 {
					t.Fatalf("%s: missing initialization diagnostics", trial.ID)
				}
				for _, o := range sample.Observations {
					if o.Scope == "adapter_process_go_heap" && (o.Phase != "app-init/api_window" || o.Denominator != "initialization_call_excluding_input_verification_release") {
						t.Fatalf("%s: initialization allocator boundary changed: %+v", trial.ID, o)
					}
				}
			}
			if (scenario == "teardown" || scenario == "app-init" || scenario == "instantiate") && w.Oracle.Kind == "exact_u64" && !slices.Equal(sample.Result, w.Oracle.Expected) {
				t.Fatalf("%s: invalid retained scalar result evidence", trial.ID)
			}
			if command := commands[trial.Workload]; command != nil {
				if sample.CommandResult == nil || command.Verify(*sample.CommandResult) != nil {
					t.Fatalf("%s: invalid command result evidence", trial.ID)
				}
				for _, o := range sample.Observations {
					denominator := "operation_excluding_verification_release_and_barriers"
					if scenario == "teardown" {
						denominator = "remaining_command_resources_excluding_execution_verification_fixture_cleanup"
					}
					if o.Scope == "adapter_process_go_heap" && (o.Phase != scenario+"/api_window" || o.Denominator != denominator) {
						t.Fatalf("%s: command compile allocation boundary changed: %+v", trial.ID, o)
					}
				}
			}
		}
		for i, record := range trial.PhaseEvents {
			stage := stages[i%len(stages)]
			if record.Event.Stage != stage || record.Event.SampleIndex != i/len(stages) {
				t.Fatalf("%s: unordered event %d", trial.ID, i)
			}
			require := func(metric, scope, phase, quality string) {
				t.Helper()
				var found []protocol.Observation
				for _, o := range record.Observations {
					if o.Metric == metric {
						found = append(found, o)
					}
				}
				if len(found) != 1 {
					t.Fatalf("%s %s: expected one %s, got %d", trial.ID, stage, metric, len(found))
				}
				o := found[0]
				if o.Status != "available" || o.Value == nil || *o.Value < 0 || o.Scope != scope || o.Phase != phase || o.Quality != quality {
					t.Fatalf("%s: invalid observation %+v", trial.ID, o)
				}
			}
			for _, metric := range []string{"process.rss", "process.pss", "process.private", "process.virtual"} {
				require(metric, "adapter_process", scenario+"/"+stage, "boundary_snapshot_only")
			}
			for _, suffix := range []string{"current", "anon", "file", "kernel", "sock", "pagetables", "slab"} {
				require("cgroup.memory."+suffix, "adapter_cgroup", scenario+"/"+stage, "boundary_snapshot_only")
			}
			if stage == "compiled" || stage == "torn_down" || stage == "app_initialized" || stage == "instantiated" {
				require("cgroup.memory.phase_peak", "adapter_cgroup", scenario+"/barrier_window", "kernel_accounted_peak")
				for _, suffix := range []string{"total", "user", "system"} {
					require("time.cpu."+suffix, "adapter_cgroup_process_tree", scenario+"/barrier_window", "kernel_accounted_delta")
				}
				for _, suffix := range []string{"periods", "throttled_periods", "throttled_time"} {
					require("cgroup.cpu."+suffix, "adapter_cgroup_local_bandwidth", scenario+"/barrier_window", "kernel_accounted_delta")
				}
			}
		}
	}
	if measured == 0 {
		t.Fatal("no measured trials")
	}
	if len(expected) != 0 {
		t.Fatalf("missing measured cells: %v", expected)
	}
	t.Logf("verified Linux measurement coverage for %d trials", measured)
}
