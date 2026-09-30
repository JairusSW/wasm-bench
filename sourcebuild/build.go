// Package sourcebuild produces separately sealed source-to-Wasm build evidence.
// It never treats a successful compiler exit as a workload correctness oracle.
package sourcebuild

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

type File struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}
type Step struct {
	Tool string   `json:"tool"`
	Args []string `json:"args"`
}
type Recipe struct {
	Schema         int               `json:"schema"`
	ID             string            `json:"id"`
	SourceRevision string            `json:"source_revision"`
	License        string            `json:"license"`
	Inputs         map[string]File   `json:"inputs"`
	Tools          map[string]File   `json:"tools"`
	Environment    map[string]string `json:"environment"`
	Steps          []Step            `json:"steps"`
	Output         string            `json:"output"`
	Workload       protocol.Workload `json:"workload"`
}
type Lock struct {
	Schema       int                      `json:"schema"`
	Recipe       Recipe                   `json:"recipe"`
	Analyzer     *experiment.AnalyzerLock `json:"analyzer"`
	RunnerSHA256 string                   `json:"runner_sha256"`
}
type StepResult struct {
	ContextError string               `json:"context_error,omitempty"`
	Tool         string               `json:"tool"`
	ElapsedNS    int64                `json:"process_wall_ns"`
	Log          string               `json:"log"`
	CPU          *CPUAccounting       `json:"cpu,omitempty"`
	Resources    *agent.ToolExecution `json:"resources,omitempty"`
}
type Result struct {
	RunnerArchiveVersion string                `json:"runner_archive_version,omitempty"`
	ToolTimeoutNS        time.Duration         `json:"tool_timeout_ns,omitempty"`
	CollectionProfile    string                `json:"collection_profile,omitempty"`
	ResourcePolicy       *agent.ResourcePolicy `json:"resource_policy,omitempty"`
	Schema               int                   `json:"schema"`
	Kind                 string                `json:"kind"`
	Status               string                `json:"status"`
	Interpretation       string                `json:"interpretation"`
	Lock                 Lock                  `json:"lock"`
	LockSHA256           string                `json:"lock_sha256"`
	Host                 agent.Host            `json:"build_host"`
	Steps                []StepResult          `json:"steps"`
	ArtifactSHA256       string                `json:"artifact_sha256"`
	ReproducesSHA256     string                `json:"reproduces_artifact_sha256,omitempty"`
}

