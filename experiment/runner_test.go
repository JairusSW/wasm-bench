package experiment_test

import (
	"context"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/experiment"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRealAdapterLifecycleAndIncorrectOracle(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "adapter")
	build := exec.Command("go", "build", "-o", binary, "../adapters/wazero")
	if b, e := build.CombinedOutput(); e != nil {
		t.Fatalf("build adapter: %v\n%s", e, b)
	}
	workloads, e := corpus.Generate(filepath.Join(root, "corpus"), "core")
	if e != nil {
		t.Fatal(e)
	}
	hash, e := experiment.DigestFile(binary)
	if e != nil {
		t.Fatal(e)
	}
	runtimes := []experiment.Runtime{{ID: "wazero", Command: []string{binary}, Files: map[string]string{binary: hash}}}
	options := experiment.Options{Suite: "core", Profile: "timing", Scenarios: []string{"compile", "instantiate", "first-call", "steady", "cold-process", "teardown"}, Launches: 1, Samples: 2, Operations: 3, Warmup: 2, Timeout: 10 * time.Second}
	lock, e := experiment.NewLock(options, runtimes, workloads)
	if e != nil {
		t.Fatal(e)
	}
	partitionLock := lock
	irqLock := lock
	irqLock.RequireIRQAffinity = true
	irqLock.Options.Resources.CgroupParent = root
	irqLock.Options.Resources.CPUs = "4095"
	irqOut := filepath.Join(root, "irq-refused")
	if _, err := experiment.Run(context.Background(), irqLock, root, irqOut, func(string) {}); err == nil || !strings.Contains(err.Error(), "IRQ") {
		t.Fatal("IRQ refusal missing", err)
	}
	if _, err := os.Stat(irqOut); !os.IsNotExist(err) {
		t.Fatal("output created before IRQ check", err)
	}
	partitionLock.RequireIsolatedCPUPartition = true
	partitionLock.Options.Resources.CgroupParent = root // Ordinary test directory, never a cgroup.
	partitionLock.Options.Resources.CPUs = "0"
	partitionOut := filepath.Join(root, "partition-refused")
	if _, err := experiment.Run(context.Background(), partitionLock, root, partitionOut, func(string) {}); err == nil || !strings.Contains(err.Error(), "CPU partition") {
		t.Fatal("partition refusal missing", err)
	}
	if _, err := os.Stat(partitionOut); !os.IsNotExist(err) {
		t.Fatal("output created before partition check", err)
	}
	out, e := experiment.Run(context.Background(), lock, root, filepath.Join(root, "correct"), func(string) {})
	if e != nil {
		t.Fatal(e)
	}
	b, e := experiment.Load(out)
	if e != nil {
		t.Fatal(e)
	}
	checks, measurements := 0, 0
	for _, trial := range b.Trials {
		if trial.Status != "ok" {
			t.Fatalf("%s: %s", trial.ID, trial.Reason)
		}
		if trial.Block < 0 {
			checks++
			if len(trial.Samples) != 1 {
				t.Fatal("check used timing budget")
			}
		} else {
			measurements++
			if trial.Scenario == "steady" {
				warm := 0
				for _, s := range trial.Samples {
					if s.Warmup {
						warm++
					}
				}
				if warm != 2 || len(trial.Samples) != 4 {
					t.Fatal("lost warmup")
				}
			}
			if trial.Scenario == "cold-process" && trial.Samples[0].SampleType != "individual_process" {
				t.Fatal("cold process mislabeled")
			}
			if trial.Scenario == "cold-process" && (len(trial.AdapterSamples) != 1 || !trial.AdapterSamples[0].Verified || trial.AdapterSamples[0].SampleType == "individual_process") {
				t.Fatal("lost raw adapter evidence")
			}
		}
	}
	if checks != 2 || measurements != 12 {
		t.Fatalf("wrong replication %d / %d", checks, measurements)
	}
	reactors, err := corpus.Generate(filepath.Join(root, "reactors"), "reactors")
	if err != nil {
		t.Fatal(err)
	}
	reactorOptions := options
	reactorOptions.Suite = "reactors"
	reactorOptions.Scenarios = []string{"compile", "instantiate", "app-init", "first-call", "steady", "cold-process", "teardown"}
	reactorLock, err := experiment.NewLock(reactorOptions, runtimes, reactors)
	if err != nil {
		t.Fatal(err)
	}
	reactorOut, err := experiment.Run(context.Background(), reactorLock, root, filepath.Join(root, "reactor-run"), func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	reactorBundle, err := experiment.Load(reactorOut)
	if err != nil {
		t.Fatal(err)
	}
	if len(reactorBundle.Trials) != 16 {
		t.Fatal("missing reactor matrix", len(reactorBundle.Trials))
	}
	for _, trial := range reactorBundle.Trials {
		if trial.Status != "ok" {
			t.Fatal(trial.ID, trial.Reason)
		}
	}
	if lock.Workloads[0].Artifact != workloads[0].Artifact || !filepath.IsAbs(lock.Workloads[0].Artifact) {
		t.Fatal("run mutated the reusable plan")
	}
	changed := lock
	// Phase settings must not reject the unbarriered sacrificial float check.
	floats, err := corpus.Generate(filepath.Join(root, "floats"), "floats")
	if err != nil {
		t.Fatal(err)
	}
	phaseOptions := options
	phaseOptions.Profile, phaseOptions.PhaseBarriers = "memory", true
	phaseOptions.Scenarios = []string{"compile", "instantiate"}
	phaseLock, err := experiment.NewLock(phaseOptions, runtimes, floats[:1])
	if err != nil {
		t.Fatal(err)
	}
	phaseOut, err := experiment.Run(context.Background(), phaseLock, root, filepath.Join(root, "float-phases"), func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	phaseBundle, err := experiment.Load(phaseOut)
	if err != nil {
		t.Fatal(err)
	}
	if len(phaseBundle.Trials) != 3 {
		t.Fatal("missing float phase trials")
	}
	for _, trial := range phaseBundle.Trials {
		if trial.Status != "ok" {
			t.Fatal(trial.Status, trial.Reason)
		}
	}
	changed.RunnerSHA256 = "changed"
	if _, e = experiment.Run(context.Background(), changed, root, filepath.Join(root, "different-runner"), func(string) {}); e == nil {
		t.Fatal("different runner accepted for locked experiment")
	}
	lock.Workloads[0].Oracle.Expected[0]++
	_, e = experiment.Run(context.Background(), lock, root, filepath.Join(root, "wrong"), func(string) {})
	if e != nil {
		t.Fatal(e)
	}
	b, e = experiment.Load(filepath.Join(root, "wrong"))
	if e != nil {
		t.Fatal(e)
	}
	for _, trial := range b.Trials {
		if trial.Workload != workloads[0].ID {
			continue
		}
		if trial.Block < 0 {
			if trial.Status != "incorrect_result" {
				t.Fatalf("bad oracle admitted: %+v", trial)
			}
		} else if trial.Status != "preflight_failed" || len(trial.Samples) != 0 {
			t.Fatal("incorrect workload produced performance data")
		}
	}
}
