package experiment

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func ResolveRuntimes(root string, ids []string) ([]Runtime, error) {
	var out []Runtime
	for _, id := range ids {
		r := Runtime{ID: id, Files: map[string]string{}}
		switch id {
		case "wasmtime-process-snapshot", "wasmtime-winch-process-snapshot":
			backend := "cranelift"
			if id == "wasmtime-winch-process-snapshot" {
				backend = "winch"
			}
			r.Command = []string{filepath.Join(root, "adapters", "wasmtime", "target", "process-snapshot", "release", "qualify-process-snapshot"), "--adapter=" + backend}
		case "wasmtime-component-async":
			r.Command = []string{filepath.Join(root, "adapters", "wasmtime", "target", "component-async", "release", "adapter-wasmtime"), "--component-async"}
		case "wasmtime", "wasmtime-winch", "wasmtime-pooling", "wasmtime-allocator", "wasmtime-winch-allocator", "wasmtime-code-lifetime", "wasmtime-winch-code-lifetime":
			r.Command = []string{filepath.Join(root, "adapters", "wasmtime", "target", "release", "adapter-wasmtime")}
			if strings.HasSuffix(id, "-allocator") {
				r.Command[0] = filepath.Join(root, "adapters", "wasmtime", "target", "allocator", "release", "adapter-wasmtime")
			}
			if strings.HasSuffix(id, "-code-lifetime") {
				r.Command[0] = filepath.Join(root, "adapters", "wasmtime", "target", "code-lifetime", "release", "adapter-wasmtime")
			}
			if id == "wasmtime-winch" || strings.HasPrefix(id, "wasmtime-winch-") {
				r.Command = append(r.Command, "--winch")
			}
			if id == "wasmtime-pooling" {
				r.Command = append(r.Command, "--pooling")
			}
		case "wago":
			r.Command = []string{filepath.Join(root, "bin", "adapter-wago")}
		case "wazero":
			r.Command = []string{filepath.Join(root, "bin", "adapter-wazero")}
		case "wazero-interpreter":
			r.Command = []string{filepath.Join(root, "bin", "adapter-wazero"), "--interpreter"}
		case "v8", "v8-wasmfx", "v8-liftoff-only", "v8-optimizing-only":
			node := os.Getenv("WASMBENCH_NODE")
			if node == "" {
				node = "node"
			}
			node, e := exec.LookPath(node)
			if e != nil {
				return nil, e
			}
			r.Command = []string{node}
			if id == "v8-wasmfx" {
				r.Command = append(r.Command, "--experimental-wasm-wasmfx")
			}
			mode := ""
			if id == "v8" {
				mode = os.Getenv("WASMBENCH_V8_COMPILER_MODE")
				if mode != "" && mode != "optimizing-only" && mode != "liftoff-only" {
					return nil, fmt.Errorf("invalid WASMBENCH_V8_COMPILER_MODE %q", mode)
				}
			}
			if id == "v8-liftoff-only" || mode == "liftoff-only" {
				mode = "liftoff-only"
				r.Command = append(r.Command, "--allow-natives-syntax", "--liftoff-only", "--no-wasm-tier-up", "--no-wasm-lazy-compilation")
			}
			if id == "v8-optimizing-only" || mode == "optimizing-only" {
				mode = "optimizing-only"
				r.Command = append(r.Command, "--allow-natives-syntax", "--no-liftoff", "--no-wasm-tier-up", "--no-wasm-lazy-compilation")
			}
			if id == "v8" && mode != "" {
				r.Command = append(r.Command, "--no-wasm-native-module-cache")
			}
			r.Command = append(r.Command, filepath.Join(root, "adapters", "v8", "adapter.mjs"))
			if mode != "" {
				r.Command = append(r.Command, "--compiler-mode="+mode)
			}
		case "v8-tier-observed", "v8-tier-traced":
			node := os.Getenv("WASMBENCH_NODE")
			if node == "" {
				node = "node"
			}
			node, err := exec.LookPath(node)
			if err != nil {
				return nil, err
			}
			r.Command = []string{node, "--allow-natives-syntax", filepath.Join(root, "adapters/v8/tier-adapter.mjs")}
			if id == "v8-tier-traced" {
				r.Command = append(r.Command, "--trace-wasm-events")
			}
		default:
			command, supported, err := extraRuntimeCommand(root, id)
			if err != nil {
				return nil, err
			}
			if !supported {
				return nil, fmt.Errorf("unknown runtime configuration %q", id)
			}
			r.Command = command
		}
		if id != "v8" && !strings.HasPrefix(id, "v8-") {
			r.Command[0] = NativeExecutable(r.Command[0])
		}
		for _, path := range r.Command {
			if strings.HasPrefix(path, "--") || strings.HasPrefix(path, "runtime=") || strings.HasPrefix(path, "runtime-version=") || strings.HasPrefix(path, "binary-sha256=") || strings.HasPrefix(path, "tier-mode=") || path == "-f" || path == "-jar" || path == "run" {
				continue
			}
			hash, e := DigestFile(path)
			if e != nil {
				return nil, fmt.Errorf("build adapter %s first: %w", id, e)
			}
			r.Files[path] = hash
		}
		if id == "v8" || id == "v8-liftoff-only" || id == "v8-optimizing-only" || id == "v8-tier-observed" || id == "v8-tier-traced" {
			helpers := []string{"floats.mjs", "profiling.mjs", "compiler-mode.mjs", "harness.mjs", "wasi-readonly.mjs", "native-size.mjs"}
			if id == "v8-tier-observed" || id == "v8-tier-traced" {
				helpers = []string{"floats.mjs"}
			}
			if id == "v8-tier-traced" {
				helpers = append(helpers, "tracing.mjs")
			}
			for _, helper := range helpers {
				path := filepath.Join(root, "adapters", "v8", helper)
				digest, err := DigestFile(path)
				if err != nil {
					return nil, err
				}
				r.Files[path] = digest
			}
		}
		if err := pinNativeDependencies(&r); err != nil {
			return nil, fmt.Errorf("runtime %s native dependencies: %w", id, err)
		}
		out = append(out, r)
	}
	return out, nil
}
func NewLock(options Options, runtimes []Runtime, workloads []protocol.Workload) (Lock, error) {
	if options.Workers == 0 {
		options.Workers = 1
	}
	exe, e := os.Executable()
	if e != nil {
		return Lock{}, e
	}
	hash, e := DigestFile(exe)
	if e != nil {
		return Lock{}, e
	}
	l := Lock{Schema: 1, RunnerVersion: Version, RunnerSHA256: hash, Protocol: 1, Options: options, Runtimes: runtimes, Workloads: workloads}
	return l, ValidateLock(l)
}
func ValidateLock(l Lock) error {
	if err := validateIRQPolicy(l); err != nil {
		return err
	}
	for _, r := range l.Runtimes {
		if err := validateELFContract(r); err != nil {
			return err
		}
	}
	if l.ArchiveTools {
		if _, err := toolArchiveSpec(l); err != nil {
			return err
		}
	}
	if l.RequireIsolatedCPUPartition {
		if err := agent.ValidateCPUPartitionPolicy(l.Options.Resources); err != nil {
			return err
		}
	}
	if err := l.HostPolicy.Validate(); err != nil {
		return err
	}
	if err := validatePilotPlan(l); err != nil {
		return err
	}
	if err := l.Analyzer.validate(); err != nil {
		return err
	}
	if l.Schema != 1 || l.Protocol != 1 {
		return fmt.Errorf("unsupported schema or protocol")
	}
	o := l.Options
	if o.TimingPeakRSS {
		if o.Profile != "timing" {
			return fmt.Errorf("timing peak RSS requires the timing profile")
		}
		for _, scenario := range o.Scenarios {
			if !slices.Contains([]string{"compile", "instantiate", "first-call", "steady"}, scenario) {
				return fmt.Errorf("timing peak RSS requires ordinary lifecycle scenarios")
			}
		}
	}
	if o.Workers < 0 || o.Workers > 3 {
		return fmt.Errorf("workers must be within 1..3")
	}
	if o.SustainedPostCollection && (o.Profile != "memory" || !slices.Contains(o.Scenarios, "sustained")) {
		return fmt.Errorf("sustained post-collection requires sustained scenario in a dedicated memory pass")
	}
	if slices.Contains(o.Scenarios, "sustained") {
		if o.SustainedDuration < time.Millisecond || o.SustainedDuration > time.Hour || o.SustainedDuration >= o.Timeout {
			return fmt.Errorf("sustained duration must be 1 ms to 1 h and below the trial timeout")
		}
	} else if o.SustainedDuration != 0 {
		return fmt.Errorf("sustained duration requires a sustained scenario")
	}
	if o.PhaseBarriers && o.Profile != "memory" && o.Profile != "counters" {
		return fmt.Errorf("phase barriers require a dedicated memory or counters pass")
	}
	if o.Profile == "counters" {
		for _, scenario := range o.Scenarios {
			if _, err := protocol.CounterSampleCount(&protocol.RunRequest{Scenario: scenario, Samples: o.Samples, Operations: o.Operations, Warmup: o.Warmup, PhaseBarriers: o.PhaseBarriers}); err != nil {
				return err
			}
		}
	}
	if err := o.Resources.Validate(); err != nil {
		return err
	}
	if o.Launches < 1 || o.Samples < 1 || o.Operations < 1 || o.Warmup < 0 || o.Timeout <= 0 {
		return fmt.Errorf("positive launches, samples, operations and timeout required")
	}
	for scenario, samples := range o.ScenarioSamples {
		if (scenario != "*" && !slices.Contains(o.Scenarios, scenario)) || samples < 1 || samples > 100000 {
			return fmt.Errorf("scenario sample override %q=%d must name a selected scenario and be within 1..100000", scenario, samples)
		}
	}
	if o.Samples > 100000 || o.Operations > 1000000 || o.Launches > 10000 {
		return fmt.Errorf("batch exceeds protocol safety bounds")
	}
	if !slices.Contains([]string{"timing", "memory", "code", "counters", "profiling"}, o.Profile) {
		return fmt.Errorf("unknown collection profile %q", o.Profile)
	}
	if len(l.Workloads) == 0 || len(l.Runtimes) == 0 || len(o.Scenarios) == 0 {
		return fmt.Errorf("empty experiment")
	}
	ids := map[string]bool{}
	for _, w := range l.Workloads {
		if w.SnapshotDensity != nil || w.Oracle.Kind == "linux_process_snapshot_density_v1" {
			if err := protocol.ValidateSnapshotDensityWorkload(w); err != nil {
				return err
			}
		}
		if w.ProcessSnapshot != nil || w.Oracle.Kind == "linux_process_snapshot_v1" || w.Reset == "fresh_process_snapshot_per_sample" {
			if err := protocol.ValidateProcessSnapshotWorkload(w); err != nil {
				return err
			}
		}
		if w.Oracle.Kind == "expected_trap" {
			if err := protocol.ValidateTrap(w); err != nil {
				return err
			}
		} else if w.Oracle.ExpectedTrap != "" {
			return fmt.Errorf("expected_trap requires its own oracle kind")
		}
		if ids[w.ID] {
			return fmt.Errorf("duplicate workload %q", w.ID)
		}
		ids[w.ID] = true
		if w.Units == 0 {
			return fmt.Errorf("workload %s has no work units", w.ID)
		}
		if w.SHA256 == "" {
			return fmt.Errorf("missing artifact digest")
		}
	}
	return nil
}
func VerifyInputs(l Lock, artifactBase string) error {
	if err := l.Analyzer.verify(); err != nil {
		return err
	}
	for _, w := range l.Workloads {
		if err := verifyCommandFiles(w, artifactBase); err != nil {
			return err
		}
		path := w.Artifact
		if !filepath.IsAbs(path) {
			path = filepath.Join(artifactBase, path)
		}
		hash, e := DigestFile(path)
		if e != nil {
			return e
		}
		if hash != w.SHA256 {
			return fmt.Errorf("workload %s artifact changed", w.ID)
		}
	}
	for _, r := range l.Runtimes {
		if err := verifyELFControls(r); err != nil {
			return err
		}
		for path, want := range r.HostFiles {
			got, err := DigestFile(path)
			if err != nil {
				return fmt.Errorf("runtime %s host library %s: %w", r.ID, path, err)
			}
			if got != want {
				return fmt.Errorf("runtime %s host library changed: %s", r.ID, path)
			}
		}
		for path, want := range r.Files {
			got, e := DigestFile(path)
			if e != nil {
				return e
			}
			if got != want {
				return fmt.Errorf("runtime %s changed: %s", r.ID, path)
			}
		}
	}
	return nil
}

