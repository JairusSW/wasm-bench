//go:build linux || darwin

package sourcebuild

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/experiment"
)

func TestFailedAdmissionRetainsPartialBuild(t *testing.T) {
	r, dir, a := fixture(t)
	r.Steps[0].Args = []string{"-c", "echo compiler-failed >&2; exit 7"}
	l, err := Pin(r, dir, a)
	if err != nil {
		t.Fatal(err)
	}
	c := BenchmarkConfig{Schema: 1, Profile: "cpu", Variants: []Lock{l}, Blocks: 3, Timeout: time.Second, CorrectnessRuntimes: []experiment.Runtime{{ID: "never-invoked"}}}
	out := filepath.Join(dir, "failed")
	result, err := Benchmark(context.Background(), c, out)
	if err == nil || result.Status != "admission_failed" || len(result.Trials) != 0 || result.Admissions[0].FailedBuild == nil {
		t.Fatal(result, err)
	}
	if _, err := VerifyBenchmark(out); err != nil {
		t.Fatal(err)
	}
	s := result.Admissions[0].FailedBuild.Steps[0]
	if s.CPU == nil || s.Log != "step-000.log" {
		t.Fatal("lost failed step", s)
	}
	// Resealing does not excuse inconsistent partial observation metadata.
	result.Admissions[0].FailedBuild.Steps[0].CPU.Unit = "bytes"
	if err := os.Remove(filepath.Join(out, "benchmark.json")); err != nil {
		t.Fatal(err)
	}
	if err := experiment.WriteJSON(filepath.Join(out, "benchmark.json"), result); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(out, "checksums.json")); err != nil {
		t.Fatal(err)
	}
	if err := experiment.Seal(out); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyBenchmark(out); err == nil {
		t.Fatal("resealed invalid failure receipt accepted")
	}
}

func TestContextFailureAdmission(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		name := "timeout"
		if canceled {
			name = "canceled"
		}
		t.Run(name, func(t *testing.T) {
			r, dir, a := fixture(t)
			r.Steps[0].Args = []string{"-c", "/bin/sleep 30 & wait"}
			l, err := Pin(r, dir, a)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if canceled {
				cancel()
			}
			c := BenchmarkConfig{Schema: 1, Profile: "cpu", Variants: []Lock{l}, Blocks: 3, Timeout: 100 * time.Millisecond, CorrectnessRuntimes: []experiment.Runtime{{ID: "never-invoked"}}}
			out := filepath.Join(dir, "failed")
			result, err := Benchmark(ctx, c, out)
			if err == nil || result.Status != "admission_failed" || len(result.Trials) != 0 || result.Admissions[0].Status != name {
				t.Fatal(result, err)
			}
			if _, err := VerifyBenchmark(out); err != nil {
				t.Fatal(err)
			}
			// A timeout must be supported by an observed deadline condition,
			// not merely a textual reason or the elapsed wall time.
			admission := result.Admissions[0]
			admission.FailedBuild.Steps[0].ContextError = ""
			if verifyFailedBuild(out, admission.Build, *admission.FailedBuild, l, c, name) == nil {
				t.Fatal("missing context evidence accepted")
			}
		})
	}
}
