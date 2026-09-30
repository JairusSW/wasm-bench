package sourcebuild

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"time"

	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

const BuildMeasurementVersion = "source-tool-process-wall-v1"
const BuildCPUVersion = "source-tool-wait-cpu-v1"
const BuildMemoryVersion = "source-tool-step-cgroup-memory-v1"

type BenchmarkConfig struct {
	RequireIRQAffinity          bool                  `json:"require_irq_affinity,omitempty"`
	RequireIsolatedCPUPartition bool                  `json:"require_isolated_cpu_partition,omitempty"`
	HostPolicy                  *agent.HostPolicy     `json:"host_policy,omitempty"`
	Resources                   *agent.ResourcePolicy `json:"resources,omitempty"`
	Profile                     string                `json:"profile,omitempty"`
	Schema                      int                   `json:"schema"`
	Variants                    []Lock                `json:"variants"`
	CorrectnessRuntimes         []experiment.Runtime  `json:"correctness_runtimes"`
	Blocks                      int                   `json:"blocks"`
	WarmupBlocks                int                   `json:"warmup_blocks"`
	Seed                        int64                 `json:"seed"`
	Timeout                     time.Duration         `json:"timeout_ns"`
}

type BuildAdmission struct {
	FailedBuild    *Result `json:"failed_build_result,omitempty"`
	Variant        int     `json:"variant"`
	Build          string  `json:"build_bundle"`
	Check          string  `json:"correctness_bundle"`
	ArtifactSHA256 string  `json:"artifact_sha256"`
	Status         string  `json:"status"`
	Reason         string  `json:"reason,omitempty"`
}

type BuildTrial struct {
	MemoryPeakBytes *float64 `json:"max_step_cgroup_peak_bytes,omitempty"`
	MemoryStatus    string   `json:"memory_status,omitempty"`
	ToolCPUNS       *int64   `json:"summed_tool_wait_cpu_ns,omitempty"`
	CPUStatus       string   `json:"cpu_status,omitempty"`
	Block           int      `json:"block"`
	Variant         int      `json:"variant"`
	Warmup          bool     `json:"warmup"`
	Bundle          string   `json:"build_bundle"`
	Status          string   `json:"status"`
	Reason          string   `json:"reason,omitempty"`
	ToolWallNS      *int64   `json:"summed_tool_process_wall_ns"`
	Result          Result   `json:"build_result"`
}

type BuildBenchmark struct {
	IRQAffinityStart   *agent.IRQAffinityProbe  `json:"irq_affinity_start,omitempty"`
	IRQAffinityEnd     *agent.IRQAffinityProbe  `json:"irq_affinity_end,omitempty"`
	CPUPartitionStart  *agent.CPUPartitionProbe `json:"cpu_partition_start,omitempty"`
	CPUPartitionEnd    *agent.CPUPartitionProbe `json:"cpu_partition_end,omitempty"`
	HostEnd            *agent.Host              `json:"host_end,omitempty"`
	HostStartCheck     *agent.HostPolicyCheck   `json:"host_start_check,omitempty"`
	HostEndCheck       *agent.HostPolicyCheck   `json:"host_end_check,omitempty"`
	Publication        string                   `json:"publication,omitempty"`
	Schema             int                      `json:"schema"`
	Kind               string                   `json:"kind"`
	Status             string                   `json:"status"`
	MeasurementVersion string                   `json:"measurement_version"`
	Interpretation     string                   `json:"interpretation"`
	Config             BenchmarkConfig          `json:"config"`
	ConfigSHA256       string                   `json:"config_sha256"`
	Host               agent.Host               `json:"host"`
	Admissions         []BuildAdmission         `json:"admissions"`
	Trials             []BuildTrial             `json:"trials"`
	Reproduction       *BenchmarkReproduction   `json:"reproduction,omitempty"`
}

