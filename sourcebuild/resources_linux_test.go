//go:build linux

package sourcebuild

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/experiment"
)

// This exercises build receipts and replay with fixture tools, not compiler
// performance or real analyzer/runtime correctness admission.
func TestResourceBuildReceiptAndReplay(t *testing.T) {
	parent := os.Getenv("WASMBENCH_TEST_CGROUP_PARENT")
	if parent == "" {
		t.Skip("delegated cgroup required")
	}
	r, dir, a := fixture(t)
	l, err := Pin(r, dir, a)
	if err != nil {
		t.Fatal(err)
	}
	cpus, err := os.ReadFile(filepath.Join(parent, "cpuset.cpus.effective"))
	if err != nil {
		t.Fatal(err)
	}
	p := &agent.ResourcePolicy{CgroupParent: parent, MemoryMaxBytes: 128 << 20, DisableSwap: true, CPUQuotaUS: 100000, PidsMax: 32, CPUs: strings.TrimSpace(string(cpus))}
	out := filepath.Join(dir, "memory-build")
	built, err := buildWithResources(context.Background(), l, out, 10*time.Second, "", "", "memory", p)
	if err != nil {
		t.Fatal(err)
	}
	if maximumStepPeak(built.Steps) == nil {
		t.Fatal("missing build peak")
	}
	if _, err = Verify(out); err != nil {
		t.Fatal(err)
	}
	replayed, err := Rebuild(context.Background(), out, filepath.Join(dir, "replayed"), 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.CollectionProfile != "memory" || replayed.ResourcePolicy == nil || replayed.ResourcePolicy.CPUs != p.CPUs || maximumStepPeak(replayed.Steps) == nil {
		t.Fatal("lost resource profile in replay", replayed)
	}
	if _, err = Verify(filepath.Join(dir, "replayed")); err != nil {
		t.Fatal(err)
	}
}

func TestOOMAdmissionEvidence(t *testing.T) {
	parent := os.Getenv("WASMBENCH_TEST_CGROUP_PARENT")
	if parent == "" {
		t.Skip("delegated cgroup required")
	}
	r, dir, a := fixture(t)
	r.Steps[0].Args = []string{"-c", "/bin/dd if=/dev/zero of=/dev/null bs=64M count=1"}
	l, err := Pin(r, dir, a)
	if err != nil {
		t.Fatal(err)
	}
	c := BenchmarkConfig{Schema: 1, Profile: "memory", Variants: []Lock{l}, Blocks: 3, Timeout: 10 * time.Second,
		Resources:           &agent.ResourcePolicy{CgroupParent: parent, MemoryMaxBytes: 16 << 20, DisableSwap: true},
		CorrectnessRuntimes: []experiment.Runtime{{ID: "never-invoked"}}}
	out := filepath.Join(dir, "oom")
	result, err := Benchmark(context.Background(), c, out)
	if err == nil || result.Admissions[0].Status != "oom" || result.Admissions[0].FailedBuild == nil || len(result.Trials) != 0 {
		t.Fatal(result, err)
	}
	if _, err := VerifyBenchmark(out); err != nil {
		t.Fatal(err)
	}
	step := result.Admissions[0].FailedBuild.Steps[0]
	if !step.Resources.OOM || len(step.Resources.Observations) != 2 {
		t.Fatal("missing OOM observations", step)
	}
}
