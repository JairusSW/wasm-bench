package experiment_test

import (
	"context"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/publish"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestV8EngineTraceControllerRoundTrip(t *testing.T) {
	if os.Getenv("WASMBENCH_V8_TIER_TEST") != "1" {
		t.Skip("set WASMBENCH_V8_TIER_TEST=1 with supported Node/V8")
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	runtimes, err := experiment.ResolveRuntimes(root, []string{"v8-tier-traced"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := runtimes[0].Files[filepath.Join(root, "adapters/v8/tracing.mjs")]; !ok {
		t.Fatal("trace helper not pinned")
	}
	tmp := t.TempDir()
	workloads, err := corpus.Generate(filepath.Join(tmp, "corpus"), "core")
	if err != nil {
		t.Fatal(err)
	}
	lock, err := experiment.NewLock(experiment.Options{Suite: "core", Profile: "profiling", Scenarios: []string{"trajectory"}, Launches: 1, Samples: 3, Warmup: 2, Operations: 1, Timeout: 15 * time.Second}, runtimes, workloads)
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
	report := filepath.Join(tmp, "report")
	if err := publish.Report(out, report); err != nil {
		t.Fatal(err)
	}
	if err := publish.VerifyReport(report); err != nil {
		t.Fatal(err)
	}
	// Resealing changed derived data must not bypass raw-evidence recomputation.
	parquetPath := filepath.Join(report, "engine-events.parquet")
	original, err := os.ReadFile(parquetPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(parquetPath, []byte("forged typed evidence"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(report, "checksums.json")); err != nil {
		t.Fatal(err)
	}
	if err := experiment.Seal(report); err != nil {
		t.Fatal(err)
	}
	if publish.VerifyReport(report) == nil {
		t.Fatal("resealed forged engine-event export accepted")
	}
	if err := os.WriteFile(parquetPath, original, 0644); err != nil {
		t.Fatal(err)
	}
	for _, trial := range b.Trials {
		if trial.Status != "ok" {
			t.Fatal(trial.Status, trial.Reason)
		}
		if trial.Block < 0 {
			if trial.EngineTrace != nil || trial.Samples[0].TierWindow != nil {
				t.Fatal("admission instrumented")
			}
			continue
		}
		if trial.EngineTrace == nil || trial.EngineTrace.Status != "collected" || len(trial.Samples) != 5 {
			t.Fatal("missing real trace", trial)
		}
		events, err := trial.EngineTrace.Events()
		if err != nil || len(events) == 0 {
			t.Fatal(events, err)
		}
		compile := false
		for _, e := range events {
			compile = compile || e.Name == "wasm.SyncCompile"
		}
		if !compile {
			t.Fatal("missing native compile event")
		}
		for name, change := range map[string]func(*experiment.Trial){
			"missing": func(x *experiment.Trial) { x.EngineTrace = nil },
			"version": func(x *experiment.Trial) {
				copy := *x.EngineTrace
				x.EngineTrace = &copy
				x.EngineTrace.CollectorVersion = "forged"
			},
			"bridge": func(x *experiment.Trial) {
				copy := *x.EngineTrace
				x.EngineTrace = &copy
				x.EngineTrace.TrajectoryEpochNS = ""
			},
			"outside_profile": func(x *experiment.Trial) { x.Profile = "timing" },
			"hash": func(x *experiment.Trial) {
				copy := *x.EngineTrace
				x.EngineTrace = &copy
				x.EngineTrace.SHA256 = "forged"
			},
		} {
			t.Run(name, func(t *testing.T) {
				changed := b
				changed.Trials = append([]experiment.Trial(nil), b.Trials...)
				for i := range changed.Trials {
					if changed.Trials[i].ID == trial.ID {
						change(&changed.Trials[i])
					}
				}
				if experiment.ValidateEngineTraceEvidence(changed) == nil {
					t.Fatal("forged trace accepted")
				}
			})
		}
	}
}