type BenchmarkReproduction struct {
	BundleSHA256 string   `json:"parent_checksums_sha256"`
	ConfigSHA256 string   `json:"parent_config_sha256"`
	Artifacts    []string `json:"parent_artifact_sha256"`
}

type benchmarkReplay struct {
	root            string
	original        BuildBenchmark
	checksumsSHA256 string
}

// ReplayBenchmark reruns the exact schedule and contracts from sealed source
// snapshots. Timing samples are newly collected, never expected to be identical.
func ReplayBenchmark(ctx context.Context, bundle, out string) (BuildBenchmark, error) {
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
	digest, err := experiment.DigestFile(filepath.Join(bundle, "checksums.json"))
	if err != nil {
		return BuildBenchmark{}, err
	}
	return benchmark(ctx, original.Config, out, &benchmarkReplay{root: bundle, original: original, checksumsSHA256: digest})
}

func (c BenchmarkConfig) validate() error {
	if c.RequireIRQAffinity {
		if c.Resources == nil {
			return fmt.Errorf("IRQ affinity requirement needs explicit resource policy")
		}
		if err := agent.ValidateIRQAffinityPolicy(*c.Resources); err != nil {
			return err
		}
	}
	if c.RequireIsolatedCPUPartition {
		if c.Resources == nil {
			return fmt.Errorf("CPU partition requirement needs explicit resource policy")
		}
		if err := agent.ValidateCPUPartitionPolicy(*c.Resources); err != nil {
			return err
		}
	}
	if err := c.HostPolicy.Validate(); err != nil {
		return err
	}
	if c.Profile != "" && c.Profile != "timing" && c.Profile != "cpu" && c.Profile != "memory" {
		return fmt.Errorf("source benchmark profile must be timing, cpu or memory")
	}
	if c.Resources != nil {
		if err := c.Resources.Validate(); err != nil {
			return err
		}
	}
	if c.Profile == "memory" && (c.Resources == nil || c.Resources.CgroupParent == "") {
		return fmt.Errorf("source memory profile requires a delegated cgroup")
	}
	if c.Schema != 1 || len(c.Variants) < 1 || len(c.Variants) > 16 || len(c.CorrectnessRuntimes) == 0 || c.Blocks < 1 || c.Blocks > 1000 || c.WarmupBlocks < 0 || c.WarmupBlocks > 100 || c.Timeout <= 0 || c.Timeout > time.Hour {
		return fmt.Errorf("invalid source benchmark configuration")
	}
	ids := map[string]bool{}
	for _, l := range c.Variants {
		if ids[l.Recipe.ID] {
			return fmt.Errorf("duplicate source variant ID")
		}
		ids[l.Recipe.ID] = true
		if err := l.Recipe.validate(); err != nil {
			return err
		}
		if err := SameTask(Result{Lock: c.Variants[0]}, Result{Lock: l}); err != nil {
			return err
		}
		if l.RunnerSHA256 != c.Variants[0].RunnerSHA256 {
			return fmt.Errorf("source variants require the same runner")
		}
	}
	return nil
}

func buildSchedule(c BenchmarkConfig) []BuildTrial {
	rng := rand.New(rand.NewSource(c.Seed))
	var trials []BuildTrial
	for block := -c.WarmupBlocks; block < c.Blocks; block++ {
		for _, variant := range rng.Perm(len(c.Variants)) {
			trials = append(trials, BuildTrial{Block: block, Variant: variant, Warmup: block < 0, Status: "not_run", Bundle: fmt.Sprintf("trials/%06d", len(trials))})
		}
	}
	return trials
}

// Benchmark retains independent complete builds in randomized blocks. Exact
// output identity transfers the admitted correctness result to each trial;
// differing output is retained but never eligible for timing analysis.
func Benchmark(ctx context.Context, c BenchmarkConfig, out string) (BuildBenchmark, error) {
	return benchmark(ctx, c, out, nil)
}

