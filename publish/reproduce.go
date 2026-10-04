package publish

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/sourcebuild"
)

type reportReplayPass struct {
	Name, Source, Runner string
	Lock                 experiment.Lock
	Kind, RunnerArchive  string
	Timeout              time.Duration
	BuildInputs          []reportReplayBuildInput
}

// ReproduceReport reruns each sealed input with its exact runner, then analyzes
// the new evidence with this build. It never relocates or relaxes a locked
// runtime identity, runs passes concurrently, or overwrites an existing output.
func ReproduceReport(ctx context.Context, source, out string, progress func(string)) error {
	return reproduceReport(ctx, source, out, progress, preflightReportPass, replayReportPass)
}

func reportReplayPasses(source string) ([]reportReplayPass, error) {
	if err := VerifyReport(source); err != nil {
		return nil, fmt.Errorf("source report: %w", err)
	}
	var d Dataset
	if err := experiment.ReadJSON(filepath.Join(source, "data.json"), &d); err != nil {
		return nil, err
	}
	passes := []reportReplayPass{{Name: "primary", Source: filepath.Join(source, "raw")}}
	if d.MemorySource != nil && d.MemorySource.Profile == "memory" {
		passes = append(passes, reportReplayPass{Name: "memory", Source: filepath.Join(source, "raw-memory")})
	}
	if d.CodeSource != nil {
		passes = append(passes, reportReplayPass{Name: "code", Source: filepath.Join(source, "code", "raw")})
	}
	for i := range passes {
		b, err := experiment.Load(passes[i].Source)
		if err != nil {
			return nil, err
		}
		passes[i].Lock = b.Manifest.Lock
	}
	return passes, nil
}

func preflightReportPass(pass *reportReplayPass) error {
	if pass.Kind != "" {
		return preflightSourceReportPass(pass)
	}
	if err := experiment.ValidateLock(pass.Lock); err != nil {
		return err
	}
	if err := experiment.VerifyInputs(pass.Lock, pass.Source); err != nil {
		return err
	}
	current, err := os.Executable()
	if err != nil {
		return err
	}
	pass.Runner, err = exactReportRunner(current, pass.Source, pass.Lock.RunnerSHA256)
	pass.RunnerArchive = filepath.Join(pass.Source, "tools/runner/wasmbench")
	return err
}

func exactReportRunner(current, source, digest string) (string, error) {
	for _, path := range []string{current, filepath.Join(source, "tools", "runner", "wasmbench")} {
		got, err := experiment.DigestFile(path)
		if err == nil && got == digest {
			return filepath.Abs(path)
		}
	}
	return "", fmt.Errorf("exact locked runner unavailable; restore the recorded runner build (SHA-256 %s); no new measurements started", digest)
}

func replayReportPass(ctx context.Context, pass reportReplayPass, out string, progress func(string)) error {
	runner, err := executableReportRunner(pass, filepath.Join(filepath.Dir(out), "runners", pass.Name, "wasmbench"))
	if err != nil {
		return err
	}
	args := []string{"reproduce", pass.Source, "--out", out}
	switch pass.Kind {
	case "source-build":
		args = []string{"source-rebuild", "--bundle", pass.Source, "--out", out, "--timeout", pass.Timeout.String()}
	case "source-benchmark":
		args = []string{"source-bench-replay", "--bundle", pass.Source, "--out", out}
	}
	cmd := exec.CommandContext(ctx, runner, args...)
	// Keep logs streamed and bounded; the original runner writes per-trial
	// evidence into the new bundle, not into the controller's memory.
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if pass.Kind == "source-build" {
		// source-rebuild prints its whole Result as JSON. That exact result and
		// bounded compiler logs are already preserved in the new build bundle;
		// keep multi-workload replay progress readable instead of dumping it.
		cmd.Stdout = io.Discard
	}
	if err := cmd.Run(); err != nil {
		return err
	}
	switch pass.Kind {
	case "source-build":
		_, err = sourcebuild.Verify(out)
	case "source-benchmark":
		_, err = sourcebuild.VerifyBenchmark(out)
	default:
		_, err = experiment.Load(out)
	}
	return err
}

// Archives are intentionally non-executable. Stage only the exact runner in
// the new output; never chmod the immutable source or relocate adapter paths.
func executableReportRunner(pass reportReplayPass, destination string) (string, error) {
	archivePath := pass.RunnerArchive
	if archivePath == "" {
		archivePath = filepath.Join(pass.Source, "tools", "runner", "wasmbench")
	}
	archive, err := filepath.Abs(archivePath)
	if err != nil {
		return "", err
	}
	if pass.Runner != archive {
		return pass.Runner, nil
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return "", err
	}
	source, err := os.Open(pass.Runner)
	if err != nil {
		return "", err
	}
	defer source.Close()
	dest, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0755)
	if err != nil {
		return "", err
	}
	_, copyErr := io.Copy(dest, source)
	closeErr := dest.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	digest, err := experiment.DigestFile(destination)
	if err != nil {
		return "", err
	}
	if digest != pass.Lock.RunnerSHA256 {
		return "", fmt.Errorf("staged runner differs from locked SHA-256")
	}
	return destination, nil
}

func reproduceReport(ctx context.Context, source, out string, progress func(string), preflight func(*reportReplayPass) error, replay func(context.Context, reportReplayPass, string, func(string)) error) error {
	if err := reportOutsideInputs([]string{source}, out); err != nil {
		return err
	}
	plan, err := newReportReplayPlan(source)
	if err != nil {
		return err
	}
	passes := plan.Passes
	// Check every pass before creating output or starting any measurement.
	for i := range passes {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := preflight(&passes[i]); err != nil {
			return fmt.Errorf("%s pass preflight: %w", passes[i].Name, err)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if plan.Preflight != nil {
		if err := plan.Preflight(ctx); err != nil {
			return fmt.Errorf("report diagnostic preflight: %w", err)
		}
	}
	out, err = filepath.Abs(out)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		return err
	}
	if err := os.Mkdir(out, 0755); err != nil {
		return err
	}
	paths := map[string]string{}
	for _, pass := range passes {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("reproduction stopped; partial evidence preserved at %s: %w", out, err)
		}
		path := filepath.Join(out, pass.Name)
		if len(pass.BuildInputs) != 0 {
			staged, err := stageRebuiltRuntimeInputs(pass, paths, filepath.Join(out, "inputs", pass.Name))
			if err != nil {
				return fmt.Errorf("%s rebuilt input binding failed; partial evidence preserved at %s: %w", pass.Name, out, err)
			}
			pass.Source = staged
		}
		if progress != nil {
			progress("Reproducing " + pass.Name + " pass")
		}
		if err := replay(ctx, pass, path, progress); err != nil {
			return fmt.Errorf("%s pass failed; partial evidence preserved at %s: %w", pass.Name, out, err)
		}
		paths[pass.Name] = path
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("reproduction stopped; measurements preserved at %s: %w", out, err)
	}
	if err := plan.Finish(ctx, paths, filepath.Join(out, "report")); err != nil {
		return fmt.Errorf("measurements preserved at %s; report generation: %w", out, err)
	}
	return nil
}