func Run(ctx context.Context, lock Lock, artifactBase, out string, progress func(string)) (string, error) {
	// A lock is a reusable plan. Bundle-relative paths and discovered descriptions
	// belong to this run only; do not mutate the caller's slices or maps.
	encoded, cloneErr := json.Marshal(lock)
	if cloneErr != nil {
		return "", cloneErr
	}
	var owned Lock
	if cloneErr = json.Unmarshal(encoded, &owned); cloneErr != nil {
		return "", cloneErr
	}
	lock = owned
	if err := ValidateLock(lock); err != nil {
		return "", err
	}
	executable, runnerErr := os.Executable()
	if runnerErr != nil {
		return "", runnerErr
	}
	runnerHash, runnerErr := DigestFile(executable)
	if runnerErr != nil {
		return "", runnerErr
	}
	if runnerHash != lock.RunnerSHA256 {
		return "", fmt.Errorf("runner executable differs from locked SHA-256; restore the recorded runner build or create a new experiment plan")
	}
	if err := VerifyInputs(lock, artifactBase); err != nil {
		return "", err
	}
	// Explicit execution may invoke the native loader. Offline inspection and
	// restoration never do. Re-resolve after relocation before creating output.
	for i := range lock.Runtimes {
		if err := qualifyELFRuntime(ctx, &lock.Runtimes[i]); err != nil {
			return "", err
		}
	}
	var baselineHost agent.Host
	var baselineStart *agent.HostPolicyCheck
	if lock.HostPolicy != nil {
		baselineHost = agent.IdentifyHost()
		check := agent.CheckHostPolicy(lock.HostPolicy, baselineHost, "before_run_preparation")
		baselineStart = &check
		if err := check.Err(); err != nil {
			return "", err
		}
	}
	// Directory creation is exclusive: completed runs are never overwritten.
	var partitionStart *agent.CPUPartitionProbe
	var irqStart *agent.IRQAffinityProbe
	if lock.RequireIRQAffinity {
		p := agent.ProbeIRQAffinity(lock.Options.Resources.CPUs)
		irqStart = &p
		if err := p.Err(); err != nil {
			return "", err
		}
	}
	if lock.RequireIsolatedCPUPartition {
		p := agent.ProbeCPUPartition(lock.Options.Resources.CgroupParent, lock.Options.Resources.CPUs)
		partitionStart = &p
		if err := p.Err(); err != nil {
			return "", err
		}
	}
	abs, err := filepath.Abs(out)
	if err != nil {
		return "", err
	}
	out = abs
	if err = os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		return "", err
	}
	if err = os.Mkdir(out, 0755); err != nil {
		return "", err
	}
	for _, name := range []string{"artifacts", "trials", "logs", "structures", "validation"} {
		if err = os.Mkdir(filepath.Join(out, name), 0755); err != nil {
			return out, err
		}
	}
	if lock.ArchiveTools {
		if err := archiveTools(lock, executable, out); err != nil {
			return out, err
		}
	}
	lock.Options.requiredValidatorFeatures = make(map[string][]string)
	lock.Options.monitorCPUPartition = lock.RequireIsolatedCPUPartition
	artifactEncodings := map[string]bool{}
	for i, w := range lock.Workloads {
		src := w.Artifact
		if !filepath.IsAbs(src) {
			src = filepath.Join(artifactBase, src)
		}
		rel := filepath.Join("artifacts", w.SHA256+".wasm")
		dst := filepath.Join(out, rel)
		if _, e := os.Stat(dst); os.IsNotExist(e) {
			if e = CopyExclusive(src, dst); e != nil {
				return out, e
			}
			b, e := os.ReadFile(dst)
			if e != nil {
				return out, e
			}
			component := len(b) >= 8 && string(b[:8]) == "\x00asm\x0d\x00\x01\x00"
			artifactEncodings[w.SHA256] = component
			if component && (lock.Analyzer == nil || lock.Analyzer.AnalysisVersion != "artifact-structure-v1") {
				return out, fmt.Errorf("component admission requires an artifact-structure-v1 analyzer lock")
			}
			if !component {
				structure, e := corpus.Analyze(b)
				if e != nil {
					return out, e
				}
				if e = WriteJSON(filepath.Join(out, "structures", w.SHA256+".json"), structure); e != nil {
					return out, e
				}
			}
			if lock.Analyzer != nil {
				evidence, e := lock.Analyzer.analyze(ctx, dst, w.SHA256)
				if e != nil {
					return out, fmt.Errorf("workload %s admission: %w", w.ID, e)
				}
				if e = WriteJSON(filepath.Join(out, "validation", w.SHA256+".json"), evidence); e != nil {
					return out, e
				}
				if component {
					if e = WriteJSON(filepath.Join(out, "structures", w.SHA256+".json"), evidence); e != nil {
						return out, e
					}
				}
				lock.Options.requiredValidatorFeatures[w.SHA256] = requiredValidatorFeatures(evidence)
			}
		}
		if artifactEncodings[w.SHA256] != (w.ABI == "component") {
			return out, fmt.Errorf("workload %s ABI does not match core/component artifact encoding", w.ID)
		}
		lock.Workloads[i].Artifact = filepath.ToSlash(rel)
		if err := bundleCommandFiles(&lock.Workloads[i], artifactBase, out); err != nil {
			return out, err
		}
	}
	for i, r := range lock.Runtimes {
		lock.Runtimes[i].Description = nil
		lock.Runtimes[i].UnavailableReason = ""
		c, e := agent.StartIsolated(ctx, r.Command, filepath.Join(out, "logs", fmt.Sprintf("describe-%d.log", i)), lock.Options.Timeout, lock.Options.Resources)
		if e != nil {
			progress(fmt.Sprintf("%s unavailable: %v", r.ID, e))
			lock.Runtimes[i].Description = nil
			lock.Runtimes[i].UnavailableReason = e.Error()
			continue
		}
		resp, e := c.Call(protocol.Request{Method: "describe"})
		c.Close()
		if e != nil {
			progress(fmt.Sprintf("%s describe failed: %v", r.ID, e))
			lock.Runtimes[i].UnavailableReason = e.Error()
			continue
		}
		lock.Runtimes[i].Description = resp.Description
	}
	lb, _ := json.Marshal(lock)
	manifest := Manifest{Schema: 1, ID: filepath.Base(out), Created: time.Now().UTC(), Kind: "measurement", Lock: lock, LockSHA256: corpus.Hash(lb), Host: agent.IdentifyHost(), Publication: "local_exploratory"}
	manifest.CPUPartitionStart = partitionStart
	manifest.IRQAffinityStart = irqStart
	if baselineStart != nil {
		manifest.Host = baselineHost
		manifest.HostStartCheck = baselineStart
	}
	if lock.Options.Check {
		manifest.Kind = "correctness_only"
		manifest.Publication = "prohibited"
	}
	// Preflight is sacrificial and never supplies measurement samples.
	preflight := make([]Trial, len(lock.Workloads)*len(lock.Runtimes))
	var partitionSampleFailed atomic.Bool
	var progressMu sync.Mutex
	preflightOutcomes := make(map[string]int)
	preflightErr := parallel(ctx, lock.Options.Workers, len(preflight), func(index int) error {
		wi, ri := index/len(lock.Runtimes), index%len(lock.Runtimes)
		w, r := lock.Workloads[wi], lock.Runtimes[ri]
		checkScenario := "first-call"
		if w.SnapshotDensity != nil {
			checkScenario = protocol.SnapshotDensityScenario
		}
		if w.ProcessSnapshot != nil {
			checkScenario = "process-snapshot-restore"
		}
		if w.Continuation != nil {
			checkScenario = "continuation-resume"
		}
		if w.Checkpoint != nil {
			checkScenario = "checkpoint-restore"
		}
		if w.GuestDensity != nil {
			checkScenario = "guest-density"
		}
		if w.ABI == "component" && w.Oracle.Kind == "component_compile_only" {
			checkScenario = "compile"
		}
		if w.Density != nil && len(lock.Options.Scenarios) == 1 && lock.Options.Scenarios[0] == "density-cycle" {
			checkScenario = "density-cycle"
		}
		t := runTrial(ctx, out, lock.Options, r, w, checkScenario, -1, fmt.Sprintf("check-%d-%d", wi, ri))
		if t.PartitionMonitor != nil && t.PartitionMonitor.Status != "ready_at_samples" {
			partitionSampleFailed.Store(true)
		}
		preflight[index] = t
		progressMu.Lock()
		preflightOutcomes[t.Status]++
		progressMu.Unlock()
		if e := WriteJSON(filepath.Join(out, "trials", t.ID+".json"), t); e != nil {
			return e
		}
		return nil
	})
	if progress != nil && len(preflightOutcomes) > 0 {
		progress(fmt.Sprintf("Preflight outcomes: %s", formatOutcomeCounts(preflightOutcomes)))
	}
	if preflightErr != nil {
		return out, preflightErr
	}
	if !lock.Options.Check {
		rng := rand.New(rand.NewSource(lock.Options.Seed))
		counter := 0
		type measurement struct {
			id, scenario      string
			workload, runtime int
			block             int
		}
		measurementsByScenario := make(map[string][]measurement, len(lock.Options.Scenarios))
		for block := 0; block < lock.Options.Launches; block++ {
			for wi := range lock.Workloads {
				for _, scenario := range lock.Options.Scenarios {
					for _, ri := range rng.Perm(len(lock.Runtimes)) {
						id := fmt.Sprintf("trial-%06d", counter)
						counter++
						manifest.Order = append(manifest.Order, id)
						measurementsByScenario[scenario] = append(measurementsByScenario[scenario], measurement{id: id, scenario: scenario, workload: wi, runtime: ri, block: block})
					}
				}
			}
		}
		phases := make([][]measurement, 0, len(lock.Options.Scenarios))
		for _, scenario := range lock.Options.Scenarios {
			phases = append(phases, measurementsByScenario[scenario])
		}
		measurementOutcomes := make(map[string]int)
		measurementErr := parallelPhases(ctx, lock.Options.Workers, phases, func(job measurement) error {
			r, w := lock.Runtimes[job.runtime], lock.Workloads[job.workload]
			var t Trial
			check := preflight[job.workload*len(lock.Runtimes)+job.runtime]
			if check.Status != "ok" {
				status := "preflight_failed"
				if check.Status == "unsupported" || check.Status == "unavailable" {
					status = check.Status
				}
				t = Trial{ID: job.id, Runtime: r.ID, Workload: w.ID, Scenario: job.scenario, Profile: lock.Options.Profile, Block: job.block, Status: status, Reason: "sacrificial check " + check.ID + ": " + check.Reason}
			} else {
				t = runTrial(ctx, out, lock.Options, r, w, job.scenario, job.block, job.id)
			}
			if t.PartitionMonitor != nil && t.PartitionMonitor.Status != "ready_at_samples" {
				partitionSampleFailed.Store(true)
			}
			if e := WriteJSON(filepath.Join(out, "trials", job.id+".json"), t); e != nil {
				return e
			}
			progressMu.Lock()
			measurementOutcomes[t.Status]++
			progressMu.Unlock()
			return nil
		})
		if measurementErr != nil {
			return out, measurementErr
		}
		if progress != nil {
			progress(fmt.Sprintf("Measurement outcomes: %s", formatOutcomeCounts(measurementOutcomes)))
		}
	}
	if lock.RequireIRQAffinity {
		p := agent.ProbeIRQAffinity(lock.Options.Resources.CPUs)
		manifest.IRQAffinityEnd = &p
		if p.Err() != nil {
			manifest.Publication = "prohibited_irq_affinity_mismatch"
		}
	}
	if lock.RequireIsolatedCPUPartition {
		p := agent.ProbeCPUPartition(lock.Options.Resources.CgroupParent, lock.Options.Resources.CPUs)
		manifest.CPUPartitionEnd = &p
		if p.Err() != nil {
			manifest.Publication = "prohibited_cpu_partition_mismatch"
		} else if partitionSampleFailed.Load() {
			manifest.Publication = "prohibited_cpu_partition_sample_mismatch"
		}
	}
	if lock.HostPolicy != nil {
		end := agent.IdentifyHost()
		check := agent.CheckHostPolicy(lock.HostPolicy, end, "after_trials_before_seal")
		manifest.HostEnd = &end
		manifest.HostEndCheck = &check
		if check.Err() != nil {
			manifest.Publication = "prohibited_host_baseline_mismatch"
		}
	}
	if err = WriteJSON(filepath.Join(out, "manifest.json"), manifest); err != nil {
		return out, err
	}
	if err = Seal(out); err != nil {
		return out, err
	}
	if manifest.HostEndCheck != nil {
		if err := manifest.HostEndCheck.Err(); err != nil {
			return out, fmt.Errorf("%w; diagnostic bundle sealed at %s", err, out)
		}
	}
	if manifest.CPUPartitionEnd != nil {
		if err := manifest.CPUPartitionEnd.Err(); err != nil {
			return out, fmt.Errorf("%w; diagnostic bundle sealed at %s", err, out)
		}
	}
	if partitionSampleFailed.Load() {
		return out, fmt.Errorf("active CPU partition sample failed; diagnostic bundle sealed at %s", out)
	}
	if manifest.IRQAffinityEnd != nil {
		if err := manifest.IRQAffinityEnd.Err(); err != nil {
			return out, fmt.Errorf("%w; diagnostic bundle sealed at %s", err, out)
		}
	}
	return out, nil
}