func benchmark(ctx context.Context, c BenchmarkConfig, out string, replay *benchmarkReplay) (BuildBenchmark, error) {
	r := BuildBenchmark{Schema: 1, Kind: "source_build_benchmark", Status: "incomplete", MeasurementVersion: BuildMeasurementVersion, Config: c,
		Interpretation: "Exploratory trusted-local source compilation. Independent fresh tool processes and scratch directories; caches, scheduling and machine resources uncontrolled. Metric is the sum of each recipe step's exec.Cmd.Run wall time, including process startup, output capture and waiting, excluding source staging, hashing, validation, oracle checks and inter-step gaps. It is not whole-pipeline elapsed latency. Warmup builds are retained and excluded from headline summaries. Byte-identical outputs inherit sacrificial oracle admission. No hermeticity or official publication qualification is claimed."}
	if c.Profile == "cpu" {
		r.MeasurementVersion = BuildCPUVersion
		r.Interpretation = "Dedicated exploratory CPU accounting pass. User/system CPU from Go os.ProcessState after each tool exits, summed over recipe steps. OS wait accounting may include waited descendants; not guaranteed full process-tree/cgroup coverage. Includes tool startup and shutdown; excludes controller, hashing, validation and oracle processes. Nanosecond storage does not imply nanosecond accounting precision. Wall times retained as diagnostics, not headline timing results. Warmup builds are retained and excluded from summaries; only correctness-admitted exact outputs qualify. No enforced resource budget, hermeticity or official publication qualification."
	}
	if c.Profile == "memory" {
		r.MeasurementVersion = BuildMemoryVersion
		r.Interpretation = "Dedicated source memory pass. Each recipe step starts inside a fresh cgroup with requested local limits; ancestors still apply and CPUs are not exclusive. Captures current charged memory and lifetime peak after command wait, before descendant cleanup. Summary is maximum step cgroup peak, not a simultaneous whole-build peak, RSS or heap. Controller staging, validation and correctness processes are outside these tool cgroups. Cached-page charge ownership may differ across steps. Only correctness-admitted exact outputs qualify. Wall times are diagnostic; no official publication qualification."
	}
	if err := c.validate(); err != nil {
		return r, err
	}
	snapshots := func(i int) string {
		if replay == nil {
			return ""
		}
		return filepath.Join(replay.root, replay.original.Admissions[i].Build, "sources")
	}
	for i, l := range c.Variants {
		if err := l.verify(snapshots(i)); err != nil {
			return r, err
		}
	}
	r.Host = agent.IdentifyHost()
	if c.RequireIRQAffinity {
		p := agent.ProbeIRQAffinity(c.Resources.CPUs)
		r.IRQAffinityStart = &p
		if err := p.Err(); err != nil {
			return r, err
		}
		r.Publication = "local_exploratory"
	}
	if c.RequireIsolatedCPUPartition {
		p := agent.ProbeCPUPartition(c.Resources.CgroupParent, c.Resources.CPUs)
		r.CPUPartitionStart = &p
		if err := p.Err(); err != nil {
			return r, err
		}
		r.Publication = "local_exploratory"
	}
	if c.HostPolicy != nil {
		check := agent.CheckHostPolicy(c.HostPolicy, r.Host, "before_run_preparation")
		r.HostStartCheck = &check
		if err := check.Err(); err != nil {
			return r, err
		}
		r.Publication = "local_exploratory"
	}
	if replay != nil {
		if !reflect.DeepEqual(r.Host, replay.original.Host) {
			return r, fmt.Errorf("source benchmark replay host fingerprint or environment differs")
		}
		r.Reproduction = &BenchmarkReproduction{BundleSHA256: replay.checksumsSHA256, ConfigSHA256: replay.original.ConfigSHA256}
		for _, a := range replay.original.Admissions {
			r.Reproduction.Artifacts = append(r.Reproduction.Artifacts, a.ArtifactSHA256)
		}
	}
	var err error
	out, err = filepath.Abs(out)
	if err != nil {
		return r, err
	}
	if err = os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		return r, err
	}
	if err = os.Mkdir(out, 0755); err != nil {
		return r, err
	}
	data, _ := json.Marshal(c)
	r.ConfigSHA256 = corpus.Hash(data)
	if err = experiment.WriteJSON(filepath.Join(out, "benchmark.lock.json"), c); err != nil {
		return r, err
	}
	finish := func(cause error) (BuildBenchmark, error) {
		if c.RequireIRQAffinity {
			p := agent.ProbeIRQAffinity(c.Resources.CPUs)
			r.IRQAffinityEnd = &p
			if err := p.Err(); err != nil {
				r.Publication = "prohibited_irq_affinity_mismatch"
				cause = errors.Join(cause, fmt.Errorf("%w; diagnostic bundle retained at %s", err, out))
			}
		}
		if c.RequireIsolatedCPUPartition {
			p := agent.ProbeCPUPartition(c.Resources.CgroupParent, c.Resources.CPUs)
			r.CPUPartitionEnd = &p
			if err := p.Err(); err != nil {
				r.Publication = "prohibited_cpu_partition_mismatch"
				cause = errors.Join(cause, fmt.Errorf("%w; diagnostic bundle retained at %s", err, out))
			}
		}
		if c.HostPolicy != nil {
			end := agent.IdentifyHost()
			check := agent.CheckHostPolicy(c.HostPolicy, end, "after_trials_before_seal")
			r.HostEnd, r.HostEndCheck = &end, &check
			if err := check.Err(); err != nil {
				r.Publication = "prohibited_host_baseline_mismatch"
				cause = errors.Join(cause, fmt.Errorf("%w; diagnostic bundle retained at %s", err, out))
			}
		}
		if err := experiment.WriteJSON(filepath.Join(out, "benchmark.json"), r); err != nil {
			return r, err
		}
		if err := experiment.Seal(out); err != nil {
			return r, err
		}
		return r, cause
	}
	for i, l := range c.Variants {
		a := BuildAdmission{Variant: i, Build: fmt.Sprintf("admission/%02d/build", i), Check: fmt.Sprintf("admission/%02d/check", i), Status: "build_failed"}
		expected := ""
		if replay != nil {
			expected = replay.original.Admissions[i].ArtifactSHA256
		}
		built, e := buildWithResources(ctx, l, filepath.Join(out, a.Build), c.Timeout, snapshots(i), expected, c.Profile, c.Resources)
		if e != nil {
			a.FailedBuild = &built
		}
		if e == nil {
			a.ArtifactSHA256 = built.ArtifactSHA256
			w := emittedWorkload(built, filepath.Join(out, a.Build))
			check, e2 := experiment.NewLock(experiment.Options{Suite: "source-build-admission", Profile: "timing", Scenarios: []string{"first-call"}, Launches: 1, Samples: 1, Operations: 1, Timeout: c.Timeout, Check: true}, slices.Clone(c.CorrectnessRuntimes), []protocol.Workload{w})
			if e2 == nil {
				check.Analyzer = l.Analyzer
				_, e2 = experiment.Run(ctx, check, out, filepath.Join(out, a.Check), func(string) {})
			}
			if e2 == nil {
				var evidence experiment.Bundle
				evidence, e2 = experiment.Load(filepath.Join(out, a.Check))
				if e2 == nil {
					e2 = verifyAdmission(evidence, built, c.CorrectnessRuntimes)
				}
			}
			e = e2
			a.Status = "correctness_failed"
		}
		if e == nil {
			a.Status = "ok"
		} else {
			a.Reason = e.Error()
			if a.FailedBuild != nil {
				a.Status = buildFailureStatus(built)
			}
		}
		r.Admissions = append(r.Admissions, a)
	}
	for _, a := range r.Admissions {
		if a.Status != "ok" {
			r.Status = "admission_failed"
			return finish(fmt.Errorf("source benchmark admission failed; evidence retained"))
		}
	}
	r.Trials = buildSchedule(c)
	failed := false
	for i := range r.Trials {
		t := &r.Trials[i]
		if ctx.Err() != nil {
			t.Reason = ctx.Err().Error()
			failed = true
			continue
		}
		built, e := buildWithResources(ctx, c.Variants[t.Variant], filepath.Join(out, t.Bundle), c.Timeout, snapshots(t.Variant), "", c.Profile, c.Resources)
		t.Result = built
		switch {
		case e != nil:
			t.Status = buildFailureStatus(built)
			t.Reason = e.Error()
		case built.ArtifactSHA256 != r.Admissions[t.Variant].ArtifactSHA256:
			t.Status = "different_output"
			t.Reason = "output does not match correctness-admitted artifact"
		case !reflect.DeepEqual(built.Host, r.Host):
			t.Status = "host_changed"
			t.Reason = "build host fingerprint changed"
		default:
			t.Status = "ok"
			var total int64
			for _, step := range built.Steps {
				total += step.ElapsedNS
			}
			t.ToolWallNS = &total
			if c.Profile == "cpu" {
				t.ToolCPUNS = summedCPU(built.Steps)
				t.CPUStatus = "available"
				if t.ToolCPUNS == nil {
					t.CPUStatus = "unavailable"
				}
			}
			if c.Profile == "memory" {
				t.MemoryPeakBytes = maximumStepPeak(built.Steps)
				t.MemoryStatus = "available"
				if t.MemoryPeakBytes == nil {
					t.MemoryStatus = "unavailable"
				}
			}
		}
		if t.Status != "ok" {
			failed = true
		}
	}
	r.Status = "complete"
	if failed {
		r.Status = "completed_with_failures"
		return finish(fmt.Errorf("source benchmark contains failed trials; evidence retained"))
	}
	return finish(nil)
}