func local(path string) bool {
	return filepath.IsLocal(path) && filepath.Clean(path) == path && path != "." && !strings.Contains(path, "\\")
}
func (r Recipe) validate() error {
	if r.Schema != 1 || r.ID == "" || r.SourceRevision == "" || r.License == "" || len(r.Inputs) == 0 || len(r.Tools) == 0 || len(r.Steps) == 0 || !local(r.Output) {
		return fmt.Errorf("incomplete source build recipe")
	}
	for name := range r.Inputs {
		if !local(name) || name == r.Output || name == ".tmp" || strings.HasPrefix(name, ".tmp/") {
			return fmt.Errorf("invalid input path %q", name)
		}
	}
	for _, step := range r.Steps {
		if _, ok := r.Tools[step.Tool]; !ok {
			return fmt.Errorf("unknown build tool %q", step.Tool)
		}
		for _, arg := range step.Args {
			if strings.ContainsRune(arg, 0) {
				return fmt.Errorf("invalid tool argument")
			}
		}
	}
	for k, v := range r.Environment {
		if k == "" || strings.ContainsAny(k, "=\x00") || strings.ContainsRune(v, 0) || k == "HOME" || k == "TMPDIR" || k == "TMP" || k == "TEMP" {
			return fmt.Errorf("invalid or reserved build environment key %q", k)
		}
	}
	w := r.Workload
	if w.Schema != 1 || w.WorkUnit == "" || w.Oracle.ExpectedTrap != "" {
		return fmt.Errorf("source recipe requires a versioned non-trapping workload with a work unit")
	}
	if w.ID == "" || w.Units == 0 || w.ABI != "core" || w.Export == "" || w.Oracle.Kind != "exact_u64" || len(w.Oracle.Expected) == 0 || (w.Reset != "stateless" && w.Reset != "fresh_instance_per_sample") || w.Command != nil || w.Vectors != nil || w.Artifact != "" || w.SHA256 != "" || len(w.Provenance) != 0 {
		return fmt.Errorf("source recipe requires an unbound core scalar correctness contract")
	}
	return nil
}
func pin(path, base string) (File, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return File{}, err
	}
	// Preserve argv[0] for multi-call tools (for example wasm-ld -> lld).
	// Stat and DigestFile follow the link; verification still pins actual bytes.
	info, err := os.Stat(path)
	if err != nil {
		return File{}, err
	}
	if !info.Mode().IsRegular() {
		return File{}, fmt.Errorf("not a regular pinned file")
	}
	hash, err := experiment.DigestFile(path)
	return File{Path: path, SHA256: hash}, err
}
func Pin(r Recipe, base string, analyzer *experiment.AnalyzerLock) (Lock, error) {
	if err := r.validate(); err != nil {
		return Lock{}, err
	}
	if analyzer == nil {
		return Lock{}, fmt.Errorf("analyzer required")
	}
	if err := analyzer.Verify(); err != nil {
		return Lock{}, err
	}
	// Pinning must not rewrite the caller's unresolved recipe maps.
	r.Inputs = maps.Clone(r.Inputs)
	r.Tools = maps.Clone(r.Tools)
	for name, file := range r.Inputs {
		p, err := pin(file.Path, base)
		if err != nil {
			return Lock{}, err
		}
		if file.SHA256 != "" && file.SHA256 != p.SHA256 {
			return Lock{}, fmt.Errorf("input pin mismatch: %s", name)
		}
		r.Inputs[name] = p
	}
	for name, file := range r.Tools {
		p, err := pin(file.Path, base)
		if err != nil {
			return Lock{}, err
		}
		if file.SHA256 != "" && file.SHA256 != p.SHA256 {
			return Lock{}, fmt.Errorf("tool pin mismatch: %s", name)
		}
		r.Tools[name] = p
	}
	exe, err := os.Executable()
	if err != nil {
		return Lock{}, err
	}
	hash, err := experiment.DigestFile(exe)
	if err != nil {
		return Lock{}, err
	}
	return Lock{Schema: 1, Recipe: r, Analyzer: analyzer, RunnerSHA256: hash}, nil
}
func verifyFile(file File) error {
	if !filepath.IsAbs(file.Path) || len(file.SHA256) != 64 {
		return fmt.Errorf("invalid source build file pin")
	}
	got, err := experiment.DigestFile(file.Path)
	if err != nil {
		return err
	}
	if got != file.SHA256 {
		return fmt.Errorf("source build pin changed: %s", file.Path)
	}
	return nil
}
func (l Lock) inputFiles(snapshotRoot string) map[string]File {
	files := maps.Clone(l.Recipe.Inputs)
	if snapshotRoot != "" {
		for name, file := range files {
			file.Path = filepath.Join(snapshotRoot, name)
			files[name] = file
		}
	}
	return files
}

func (l Lock) verify(snapshotRoot string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return l.verifyUsingRunner(snapshotRoot, exe)
}

func (l Lock) verifyUsingRunner(snapshotRoot, runner string) error {
	if l.Schema != 1 || l.Analyzer == nil {
		return fmt.Errorf("invalid source build lock")
	}
	if err := l.Recipe.validate(); err != nil {
		return err
	}
	if err := verifyFile(File{runner, l.RunnerSHA256}); err != nil {
		return err
	}
	if err := l.Analyzer.Verify(); err != nil {
		return err
	}
	for _, files := range []map[string]File{l.inputFiles(snapshotRoot), l.Recipe.Tools} {
		for _, file := range files {
			if err := verifyFile(file); err != nil {
				return err
			}
		}
	}
	return nil
}

type logBuffer struct {
	data     bytes.Buffer
	overflow bool
}

func (b *logBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := (1 << 20) - b.data.Len()
	if len(p) > remaining {
		b.overflow = true
		p = p[:remaining]
	}
	_, err := b.data.Write(p)
	return n, err
}

// Build runs explicitly trusted local tools. It does not inherit the caller's
// environment. Tool libraries, system headers and subprocess dependencies are
// not automatically discovered or claimed hermetic. Builds are not timed trials.
func Build(ctx context.Context, l Lock, out string, timeout time.Duration) (Result, error) {
	return build(ctx, l, out, timeout, "", "")
}

// Rebuild uses the sealed input snapshots, never the original source paths.
// Exact runner, tool and analyzer pins are still required. A different output
// is a failed reproduction and is not sealed as successful build evidence.
func Rebuild(ctx context.Context, bundle, out string, timeout time.Duration) (Result, error) {
	original, err := Verify(bundle)
	if err != nil {
		return Result{}, err
	}
	bundle, err = filepath.Abs(bundle)
	if err != nil {
		return Result{}, err
	}
	return buildWithResources(ctx, original.Lock, out, timeout, filepath.Join(bundle, "sources"), original.ArtifactSHA256, original.CollectionProfile, original.ResourcePolicy)
}