func formatOutcomeCounts(counts map[string]int) string {
	statuses := make([]string, 0, len(counts))
	for status := range counts {
		statuses = append(statuses, status)
	}
	slices.Sort(statuses)
	parts := make([]string, 0, len(statuses))
	total := 0
	for _, status := range statuses {
		parts = append(parts, fmt.Sprintf("%s=%d", status, counts[status]))
		total += counts[status]
	}
	return fmt.Sprintf("%d total (%s)", total, strings.Join(parts, ", "))
}

func runTrial(ctx context.Context, root string, o Options, r Runtime, w protocol.Workload, scenario string, block int, id string) (t Trial) {
	t = Trial{ID: id, Runtime: r.ID, Workload: w.ID, Scenario: scenario, Profile: o.Profile, Block: block, Started: time.Now().UTC(), Status: "error", Log: filepath.ToSlash(filepath.Join("logs", id+".log"))}
	start := time.Now()
	defer func() { t.DurationNS = time.Since(start).Nanoseconds() }()
	if r.Description == nil {
		t.Status = "unavailable"
		t.Reason = "adapter description unavailable"
		if r.UnavailableReason != "" {
			t.Reason += ": " + r.UnavailableReason
		}
		return
	}
	if r.Description.Capabilities["requires_memory_profile"] && o.Profile != "memory" {
		t.Status, t.Reason = "unsupported", "instrumented allocator adapter requires a dedicated memory pass"
		return
	}
	if r.Description.Capabilities["requires_memory_profile"] {
		if err := protocol.ValidateRustAllocatorWorkload(w, scenario); err != nil {
			t.Status, t.Reason = "unsupported", err.Error()
			return
		}
	}
	if scenario == "sustained" && block >= 0 {
		request := trialRequest(o, w, scenario, block)
		if err := protocol.ValidateSustained(&protocol.Preparation{Workload: w, Profile: o.Profile}, &request); err != nil {
			t.Status, t.Reason = "unsupported", err.Error()
			return
		}
		if !r.Description.Capabilities["can_sustained_execution"] {
			t.Status, t.Reason = "unsupported", "retained-instance sustained execution capability not advertised"
			return
		}
		if request.SustainedPostCollection && !r.Description.Capabilities["can_sustained_post_collection"] {
			t.Status, t.Reason = "unsupported", "sustained post-collection capability not advertised"
			return
		}
	}
	if block >= 0 && r.Description.Capabilities["can_profile_tier_trajectory"] && o.Profile != "profiling" {
		t.Status, t.Reason = "unsupported", "tier-observation adapter requires a dedicated profiling pass"
		return
	}
	if supported, declared := r.Description.Capabilities["can_code_profile"]; o.Profile == "code" && declared && !supported {
		t.Status, t.Reason = "unsupported", "adapter explicitly declares native-code image collection unavailable"
		return
	}
	if w.Oracle.Float != nil && w.Oracle.Kind != "float_bits_v1" {
		t.Status, t.Reason = "unsupported", "float policy requires float_bits_v1 oracle"
		return
	}
	if w.GuestDensity != nil || scenario == "guest-density" || w.Oracle.Kind == "guest_density_v1" {
		request := trialRequest(o, w, scenario, block)
		profile := o.Profile
		if block < 0 && (profile == "counters" || profile == "profiling") {
			profile = "timing"
		}
		if err := protocol.ValidateGuestDensity(&protocol.Preparation{Workload: w, Profile: profile}, &request); err != nil {
			t.Status, t.Reason = "unsupported", err.Error()
			return
		}
		if !r.Description.Capabilities["can_guest_density"] {
			t.Status, t.Reason = "unsupported", "adapter does not advertise fixed-state fresh/restored guest density"
			return
		}
	}
	if w.Continuation != nil || protocol.IsContinuationScenario(scenario) || w.Oracle.Kind == "native_continuation_v1" {
		request := trialRequest(o, w, scenario, block)
		if err := protocol.ValidateContinuation(&protocol.Preparation{Workload: w, Profile: o.Profile}, &request); err != nil {
			t.Status, t.Reason = "unsupported", err.Error()
			return
		}
		if !r.Description.Capabilities["can_native_continuation"] || r.Description.Configuration["native_continuation_protocol"] != protocol.NativeContinuationMode || r.Description.Runtime != "wazero" || r.Description.Version != "1.12.0" || r.Description.Backend != "compiler" {
			t.Status, t.Reason = "unsupported", "adapter is not qualified for pinned native execution-stack continuations"
			return
		}
	}
	if w.SnapshotDensity != nil || scenario == protocol.SnapshotDensityScenario || w.Oracle.Kind == "linux_process_snapshot_density_v1" {
		request := trialRequest(o, w, scenario, block)
		if err := protocol.ValidateSnapshotDensityRequest(protocol.Preparation{Workload: w, Profile: o.Profile}, request); err != nil {
			t.Status, t.Reason = "unsupported", err.Error()
			return
		}
		if err := protocol.ValidateProcessSnapshotDescription(r.Description); err != nil {
			t.Status, t.Reason = "unsupported", err.Error()
			return
		}
		if runtime.GOOS != "linux" || !r.Description.Capabilities["can_inspect_linux_snapshot_density"] || !slices.Contains(r.Description.Scenarios, protocol.SnapshotDensityScenario) {
			t.Status, t.Reason = "unsupported", "native Linux restored density inspection not advertised"
			return
		}
	}
	if w.ProcessSnapshot != nil || protocol.IsProcessSnapshotScenario(scenario) || w.Oracle.Kind == "linux_process_snapshot_v1" || w.Reset == "fresh_process_snapshot_per_sample" {
		request := trialRequest(o, w, scenario, block)
		profile := o.Profile
		if block < 0 && profile == "memory" {
			profile = "timing"
		}
		var policyErr error
		if profile == "memory" {
			policyErr = protocol.ValidateSnapshotInspection(protocol.Preparation{Workload: w, Profile: profile}, request)
			if policyErr == nil && (r.Description == nil || !r.Description.Capabilities["can_inspect_linux_snapshot_lineage"]) {
				policyErr = fmt.Errorf("snapshot live memory inspection is not advertised")
			}
		} else {
			policyErr = protocol.ValidateProcessSnapshot(&protocol.Preparation{Workload: w, Profile: profile}, &request)
		}
		if err := policyErr; err != nil {
			t.Status, t.Reason = "unsupported", err.Error()
			return
		}
		if err := protocol.ValidateProcessSnapshotDescription(r.Description); err != nil {
			t.Status, t.Reason = "unsupported", err.Error()
			return
		}
		if runtime.GOOS != "linux" {
			t.Status, t.Reason = "unsupported", "process snapshots require native Linux"
			return
		}
	}
	if w.Checkpoint != nil || protocol.IsCheckpointScenario(scenario) || w.Oracle.Kind == "guest_checkpoint_v1" {
		request := trialRequest(o, w, scenario, block)
		profile := o.Profile
		if block < 0 && (profile == "counters" || profile == "profiling") {
			profile = "timing"
		}
		if err := protocol.ValidateCheckpoint(&protocol.Preparation{Workload: w, Profile: profile}, &request); err != nil {
			t.Status, t.Reason = "unsupported", err.Error()
			return
		}
		if !r.Description.Capabilities["can_guest_checkpoint"] {
			t.Status, t.Reason = "unsupported", "adapter does not advertise eager fixed-memory scalar guest checkpoints"
			return
		}
	}
	if w.Density != nil || scenario == "density" || scenario == "density-cycle" {
		if err := protocol.ValidateDensityWorkload(w); err != nil {
			t.Status, t.Reason = "unsupported", err.Error()
			return
		}
		if !r.Description.Capabilities["can_density"] || (block >= 0 && scenario != "density" && scenario != "density-cycle") {
			t.Status, t.Reason = "unsupported", "density requires advertised instance-group support and the density scenario"
			return
		}
		request := trialRequest(o, w, scenario, block)
		if supported, declared := r.Description.Capabilities["can_density_"+w.Density.Sharing]; declared && !supported {
			t.Status, t.Reason = "unsupported", "adapter does not expose density sharing policy: "+w.Density.Sharing
			return
		}
		validate := protocol.ValidateDensity
		if request.Scenario == "density-cycle" {
			if !r.Description.Capabilities["can_density_cycle"] {
				t.Status, t.Reason = "unsupported", "adapter does not expose retained-engine density cycles"
				return
			}
			validate = protocol.ValidateDensityCycle
		}
		if err := validate(&protocol.Preparation{Workload: w, Profile: o.Profile}, &request); err != nil {
			t.Status, t.Reason = "unsupported", err.Error()
			return
		}
	}
	if w.HostProfile == "js-string-builtins-v1" || w.HostProfile == "threads-defined-v1" {
		if !r.Description.Capabilities["can_host_profile_"+w.HostProfile] {
			t.Status, t.Reason = "unsupported", "adapter does not advertise host profile: "+w.HostProfile
			return
		}
	}
	if !slices.Contains(r.Description.ABIs, w.ABI) {
		t.Status = "unsupported"
		t.Reason = "ABI not advertised"
		return
	}
	if w.ABI == "component" {
		if w.HostProfile == protocol.ComponentU64Policy {
			if err := protocol.ValidateComponentU64Workload(w); err != nil {
				t.Status, t.Reason = "unsupported", err.Error()
				return
			}
			memory := o.Profile == "memory" && r.Description.Capabilities["can_component_u64_memory_v1"]
			if !r.Description.Capabilities["can_component_u64_calls_v1"] || !slices.Contains([]string{"compile", "instantiate", "first-call"}, scenario) || (o.Profile != "timing" && !memory) || o.Operations != 1 || (o.PhaseBarriers && !memory) {
				t.Status, t.Reason = "unsupported", "component-u64-v1 requires advertised single-operation timing/memory lifecycle support; barriers require memory capability"
				return
			}
		} else if w.Command != nil {
			if err := protocol.ValidateCommand(w); err != nil {
				t.Status, t.Reason = "unsupported", err.Error()
				return
			}
			legacy := scenario == "first-call" && o.Profile == "timing" && !o.PhaseBarriers
			lifecycle := r.Description.Capabilities["can_component_command_lifecycle"] && slices.Contains([]string{"compile", "instantiate", "first-call"}, scenario) && slices.Contains([]string{"timing", "memory"}, o.Profile) && (!o.PhaseBarriers || (o.Profile == "memory" && r.Description.Capabilities["can_component_command_phases"]))
			if (!legacy && !lifecycle) || o.Operations != 1 || !r.Description.Capabilities["can_run_component_commands"] {
				t.Status, t.Reason = "unsupported", "WASI Preview 2 command requires an advertised single-operation lifecycle/profile/barrier contract"
				return
			}
		} else {
			if err := protocol.ValidateComponentCompileWorkload(w); err != nil {
				t.Status, t.Reason = "unsupported", err.Error()
				return
			}
			if scenario != "compile" || o.Profile != "timing" || o.Operations != 1 || o.PhaseBarriers || !r.Description.Capabilities["can_compile_components"] {
				t.Status, t.Reason = "unsupported", "this adapter supports single-operation Component Model compilation only, with no phase barriers and minimally instrumented timing"
				return
			}
		}
	}
	if w.ABI == "wasi-reactor" {
		req := trialRequest(o, w, scenario, block)
		profile := o.Profile
		if block < 0 {
			profile = "timing"
		}
		if err := protocol.ValidateReactorRun(&protocol.Preparation{Workload: w, Profile: profile}, &req); err != nil {
			t.Status, t.Reason = "unsupported", err.Error()
			return
		}
		if !r.Description.Capabilities["can_run_reactors"] {
			t.Status, t.Reason = "unsupported", "WASI reactor capability not advertised"
			return
		}
	}
	if reason := incompatibleValidatorFeatures(r.Description.ValidatorFeatures, o.requiredValidatorFeatures[w.SHA256]); reason != "" {
		t.Status, t.Reason = "unsupported", reason
		return
	}
	if w.SnapshotDensity == nil && ((w.Oracle.Kind != "exact_u64" && w.Oracle.Kind != "float_bits_v1" && w.Oracle.Kind != "exact_vectors" && w.Oracle.Kind != "exact_command" && w.Oracle.Kind != "expected_trap" && w.Oracle.Kind != "component_compile_only" && w.Oracle.Kind != "guest_checkpoint_v1" && w.Oracle.Kind != "guest_density_v1" && w.Oracle.Kind != "native_continuation_v1" && w.Oracle.Kind != "linux_process_snapshot_v1") || (w.Reset != "stateless" && w.Reset != "fresh_instance_per_sample" && w.Reset != "fresh_process_snapshot_per_sample")) {
		t.Status = "unsupported"
		t.Reason = "workload requires an unsupported oracle or reset policy"
		if w.UnsupportedReason != "" {
			t.Reason = w.UnsupportedReason
		}
		return
	}
	if w.Oracle.Kind == "float_bits_v1" {
		if err := protocol.ValidateFloatWorkload(w); err != nil {
			t.Status, t.Reason = "unsupported", err.Error()
			return
		}
		floatPhasesUnsupported := block >= 0 && o.PhaseBarriers && (!r.Description.Capabilities["can_float_phases"] || (!slices.Contains([]string{"compile", "instantiate", "teardown"}, scenario) && !r.Description.Capabilities["can_rust_allocator_phase_boundaries"]))
		if !r.Description.Capabilities["can_verify_float_bits_v1"] || (scenario == "trajectory" && !r.Description.Capabilities["can_float_trajectory"]) || (scenario == "teardown" && !r.Description.Capabilities["can_float_teardown"]) || floatPhasesUnsupported || !slices.Contains([]string{"timing", "memory"}, o.Profile) || !slices.Contains([]string{"compile", "instantiate", "first-call", "steady", "cold-process", "trajectory", "teardown"}, scenario) {
			t.Status, t.Reason = "unsupported", "float oracle requires advertised support and a supported timing/memory scenario and barrier contract"
			return
		}
	}
	vectorPhasesUnsupported := o.PhaseBarriers && (!slices.Contains([]string{"compile", "instantiate", "first-call", "teardown"}, scenario) || !r.Description.Capabilities["can_vector_"+scenario+"_phases"])
	if scenario == protocol.HarnessCalibrationScenario {
		request := protocol.RunRequest{Scenario: scenario, Samples: o.Samples, Operations: o.Operations, Warmup: o.Warmup, PhaseBarriers: o.PhaseBarriers}
		if err := protocol.ValidateHarnessCalibration(&protocol.Preparation{Workload: w, Profile: o.Profile}, &request); err != nil {
			t.Status, t.Reason = "unsupported", err.Error()
			return
		}
	}
	if scenario == "trajectory" {
		request := protocol.RunRequest{Scenario: scenario, Samples: o.Samples, Warmup: o.Warmup, Operations: o.Operations, PhaseBarriers: o.PhaseBarriers}
		validate := protocol.ValidateTrajectory
		if r.Description.Capabilities["can_profile_tier_trajectory"] && o.Profile == "profiling" {
			validate = protocol.ValidateTierRun
		}
		if r.Description.Capabilities["can_trace_v8_wasm_events"] && o.Profile == "profiling" {
			validate = protocol.ValidateEngineTraceRun
		}
		if err := validate(&protocol.Preparation{Workload: w, Profile: o.Profile}, &request); err != nil {
			t.Status, t.Reason = "unsupported", err.Error()
			return
		}
	}
	if w.Oracle.Kind == "expected_trap" && (!r.Description.Capabilities["can_verify_invocation_traps"] || !slices.Contains([]string{"first-call", "steady", "cold-process"}, scenario) || !slices.Contains([]string{"timing", "memory"}, o.Profile) || (o.Profile == "memory" && !r.Description.Capabilities["can_measure_invocation_traps"]) || (block >= 0 && o.PhaseBarriers)) {
		t.Status, t.Reason = "unsupported", "invocation-trap oracle requires advertised profile support without phase barriers"
		return
	}
	if scenario == "app-init" {
		if w.Initialize == "" {
			t.Status, t.Reason = "not_applicable", "workload has no explicit initializer"
			return
		}
		if err := protocol.ValidateAppInit(w); err != nil {
			t.Status, t.Reason = "unsupported", err.Error()
			return
		}
		if block >= 0 && !slices.Contains([]string{"timing", "memory"}, o.Profile) {
			t.Status, t.Reason = "unsupported", "app-init supports timing and memory profiles"
			return
		}
	}
	if w.Command != nil || w.Oracle.Kind == "exact_command" {
		if err := protocol.ValidateCommand(w); err != nil {
			t.Status = "unsupported"
			t.Reason = err.Error()
			return
		}
	}
	codeCompile := o.Profile == "code" && scenario == "compile" && (r.Description.Capabilities["can_export_native_code"] || r.Description.Capabilities["can_measure_native_code_size"])
	commandPhasesUnsupported := o.PhaseBarriers && (!slices.Contains([]string{"compile", "instantiate", "first-call", "teardown"}, scenario) || !r.Description.Capabilities["can_command_"+scenario+"_phases"])
	canRunCommand := r.Description.Capabilities["can_run_commands"]
	if w.ABI == "component" {
		canRunCommand = r.Description.Capabilities["can_run_component_commands"]
		commandPhasesUnsupported = o.PhaseBarriers && (!slices.Contains([]string{"compile", "instantiate", "first-call"}, scenario) || !r.Description.Capabilities["can_component_command_phases"])
	}
	if w.Oracle.Kind == "exact_command" && (!canRunCommand || !slices.Contains([]string{"compile", "instantiate", "first-call", "steady", "cold-process", "teardown"}, scenario) || (block >= 0 && (commandPhasesUnsupported || (!slices.Contains([]string{"timing", "memory"}, o.Profile) && !codeCompile)))) {
		t.Status = "unsupported"
		t.Reason = "command scenario/profile not supported"
		return
	}
	if w.Oracle.Kind == "exact_vectors" && (!r.Description.Capabilities["can_run_vectors"] || !slices.Contains([]string{"compile", "instantiate", "first-call", "steady", "cold-process", "teardown"}, scenario) || (block >= 0 && ((!slices.Contains([]string{"timing", "memory"}, o.Profile) && !codeCompile) || vectorPhasesUnsupported))) {
		t.Status = "unsupported"
		t.Reason = "ordered vector timing capability/profile not supported"
		return
	}
	if scenario != "cold-process" && !slices.Contains(r.Description.Scenarios, scenario) {
		t.Status = "unsupported"
		t.Reason = "scenario not advertised"
		return
	}
	if scenario == "compile-materialized" && block >= 0 {
		request := trialRequest(o, w, scenario, block)
		if err := protocol.ValidateMaterializedRun(&protocol.Preparation{Workload: w, Profile: o.Profile}, &request); err != nil {
			t.Status, t.Reason = "unsupported", err.Error()
			return
		}
		if !r.Description.Capabilities["can_compile_materialized"] {
			t.Status, t.Reason = "unsupported", "materialized compile capability not advertised"
			return
		}
	}
	if scenario == "code-lifetime" && block >= 0 {
		request := trialRequest(o, w, scenario, block)
		if err := protocol.ValidateCodeLifetimeRun(&protocol.Preparation{Workload: w, Profile: o.Profile}, &request); err != nil {
			t.Status, t.Reason = "unsupported", err.Error()
			return
		}
		if !qualifiedCodeLifetimeRuntime(r.Description) {
			t.Status, t.Reason = "unsupported", "qualified code lifetime capability/configuration not advertised"
			return
		}
	}
	if block >= 0 && o.Profile == "counters" {
		req := trialRequest(o, w, scenario, block)
		if err := protocol.ValidateCounterRun(&protocol.Preparation{Profile: "counters", Workload: w}, &req); err != nil {
			t.Status, t.Reason = "unsupported", err.Error()
			return
		}
		if !r.Description.Capabilities["can_counter_"+scenario] {
			t.Status, t.Reason = "unsupported", "counter phase capability not advertised"
			return
		}
	}
	if block >= 0 && o.Profile == "profiling" {
		req := trialRequest(o, w, scenario, block)
		validate := protocol.ValidateProfilingRun
		if r.Description.Capabilities["can_profile_tier_trajectory"] {
			validate = protocol.ValidateTierRun
		}
		if r.Description.Capabilities["can_trace_v8_wasm_events"] {
			validate = protocol.ValidateEngineTraceRun
		}
		if err := validate(&protocol.Preparation{Profile: "profiling", Workload: w}, &req); err != nil {
			t.Status, t.Reason = "unsupported", err.Error()
			return
		}
		if !r.Description.Capabilities["can_profile_go_cpu_steady"] && !r.Description.Capabilities["can_profile_v8_cpu_steady"] && !r.Description.Capabilities["can_profile_tier_trajectory"] {
			t.Status, t.Reason = "unsupported", "CPU profiling capability not advertised"
			return
		}
	}
	if block >= 0 && o.PhaseBarriers && !((w.ProcessSnapshot != nil || w.SnapshotDensity != nil) && o.Profile == "memory") && !slices.Contains(r.Description.PhaseBarrierScenarios, scenario) {
		t.Status = "unsupported"
		t.Reason = "phase barriers not advertised for this scenario"
		return
	}
	trialCtx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()
	launchStart := time.Now()
	c, e := agent.StartIsolated(trialCtx, r.Command, filepath.Join(root, t.Log), o.Timeout, o.Resources)
	if e != nil {
		t.Reason = e.Error()
		return
	}
	t.Isolation = c.Isolation()
	defer func() {
		t.Isolation.FinalVerification = c.VerifyResources()
		if v := t.Isolation.FinalVerification; v != nil {
			if err := v.Err(); err != nil {
				if t.Status == "ok" {
					t.Status = "error"
				}
				t.Reason += "; " + err.Error()
			}
		}
		observations, oom := c.ResourceObservations()
		if o.Profile == "memory" && !(w.ProcessSnapshot != nil && block < 0) {
			t.Observations = append(t.Observations, observations...)
		}
		if oom {
			t.Status = "out_of_memory"
			t.Reason = "adapter cgroup recorded an OOM kill"
		}
		if err := c.Close(); err != nil {
			if t.Status == "ok" {
				t.Status = "error"
			}
			t.Reason += "; cgroup cleanup: " + err.Error()
		}
		if block >= 0 && (o.Profile == "memory" || o.TimingPeakRSS) && w.ProcessSnapshot == nil && w.SnapshotDensity == nil {
			peak := c.PeakRSSObservation(scenario)
			peak.Profile = o.Profile
			t.Observations = append(t.Observations, peak)
		}
	}()
	if o.monitorCPUPartition {
		monitor := agent.StartPartitionMonitor(o.Resources.CgroupParent, t.Isolation.Path, o.Resources.CPUs, c.PID())
		defer func() { t.PartitionMonitor = monitor.Stop() }()
	}
	defer func() {
		if trialCtx.Err() == context.DeadlineExceeded {
			t.Status = "timeout"
			t.Reason = "adapter exceeded trial deadline"
		}
	}()
	prepareProfile := o.Profile
	if block < 0 && (o.Profile == "counters" || o.Profile == "profiling" || o.Profile == "code" || (w.ProcessSnapshot != nil && o.Profile == "memory")) {
		prepareProfile = "timing"
	}
	resp, e := c.Call(protocol.Request{Method: "prepare", Prepare: &protocol.Preparation{Artifact: filepath.Join(root, w.Artifact), ArtifactSHA256: w.SHA256, Workload: resolveCommandFiles(w, root), Profile: prepareProfile}})
	if e != nil {
		if !setAdapterFailure(&t, resp, e) {
			t.Reason = e.Error()
		}
		return
	}
	if w.SnapshotDensity != nil && o.Profile == "memory" {
		request := trialRequest(o, w, scenario, block)
		t.SnapshotDensity, e = collectSnapshotDensity(c, w, request, r.Description.Backend)
		if e != nil {
			t.Reason = e.Error()
			return
		}
		t.Status = "ok"
		return
	}
	if block >= 0 && w.ProcessSnapshot != nil && o.Profile == "memory" {
		request := trialRequest(o, w, scenario, block)
		t.SnapshotMemory, e = collectSnapshotMemory(c, w, request)
		if e != nil {
			t.Reason = e.Error()
			return
		}
		t.Status = "ok"
		return
	}
	if block >= 0 && o.Profile == "code" {
		message := protocol.Request{Method: "inspect"}
		if scenario == "compile-materialized" || scenario == "code-lifetime" {
			request := trialRequest(o, w, scenario, block)
			message = protocol.Request{Method: "run", Run: &request}
		}
		resp, e := c.Call(message)
		if e != nil {
			if !setAdapterFailure(&t, resp, e) {
				t.Reason = e.Error()
			}
			return
		}
		t.Observations = resp.Diagnostics
		if resp.CodeImage != nil {
			if err := resp.CodeImage.Validate(w.SHA256); err != nil {
				t.Reason = err.Error()
				return
			}
			t.CodeImage = resp.CodeImage
		}
		if scenario == "code-lifetime" {
			if len(resp.Samples) != 0 || resp.CodeLifetime == nil {
				t.Reason = "code lifetime omitted event evidence or returned timing samples"
				t.CodeImage = nil
				return
			}
			if resp.CodeLifetime.Backend != r.Description.Backend || resp.CodeLifetime.CollectorVersion != r.Description.Version || resp.CodeLifetime.PageSize != uint64(os.Getpagesize()) {
				t.Reason = "code lifetime differs from qualified runtime or host page size"
				t.CodeImage = nil
				return
			}
			if err := validateCodeLifetimeResult(root, w, resp.CodeLifetime, t.CodeImage); err != nil {
				t.Reason = err.Error()
				t.CodeImage = nil
				return
			}
			t.CodeLifetime = resp.CodeLifetime
		} else if resp.CodeLifetime != nil {
			t.Reason = "unexpected code lifetime evidence outside diagnostic scenario"
			t.CodeImage = nil
			return
		}
		if scenario == "compile-materialized" {
			request := trialRequest(o, w, scenario, block)
			if t.CodeImage == nil || t.CodeImage.Version != 3 || t.CodeImage.Materialization.CollectorVersion != r.Description.Version || t.CodeImage.Backend != r.Description.Backend {
				t.CodeImage = nil
				t.Reason = "adapter omitted or mismatched materialized compile image"
				return
			}
			if err := validateSampleSequence(request, resp.Samples); err != nil {
				t.CodeImage = nil
				t.AdapterSamples = resp.Samples
				t.Reason = err.Error()
				return
			}
			if len(resp.Samples) != 1 || !resp.Samples[0].Verified || resp.Samples[0].ElapsedNS != t.CodeImage.Materialization.CompileElapsedNS {
				t.CodeImage = nil
				t.AdapterSamples = resp.Samples
				t.Reason = "materialized compile timer differs from image evidence"
				return
			}
			t.Samples = resp.Samples
			if err := validateMaterializationInput(root, w.SHA256, t.CodeImage.Materialization); err != nil {
				t.CodeImage = nil
				t.AdapterSamples = resp.Samples
				t.Samples = nil
				t.Reason = err.Error()
				return
			}
		}
		t.Status = "ok"
		return
	}
	request := trialRequest(o, w, scenario, block)
	var sampler *collectors.Sampler
	if o.Profile == "memory" && block >= 0 {
		t.Observations = append(t.Observations, collectors.Snapshot(c.PID(), scenario+"/before_batch")...)
		sampler = collectors.StartSampler(c.PID(), scenario, 2*time.Millisecond)
	}
	var phases *phaseTracker
	var handler func(protocol.PhaseEvent) error
	if request.PhaseBarriers && o.Profile != "counters" {
		phases = &phaseTracker{scenario: scenario, allocator: r.Description.Capabilities["can_rust_allocator_phase_boundaries"], samples: request.Samples, collect: func(stage string) []protocol.Observation {
			out := collectors.Snapshot(c.PID(), scenario+"/"+stage)
			return append(out, c.PhaseMemory(stage)...)
		}}
		handler = phases.handle
	}
	if o.Profile == "counters" && block >= 0 {
		resp, t.CounterPhases, e = c.CallCounterPhases(protocol.Request{Method: "run", Run: &request})
	} else {
		resp, e = c.CallPhased(protocol.Request{Method: "run", Run: &request}, handler)
	}
	if phases != nil {
		t.PhaseEvents = phases.records
	}
	if resp.EngineTrace != nil {
		candidate := t
		candidate.EngineTrace = resp.EngineTrace
		candidate.Samples = resp.Samples
		if err := validateEngineTraceTrial(w, r.Description, o, candidate); err != nil {
			t.AdapterSamples = resp.Samples
			t.Reason = err.Error()
			return
		}
		t.EngineTrace = resp.EngineTrace
	} else if e == nil && block >= 0 && o.Profile == "profiling" && r.Description.Capabilities["can_trace_v8_wasm_events"] {
		t.AdapterSamples = resp.Samples
		t.Reason = "adapter omitted engine trace outcome"
		return
	}
	if resp.CPUProfile != nil {
		if o.Profile != "profiling" || block < 0 {
			t.Reason = "unexpected CPU profile outside profiling pass"
			return
		}
		if err := resp.CPUProfile.Validate(w.SHA256); err != nil {
			t.Reason = err.Error()
			return
		}
		t.CPUProfile = resp.CPUProfile
	}
	if e == nil && o.Profile == "profiling" && block >= 0 && t.CPUProfile == nil && !r.Description.Capabilities["can_profile_tier_trajectory"] {
		t.Reason = "adapter omitted CPU profile outcome"
		return
	}
	if e == nil && phases != nil {
		e = phases.attach(resp.Samples)
	}
	coldElapsed := time.Since(launchStart).Nanoseconds()
	if sampler != nil {
		t.Observations = append(t.Observations, sampler.Stop()...)
		t.Observations = append(t.Observations, collectors.Snapshot(c.PID(), scenario+"/after_batch")...)
	}
	if e != nil {
		if setAdapterFailure(&t, resp, e) {
			return
		}
		t.Reason = e.Error()
		t.AdapterSamples = resp.Samples
		if strings.Contains(t.Reason, "incorrect result") {
			t.Status = "incorrect_result"
		}
		if block >= 0 && o.Profile == "profiling" && r.Description.Capabilities["can_profile_tier_trajectory"] && resp.Status == "error" && len(resp.Samples) > 0 {
			last := resp.Samples[len(resp.Samples)-1].TierWindow
			if last != nil && (last.InvocationOutcome == "oracle_mismatch" || last.InvocationOutcome == "guest_trap") {
				candidate := t
				candidate.Samples = resp.Samples
				if err := validateTierTrial(w, r.Description.Version, true, o, candidate); err == nil {
					t.Samples = resp.Samples
					t.AdapterSamples = nil
				}
			}
		}
		return
	}
	if err := validateSampleSequence(request, resp.Samples); err != nil {
		t.AdapterSamples = resp.Samples
		t.Reason = err.Error()
		return
	}
	if w.ProcessSnapshot != nil {
		if err := protocol.VerifyProcessSnapshotSequence(w, request, resp.Samples); err != nil {
			t.AdapterSamples = resp.Samples
			t.Status, t.Reason = "incorrect_result", err.Error()
			return
		}
		for _, sample := range resp.Samples {
			if sample.ProcessSnapshotResult.Source.PID != uint32(c.PID()) {
				t.AdapterSamples = resp.Samples
				t.Status, t.Reason = "incorrect_result", "process snapshot source is not launched adapter process"
				return
			}
		}
	}
	var tierPreviousEnd int64
	for _, s := range resp.Samples {
		if r.Description.Capabilities["requires_memory_profile"] {
			if err := protocol.ValidateRustAllocatorSample(s, scenario); err != nil {
				t.AdapterSamples, t.Reason = resp.Samples, err.Error()
				return
			}
			if r.Description.Capabilities["can_rust_allocator_release_windows"] {
				if err := protocol.ValidateRustAllocatorRelease(s, scenario, s.Index+1 == len(resp.Samples)); err != nil {
					t.AdapterSamples, t.Reason = resp.Samples, err.Error()
					return
				}
			}
		}
		if s.TierWindow != nil {
			if o.Profile != "profiling" || block < 0 || !r.Description.Capabilities["can_profile_tier_trajectory"] || s.TierWindow.Validate(w, s) != nil || s.TierWindow.CollectorVersion != r.Description.Version || s.TierWindow.Before.StartNS < tierPreviousEnd {
				t.AdapterSamples = resp.Samples
				t.Reason = "invalid tier diagnostic outside declared profiling trajectory"
				return
			}
			tierPreviousEnd = s.TierWindow.After.EndNS
		} else if block >= 0 && o.Profile == "profiling" && r.Description.Capabilities["can_profile_tier_trajectory"] {
			t.AdapterSamples = resp.Samples
			t.Reason = "adapter omitted tier diagnostic reading"
			return
		}
		if w.GuestDensity != nil {
			if err := protocol.VerifyGuestDensitySample(w, s); err != nil {
				t.AdapterSamples = resp.Samples
				t.Status, t.Reason = "incorrect_result", err.Error()
				return
			}
		}
		if w.Checkpoint != nil {
			if err := protocol.VerifyCheckpointSample(w, request.Scenario, s); err != nil {
				t.AdapterSamples = resp.Samples
				t.Status, t.Reason = "incorrect_result", err.Error()
				return
			}
		}
		if w.Continuation != nil {
			if err := protocol.VerifyContinuationSample(w, request.Scenario, s); err != nil {
				t.AdapterSamples = resp.Samples
				t.Status, t.Reason = "incorrect_result", err.Error()
				return
			}
		}
		if w.ProcessSnapshot != nil {
			if err := protocol.VerifyProcessSnapshotSample(w, request.Scenario, s); err != nil {
				t.AdapterSamples = resp.Samples
				t.Status, t.Reason = "incorrect_result", err.Error()
				return
			}
		} else if s.ProcessSnapshotResult != nil {
			t.AdapterSamples = resp.Samples
			t.Status, t.Reason = "incorrect_result", "process snapshot proof outside its workload contract"
			return
		}
		if w.Oracle.Kind == "expected_trap" && (protocol.VerifyTrap(w, s.TrapResult) != nil || s.Operations != 1 || s.SampleType != "individual_operation" || len(s.Result) != 0) {
			t.AdapterSamples = resp.Samples
			t.Status, t.Reason = "incorrect_result", "missing or incorrect invocation-trap evidence"
			return
		}
		if w.Command != nil && (s.CommandResult == nil || w.Command.Verify(*s.CommandResult) != nil) {
			t.AdapterSamples = resp.Samples
			t.Status = "incorrect_result"
			t.Reason = "missing or incorrect command exit/output evidence"
			return
		}
		if !s.Verified || s.Operations < 1 || s.ElapsedNS < 0 {
			t.AdapterSamples = resp.Samples
			t.Status = "incorrect_result"
			t.Reason = "invalid or unverified measurement"
			return
		}
	}
	t.Samples = resp.Samples
	if scenario == "sustained" && block >= 0 {
		total, err := protocol.VerifySustainedSequence(w, request, resp.Samples)
		if err != nil {
			t.Status, t.Reason = "incorrect_result", err.Error()
			return
		}
		if total < request.SustainedDurationNS {
			t.Status, t.Reason = "duration_budget_not_met", fmt.Sprintf("measured non-warmup API time %d ns below locked target %d ns; fixed sample budget exhausted; raw diagnostics retained", total, request.SustainedDurationNS)
			return
		}
	}
	if scenario == "cold-process" {
		t.AdapterSamples = resp.Samples
		t.Samples = []protocol.Sample{{Index: 0, Operations: 1, ElapsedNS: coldElapsed, Verified: true, SampleType: "individual_process", Result: resp.Samples[0].Result, TrapResult: resp.Samples[0].TrapResult}}
	}
	t.Status = "ok"
	return
}