func verifyAdmission(b experiment.Bundle, built Result, runtimes []experiment.Runtime) error {
	if b.Manifest.Kind != "correctness_only" || len(b.Manifest.Lock.Workloads) != 1 || !built.MatchesWorkload(b.Manifest.Lock.Workloads[0]) {
		return fmt.Errorf("incorrect source admission identity")
	}
	if len(b.Trials) != len(runtimes) {
		return fmt.Errorf("incomplete source oracle checks")
	}
	seen := map[string]bool{}
	for _, t := range b.Trials {
		if t.Status != "ok" || seen[t.Runtime] || len(t.Samples) == 0 {
			return fmt.Errorf("source oracle check %s: %s", t.Runtime, t.Status)
		}
		for _, s := range t.Samples {
			if !s.Verified {
				return fmt.Errorf("unverified source oracle result")
			}
		}
		seen[t.Runtime] = true
	}
	for _, runtime := range runtimes {
		if !seen[runtime.ID] {
			return fmt.Errorf("missing source oracle runtime")
		}
	}
	if len(b.Manifest.Lock.Runtimes) != len(runtimes) {
		return fmt.Errorf("source oracle runtime identity mismatch")
	}
	for i, want := range runtimes {
		got := b.Manifest.Lock.Runtimes[i]
		got.Description, want.Description = nil, nil
		// HostFiles is omitted in JSON when empty. Run's immutable JSON clone
		// round-trips an empty map to nil; both mean no pinned host libraries,
		// not different runtime identities. Nonempty pins stay exact.
		if len(got.HostFiles) == 0 && len(want.HostFiles) == 0 {
			got.HostFiles, want.HostFiles = nil, nil
		}
		if !reflect.DeepEqual(got, want) {
			fields := []string{}
			x, y := reflect.ValueOf(got), reflect.ValueOf(want)
			for j := 0; j < x.NumField(); j++ {
				if !reflect.DeepEqual(x.Field(j).Interface(), y.Field(j).Interface()) {
					fields = append(fields, x.Type().Field(j).Name)
				}
			}
			return fmt.Errorf("source oracle runtime pins differ for %s: %v", want.ID, fields)
		}
	}
	return nil
}

