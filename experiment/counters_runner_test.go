package experiment_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/analysis"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/experiment"
)

func TestCounterRunSealsUnavailableEvidence(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "adapter")
	if b, err := exec.Command("go", "build", "-o", binary, "../adapters/wazero").CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, b)
	}
	ws, err := corpus.Generate(filepath.Join(root, "corpus"), "core")
	if err != nil {
		t.Fatal(err)
	}
	ws = ws[:1]
	hash, err := experiment.DigestFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	rts := []experiment.Runtime{{ID: "wazero", Command: []string{binary}, Files: map[string]string{binary: hash}}}
	opts := experiment.Options{Suite: "core", Profile: "counters", Scenarios: []string{"compile", "instantiate", "teardown"}, Launches: 1, Samples: 2, Operations: 1, PhaseBarriers: true, Timeout: 10 * time.Second}
	lock, err := experiment.NewLock(opts, rts, ws)
	if err != nil {
		t.Fatal(err)
	}
	out, err := experiment.Run(context.Background(), lock, root, filepath.Join(root, "counter-run"), func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	b, err := experiment.Load(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Trials) != 4 {
		t.Fatal(len(b.Trials))
	}
	for _, trial := range b.Trials {
		if trial.Block < 0 {
			if trial.Status != "ok" || len(trial.CounterPhases) != 0 {
				t.Fatal(trial)
			}
			continue
		}
		if trial.Scenario == "teardown" {
			if trial.Status != "unsupported" || len(trial.CounterPhases) != 0 {
				t.Fatal(trial)
			}
			continue
		}
		if trial.Status != "ok" || len(trial.CounterPhases) != 2 || len(trial.Samples) != 2 || len(trial.PhaseEvents) != 0 || len(trial.Observations) != 0 {
			t.Fatal(trial)
		}
		for i, p := range trial.CounterPhases {
			if p.Sample != i || p.Status != "unavailable" || p.Reason == "" || p.CollectorVersion == "" || len(p.Readings) != 0 {
				t.Fatal(p)
			}
		}
		for _, s := range trial.Samples {
			if !s.Verified || len(s.Observations) != 0 {
				t.Fatal(s)
			}
		}
	}
	for _, s := range analysis.Summarize(b) {
		if s.Median != nil || s.Mean != nil || s.Launches != 0 {
			t.Fatal("counter timers promoted to latency", s)
		}
	}
}