func setAdapterFailure(t *Trial, resp protocol.Response, err error) bool {
	if err == nil || len(resp.Samples) != 0 || (resp.Status != "unsupported" && resp.Status != "unavailable") {
		return false
	}
	t.Status = resp.Status
	t.Reason = resp.Reason
	return true
}

func trialRequest(o Options, w protocol.Workload, scenario string, block int) protocol.RunRequest {
	warmup := 0
	if scenario == "steady" || scenario == "trajectory" || scenario == "sustained" {
		warmup = o.Warmup
	}
	r := protocol.RunRequest{Scenario: scenario, Samples: o.Samples, Operations: o.Operations, Warmup: warmup, PhaseBarriers: o.PhaseBarriers && block >= 0}
	if block >= 0 && o.Profile == "timing" {
		if samples, ok := o.ScenarioSamples[scenario]; ok {
			r.Samples = samples
		} else if samples, ok := o.ScenarioSamples["*"]; ok {
			r.Samples = samples
		}
	}
	if scenario == "sustained" && block >= 0 {
		r.SustainedDurationNS = int64(o.SustainedDuration)
		r.SustainedPostCollection = o.SustainedPostCollection
	}
	if block < 0 || scenario == "cold-process" {
		r = protocol.RunRequest{Scenario: "first-call", Samples: 1, Operations: 1}
		// Only sacrificial admission repeats on two fresh instances. The measured
		// cold process stops at its first correct complete workload sequence.
		if block < 0 && (w.Oracle.Kind == "exact_vectors" || w.Oracle.Kind == "exact_command" || w.Oracle.Kind == "expected_trap") {
			r.Samples = 2
		}
	}
	if block < 0 && w.ABI == "component" && w.Oracle.Kind == "component_compile_only" {
		r = protocol.RunRequest{Scenario: "compile", Samples: 1, Operations: 1}
	}
	if w.Density != nil {
		r.Scenario, r.Operations, r.Warmup = "density", 1, 0
		if scenario == "density-cycle" {
			r.Scenario = "density-cycle"
			if block < 0 {
				r.Samples = 2
			}
		}
	}
	if block < 0 && w.Checkpoint != nil {
		r = protocol.RunRequest{Scenario: "checkpoint-restore", Samples: 2, Operations: 1}
	}
	if block < 0 && w.Continuation != nil {
		r = protocol.RunRequest{Scenario: "continuation-resume", Samples: 2, Operations: 1}
	}
	if block < 0 && w.ProcessSnapshot != nil {
		r = protocol.RunRequest{Scenario: "process-snapshot-restore", Samples: 2, Operations: 1}
	}
	if block < 0 && w.GuestDensity != nil {
		r = protocol.RunRequest{Scenario: "guest-density", Samples: 2, Operations: 1}
	}
	if block < 0 && w.SnapshotDensity != nil {
		r = protocol.RunRequest{Scenario: protocol.SnapshotDensityScenario, Samples: 2, Operations: 1, PhaseBarriers: o.PhaseBarriers}
	}
	return r
}

