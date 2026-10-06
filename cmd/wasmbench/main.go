package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/analysis"
	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/metrics"
	"github.com/wasmbench/wasmbench/publish"
	"github.com/wasmbench/wasmbench/sourcebuild"
	"github.com/wasmbench/wasmbench/storage"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "wasmbench:", err)
		os.Exit(1)
	}
}
func output(v any) error              { e := json.NewEncoder(os.Stdout); e.SetIndent("", "  "); return e.Encode(v) }
func flags(name string) *flag.FlagSet { return flag.NewFlagSet(name, flag.ContinueOnError) }
func run(ctx context.Context, args []string) error {
	if err := metrics.Validate(); err != nil {
		return fmt.Errorf("metric/scenario registry: %w", err)
	}
	if len(args) == 0 {
		usage()
		return nil
	}
	root, e := os.Getwd()
	if e != nil {
		return e
	}
	switch args[0] {
	case "verify-stack-report":
		f := flags(args[0])
		report := f.String("report", "", "sealed stack report directory")
		if e = f.Parse(args[1:]); e != nil {
			return e
		}
		if *report == "" || f.NArg() != 0 {
			return fmt.Errorf("verify-stack-report requires --report")
		}
		return publish.VerifyStackReport(*report)
	case "publication-check":
		return publicationCheck(args[1:])
	case "qualification-draft", "qualification-sign", "qualification-public-key":
		return qualificationCommand(args[0], args[1:])
	case "host-policy":
		return captureHostPolicy(args[1:])
	case "pilot-plan":
		return pilotPlan(args[1:])
	case "pilot-run":
		return pilotRun(ctx, args[1:])
	case "aggregate-set", "aggregate", "aggregate-report":
		f := flags(args[0])
		path := f.String("run", "", "verified timing measurement bundle")
		setPath := f.String("set", "", "fixed versioned aggregate set JSON")
		out := f.String("out", "", "new aggregate set JSON file or portable aggregate-report directory")
		id := f.String("id", "", "versioned set identity (aggregate-set)")
		scenario := f.String("scenario", "steady", "single scenario (aggregate-set)")
		baseline := f.String("baseline", "", "baseline configuration (aggregate)")
		candidate := f.String("candidate", "", "candidate configuration (aggregate)")
		if e = f.Parse(args[1:]); e != nil {
			return e
		}
		if *path == "" || f.NArg() != 0 {
			return fmt.Errorf("--run required; no positional arguments accepted")
		}
		b, err := experiment.Load(*path)
		if err != nil {
			return err
		}
		if args[0] == "aggregate-set" {
			if *out != "" {
				return publish.ExportAggregateSet(*path, *id, *scenario, *out)
			}
			set, err := analysis.NewAggregateSet(b, *id, *scenario)
			if err != nil {
				return err
			}
			return output(set)
		}
		if *setPath == "" {
			return fmt.Errorf("--set required")
		}
		file, err := os.Open(*setPath)
		if err != nil {
			return err
		}
		defer file.Close()
		var set analysis.AggregateSet
		decoder := json.NewDecoder(file)
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&set); err != nil {
			return err
		}
		var extra any
		if err = decoder.Decode(&extra); err != io.EOF {
			return fmt.Errorf("aggregate set must contain exactly one JSON value")
		}
		if args[0] == "aggregate-report" {
			if *out == "" {
				return fmt.Errorf("--out required")
			}
			return publish.AggregateReport(*path, set, *baseline, *candidate, *out)
		}
		result, err := analysis.Aggregate(b, set, *baseline, *candidate)
		if err != nil {
			return err
		}
		return output(result)
	case "source-bench-replay":
		f := flags(args[0])
		bundle := f.String("bundle", "", "sealed correctness-admitted source benchmark")
		out := f.String("out", "", "new replay bundle")
		if e = f.Parse(args[1:]); e != nil {
			return e
		}
		if *bundle == "" || *out == "" || f.NArg() != 0 {
			return fmt.Errorf("source-bench-replay requires --bundle and --out")
		}
		_, e := sourcebuild.ReplayBenchmark(ctx, *bundle, *out)
		if e != nil {
			return e
		}
		fmt.Println(*out)
		return nil
	case "source-bench-html":
		f := flags(args[0])
		bundle := f.String("bundle", "", "sealed source benchmark")
		out := f.String("out", "", "new portable HTML report directory")
		if e = f.Parse(args[1:]); e != nil {
			return e
		}
		if *bundle == "" || *out == "" || f.NArg() != 0 {
			return fmt.Errorf("source-bench-html requires --bundle and --out")
		}
		return publish.SourceReport(*bundle, *out)
	case "source-bench-report":
		f := flags(args[0])
		bundle := f.String("bundle", "", "sealed source benchmark")
		out := f.String("out", "", "optional new report JSON")
		if e = f.Parse(args[1:]); e != nil {
			return e
		}
		if *bundle == "" || f.NArg() != 0 {
			return fmt.Errorf("source-bench-report requires --bundle")
		}
		b, e := sourcebuild.VerifyBenchmark(*bundle)
		if e != nil {
			return e
		}
		report := analysis.SummarizeSourceBuilds(b)
		if *out != "" {
			return experiment.WriteJSON(*out, report)
		}
		return output(report)
	case "source-bench":
		f := flags(args[0])
		requireIRQ := f.Bool("require-irq-affinity", false, "require device IRQ masks disjoint from --cpus at source benchmark boundaries")
		requirePartition := f.Bool("require-isolated-cpu-partition", false, "require empty isolated cgroup-parent and disjoint controller CPUs at benchmark boundaries")
		hostPolicyPath := f.String("host-policy", "", "observed host baseline JSON; preserved during replay")
		profile := f.String("profile", "timing", "timing, cpu or cgroup memory")
		parent := f.String("cgroup-parent", "", "delegated Linux cgroup v2 parent; no unisolated fallback")
		memoryMax := f.Uint64("memory-max", 0, "per-tool cgroup memory.max bytes")
		noSwap := f.Bool("no-swap", false, "disable swap in tool cgroups")
		quota := f.Uint64("cpu-quota-us", 0, "per-tool CPU quota per 100000 microseconds")
		cpus := f.String("cpus", "", "tool cgroup CPU list; not exclusive")
		mems := f.String("mems", "", "allowed tool NUMA node list; empty inherits parent")
		pids := f.Uint64("pids-max", 0, "tool cgroup task limit")
		paths := f.String("locks", "", "comma-separated pinned source locks; first is baseline")
		runtimeIDs := f.String("runtimes", "wazero", "correctness oracle configurations")
		blocks := f.Int("blocks", 6, "independent randomized build blocks")
		warmup := f.Int("warmup", 1, "retained warmup blocks")
		seed := f.Int64("seed", 1, "variant-order randomization seed")
		timeout := f.Duration("timeout", time.Minute, "per-build and correctness-process deadline")
		out := f.String("out", "", "new benchmark bundle")
		if e = f.Parse(args[1:]); e != nil {
			return e
		}
		if *paths == "" || *out == "" || f.NArg() != 0 {
			return fmt.Errorf("source-bench requires --locks and --out")
		}
		c := sourcebuild.BenchmarkConfig{Schema: 1, Profile: *profile, Blocks: *blocks, WarmupBlocks: *warmup, Seed: *seed, Timeout: *timeout}
		c.RequireIsolatedCPUPartition = *requirePartition
		c.RequireIRQAffinity = *requireIRQ
		if *hostPolicyPath != "" {
			c.HostPolicy, e = loadHostPolicy(*hostPolicyPath)
			if e != nil {
				return e
			}
		}
		if *parent != "" || *memoryMax != 0 || *noSwap || *quota != 0 || *cpus != "" || *mems != "" || *pids != 0 {
			c.Resources = &agent.ResourcePolicy{CgroupParent: *parent, MemoryMaxBytes: *memoryMax, DisableSwap: *noSwap, CPUQuotaUS: *quota, CPUs: *cpus, Mems: *mems, PidsMax: *pids}
		}
		for _, path := range strings.Split(*paths, ",") {
			var l sourcebuild.Lock
			if e = experiment.ReadJSON(path, &l); e != nil {
				return e
			}
			c.Variants = append(c.Variants, l)
		}
		c.CorrectnessRuntimes, e = experiment.ResolveRuntimes(root, strings.Split(*runtimeIDs, ","))
		if e != nil {
			return e
		}
		_, e = sourcebuild.Benchmark(ctx, c, *out)
		if e != nil {
			return e
		}
		fmt.Println(*out)
		return nil
	case "source-compare":
		f := flags(args[0])
		baseBuild := f.String("baseline-build", "", "verified baseline source build bundle")
		candidateBuild := f.String("candidate-build", "", "verified candidate source build bundle")
		baseRun := f.String("baseline-run", "", "baseline runtime measurement bundle")
		candidateRun := f.String("candidate-run", "", "candidate runtime measurement bundle")
		runtime := f.String("runtime", "wazero", "fixed runtime configuration ID")
		out := f.String("out", "", "optional new comparison JSON file")
		if e = f.Parse(args[1:]); e != nil {
			return e
		}
		if *baseBuild == "" || *candidateBuild == "" || *baseRun == "" || *candidateRun == "" || f.NArg() != 0 {
			return fmt.Errorf("source-compare requires both source builds and runtime runs")
		}
		ab, e := sourcebuild.Verify(*baseBuild)
		if e != nil {
			return e
		}
		bb, e := sourcebuild.Verify(*candidateBuild)
		if e != nil {
			return e
		}
		a, e := experiment.Load(*baseRun)
		if e != nil {
			return e
		}
		b, e := experiment.Load(*candidateRun)
		if e != nil {
			return e
		}
		comparison, e := analysis.CompareSourceRuns(a, b, ab, bb, *runtime)
		if e != nil {
			return e
		}
		if *out != "" {
			return experiment.WriteJSON(*out, comparison)
		}
		return output(comparison)
	case "stack-compare":
		f := flags(args[0])
		baseBuild := f.String("baseline-build", "", "verified baseline source build bundle")
		candidateBuild := f.String("candidate-build", "", "verified candidate source build bundle")
		baseRun := f.String("baseline-run", "", "baseline runtime measurement bundle")
		candidateRun := f.String("candidate-run", "", "candidate runtime measurement bundle")
		baseRuntime := f.String("baseline-runtime", "", "baseline runtime configuration ID")
		candidateRuntime := f.String("candidate-runtime", "", "candidate runtime configuration ID")
		out := f.String("out", "", "optional new stack comparison JSON file")
		if e = f.Parse(args[1:]); e != nil {
			return e
		}
		if *baseBuild == "" || *candidateBuild == "" || *baseRun == "" || *candidateRun == "" || *baseRuntime == "" || *candidateRuntime == "" || f.NArg() != 0 {
			return fmt.Errorf("stack-compare requires both builds, both runs and both runtime configuration IDs")
		}
		ab, e := sourcebuild.Verify(*baseBuild)
		if e != nil {
			return e
		}
		bb, e := sourcebuild.Verify(*candidateBuild)
		if e != nil {
			return e
		}
		a, e := experiment.Load(*baseRun)
		if e != nil {
			return e
		}
		b, e := experiment.Load(*candidateRun)
		if e != nil {
			return e
		}
		comparison, e := analysis.CompareStackRuns(a, b, ab, bb, *baseRuntime, *candidateRuntime)
		if e != nil {
			return e
		}
		if *out != "" {
			return experiment.WriteJSON(*out, comparison)
		}
		return output(comparison)
	case "stack-report":
		f := flags(args[0])
		baseBuild := f.String("baseline-build", "", "verified baseline source build bundle")
		candidateBuild := f.String("candidate-build", "", "verified candidate source build bundle")
		baseRun := f.String("baseline-run", "", "baseline runtime measurement bundle")
		candidateRun := f.String("candidate-run", "", "candidate runtime measurement bundle")
		baseRuntime := f.String("baseline-runtime", "", "baseline runtime configuration ID")
		candidateRuntime := f.String("candidate-runtime", "", "candidate runtime configuration ID")
		out := f.String("out", "", "new portable offline stack report directory")
		if e = f.Parse(args[1:]); e != nil {
			return e
		}
		if *baseBuild == "" || *candidateBuild == "" || *baseRun == "" || *candidateRun == "" || *baseRuntime == "" || *candidateRuntime == "" || *out == "" || f.NArg() != 0 {
			return fmt.Errorf("stack-report requires both builds, both runs, both runtime IDs and --out")
		}
		return publish.StackReport(*baseBuild, *candidateBuild, *baseRun, *candidateRun, *baseRuntime, *candidateRuntime, *out)
	case "source-compare-set":
		f := flags(args[0])
		baseBuilds := f.String("baseline-builds", "", "comma-separated verified baseline source build bundles")
		candidateBuilds := f.String("candidate-builds", "", "comma-separated verified candidate source build bundles, matched by workload ID")
		baseRun := f.String("baseline-run", "", "baseline runtime measurement bundle containing the complete source workload set")
		candidateRun := f.String("candidate-run", "", "candidate runtime measurement bundle containing the complete source workload set")
		runtime := f.String("runtime", "wazero", "fixed runtime configuration ID")
		out := f.String("out", "", "optional new comparison JSON file")
		if e = f.Parse(args[1:]); e != nil {
			return e
		}
		if *baseBuilds == "" || *candidateBuilds == "" || *baseRun == "" || *candidateRun == "" || f.NArg() != 0 {
			return fmt.Errorf("source-compare-set requires build lists and both runtime runs")
		}
		loadBuilds := func(paths string) ([]sourcebuild.Result, error) {
			var builds []sourcebuild.Result
			for _, path := range strings.Split(paths, ",") {
				if strings.TrimSpace(path) == "" {
					return nil, fmt.Errorf("empty path in source build list")
				}
				build, err := sourcebuild.Verify(strings.TrimSpace(path))
				if err != nil {
					return nil, err
				}
				builds = append(builds, build)
			}
			return builds, nil
		}
		ab, e := loadBuilds(*baseBuilds)
		if e != nil {
			return e
		}
		bb, e := loadBuilds(*candidateBuilds)
		if e != nil {
			return e
		}
		a, e := experiment.Load(*baseRun)
		if e != nil {
			return e
		}
		b, e := experiment.Load(*candidateRun)
		if e != nil {
			return e
		}
		comparison, e := analysis.CompareSourceRunSet(a, b, ab, bb, *runtime)
		if e != nil {
			return e
		}
		if *out != "" {
			return experiment.WriteJSON(*out, comparison)
		}
		return output(comparison)
	case "source-compare-set-report":
		f := flags(args[0])
		baseBuilds := f.String("baseline-builds", "", "comma-separated verified baseline source build bundles")
		candidateBuilds := f.String("candidate-builds", "", "comma-separated verified candidate source build bundles, matched by workload ID")
		baseRun := f.String("baseline-run", "", "baseline runtime measurement bundle containing the complete source workload set")
		candidateRun := f.String("candidate-run", "", "candidate runtime measurement bundle containing the complete source workload set")
		runtime := f.String("runtime", "wazero", "fixed runtime configuration ID")
		out := f.String("out", "", "new portable offline report directory")
		if e = f.Parse(args[1:]); e != nil {
			return e
		}
		if *baseBuilds == "" || *candidateBuilds == "" || *baseRun == "" || *candidateRun == "" || *out == "" || f.NArg() != 0 {
			return fmt.Errorf("source-compare-set-report requires build lists, both runtime runs and --out")
		}
		ab, e := publish.SplitSourceBuildPaths(*baseBuilds)
		if e != nil {
			return e
		}
		bb, e := publish.SplitSourceBuildPaths(*candidateBuilds)
		if e != nil {
			return e
		}
		return publish.SourceSetReport(ab, bb, *baseRun, *candidateRun, *runtime, *out)
	case "verify-source-set-report":
		f := flags(args[0])
		path := f.String("report", "", "portable source-set comparison report directory")
		if e = f.Parse(args[1:]); e != nil {
			return e
		}
		if *path == "" || f.NArg() != 0 {
			return fmt.Errorf("verify-source-set-report requires --report and no positional arguments")
		}
		if e = publish.VerifySourceSetReport(*path); e != nil {
			return e
		}
		fmt.Println("Source comparison report and recomputed analysis verified.")
		return nil
	case "source-verify":
		f := flags(args[0])
		bundle := f.String("bundle", "", "sealed source build bundle")
		if e = f.Parse(args[1:]); e != nil {
			return e
		}
		if *bundle == "" || f.NArg() != 0 {
			return fmt.Errorf("source-verify requires --bundle, no positional arguments")
		}
		result, e := sourcebuild.Verify(*bundle)
		if e != nil {
			return e
		}
		return output(map[string]string{"status": "verified", "artifact_sha256": result.ArtifactSHA256, "lock_sha256": result.LockSHA256})
	case "source-rebuild":
		f := flags(args[0])
		bundle := f.String("bundle", "", "sealed source build bundle")
		out := f.String("out", "", "new reproduction bundle")
		timeout := f.Duration("timeout", time.Minute, "total build/validation deadline")
		if e = f.Parse(args[1:]); e != nil {
			return e
		}
		if *bundle == "" || *out == "" || f.NArg() != 0 {
			return fmt.Errorf("source-rebuild requires --bundle and --out, no positional arguments")
		}
		result, e := sourcebuild.Rebuild(ctx, *bundle, *out, *timeout)
		if e != nil {
			return e
		}
		return output(result)
	case "source-lock", "source-build":
		f := flags(args[0])
		recipePath := f.String("recipe", "", "trusted local source build recipe")
		lockPath := f.String("lock", "", "pinned source build lock")
		out := f.String("out", "", "new lock file or build bundle directory")
		profile := f.String("validation-profile", "default", "independent Wasm feature policy")
		timeout := f.Duration("timeout", time.Minute, "total build/validation deadline")
		if e = f.Parse(args[1:]); e != nil {
			return e
		}
		if *out == "" || f.NArg() != 0 {
			return fmt.Errorf("--out required; no positional arguments accepted")
		}
		if args[0] == "source-lock" {
			if *recipePath == "" || *lockPath != "" {
				return fmt.Errorf("source-lock requires --recipe, not --lock")
			}
			var recipe sourcebuild.Recipe
			if e = experiment.ReadJSON(*recipePath, &recipe); e != nil {
				return e
			}
			analyzer, e := experiment.PinAnalyzer(filepath.Join(root, "adapters/wasmtime/target/release/wasm-analyze"), *profile)
			if e != nil {
				return e
			}
			lock, e := sourcebuild.Pin(recipe, filepath.Dir(*recipePath), analyzer)
			if e != nil {
				return e
			}
			return experiment.WriteJSON(*out, lock)
		}
		if *lockPath == "" || *recipePath != "" {
			return fmt.Errorf("source-build requires --lock, not --recipe")
		}
		var lock sourcebuild.Lock
		if e = experiment.ReadJSON(*lockPath, &lock); e != nil {
			return e
		}
		var override bool
		f.Visit(func(flag *flag.Flag) {
			if flag.Name == "validation-profile" {
				override = true
			}
		})
		if override {
			return fmt.Errorf("source-build uses the locked validation profile")
		}
		result, e := sourcebuild.Build(ctx, lock, *out, *timeout)
		if e != nil {
			return e
		}
		return output(result)
	case "analyze":
		f := flags("analyze")
		artifact := f.String("artifact", "", "Wasm input file")
		featureProfile := f.String("features", "default", "pinned wasmparser validation profile: default, wasm1, wasm2, wasm3, all")
		if e = f.Parse(args[1:]); e != nil {
			return e
		}
		if *artifact == "" {
			return fmt.Errorf("--artifact required")
		}
		cmd := exec.CommandContext(ctx, experiment.NativeExecutable(filepath.Join(root, "adapters", "wasmtime", "target", "release", "wasm-analyze")), *artifact, *featureProfile)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	case "list", "index", "queue", "worker":
		f := flags(args[0])
		dbPath := f.String("db", ".wasmbench/index.sqlite", "local SQLite index")
		runPath := f.String("run", "", "bundle to index")
		lockPath := f.String("lock", "", "lock to enqueue")
		out := f.String("out", "", "queued run output directory")
		if e = f.Parse(args[1:]); e != nil {
			return e
		}
		db, e := storage.Open(*dbPath)
		if e != nil {
			return e
		}
		defer db.Close()
		switch args[0] {
		case "list":
			rows, e := db.Runs()
			if e != nil {
				return e
			}
			return output(rows)
		case "index":
			if *runPath == "" {
				return fmt.Errorf("--run required")
			}
			return db.AddRun(*runPath)
		case "queue":
			if *lockPath == "" {
				jobs, e := db.Jobs()
				if e != nil {
					return e
				}
				return output(jobs)
			}
			if *out == "" {
				return fmt.Errorf("--out required when enqueuing")
			}
			var lock experiment.Lock
			if e = experiment.ReadJSON(*lockPath, &lock); e != nil {
				return e
			}
			id, e := db.Enqueue(lock, filepath.Dir(*lockPath), *out)
			if e == nil {
				fmt.Printf("Queued job %d\n", id)
			}
			return e
		case "worker":
			for {
				worked, e := db.WorkOne(ctx, func(s string) { fmt.Fprintln(os.Stderr, s) })
				if e != nil {
					return e
				}
				if !worked {
					return nil
				}
			}
		}
		return fmt.Errorf("unhandled index command")
	case "import-wago":
		f := flags("import-wago")
		source := f.String("source", "../../Wago/wago", "Wago checkout")
		ids := f.String("ids", "tiny,fib_rec,dispatch,matmul,sha256", "comma-separated IDs, or all")
		out := f.String("out", "wago-suite.json", "new suite manifest")
		if e = f.Parse(args[1:]); e != nil {
			return e
		}
		var selected []string
		if *ids != "all" {
			selected = strings.Split(*ids, ",")
		}
		workloads, e := corpus.ImportWago(*source, selected)
		if e != nil {
			return e
		}
		if e = experiment.WriteJSON(*out, workloads); e != nil {
			return e
		}
		fmt.Printf("Imported %d workload contracts into %s\n", len(workloads), *out)
		return nil
	case "corpus":
		f := flags("corpus")
		suite := f.String("suite", "calls", "source-defined corpus suite")
		out := f.String("out", "corpus.json", "new suite manifest; artifacts are written beside it")
		if e = f.Parse(args[1:]); e != nil {
			return e
		}
		manifest, e := filepath.Abs(*out)
		if e != nil {
			return e
		}
		workloads, e := corpus.Generate(manifest+".artifacts", *suite)
		if e != nil {
			return e
		}
		if e = experiment.WriteJSON(manifest, workloads); e != nil {
			return e
		}
		fmt.Printf("Built %d %s corpus workloads from Go source into %s\n", len(workloads), *suite, manifest)
		return nil
	case "help", "--help", "-h":
		usage()
		return nil
	case "version", "--version":
		fmt.Println(experiment.Version)
		return nil
	case "metrics":
		return output(struct {
			Metrics   any `json:"metrics"`
			Scenarios any `json:"scenarios"`
		}{metrics.Registry, metrics.Scenarios})
	case "doctor":
		f := flags("doctor")
		cpuPartition := f.String("cpu-partition", "", "read-only probe of an existing empty isolated Linux cgroup partition")
		measurementCPUs := f.String("measurement-cpus", "", "complete CPU set reserved by --cpu-partition")
		irqCPUs := f.String("irq-cpus", "", "read-only default/requested/effective IRQ-affinity readiness for the complete measurement CPU set")
		perfCgroup := f.String("perf-cgroup", "", "probe disabled perf event opens on an existing absolute cgroup v2 path; does not count or change permissions")
		if e = f.Parse(args[1:]); e != nil {
			return e
		}
		if f.NArg() != 0 {
			return fmt.Errorf("doctor does not accept positional arguments")
		}
		checks := map[string]string{}
		for _, tool := range []string{"go", "node", "wasmtime", "wago", "cargo", "perf", "duckdb", "d8", "spidermonkey", "jsc", "deno", "wavm", "wasm3", "wasmedge"} {
			path, e := exec.LookPath(tool)
			if e != nil {
				checks[tool] = "unavailable"
			} else {
				checks[tool] = path
			}
		}
		checks["official_measurements"] = "requires independently passing publication-check evidence and an explicitly trusted operator qualification; doctor does not certify a dedicated host"
		if a, err := experiment.PinAnalyzer(filepath.Join(root, "adapters/wasmtime/target/release/wasm-analyze"), "default"); err != nil {
			checks["independent_analyzer"] = err.Error()
		} else {
			checks["independent_analyzer"] = a.Name + "/" + a.Version + " sha256:" + a.SHA256
		}
		return output(struct {
			IRQAffinity  agent.IRQAffinityProbe  `json:"irq_affinity_probe"`
			CPUPartition agent.CPUPartitionProbe `json:"cpu_partition_probe"`
			Host         agent.Host              `json:"host"`
			Checks       map[string]string       `json:"checks"`
			Perf         collectors.PerfProbe    `json:"perf_cgroup_probe"`
		}{agent.ProbeIRQAffinity(*irqCPUs), agent.ProbeCPUPartition(*cpuPartition, *measurementCPUs), agent.IdentifyHost(), checks, collectors.ProbePerfCgroup(*perfCgroup)})
	case "build":
		f := flags("build")
		wagoSource := f.String("wago-source", "../../Wago/wago", "local Wago checkout for source-pinned adapter")
		rts := f.String("runtimes", "wazero,v8", "adapter configurations")
		if e = f.Parse(args[1:]); e != nil {
			return e
		}
		// Build admission tooling before measurement, including Go/JS-only runs.
		analyzerBuild := exec.CommandContext(ctx, "cargo", "build", "--release", "--locked", "--manifest-path", filepath.Join(root, "adapters/wasmtime/Cargo.toml"), "--bin", "wasm-analyze")
		analyzerBuild.Stdout, analyzerBuild.Stderr = os.Stdout, os.Stderr
		if e = analyzerBuild.Run(); e != nil {
			return fmt.Errorf("build independent analyzer: %w", e)
		}
		for _, rt := range strings.Split(*rts, ",") {
			switch rt {
			case "wasmtime-process-snapshot", "wasmtime-winch-process-snapshot":
				cmd := exec.CommandContext(ctx, "cargo", "build", "--release", "--locked", "--features", "process-snapshot", "--target-dir", "target/process-snapshot", "--bin", "qualify-process-snapshot")
				cmd.Dir = filepath.Join(root, "adapters", "wasmtime")
				cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
				if e = cmd.Run(); e != nil {
					return e
				}
			case "wasmtime-component-async":
				cmd := exec.CommandContext(ctx, "cargo", "build", "--release", "--locked", "--features", "component-async-probes", "--target-dir", "target/component-async", "--bin", "adapter-wasmtime")
				cmd.Dir = filepath.Join(root, "adapters", "wasmtime")
				cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
				if e = cmd.Run(); e != nil {
					return e
				}
			case "wasmtime", "wasmtime-winch", "wasmtime-pooling", "wasmtime-allocator", "wasmtime-winch-allocator", "wasmtime-code-lifetime", "wasmtime-winch-code-lifetime":
				cmd := exec.CommandContext(ctx, "cargo", "build", "--release", "--locked")
				if strings.HasSuffix(rt, "-allocator") {
					cmd = exec.CommandContext(ctx, "cargo", "build", "--release", "--locked", "--features", "native-allocator", "--target-dir", "target/allocator", "--bin", "adapter-wasmtime")
				}
				if strings.HasSuffix(rt, "-code-lifetime") {
					cmd = exec.CommandContext(ctx, "cargo", "build", "--release", "--locked", "--features", "native-code-lifetime", "--target-dir", "target/code-lifetime", "--bin", "adapter-wasmtime")
				}
				cmd.Dir = filepath.Join(root, "adapters", "wasmtime")
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				if e = cmd.Run(); e != nil {
					return e
				}
			case "wago":
				if e = experiment.BuildWago(ctx, root, *wagoSource); e != nil {
					return e
				}
			case "wazero", "wazero-interpreter":
				cmd := exec.CommandContext(ctx, "go", "build", "-trimpath", "-o", experiment.NativeExecutable(filepath.Join(root, "bin", "adapter-wazero")), "./adapters/wazero")
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				if e = cmd.Run(); e != nil {
					return e
				}
			case "v8", "v8-wasmfx", "v8-liftoff-only", "v8-optimizing-only", "v8-tier-observed", "v8-tier-traced":
				script := "adapters/v8/adapter.mjs"
				if rt == "v8-tier-observed" || rt == "v8-tier-traced" {
					script = "adapters/v8/tier-adapter.mjs"
				}
				cmd := exec.CommandContext(ctx, "node", "--check", script)
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				if e = cmd.Run(); e != nil {
					return e
				}
			default:
				supported, err := experiment.BuildExtraRuntime(ctx, root, rt)
				if err != nil {
					return err
				}
				if !supported {
					return fmt.Errorf("unknown runtime %q", rt)
				}
			}
		}
		return nil
	case "run", "check", "plan":
		f := flags(args[0])
		requireIRQ := f.Bool("require-irq-affinity", false, "require device IRQ masks disjoint from locked --cpus at run boundaries; not continuous host qualification")
		timingPeakRSS := f.Bool("timing-peak-rss", true, "capture kernel-accounted process lifetime peak RSS from the same timing trial (timing profile default)")
		archiveTools := f.Bool("archive-tools", false, "retain exact runner, adapters and analyzer as shared-cache links in the run bundle")
		requirePartition := f.Bool("require-isolated-cpu-partition", false, "require empty isolated cgroup-parent and disjoint controller CPUs at run boundaries")
		hostPolicyPath := f.String("host-policy", "", "observed host baseline JSON; immutable once locked")
		validationProfile := f.String("validation-profile", "default", "independent admission: default, wasm1, wasm2, wasm3, all (existing locks preserve their policy)")
		suite := f.String("suite", "core", "core, scaling, lifecycle, reactors, traps, floats, checkpoints, continuations, process-snapshots, process-snapshot-density, guest-density, sustained, or workload manifest path")
		rts := f.String("runtimes", "wazero,v8", "comma-separated adapter configurations")
		profile := f.String("profile", "timing", "timing, memory, code, counters, profiling")
		scenarios := f.String("scenarios", "compile,instantiate,first-call,steady", "comma-separated lifecycle scenarios")
		launches := f.Int("launches", 1, "independent process launches per cell")
		samples := f.Int("samples", 1, "batches per launch; timing compile/instantiate/steady default to 3 when sample flags are omitted")
		workers := f.Int("workers", 1, "maximum concurrent isolated trial workers (1..3)")
		samplesByScenario := f.String("samples-by-scenario", "", "JSON object overriding timing samples by scenario, e.g. {\"compile\":3,\"instantiate\":3,\"steady\":3}")
		operations := f.Int("operations", 1, "operations per batch")
		warmup := f.Int("warmup", 3, "retained warmup batches for steady execution")
		sustainedDuration := f.Duration("sustained-duration", 0, "minimum cumulative measured API time for sustained scenario; fixed samples, not a wall-time/service throughput target")
		sustainedPostCollection := f.Bool("sustained-post-collection", false, "memory-only diagnostic: one forced Go GC after sustained logical close; not physical reclamation")
		seed := f.Int64("seed", 1, "interleaving seed")
		timeout := f.Duration("timeout", 30*time.Second, "per-process deadline")
		cgroupParent := f.String("cgroup-parent", "", "Linux delegated cgroup v2 parent; fail closed if unavailable")
		memoryMax := f.Uint64("memory-max", 0, "cgroup memory.max bytes (0 inherits parent)")
		noSwap := f.Bool("no-swap", false, "disable swap in adapter cgroup")
		phaseBarriers := f.Bool("phase-barriers", false, "memory/counters diagnostic phase handshakes (requires adapter support)")
		cpuQuota := f.Uint64("cpu-quota-us", 0, "cgroup CPU quota per 100000 microseconds (0 inherits parent)")
		cpus := f.String("cpus", "", "cgroup CPU list, e.g. 2-3; empty inherits parent")
		mems := f.String("mems", "", "allowed NUMA node list, e.g. 0; empty inherits parent")
		pidsMax := f.Uint64("pids-max", 0, "cgroup task limit (0 inherits parent)")
		out := f.String("out", "", "new output directory (or plan lockfile)")
		lockPath := f.String("lock", "", "use an existing lockfile")
		if e = f.Parse(args[1:]); e != nil {
			return e
		}
		var scenarioSamples map[string]int
		if strings.TrimSpace(*samplesByScenario) != "" {
			if e = json.Unmarshal([]byte(*samplesByScenario), &scenarioSamples); e != nil {
				return fmt.Errorf("invalid --samples-by-scenario JSON: %w", e)
			}
		}
		var lock experiment.Lock
		base := root
		validationExplicit := false
		memsExplicit := false
		hostPolicyExplicit := false
		partitionExplicit := false
		workersExplicit := false
		samplesExplicit := false
		scenarioSamplesExplicit := false
		irqExplicit := false
		timingPeakExplicit := false
		archiveExplicit := false
		sustainedExplicit := false
		f.Visit(func(v *flag.Flag) {
			if v.Name == "samples" {
				samplesExplicit = true
			}
			if v.Name == "samples-by-scenario" {
				scenarioSamplesExplicit = true
			}
			if v.Name == "require-irq-affinity" {
				irqExplicit = true
			}
			if v.Name == "sustained-duration" || v.Name == "sustained-post-collection" {
				sustainedExplicit = true
			}
			if v.Name == "timing-peak-rss" {
				timingPeakExplicit = true
			}
			if v.Name == "archive-tools" {
				archiveExplicit = true
			}
			if v.Name == "require-isolated-cpu-partition" {
				partitionExplicit = true
			}
			if v.Name == "workers" {
				workersExplicit = true
			}
			if v.Name == "host-policy" {
				hostPolicyExplicit = true
			}
			if v.Name == "mems" {
				memsExplicit = true
			}
			if v.Name == "validation-profile" {
				validationExplicit = true
			}
		})
		if *lockPath != "" {
			if workersExplicit {
				return fmt.Errorf("--workers cannot override an existing lock")
			}
			if irqExplicit {
				return fmt.Errorf("--require-irq-affinity cannot override an existing lock")
			}
			if sustainedExplicit {
				return fmt.Errorf("sustained duration/post-collection flags cannot override an existing lock")
			}
			if timingPeakExplicit {
				return fmt.Errorf("--timing-peak-rss cannot override an existing lock")
			}
			if archiveExplicit {
				return fmt.Errorf("--archive-tools cannot override an existing lock")
			}
			if partitionExplicit {
				return fmt.Errorf("--require-isolated-cpu-partition cannot override an existing lock")
			}
			if hostPolicyExplicit {
				return fmt.Errorf("--host-policy cannot override an existing lock")
			}
			if memsExplicit {
				return fmt.Errorf("--mems cannot override an existing lock")
			}
			if validationExplicit {
				return fmt.Errorf("--validation-profile cannot override an existing lock")
			}
			if e = experiment.ReadJSON(*lockPath, &lock); e != nil {
				return e
			}
			base = filepath.Dir(*lockPath)
		} else {
			scenarioSamples = defaultScenarioSamples(*profile, strings.Split(*scenarios, ","), scenarioSamples, samplesExplicit, scenarioSamplesExplicit)
			if *profile != "timing" && !timingPeakExplicit {
				*timingPeakRSS = false
			}
			workloads, e := corpus.Generate(filepath.Join(root, ".wasmbench", "corpus"), *suite)
			if e != nil {
				if e = experiment.ReadJSON(*suite, &workloads); e != nil {
					return fmt.Errorf("suite must be core, scaling, lifecycle, reactors, traps, floats, checkpoints, continuations, process-snapshots, guest-density, sustained, or a workload JSON array: %w", e)
				}
				base = filepath.Dir(*suite)
			}
			runtimes, e := experiment.ResolveRuntimes(root, strings.Split(*rts, ","))
			if e != nil {
				return e
			}
			lock, e = experiment.NewLock(experiment.Options{TimingPeakRSS: *timingPeakRSS, Workers: *workers, SustainedPostCollection: *sustainedPostCollection, SustainedDuration: *sustainedDuration, ScenarioSamples: scenarioSamples, PhaseBarriers: *phaseBarriers, Resources: agent.ResourcePolicy{CgroupParent: *cgroupParent, MemoryMaxBytes: *memoryMax, DisableSwap: *noSwap, CPUQuotaUS: *cpuQuota, CPUs: *cpus, Mems: *mems, PidsMax: *pidsMax}, Suite: *suite, Profile: *profile, Scenarios: strings.Split(*scenarios, ","), Launches: *launches, Samples: *samples, Operations: *operations, Warmup: *warmup, Seed: *seed, Timeout: *timeout, Check: args[0] == "check"}, runtimes, workloads)
			if e != nil {
				return e
			}
			lock.Analyzer, e = experiment.PinAnalyzer(filepath.Join(root, "adapters", "wasmtime", "target", "release", "wasm-analyze"), *validationProfile)
			if e != nil {
				return e
			}
		}
		if *lockPath == "" {
			lock.ArchiveTools = *archiveTools
			lock.RequireIsolatedCPUPartition = *requirePartition
			lock.RequireIRQAffinity = *requireIRQ
		}
		if lock.RequireIsolatedCPUPartition {
			if err := agent.ValidateCPUPartitionPolicy(lock.Options.Resources); err != nil {
				return err
			}
		}
		if *hostPolicyPath != "" {
			lock.HostPolicy, e = loadHostPolicy(*hostPolicyPath)
			if e != nil {
				return e
			}
		}
		if args[0] == "check" {
			lock.Options.Check = true
		}
		if err := experiment.ValidateLock(lock); err != nil {
			return err
		}
		if args[0] == "plan" {
			if *out == "" {
				*out = "suite.lock"
			}
			for i, w := range lock.Workloads {
				if !filepath.IsAbs(w.Artifact) {
					abs, e := filepath.Abs(filepath.Join(base, w.Artifact))
					if e != nil {
						return e
					}
					lock.Workloads[i].Artifact = abs
				}
			}
			if e = experiment.WriteJSON(*out, lock); e != nil {
				return e
			}
			fmt.Println(*out)
			return nil
		}
		if *out == "" {
			*out = filepath.Join("runs", time.Now().UTC().Format("20060102T150405.000000000Z"))
		}
		path, e := experiment.Run(ctx, lock, base, *out, func(s string) { fmt.Fprintln(os.Stderr, s) })
		if e != nil {
			return e
		}
		fmt.Println(path)
		bundle, e := experiment.Load(path)
		if e != nil {
			return e
		}
		db, indexErr := storage.Open(filepath.Join(root, ".wasmbench", "index.sqlite"))
		if indexErr == nil {
			indexErr = db.AddRun(path)
			db.Close()
		}
		if indexErr != nil {
			return fmt.Errorf("bundle saved, but indexing failed: %w", indexErr)
		}
		for _, t := range bundle.Trials {
			if t.Status != "ok" && t.Status != "unsupported" {
				return fmt.Errorf("run contains %s trials; evidence saved in %s", t.Status, path)
			}
		}
		return nil
	case "fetch":
		f := flags("fetch")
		path := f.String("lock", "suite.lock", "locked experiment")
		if e = f.Parse(args[1:]); e != nil {
			return e
		}
		var l experiment.Lock
		if e = experiment.ReadJSON(*path, &l); e != nil {
			return e
		}
		if e = experiment.VerifyInputs(l, filepath.Dir(*path)); e != nil {
			return e
		}
		fmt.Println("All locked artifacts and runtime files are present and verified.")
		return nil
	case "export-code", "disassemble-code":
		f := flags(args[0])
		objcopy := f.String("llvm-objcopy", "llvm-objcopy", "trusted local LLVM object converter")
		objdump := f.String("llvm-objdump", "llvm-objdump", "trusted local LLVM disassembler")
		timeout := f.Duration("tool-timeout", 30*time.Second, "deadline per disassembly tool invocation")
		functionsOnly := f.Bool("functions-only", false, "disassemble-code: collect only attributed function ranges")
		path := f.String("run", "", "verified code-profile run bundle")
		out := f.String("out", "", "new offline export directory")
		if e = f.Parse(args[1:]); e != nil {
			return e
		}
		if *path == "" || *out == "" {
			return fmt.Errorf("--run and --out are required")
		}
		if *functionsOnly && args[0] != "disassemble-code" {
			return fmt.Errorf("--functions-only requires disassemble-code")
		}
		if args[0] == "disassemble-code" {
			if *functionsOnly {
				return publish.DisassembleNativeFunctions(ctx, *path, *out, *objcopy, *objdump, *timeout)
			}
			return publish.DisassembleNativeCode(ctx, *path, *out, *objcopy, *objdump, *timeout)
		}
		return publish.ExportNativeCode(*path, *out)
	case "verify", "inspect", "report", "publish":
		f := flags(args[0])
		qualification := f.String("qualification", "", "publish: operator-signed dedicated-host qualification")
		qualificationKey := f.String("qualification-key", "", "publish: separately trusted operator public key JSON")
		pilotPath := f.String("pilot-run", "", "publish: relocated sealed pilot bundle")
		memoryRun := f.String("memory-run", "", "report: verified memory-profile run to show stage memory evidence")
		codeRun := f.String("code-run", "", "report: verified code-profile run with matching runtime and workload identities")
		artifactEvidence := f.Bool("artifact-evidence", false, "inspect: include verified artifact admission and full analyzer report (requires --workload)")
		path := f.String("run", "", "run bundle directory")
		workload := f.String("workload", "", "workload ID")
		out := f.String("out", "report", "new report directory")
		if e = f.Parse(args[1:]); e != nil {
			return e
		}
		if *path == "" {
			return fmt.Errorf("--run is required")
		}
		if args[0] != "publish" && (*qualification != "" || *qualificationKey != "") {
			return fmt.Errorf("qualification flags are only valid with publish")
		}
		if *memoryRun != "" && args[0] != "report" {
			return fmt.Errorf("--memory-run is only valid with report")
		}
		if *codeRun != "" && args[0] != "report" {
			return fmt.Errorf("--code-run is only valid with report")
		}
		if *artifactEvidence {
			if args[0] != "inspect" || *workload == "" {
				return fmt.Errorf("--artifact-evidence requires inspect --workload")
			}
			inspection, err := experiment.InspectWorkload(*path, *workload)
			if err != nil {
				return err
			}
			return output(inspection)
		}
		b, e := experiment.Load(*path)
		if e != nil {
			return e
		}
		if args[0] == "verify" {
			fmt.Println("Bundle checksums verified.")
			return nil
		}
		if args[0] == "report" {
			return publish.ReportWithPasses(*path, *memoryRun, *codeRun, *out)
		}
		if args[0] == "publish" {
			q := loadQualificationEvidence(*path, *qualification, *qualificationKey)
			if e = publicationAuditWithQualification(b, *pilotPath, q).Err(); e != nil {
				return e
			}
			pilot := *pilotPath
			if pilot == "" {
				p, err := analysis.DecodePilotPlan(b.Manifest.Lock.PilotPlan)
				if err != nil {
					return err
				}
				pilot = p.SourceBundle
			}
			return publish.ReportQualified(*path, pilot, *q.Qualification, q.TrustedPublicKey, *out)
		}
		if *workload != "" {
			var trials []experiment.Trial
			for _, t := range b.Trials {
				if t.Workload == *workload {
					trials = append(trials, t)
				}
			}
			return output(trials)
		}
		return output(b)
	case "export-site":
		f := flags("export-site")
		describe := f.Bool("describe", false, "print bounded export capabilities without reading or executing evidence")
		report := f.String("report", "", "verified sealed measurement report")
		out := f.String("out", "", "new bounded site-v2 export directory")
		nativeArchive := f.String("native-disassembly", "", "separately sealed offline native disassembly archive for the exact code pass")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if *describe {
			if *report != "" || *out != "" || *nativeArchive != "" || f.NArg() != 0 {
				return fmt.Errorf("export-site --describe does not accept evidence or output paths")
			}
			return output(publish.SiteExportContract())
		}
		if *report == "" || *out == "" || f.NArg() != 0 {
			return fmt.Errorf("export-site requires --report and --out")
		}
		return publish.ExportSiteWithDisassembly(*report, *out, *nativeArchive)
	case "verify-report":
		f := flags("verify-report")
		dir := f.String("dir", "", "sealed static report directory")
		recordedBuilder := f.Bool("recorded-builder", false, "explicitly execute the sealed report builder; trusted reports only")
		qualificationKey := f.String("qualification-key", "", "independently authenticate qualified publication with a separately trusted public key JSON")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if *dir == "" || f.NArg() != 0 {
			return fmt.Errorf("verify-report requires --dir and no positional arguments")
		}
		if *qualificationKey != "" {
			if *recordedBuilder {
				return fmt.Errorf("operator trust verification must use current trusted analysis, not --recorded-builder")
			}
			key, err := readQualificationPublicKey(*qualificationKey)
			if err != nil {
				return err
			}
			if err := publish.VerifyPublicationReport(*dir, key); err != nil {
				return err
			}
			fmt.Println("Qualified publication evidence verified against supplied operator key.")
			return nil
		}
		if *recordedBuilder {
			fmt.Fprintln(os.Stderr, "Executing archived report builder: only use trusted report archives.")
			return publish.VerifyReportWithRecordedBuilder(ctx, *dir)
		}
		if err := publish.VerifyAnyReport(*dir); err != nil {
			return err
		}
		fmt.Println("Report checksums and derived dataset verified.")
		return nil
	case "verify-code":
		f := flags("verify-code")
		dir := f.String("dir", "", "sealed native-code export directory")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if *dir == "" || f.NArg() != 0 {
			return fmt.Errorf("verify-code requires --dir and no positional arguments")
		}
		if err := publish.VerifyNativeCode(*dir); err != nil {
			return err
		}
		fmt.Println("Native-code report, extracted images, and copied run evidence verified.")
		return nil
	case "compare-code":
		f := flags("compare-code")
		baselineReport := f.String("baseline-report", "", "verified baseline native-code report")
		candidateReport := f.String("candidate-report", "", "verified candidate native-code report")
		baseline := f.String("baseline-runtime", "", "baseline runtime configuration ID")
		candidate := f.String("candidate-runtime", "", "candidate runtime configuration ID")
		out := f.String("out", "", "new portable generated-code comparison directory")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if *baselineReport == "" || *candidateReport == "" || *baseline == "" || *candidate == "" || *out == "" || f.NArg() != 0 {
			return fmt.Errorf("compare-code requires both reports, both runtime IDs and --out")
		}
		return publish.CompareNativeCode(*baselineReport, *candidateReport, *baseline, *candidate, *out)
	case "verify-code-comparison":
		f := flags("verify-code-comparison")
		dir := f.String("dir", "", "sealed generated-code comparison directory")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if *dir == "" || f.NArg() != 0 {
			return fmt.Errorf("verify-code-comparison requires --dir and no positional arguments")
		}
		if err := publish.VerifyNativeComparison(*dir); err != nil {
			return err
		}
		fmt.Println("Generated-code comparison and recomputed evidence verified.")
		return nil
	case "break-even":
		f := flags("break-even")
		path := f.String("run", "", "verified timing bundle with trajectory and setup phases")
		setup := f.String("setup", "auto", "comma-separated setup phases; auto uses compile,instantiate and declared app-init")
		baseline := f.String("baseline", "", "optional baseline configuration")
		candidate := f.String("candidate", "", "optional candidate configuration")
		if e = f.Parse(args[1:]); e != nil {
			return e
		}
		if *path == "" || f.NArg() != 0 {
			return fmt.Errorf("--run required; no positional arguments accepted")
		}
		b, e := experiment.Load(*path)
		if e != nil {
			return e
		}
		var phases []string
		if *setup != "auto" {
			phases = strings.Split(*setup, ",")
		}
		result, e := analysis.BreakEven(b, phases, *baseline, *candidate)
		if e != nil {
			return e
		}
		return output(result)
	case "history-report":
		f := flags("history-report")
		paths := f.String("runs", "", "ordered comma-separated verified bundles")
		runtime := f.String("runtime", "wazero", "runtime configuration ID")
		out := f.String("out", "", "new portable report directory")
		if e = f.Parse(args[1:]); e != nil {
			return e
		}
		if *paths == "" || *out == "" || f.NArg() != 0 {
			return fmt.Errorf("history-report requires --runs and --out")
		}
		return publish.HistoryReport(strings.Split(*paths, ","), *runtime, *out)
	case "history":
		f := flags("history")
		paths := f.String("runs", "", "ordered comma-separated bundles; first is fixed baseline")
		runtime := f.String("runtime", "wazero", "runtime configuration ID in each run")
		out := f.String("out", "", "optional new JSON report file")
		if e = f.Parse(args[1:]); e != nil {
			return e
		}
		if *paths == "" || f.NArg() != 0 {
			return fmt.Errorf("history requires --runs and no positional arguments")
		}
		var bundles []experiment.Bundle
		var evidence []analysis.HistoryEvidence
		for _, path := range strings.Split(*paths, ",") {
			if path == "" {
				return fmt.Errorf("empty history bundle path")
			}
			b, err := experiment.Load(path)
			if err != nil {
				return err
			}
			digest, err := experiment.DigestFile(filepath.Join(path, "checksums.json"))
			if err != nil {
				return err
			}
			bundles = append(bundles, b)
			evidence = append(evidence, analysis.HistoryEvidence{Run: b.Manifest.ID, Bundle: path, ChecksumsSHA256: digest})
		}
		report, err := analysis.History(bundles, *runtime)
		if err != nil {
			return err
		}
		report.Evidence = evidence
		if *out != "" {
			return experiment.WriteJSON(*out, report)
		}
		return output(report)
	case "compare":
		if len(args) >= 3 && !strings.HasPrefix(args[1], "-") && !strings.HasPrefix(args[2], "-") {
			f := flags("compare run-a run-b")
			baseline := f.String("baseline", "wazero", "baseline runtime configuration")
			candidate := f.String("candidate", "wazero", "candidate runtime configuration")
			if e = f.Parse(args[3:]); e != nil {
				return e
			}
			a, e := experiment.Load(args[1])
			if e != nil {
				return e
			}
			b, e := experiment.Load(args[2])
			if e != nil {
				return e
			}
			comparison, e := analysis.CompareRuns(a, b, *baseline, *candidate)
			if e != nil {
				return e
			}
			return output(comparison)
		}
		f := flags("compare")
		path := f.String("run", "", "run bundle")
		baseline := f.String("baseline", "wazero", "baseline runtime configuration")
		candidate := f.String("candidate", "v8", "candidate runtime configuration")
		if e = f.Parse(args[1:]); e != nil {
			return e
		}
		if *path == "" {
			return fmt.Errorf("--run is required")
		}
		b, e := experiment.Load(*path)
		if e != nil {
			return e
		}
		return output(analysis.CompareWithin(b, *baseline, *candidate))
	case "compare-report":
		f := flags("compare-report")
		baselineRun := f.String("baseline-run", "", "verified baseline runtime bundle")
		candidateRun := f.String("candidate-run", "", "verified candidate runtime bundle")
		baseline := f.String("baseline-runtime", "wazero", "baseline runtime configuration ID")
		candidate := f.String("candidate-runtime", "wazero", "candidate runtime configuration ID")
		out := f.String("out", "", "new portable offline comparison report directory")
		if e = f.Parse(args[1:]); e != nil {
			return e
		}
		if *baselineRun == "" || *candidateRun == "" || *out == "" || f.NArg() != 0 {
			return fmt.Errorf("compare-report requires both runs and --out")
		}
		return publish.CompareReport(*baselineRun, *candidateRun, *baseline, *candidate, *out)
	case "verify-compare-report":
		f := flags("verify-compare-report")
		path := f.String("report", "", "portable runtime comparison report directory")
		if e = f.Parse(args[1:]); e != nil {
			return e
		}
		if *path == "" || f.NArg() != 0 {
			return fmt.Errorf("verify-compare-report requires --report and no positional arguments")
		}
		if e = publish.VerifyCompareReport(*path); e != nil {
			return e
		}
		fmt.Println("Runtime comparison report and recomputed analysis verified.")
		return nil
	case "restore-tools":
		f := flags("restore-tools")
		path := f.String("run", "", "trusted sealed run bundle containing archived tools")
		out := f.String("out", "", "new replay directory; never overwrites installed tools")
		if e = f.Parse(args[1:]); e != nil {
			return e
		}
		if *path == "" || *out == "" || f.NArg() != 0 {
			return fmt.Errorf("restore-tools requires --run and --out")
		}
		record, err := experiment.RestoreTools(*path, *out)
		if err != nil {
			return err
		}
		return output(record)
	case "reproduce-report":
		f := flags("reproduce-report")
		source := f.String("dir", "", "verified source report directory")
		out := f.String("out", "", "new directory for separate reproduced passes and report")
		recordedBuilder := f.Bool("recorded-builder", false, "explicitly execute the sealed report builder to preserve original analysis; trusted reports only")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if *source == "" || *out == "" || f.NArg() != 0 {
			return fmt.Errorf("--dir and --out are required")
		}
		if *recordedBuilder {
			fmt.Fprintln(os.Stderr, "Executing archived report builder: only use trusted report archives.")
			return publish.ReproduceReportWithRecordedBuilder(ctx, *source, *out)
		}
		if err := publish.ReproduceReport(ctx, *source, *out, func(s string) { fmt.Fprintln(os.Stderr, s) }); err != nil {
			return err
		}
		fmt.Println(filepath.Join(*out, "report"))
		return nil
	case "reproduce":
		if len(args) < 2 {
			return fmt.Errorf("reproduce requires a run bundle directory")
		}
		f := flags("reproduce")
		out := f.String("out", filepath.Join("runs", time.Now().UTC().Format("20060102T150405.000000000Z")), "new output directory")
		if e = f.Parse(args[2:]); e != nil {
			return e
		}
		b, e := experiment.Load(args[1])
		if e != nil {
			return e
		}
		path, e := experiment.Run(ctx, b.Manifest.Lock, args[1], *out, func(s string) { fmt.Fprintln(os.Stderr, s) })
		if e == nil {
			fmt.Println(path)
		}
		return e
	case "serve":
		f := flags("serve")
		dir := f.String("dir", "report", "report directory")
		addr := f.String("addr", "127.0.0.1:8080", "listen address")
		if e = f.Parse(args[1:]); e != nil {
			return e
		}
		server := &http.Server{Addr: *addr, Handler: http.FileServer(http.Dir(*dir)), ReadHeaderTimeout: 5 * time.Second}
		go func() { <-ctx.Done(); server.Close() }()
		fmt.Println("http://" + *addr)
		e = server.ListenAndServe()
		if e == http.ErrServerClosed {
			return nil
		}
		return e
	default:
		return fmt.Errorf("unknown command %q; run wasmbench help", args[0])
	}
}
func usage() {
	fmt.Print(`wasmbench — reproducible WebAssembly experiments

  doctor                         Inspect host and installed measurement tools
  host-policy --out host.json     Capture observed baseline (not host certification)
  qualification-draft --run runs/CONFIRMATION --out draft.json
  qualification-public-key --seed-file /secure/operator.seed --out operator-public.json
  qualification-sign --run runs/CONFIRMATION --statement reviewed.json --seed-file /secure/operator.seed --out signed.json
  build --runtimes wazero,v8      Build independent embedding adapters
  corpus --suite calls --out calls.json  Build the source-defined call corpus
  source-lock --recipe recipe.json --out source.lock.json
  source-build --lock source.lock.json --out builds/ID
  source-rebuild --bundle builds/ID --out builds/REPLAY
  source-verify --bundle builds/ID
  source-compare --baseline-build builds/A --candidate-build builds/B --baseline-run runs/A --candidate-run runs/B --runtime wazero
  stack-compare --baseline-build builds/A --candidate-build builds/B --baseline-run runs/A --candidate-run runs/B --baseline-runtime wago --candidate-runtime wasmtime
  stack-report --baseline-build builds/A --candidate-build builds/B --baseline-run runs/A --candidate-run runs/B --baseline-runtime wago --candidate-runtime wasmtime --out reports/stack
  verify-stack-report --report reports/stack
  source-compare-set --baseline-builds builds/a,builds/b --candidate-builds builds/c,builds/d --baseline-run runs/A --candidate-run runs/B --runtime wazero
  source-compare-set-report --baseline-builds builds/a,builds/b --candidate-builds builds/c,builds/d --baseline-run runs/A --candidate-run runs/B --runtime wazero --out reports/source-set
  verify-source-set-report --report reports/source-set
  source-bench --locks source-a.lock,source-b.lock --out builds/benchmark
  source-bench-report --bundle builds/benchmark --out reports/source-benchmark.json
  source-bench-html --bundle builds/benchmark --out reports/source-benchmark
  source-bench-replay --bundle builds/benchmark --out builds/replayed
  metrics                        Show versioned metric and scenario definitions
  plan --suite core --out suite.lock
  fetch --lock suite.lock         Verify pinned local inputs
  check --suite core              Sacrificial correctness checks only
  run --suite core --profile timing
  run --suite scaling --profile memory
  verify --run runs/ID            Verify immutable evidence checksums
  inspect --run runs/ID --workload algorithms/sum
  export-code --run runs/CODE --out reports/native-code
  export-site --report reports/RUN --out exports/site-v2
  disassemble-code --run runs/CODE --out reports/disassembly --llvm-objcopy PATH --llvm-objdump PATH
  verify-code --dir reports/native-code
  compare-code --baseline-report reports/BASE --candidate-report reports/CANDIDATE --baseline-runtime wasmtime --candidate-runtime wasmtime-winch --out reports/code-comparison
  verify-code-comparison --dir reports/code-comparison
  compare --run runs/ID --baseline wazero --candidate v8
  compare runs/BASE runs/CANDIDATE --baseline wazero --candidate wazero
  compare-report --baseline-run runs/BASE --candidate-run runs/CANDIDATE --baseline-runtime wazero --candidate-runtime v8 --out reports/comparison
  verify-compare-report --report reports/comparison
  history --runs runs/BASE,runs/V2,runs/V3 --runtime wazero --out history.json
  history-report --runs runs/BASE,runs/V2,runs/V3 --runtime wazero --out reports/history
  aggregate-set --run runs/ID --id suite-v1 --scenario steady
  aggregate --run runs/ID --set suite-v1.json --baseline wazero --candidate v8
  aggregate-report --run runs/ID --set suite-v1.json --baseline wazero --candidate v8 --out report/
  break-even --run runs/ID --baseline wazero --candidate v8
  report --run runs/ID --memory-run runs/MEMORY --code-run runs/CODE --out report
  verify-report --dir report       Verify lifecycle or archived comparison/history/aggregate/source/stack/native-code reports
  verify-report --dir report --recorded-builder  Execute trusted archived analysis
  verify-report --dir report --qualification-key /trusted/operator-public.json  Authenticate publication and recompute its gates
  serve --dir report              Open report at http://127.0.0.1:8080
  reproduce runs/ID --out runs/reproduced
  reproduce-report --dir report --out runs/reproduced-report
  restore-tools --run runs/ID --out .wasmbench/replay/ID
  publish --run runs/ID --pilot-run runs/PILOT --qualification operator-signed.json --qualification-key /trusted/operator-public.json --out report  Enforce and archive official publication evidence
  publication-check --run runs/ID --out publication-audit.json
  pilot-plan --run runs/PILOT     Plan a fixed confirmation budget from sealed pilot data
  pilot-run --plan budget.json --out runs/CONFIRMATION  Execute the fixed budget separately

Run/check/plan options: --runtimes, --scenarios, --launches, --samples,
--operations, --warmup, --seed, --timeout, --out, --lock.
Reports are static, work offline, and retain raw evidence.
`)
}