// Verify checks saved source-build evidence offline. This proves consistency,
// not authenticity, hermeticity, or successful workload correctness execution.
func Verify(bundle string) (Result, error) {
	bundle, err := filepath.Abs(bundle)
	if err != nil {
		return Result{}, err
	}
	if err := experiment.Verify(bundle); err != nil {
		return Result{}, err
	}
	var l Lock
	if err := experiment.ReadJSON(filepath.Join(bundle, "source.lock.json"), &l); err != nil {
		return Result{}, err
	}
	var original Result
	if err := experiment.ReadJSON(filepath.Join(bundle, "build.json"), &original); err != nil {
		return Result{}, err
	}
	lockBytes, err := json.Marshal(l)
	if err != nil {
		return Result{}, err
	}
	resultLockBytes, err := json.Marshal(original.Lock)
	if err != nil {
		return Result{}, err
	}
	digest, err := experiment.DigestFile(filepath.Join(bundle, "module.wasm"))
	if err != nil {
		return Result{}, err
	}
	if original.Schema != 1 || original.Kind != "source_build" || original.Status != "validated_not_correctness_checked" || original.ArtifactSHA256 != digest || original.LockSHA256 != corpus.Hash(lockBytes) || !bytes.Equal(lockBytes, resultLockBytes) {
		return Result{}, fmt.Errorf("source build evidence identity mismatch")
	}
	if l.Schema != 1 || len(l.RunnerSHA256) != 64 {
		return Result{}, fmt.Errorf("invalid source build lock")
	}
	if err := l.Recipe.validate(); err != nil {
		return Result{}, err
	}
	for _, file := range l.inputFiles(filepath.Join(bundle, "sources")) {
		if err := verifyFile(file); err != nil {
			return Result{}, err
		}
	}
	validation, err := os.ReadFile(filepath.Join(bundle, "validation.json"))
	if err != nil {
		return Result{}, err
	}
	if err := l.Analyzer.VerifyEvidence(validation, digest); err != nil {
		return Result{}, err
	}
	if len(original.Steps) != len(l.Recipe.Steps) {
		return Result{}, fmt.Errorf("missing build step evidence")
	}
	if original.CollectionProfile != "" && original.CollectionProfile != "timing" && original.CollectionProfile != "cpu" && original.CollectionProfile != "memory" {
		return Result{}, fmt.Errorf("unknown source collection profile")
	}
	if original.ResourcePolicy != nil {
		if err := original.ResourcePolicy.Validate(); err != nil {
			return Result{}, err
		}
	}
	if original.CollectionProfile == "memory" && (original.ResourcePolicy == nil || original.ResourcePolicy.CgroupParent == "") {
		return Result{}, fmt.Errorf("memory build lacks cgroup policy")
	}
	for i, step := range original.Steps {
		if err := validateStepResources(step, original.ResourcePolicy, original.CollectionProfile); err != nil {
			return Result{}, err
		}
		if original.CollectionProfile == "cpu" {
			if err := step.CPU.validate(); err != nil {
				return Result{}, err
			}
		} else if step.CPU != nil {
			return Result{}, fmt.Errorf("unexpected CPU instrumentation in timing build")
		}
		if step.ContextError != "" || step.Tool != l.Recipe.Steps[i].Tool || step.ElapsedNS < 0 || step.Log != fmt.Sprintf("step-%03d.log", i) {
			return Result{}, fmt.Errorf("invalid build step evidence")
		}
		if _, err := os.Stat(filepath.Join(bundle, step.Log)); err != nil {
			return Result{}, err
		}
	}
	var suite []protocol.Workload
	if err := experiment.ReadJSON(filepath.Join(bundle, "suite.json"), &suite); err != nil {
		return Result{}, err
	}
	if len(suite) != 1 || !filepath.IsAbs(suite[0].Artifact) || filepath.Base(suite[0].Artifact) != "module.wasm" {
		return Result{}, fmt.Errorf("invalid source build suite")
	}
	want := emittedWorkload(original, bundle)
	// The suite's original absolute location is a locator, not content identity;
	// an archived bundle may have moved before offline verification or replay.
	want.Artifact = suite[0].Artifact
	wantBytes, _ := json.Marshal(want)
	gotBytes, _ := json.Marshal(suite[0])
	if !bytes.Equal(wantBytes, gotBytes) {
		return Result{}, fmt.Errorf("source build suite contract mismatch")
	}
	if err := verifySourceRunner(bundle, original); err != nil {
		return Result{}, err
	}
	return original, nil
}