// VerifyBenchmark checks raw trial arithmetic, schedule, build identity and
// correctness evidence offline before any samples may enter analysis.
func VerifyBenchmark(root string) (BuildBenchmark, error) {
	var r BuildBenchmark
	if err := experiment.Verify(root); err != nil {
		return r, err
	}
	if err := experiment.ReadJSON(filepath.Join(root, "benchmark.json"), &r); err != nil {
		return r, err
	}
	wantVersion := BuildMeasurementVersion
	if r.Config.Profile == "cpu" {
		wantVersion = BuildCPUVersion
	}
	if r.Config.Profile == "memory" {
		wantVersion = BuildMemoryVersion
	}
	if r.Schema != 1 || r.Kind != "source_build_benchmark" || r.MeasurementVersion != wantVersion {
		return r, fmt.Errorf("unsupported source benchmark")
	}
	if err := r.Config.validate(); err != nil {
		return r, err
	}
	if err := r.ValidateHostEvidence(); err != nil {
		return r, err
	}
	var lock BenchmarkConfig
	if err := experiment.ReadJSON(filepath.Join(root, "benchmark.lock.json"), &lock); err != nil {
		return r, err
	}
	data, _ := json.Marshal(lock)
	recorded, _ := json.Marshal(r.Config)
	if corpus.Hash(data) != r.ConfigSHA256 || string(data) != string(recorded) {
		return r, fmt.Errorf("source benchmark config identity mismatch")
	}
	if p := r.Reproduction; p != nil {
		validHash := func(s string) bool { b, e := hex.DecodeString(s); return e == nil && len(b) == 32 }
		if !validHash(p.BundleSHA256) || p.ConfigSHA256 != r.ConfigSHA256 || len(p.Artifacts) != len(lock.Variants) {
			return r, fmt.Errorf("invalid source benchmark reproduction identity")
		}
		for _, hash := range p.Artifacts {
			if !validHash(hash) {
				return r, fmt.Errorf("invalid source benchmark reproduction artifact")
			}
		}
	}
	if len(r.Admissions) != len(lock.Variants) {
		return r, fmt.Errorf("incomplete source admissions")
	}
	failedAdmission := false
	for i, a := range r.Admissions {
		if a.Variant != i || a.Build != fmt.Sprintf("admission/%02d/build", i) || a.Check != fmt.Sprintf("admission/%02d/check", i) {
			return r, fmt.Errorf("invalid admission paths or order")
		}
		if a.Status != "ok" {
			if !slices.Contains([]string{"build_failed", "correctness_failed", "oom", "timeout", "canceled"}, a.Status) || a.Reason == "" {
				return r, fmt.Errorf("invalid admission failure")
			}
			if (a.Status == "timeout" || a.Status == "canceled") && a.FailedBuild == nil {
				return r, fmt.Errorf("context failure lacks source receipt")
			}
			if a.FailedBuild != nil {
				if a.Status == "correctness_failed" {
					return r, fmt.Errorf("failed tool evidence on correctness failure")
				}
				if err := verifyFailedBuild(root, a.Build, *a.FailedBuild, lock.Variants[i], lock, a.Status); err != nil {
					return r, err
				}
			}
			failedAdmission = true
			continue
		}
		if a.FailedBuild != nil {
			return r, fmt.Errorf("failed build evidence on successful admission")
		}
		built, err := Verify(filepath.Join(root, a.Build))
		if err != nil {
			return r, err
		}
		if !reflect.DeepEqual(built.Lock, lock.Variants[i]) || built.ArtifactSHA256 != a.ArtifactSHA256 || built.CollectionProfile != lock.Profile || !reflect.DeepEqual(built.ResourcePolicy, lock.Resources) {
			return r, fmt.Errorf("admission build identity mismatch")
		}
		if p := r.Reproduction; p != nil && (built.ArtifactSHA256 != p.Artifacts[i] || built.ReproducesSHA256 != p.Artifacts[i]) {
			return r, fmt.Errorf("reproduced admission differs from parent artifact")
		}
		check, err := experiment.Load(filepath.Join(root, a.Check))
		if err != nil {
			return r, err
		}
		if err := verifyAdmission(check, built, lock.CorrectnessRuntimes); err != nil {
			return r, err
		}
	}
	if failedAdmission {
		if r.Status != "admission_failed" || len(r.Trials) != 0 {
			return r, fmt.Errorf("measurements after failed admission")
		}
		return r, nil
	}
	want := buildSchedule(lock)
	if len(want) != len(r.Trials) {
		return r, fmt.Errorf("incomplete source trial schedule")
	}
	failed := false
	for i, t := range r.Trials {
		w := want[i]
		if t.Block != w.Block || t.Variant != w.Variant || t.Warmup != w.Warmup || t.Bundle != w.Bundle {
			return r, fmt.Errorf("source trial schedule differs")
		}
		if t.Status != "ok" {
			if !slices.Contains([]string{"not_run", "build_failed", "oom", "timeout", "canceled", "different_output", "host_changed"}, t.Status) || t.Reason == "" || t.ToolWallNS != nil || t.ToolCPUNS != nil || t.CPUStatus != "" || t.MemoryPeakBytes != nil || t.MemoryStatus != "" {
				return r, fmt.Errorf("invalid failed source trial")
			}
			if slices.Contains([]string{"build_failed", "oom", "timeout", "canceled"}, t.Status) {
				if err := verifyFailedBuild(root, t.Bundle, t.Result, lock.Variants[t.Variant], lock, t.Status); err != nil {
					return r, err
				}
			}
			failed = true
			continue
		}
		built, err := Verify(filepath.Join(root, t.Bundle))
		if err != nil {
			return r, err
		}
		if !reflect.DeepEqual(built, t.Result) || !reflect.DeepEqual(built.Lock, lock.Variants[t.Variant]) || !reflect.DeepEqual(built.Host, r.Host) || built.ArtifactSHA256 != r.Admissions[t.Variant].ArtifactSHA256 || built.CollectionProfile != lock.Profile || !reflect.DeepEqual(built.ResourcePolicy, lock.Resources) {
			return r, fmt.Errorf("source trial identity mismatch")
		}
		var total int64
		for _, s := range built.Steps {
			total += s.ElapsedNS
		}
		if t.ToolWallNS == nil || *t.ToolWallNS != total {
			return r, fmt.Errorf("source trial timing differs from step evidence")
		}
		if lock.Profile == "cpu" {
			wantCPU := summedCPU(built.Steps)
			status := "available"
			if wantCPU == nil {
				status = "unavailable"
			}
			if !reflect.DeepEqual(wantCPU, t.ToolCPUNS) || t.CPUStatus != status {
				return r, fmt.Errorf("source CPU total differs from step evidence")
			}
		} else if t.ToolCPUNS != nil || t.CPUStatus != "" {
			return r, fmt.Errorf("CPU metric in timing-only benchmark")
		}
		if lock.Profile == "memory" {
			want := maximumStepPeak(built.Steps)
			status := "available"
			if want == nil {
				status = "unavailable"
			}
			if !reflect.DeepEqual(want, t.MemoryPeakBytes) || status != t.MemoryStatus {
				return r, fmt.Errorf("source memory summary differs from step evidence")
			}
		} else if t.MemoryPeakBytes != nil || t.MemoryStatus != "" {
			return r, fmt.Errorf("unexpected source memory instrumentation")
		}
	}
	if (!failed && r.Status != "complete") || (failed && r.Status != "completed_with_failures") {
		return r, fmt.Errorf("source benchmark status contradicts trials")
	}
	return r, nil
}
