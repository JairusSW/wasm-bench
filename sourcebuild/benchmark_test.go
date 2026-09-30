package sourcebuild

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

func TestBuildSchedule(t *testing.T) {
	c := BenchmarkConfig{Variants: make([]Lock, 3), Blocks: 5, WarmupBlocks: 2, Seed: 42}
	a, b := buildSchedule(c), buildSchedule(c)
	if !reflect.DeepEqual(a, b) || len(a) != 21 {
		t.Fatal("non-deterministic schedule")
	}
	seen := map[[2]int]bool{}
	for _, trial := range a {
		k := [2]int{trial.Block, trial.Variant}
		if seen[k] || trial.Warmup != (trial.Block < 0) {
			t.Fatal(trial)
		}
		seen[k] = true
	}
	c.Seed++
	if reflect.DeepEqual(a, buildSchedule(c)) {
		t.Fatal("seed has no effect")
	}
}

func TestLLVMSourceBenchmark(t *testing.T) {
	if os.Getenv("WASMBENCH_SOURCE_LLVM_TEST") != "1" {
		t.Skip("set WASMBENCH_SOURCE_LLVM_TEST=1 with LLVM and built analyzer/wazero")
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	a, err := experiment.PinAnalyzer(filepath.Join(root, "adapters/wasmtime/target/release/wasm-analyze"), "wasm1")
	if err != nil {
		t.Fatal(err)
	}
	c := BenchmarkConfig{Schema: 1, Blocks: 3, WarmupBlocks: 1, Seed: 42, Timeout: time.Minute}
	c.HostPolicy = &agent.HostPolicy{Version: agent.HostPolicyVersion, Expected: agent.IdentifyHost()}
	sourceBytes, err := os.ReadFile(filepath.Join(root, "recipes/source/xorshift.c"))
	if err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(t.TempDir(), "xorshift.c")
	if err := os.WriteFile(sourcePath, sourceBytes, 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"xorshift-llvm-o0.json", "xorshift-llvm.json"} {
		var recipe Recipe
		if err := experiment.ReadJSON(filepath.Join(root, "recipes/source", name), &recipe); err != nil {
			t.Fatal(err)
		}
		recipe.Inputs["xorshift.c"] = File{Path: sourcePath}
		l, err := Pin(recipe, filepath.Join(root, "recipes/source"), a)
		if err != nil {
			t.Fatal(err)
		}
		c.Variants = append(c.Variants, l)
	}
	c.CorrectnessRuntimes, err = experiment.ResolveRuntimes(root, []string{"wazero"})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	t.Run("IRQ refuses before output", func(t *testing.T) {
		irqConfig := c
		irqConfig.RequireIRQAffinity = true
		irqConfig.Resources = &agent.ResourcePolicy{CgroupParent: dir, CPUs: "4095"}
		out := filepath.Join(dir, "irq-refused")
		if _, err := Benchmark(context.Background(), irqConfig, out); err == nil || !strings.Contains(err.Error(), "IRQ") {
			t.Fatal("IRQ refusal missing", err)
		}
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			t.Fatal("output created before IRQ check", err)
		}
	})
	t.Run("partition refuses before output", func(t *testing.T) {
		partitionConfig := c
		partitionConfig.RequireIsolatedCPUPartition = true
		partitionConfig.Resources = &agent.ResourcePolicy{CgroupParent: dir, CPUs: "0"}
		out := filepath.Join(dir, "partition-refused")
		if _, err := Benchmark(context.Background(), partitionConfig, out); err == nil || !strings.Contains(err.Error(), "CPU partition") {
			t.Fatal("partition refusal missing", err)
		}
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			t.Fatal("output created before partition check", err)
		}
	})
	t.Run("baseline refuses before output", func(t *testing.T) {
		t.Setenv("GOGC", "51-host-baseline-test")
		refused := filepath.Join(dir, "refused")
		if _, err := Benchmark(context.Background(), c, refused); err == nil || !strings.Contains(err.Error(), "host baseline mismatch") {
			t.Fatal("baseline mismatch not rejected", err)
		}
		if _, err := os.Stat(refused); !os.IsNotExist(err) {
			t.Fatal("output created before baseline check", err)
		}
	})
	out := filepath.Join(dir, "benchmark")
	r, err := Benchmark(context.Background(), c, out)
	if err != nil {
		t.Fatal(err, r.Admissions)
	}
	if r.Status != "complete" || len(r.Trials) != 8 || len(r.Admissions) != 2 {
		t.Fatal(r)
	}
	if !r.HostBaselineAllowsMeasurements() || r.HostEnd == nil {
		t.Fatal("missing matched baseline evidence")
	}
	if _, err := VerifyBenchmark(out); err != nil {
		t.Fatal(err)
	}
	// Runtime discovery must not mutate the saved configuration or the caller.
	if c.CorrectnessRuntimes[0].Description != nil {
		t.Fatal("mutated caller runtime identity")
	}
	changedHost := r
	changedHost.Host.Hostname = "different-host"
	wrongHostOut := filepath.Join(dir, "wrong-host")
	if _, err := benchmark(context.Background(), c, wrongHostOut, &benchmarkReplay{root: out, original: changedHost}); err == nil || !strings.Contains(err.Error(), "host fingerprint") {
		t.Fatal("host mismatch not rejected", err)
	}
	if _, err := os.Stat(wrongHostOut); !os.IsNotExist(err) {
		t.Fatal("created replay output before host compatibility check")
	}
	// Remove only the input fixture created by this test, never the repo source.
	if err := os.Remove(sourcePath); err != nil {
		t.Fatal(err)
	}
	replayOut := filepath.Join(dir, "replayed")
	replayed, err := ReplayBenchmark(context.Background(), out, replayOut)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Reproduction == nil || replayed.ConfigSHA256 != r.ConfigSHA256 || len(replayed.Trials) != len(r.Trials) {
		t.Fatal("lost replay identity")
	}
	digest, err := experiment.DigestFile(filepath.Join(out, "checksums.json"))
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Reproduction.BundleSHA256 != digest {
		t.Fatal("wrong parent evidence hash")
	}
	for i, trial := range replayed.Trials {
		old := r.Trials[i]
		if trial.Block != old.Block || trial.Variant != old.Variant || trial.Warmup != old.Warmup || trial.Result.ArtifactSHA256 != old.Result.ArtifactSHA256 {
			t.Fatal("replay changed schedule or output")
		}
	}
	if _, err := VerifyBenchmark(replayOut); err != nil {
		t.Fatal(err)
	}
	if _, err := ReplayBenchmark(context.Background(), out, out); err == nil {
		t.Fatal("overwrote original benchmark")
	}
	if _, err := VerifyBenchmark(out); err != nil {
		t.Fatal("original benchmark damaged", err)
	}
	// Restore the test-owned input for the separate wrong-oracle test below.
	if err := os.WriteFile(sourcePath, sourceBytes, 0600); err != nil {
		t.Fatal(err)
	}
	cpuConfig := c
	cpuConfig.Profile = "cpu"
	cpuOut := filepath.Join(dir, "cpu")
	cpu, err := Benchmark(context.Background(), cpuConfig, cpuOut)
	if err != nil {
		t.Fatal(err)
	}
	if cpu.MeasurementVersion != BuildCPUVersion {
		t.Fatal("CPU pass mislabeled")
	}
	for _, trial := range cpu.Trials {
		if trial.CPUStatus != "available" || trial.ToolCPUNS == nil || *trial.ToolCPUNS <= 0 {
			t.Fatal("missing LLVM CPU", trial)
		}
	}
	if _, err := VerifyBenchmark(cpuOut); err != nil {
		t.Fatal(err)
	}
	cpuReplayOut := filepath.Join(dir, "cpu-replay")
	cpuReplay, err := ReplayBenchmark(context.Background(), cpuOut, cpuReplayOut)
	if err != nil {
		t.Fatal(err)
	}
	if cpuReplay.Config.Profile != "cpu" {
		t.Fatal("replay lost diagnostic profile")
	}
	if _, err := VerifyBenchmark(cpuReplayOut); err != nil {
		t.Fatal(err)
	}
	// A resealed end mismatch remains verifiable diagnostic evidence, while
	// removing a required endpoint must be rejected even with valid checksums.
	end := *cpu.HostEnd
	end.CPUs++
	cpu.HostEnd = &end
	check := agent.CheckHostPolicy(cpu.Config.HostPolicy, end, "after_trials_before_seal")
	cpu.HostEndCheck = &check
	cpu.Publication = "prohibited_host_baseline_mismatch"
	for _, missing := range []bool{false, true} {
		if missing {
			cpu.HostEnd = nil
		}
		if err := os.Remove(filepath.Join(cpuOut, "benchmark.json")); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.Join(cpuOut, "checksums.json")); err != nil {
			t.Fatal(err)
		}
		if err := experiment.WriteJSON(filepath.Join(cpuOut, "benchmark.json"), cpu); err != nil {
			t.Fatal(err)
		}
		if err := experiment.Seal(cpuOut); err != nil {
			t.Fatal(err)
		}
		verified, err := VerifyBenchmark(cpuOut)
		if !missing && (err != nil || verified.HostBaselineAllowsMeasurements()) {
			t.Fatal("mismatch evidence", err)
		}
		if missing && err == nil {
			t.Fatal("missing baseline endpoint accepted")
		}
	}
	replayed.Reproduction.ConfigSHA256 = "wrong-parent-config"
	if err := os.Remove(filepath.Join(replayOut, "benchmark.json")); err != nil {
		t.Fatal(err)
	}
	if err := experiment.WriteJSON(filepath.Join(replayOut, "benchmark.json"), replayed); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(replayOut, "checksums.json")); err != nil {
		t.Fatal(err)
	}
	if err := experiment.Seal(replayOut); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyBenchmark(replayOut); err == nil {
		t.Fatal("false reproduction identity accepted")
	}
	*r.Trials[0].ToolWallNS++
	if err := os.Remove(filepath.Join(out, "benchmark.json")); err != nil {
		t.Fatal(err)
	}
	if err := experiment.WriteJSON(filepath.Join(out, "benchmark.json"), r); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(out, "checksums.json")); err != nil {
		t.Fatal(err)
	}
	if err := experiment.Seal(out); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyBenchmark(out); err == nil {
		t.Fatal("resealed false timing accepted")
	}
	// A valid module with the wrong oracle must stop before measured builds.
	for i := range c.Variants {
		c.Variants[i].Recipe.Workload.Oracle.Expected = protocol.Values{0}
	}
	bad := filepath.Join(dir, "wrong-oracle")
	r, err = Benchmark(context.Background(), c, bad)
	if err == nil || r.Status != "admission_failed" || len(r.Trials) != 0 {
		t.Fatal(r, err)
	}
	if _, err := VerifyBenchmark(bad); err != nil {
		t.Fatal("failed admission evidence not readable", err)
	}
	if _, err := ReplayBenchmark(context.Background(), bad, filepath.Join(dir, "unadmitted-replay")); err == nil {
		t.Fatal("replayed an unadmitted benchmark")
	}
}
