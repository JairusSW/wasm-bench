package sourcebuild

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/experiment"
)

const SourceRunnerArchiveVersion = "source-build-runner-archive-v1"
const sourceRunnerPath = "tools/runner/wasmbench"

func archiveSourceRunner(out, want string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	path := filepath.Join(out, sourceRunnerPath)
	if err := experiment.CopyExclusive(exe, path); err != nil {
		return err
	}
	if err := os.Chmod(path, 0444); err != nil {
		return err
	}
	got, err := experiment.DigestFile(path)
	if err != nil || got != want {
		return fmt.Errorf("source runner changed during archival")
	}
	return nil
}

func verifySourceRunner(root string, result Result) error {
	if result.RunnerArchiveVersion == "" {
		if _, err := os.Stat(filepath.Join(root, sourceRunnerPath)); err == nil {
			return fmt.Errorf("unrecorded source runner archive")
		} else if !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if result.RunnerArchiveVersion != SourceRunnerArchiveVersion || result.ToolTimeoutNS <= 0 || result.ToolTimeoutNS > time.Hour {
		return fmt.Errorf("unsupported source runner archive or tool timeout")
	}
	info, err := os.Stat(filepath.Join(root, sourceRunnerPath))
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0111 != 0 {
		return fmt.Errorf("source runner archive must remain a nonexecutable regular file")
	}
	got, err := experiment.DigestFile(filepath.Join(root, sourceRunnerPath))
	if err != nil {
		return err
	}
	if got != result.Lock.RunnerSHA256 {
		return fmt.Errorf("source runner archive differs from locked runner")
	}
	return nil
}

// PreflightRebuild checks a sealed build's exact snapshots, tools, analyzer,
// selected runner and original host without executing any of them or creating
// output. The runner may be an archived executable, still nonexecutable here.
func PreflightRebuild(bundle, runner string) (Result, error) {
	bundle, err := filepath.Abs(bundle)
	if err != nil {
		return Result{}, err
	}
	original, err := Verify(bundle)
	if err != nil {
		return Result{}, err
	}
	if original.ToolTimeoutNS <= 0 {
		return Result{}, fmt.Errorf("source build predates recorded tool timeouts; exact report replay requires original timeout evidence")
	}
	if err := supported(); err != nil {
		return Result{}, err
	}
	if !reflect.DeepEqual(agent.IdentifyHost(), original.Host) {
		return Result{}, fmt.Errorf("source build replay host fingerprint or environment differs")
	}
	if err := original.Lock.verifyUsingRunner(filepath.Join(bundle, "sources"), runner); err != nil {
		return Result{}, err
	}
	return original, nil
}

// PreflightBenchmarkReplay checks all compiler variants and correctness
// adapters before any source benchmark or subsequent runtime measurement runs.
func PreflightBenchmarkReplay(bundle, runner string) (BuildBenchmark, error) {
	bundle, err := filepath.Abs(bundle)
	if err != nil {
		return BuildBenchmark{}, err
	}
	original, err := VerifyBenchmark(bundle)
	if err != nil {
		return BuildBenchmark{}, err
	}
	if original.Status == "admission_failed" {
		return BuildBenchmark{}, fmt.Errorf("replay requires a correctness-admitted source benchmark")
	}
	if err := supported(); err != nil {
		return BuildBenchmark{}, err
	}
	if !reflect.DeepEqual(agent.IdentifyHost(), original.Host) {
		return BuildBenchmark{}, fmt.Errorf("source benchmark replay host fingerprint or environment differs")
	}
	if original.Config.RequireIsolatedCPUPartition {
		p := agent.ProbeCPUPartition(original.Config.Resources.CgroupParent, original.Config.Resources.CPUs)
		if err := p.Err(); err != nil {
			return BuildBenchmark{}, err
		}
	}
	if original.Config.RequireIRQAffinity {
		p := agent.ProbeIRQAffinity(original.Config.Resources.CPUs)
		if err := p.Err(); err != nil {
			return BuildBenchmark{}, err
		}
	}
	for i, l := range original.Config.Variants {
		if err := l.verifyUsingRunner(filepath.Join(bundle, original.Admissions[i].Build, "sources"), runner); err != nil {
			return BuildBenchmark{}, err
		}
		if err := experiment.VerifyInputs(experiment.Lock{Analyzer: l.Analyzer, Runtimes: original.Config.CorrectnessRuntimes}, bundle); err != nil {
			return BuildBenchmark{}, err
		}
	}
	return original, nil
}