func validateSampleSequence(r protocol.RunRequest, samples []protocol.Sample) error {
	if r.Scenario == protocol.HarnessCalibrationScenario {
		kind := "batch_average"
		if r.Operations == 1 {
			kind = "individual_operation"
		}
		for _, s := range samples {
			if !s.Verified || s.Operations != r.Operations || s.SampleType != kind || len(s.Result) != 1 || s.Result[0] != uint64(r.Operations) || len(s.Observations) != 0 {
				return fmt.Errorf("invalid empty-harness calibration sample: requires exact iterations, verified bookkeeping and no diagnostic instrumentation")
			}
		}
	}
	warmup := 0
	if r.Scenario == "steady" || r.Scenario == "trajectory" || r.Scenario == "sustained" {
		warmup = r.Warmup
	}
	if len(samples) != r.Samples+warmup {
		return fmt.Errorf("adapter sample count mismatch: got %d want %d", len(samples), r.Samples+warmup)
	}
	for i, s := range samples {
		if (r.Scenario == "density" || r.Scenario == "density-cycle") && (s.Operations != 1 || s.SampleType != "individual_operation") {
			return fmt.Errorf("density must measure one simultaneous instance group per sample")
		}
		if r.Scenario == "trajectory" && (s.Operations != 1 || s.SampleType != "individual_operation") {
			return fmt.Errorf("trajectory must measure sequential individual calls")
		}
		if r.Scenario == "teardown" && (s.Operations != 1 || s.SampleType != "individual_operation") {
			return fmt.Errorf("teardown must measure one release per fresh sample")
		}
		if r.Scenario == "app-init" && (s.Operations != 1 || s.SampleType != "individual_operation") {
			return fmt.Errorf("app-init must measure one initialization call per fresh sample")
		}
		if s.Index != i || s.Warmup != (i < warmup) {
			return fmt.Errorf("adapter sample ordering/warmup mismatch at %d", i)
		}
	}
	return nil
}