func build(ctx context.Context, l Lock, out string, timeout time.Duration, snapshotRoot, expectedDigest string) (Result, error) {
	return buildWithProfile(ctx, l, out, timeout, snapshotRoot, expectedDigest, "")
}

func buildWithProfile(ctx context.Context, l Lock, out string, timeout time.Duration, snapshotRoot, expectedDigest, profile string) (Result, error) {
	return buildWithResources(ctx, l, out, timeout, snapshotRoot, expectedDigest, profile, nil)
}

func buildWithResources(ctx context.Context, l Lock, out string, timeout time.Duration, snapshotRoot, expectedDigest, profile string, resources *agent.ResourcePolicy) (Result, error) {
	result := Result{Schema: 1, Kind: "source_build", Status: "incomplete", Lock: l, Interpretation: "Trusted local build, not sandboxed or hermetic. Pinned tool executables and explicit input files only; dynamic libraries, system headers and transitive tool dependencies are not automatically captured. Step wall times are operational build diagnostics, not replicated performance samples. Independent Wasm validation does not establish workload correctness."}
	if profile != "" && profile != "timing" && profile != "cpu" && profile != "memory" {
		return result, fmt.Errorf("unknown source collection profile")
	}
	result.CollectionProfile = profile
	result.ToolTimeoutNS = timeout
	result.ResourcePolicy = resources
	if resources != nil {
		if err := resources.Validate(); err != nil {
			return result, err
		}
	}
	if profile == "memory" && (resources == nil || resources.CgroupParent == "") {
		return result, fmt.Errorf("source memory profile requires a delegated cgroup")
	}
	result.ReproducesSHA256 = expectedDigest
	if timeout <= 0 || timeout > time.Hour {
		return result, fmt.Errorf("build timeout must be in (0,1h]")
	}
	if err := l.verify(snapshotRoot); err != nil {
		return result, err
	}
	if err := supported(); err != nil {
		return result, err
	}
	out, err := filepath.Abs(out)
	if err != nil {
		return result, err
	}
	if err = os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		return result, err
	}
	if err = os.Mkdir(out, 0755); err != nil {
		return result, err
	}
	// Canonicalize the owned directory before comparing output paths. On macOS
	// the system temporary directory itself commonly traverses /var -> /private/var.
	out, err = filepath.EvalSymlinks(out)
	if err != nil {
		return result, err
	}
	lockBytes, _ := json.Marshal(l)
	result.LockSHA256 = corpus.Hash(lockBytes)
	result.Host = agent.IdentifyHost()
	if err = experiment.WriteJSON(filepath.Join(out, "source.lock.json"), l); err != nil {
		return result, err
	}
	work, err := os.MkdirTemp(out, "work-")
	if err != nil {
		return result, err
	}
	defer func() {
		if work != "" {
			_ = os.RemoveAll(work)
		}
	}()
	for name, file := range l.inputFiles(snapshotRoot) {
		dst := filepath.Join(out, "sources", name)
		if err = experiment.CopyExclusive(file.Path, dst); err != nil {
			return result, err
		}
		if hash, e := experiment.DigestFile(dst); e != nil || hash != file.SHA256 {
			return result, fmt.Errorf("input changed during snapshot: %s", name)
		}
		if err = experiment.CopyExclusive(dst, filepath.Join(work, name)); err != nil {
			return result, err
		}
	}
	if err = os.Mkdir(filepath.Join(work, ".tmp"), 0700); err != nil {
		return result, err
	}
	env := []string{"TMPDIR=" + filepath.Join(work, ".tmp"), "LC_ALL=C", "LANG=C"}
	keys := make([]string, 0, len(l.Recipe.Environment))
	for key := range l.Recipe.Environment {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		env = append(env, key+"="+l.Recipe.Environment[key])
	}
	deadline, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for index, step := range l.Recipe.Steps {
		tool := l.Recipe.Tools[step.Tool]
		if err = verifyFile(tool); err != nil {
			return result, err
		}
		cmd := exec.CommandContext(deadline, tool.Path, step.Args...)
		cmd.Dir, cmd.Env = work, env
		cmd.WaitDelay = time.Second
		configure(cmd)
		var stdout, stderr logBuffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		var resourceEvidence *agent.ToolExecution
		var elapsed int64
		if resources != nil {
			var execution agent.ToolExecution
			execution, err = agent.RunTool(cmd, *resources, profile == "memory")
			resourceEvidence = &execution
			elapsed = execution.WallNS
		} else {
			started := time.Now()
			err = cmd.Run()
			elapsed = time.Since(started).Nanoseconds()
		}
		log := fmt.Sprintf("step-%03d.log", index)
		logBytes := append(append([]byte("STDOUT\n"), stdout.data.Bytes()...), []byte("\nSTDERR\n")...)
		logBytes = append(logBytes, stderr.data.Bytes()...)
		if e := os.WriteFile(filepath.Join(out, log), logBytes, 0644); e != nil {
			return result, e
		}
		stepResult := StepResult{Tool: step.Tool, ElapsedNS: elapsed, Log: log, Resources: resourceEvidence}
		// Record an observed context condition on a failed tool, not an
		// inference from exit status, signal text, or elapsed-time thresholds.
		if err != nil && deadline.Err() != nil {
			if deadline.Err() == context.DeadlineExceeded {
				stepResult.ContextError = "deadline_exceeded"
			} else {
				stepResult.ContextError = "canceled"
			}
		}
		if profile == "cpu" {
			stepResult.CPU = captureCPU(cmd.ProcessState)
		}
		result.Steps = append(result.Steps, stepResult)
		if err != nil {
			return result, fmt.Errorf("build step %d failed (see %s): %w", index, log, err)
		}
		if err := validateStepResources(stepResult, resources, profile); err != nil {
			return result, err
		}
		if stdout.overflow || stderr.overflow {
			return result, fmt.Errorf("build step output exceeded 1 MiB per stream")
		}
		if err = verifyFile(tool); err != nil {
			return result, err
		}
	}
	artifact := filepath.Join(work, l.Recipe.Output)
	info, err := os.Lstat(artifact)
	if err != nil {
		return result, err
	}
	if !info.Mode().IsRegular() {
		return result, fmt.Errorf("build output is not a regular file")
	}
	// Reject symlink ancestors as well as a symlink leaf.
	resolved, err := filepath.EvalSymlinks(artifact)
	if err != nil || resolved != artifact {
		return result, fmt.Errorf("build output traverses a symlink")
	}
	result.ArtifactSHA256, err = experiment.DigestFile(artifact)
	if err != nil {
		return result, err
	}
	evidence, err := l.Analyzer.AnalyzeArtifact(deadline, artifact, result.ArtifactSHA256)
	if err != nil {
		return result, err
	}
	if err = experiment.CopyExclusive(artifact, filepath.Join(out, "module.wasm")); err != nil {
		return result, err
	}
	if hash, e := experiment.DigestFile(filepath.Join(out, "module.wasm")); e != nil || hash != result.ArtifactSHA256 {
		return result, fmt.Errorf("build output changed during copy")
	}
	if err = os.WriteFile(filepath.Join(out, "validation.json"), evidence, 0644); err != nil {
		return result, err
	}
	if err = l.verify(snapshotRoot); err != nil {
		return result, err
	}
	if expectedDigest != "" && result.ArtifactSHA256 != expectedDigest {
		return result, fmt.Errorf("source reproduction artifact differs: expected %s, got %s", expectedDigest, result.ArtifactSHA256)
	}
	result.Status = "validated_not_correctness_checked"
	if err := archiveSourceRunner(out, l.RunnerSHA256); err != nil {
		return result, err
	}
	result.RunnerArchiveVersion = SourceRunnerArchiveVersion
	if err = experiment.WriteJSON(filepath.Join(out, "build.json"), result); err != nil {
		return result, err
	}
	w := emittedWorkload(result, out)
	if err = experiment.WriteJSON(filepath.Join(out, "suite.json"), []protocol.Workload{w}); err != nil {
		return result, err
	}
	// Remove only the fresh build scratch tree owned by this invocation. Evidence
	// snapshots, logs, lock, validation and final artifact are retained separately.
	if err = os.RemoveAll(work); err != nil {
		return result, err
	}
	work = ""
	return result, experiment.Seal(out)
}

func emittedWorkload(result Result, out string) protocol.Workload {
	l := result.Lock
	w := l.Recipe.Workload
	w.Schema = 1
	w.Artifact = filepath.Join(out, "module.wasm")
	w.SHA256 = result.ArtifactSHA256
	w.License = l.Recipe.License
	w.Source = l.Recipe.SourceRevision
	w.Generator = "source-build-v1"
	w.Provenance, _ = json.Marshal(struct {
		Kind           string `json:"kind"`
		LockSHA256     string `json:"lock_sha256"`
		ArtifactSHA256 string `json:"artifact_sha256"`
		Lock           Lock   `json:"lock"`
	}{"source_build", result.LockSHA256, result.ArtifactSHA256, l})
	return w
}
