# Workflow reference

Detailed commands and measurement boundaries. Run shell commands from the repository root.
For a quick start, see the [README](../README.md).

# Wasmbench

A reproducible WebAssembly experiment runner. Runtime adapters run in separate
processes, execute local batches through embedding APIs, verify results, and
return raw samples. Reports consume immutable evidence bundles.

**Development status:** the local runner is usable. The entire product described
in the proposal is still under construction; see [the acceptance ledger](../docs/IMPLEMENTATION.md).
Native execution-stack snapshots have a separately scoped
[continuation workflow](../docs/NATIVE_CONTINUATIONS.md). They are not
whole-instance or COW snapshots.
A separate [Linux process-COW qualification](../docs/PROCESS_SNAPSHOTS.md) verifies
quiescent guest-state restoration after source teardown. It is functional
qualification with archived-binary replay. Dedicated Linux
`wasmtime-process-snapshot` and `wasmtime-winch-process-snapshot` configurations
now provide typed timing stages, sealed bundles and locked replay. Ordinary
adapters remain unsupported for process cloning. Sealed child-inclusive boundary
readings and a bounded concurrent restored-density memory workflow are available;
these are not process-tree RSS peaks or physical COW accounting. See the
workflow's remaining integration gates.
Official publication requires independently trusted operator qualification and
passing machine-control, pilot and correctness gates. No real dedicated host has
been qualified by local tests; exploratory reports remain available.

Linux Wasmtime executable-code publication and retirement has a separate
[code-lifetime workflow](../docs/NATIVE_CODE_LIFETIME.md), including locked replay,
typed exports and ownership timelines. It observes published text capacity,
not total compiler emission or physical memory reclamation.

## Start

Requires Go 1.26 or newer, Rust/Cargo (tested with 1.98.1) for the independent
artifact analyzer, and Node.js for V8. Both `make build` and `wasmbench build`
build the analyzer before adapters. No website server or database service is required.

```sh
make build
./bin/wasmbench doctor
./bin/wasmbench check --suite core
./bin/wasmbench run --suite core --out runs/example
./bin/wasmbench report --run runs/example --out reports/example
./bin/wasmbench verify-report --dir reports/example
./bin/wasmbench serve --dir reports/example
```

### Rust allocator diagnostics

`wasmtime-allocator` and `wasmtime-winch-allocator` are separate instrumented
builds, not timing configurations. They record successful Rust `GlobalAlloc`
requests routed through `System` across all threads: requested bytes and count,
logical frees, outstanding requested bytes at API entry/return, and an observed
hook-accounting high-water. They exclude mmap-backed code/guest memory, foreign
allocators, usable-size overhead and physical residency. Realloc counts one
full new-size request and an old-size logical release even when in place.
Compiler-elided allocations are not observed; see the
[Rust allocator contract](https://doc.rust-lang.org/std/alloc/trait.GlobalAlloc.html).

```sh
./bin/wasmbench build --runtimes wasmtime-allocator,wasmtime-winch-allocator
./bin/wasmbench run --suite core \
  --runtimes wasmtime-allocator,wasmtime-winch-allocator --profile memory \
  --scenarios compile,instantiate,first-call,steady \
  --launches 1 --samples 3 --operations 1 --warmup 0 \
  --out runs/rust-allocator
./bin/wasmbench report --run runs/rust-allocator --out reports/rust-allocator
./bin/wasmbench verify-report --dir reports/rust-allocator
./bin/wasmbench serve --dir reports/rust-allocator
```

The report has a dedicated allocator table with per-trial sample windows and raw
links. Zero requests stay zero, while absent counters stay unavailable. Setup,
verification and resource release are outside each API-operation window. No
forced GC/purge, physical reclamation or leak finding is implied. Core workloads
with exact-result or `float_bits_v1` oracles are accepted, including declared
floating-point tolerances, NaN and signed-zero policies. Compile, instantiate
and first-call allow stateless or fresh-instance-per-sample contracts; steady
requires stateless reuse. Initialization, input injection and verification stay
outside the measured API window (Wasm start remains inside instantiation).
Samples must be single-operation without warmup. Ordinary adapters do not include this allocator hook;
instrumented builds reject timing passes and cannot be joined to ordinary
timing runs as if their binary/configuration identities matched.

Add `--phase-barriers` for four sample-qualified diagnostic handshakes: before
the API, at API return, after verification/before logical drop, and after the
release decision. Linux records RSS, PSS, private residency and virtual address
space at each boundary. Other platforms retain explicit unsupported outcomes.
The dedicated report shows these snapshots separately from Rust allocator
counts. Earlier steady samples report Store retention at the final boundary;
only the last drops it. Barrier transport is outside allocator counter/timer
windows but can affect process footprint. These are snapshots, not exact phase
peaks or proof of physical reclamation. Cgroup API-window peaks/CPU work remain
available only when their independent isolation/collector requirements hold.

New instrumented builds also record a separate logical-release window after
correctness verification. Compile drops the measured module with its engine
held; instantiate/first-call drop measured Store/Instance state and prepared
import handles with the module and engine held. Steady retains its Store across
samples and drops it only after the final sample. Earlier steady release rows
are explicitly “Retained,” not numerical zeros. The second table shows requested
allocation activity, outstanding bytes before/after drop and diagnostic drop
time. Verification sits between API return and release entry, so those baselines
can differ. Neither a logical free nor successful drop proves physical
reclamation. Older instrumented bundles without release capability remain
readable but do not acquire invented release measurements.

### Empty local harness calibration

Wago, wazero compiler/interpreter, Wasmtime Cranelift/Winch, and V8 support
`harness-calibration`. It measures local loop bookkeeping, not Wasm execution
or an embedding API call. The resident stateless core scalar workload must pass
its exact-result oracle before sampling. Every timed iteration writes its
index plus one to a preallocated eight-byte slot; all slots are checked after
the timer stops. Clearing slots, workload qualification, and sample allocation
are untimed. The recorded result is the iteration count, not guest output.

```sh
./bin/wasmbench run --suite core \
  --runtimes wago,wazero,wazero-interpreter,wasmtime,wasmtime-winch,v8 \
  --scenarios harness-calibration --profile timing \
  --launches 3 --samples 2 --operations 1000 --warmup 0 \
  --out runs/harness-calibration
WASMBENCH_HARNESS_TEST_RUNTIMES=wago,wazero,wazero-interpreter,wasmtime,wasmtime-winch,v8 \
  go test ./experiment -run TestBuiltAdapterHarnessCalibration -count=1 -v
```

Calibration requires timing without phase barriers or warmup, 1–100,000
samples and 1–1,000,000 iterations per sample. Memory/instrumented profiles and
stateful, vector, component, or command contracts are rejected. A single
iteration records `individual_operation`; larger loops record `batch_average`.
These are harness iterations, never useful guest work units. Inspect elapsed
timers and exact operation counts in raw trial JSON or `samples.parquet`;
workload latency summaries, throughput, aggregates, scaling fits, break-even,
and official workload publication do not score calibration. Nothing is
automatically subtracted from another measurement.

This is language-local bookkeeping, not one portable overhead constant: Go
uses an untimed non-inlined checker, Rust retains the writes through a black-box
compiler barrier, and V8 uses exactly representable integer values in
`Float64Array` slots. Runtime/JIT behavior, timer resolution, and the declared
loop remain part of the evidence. The effective configuration records the
exact policy, and the V8 helper is included in locked tool archives.

The report's detailed evidence includes an “Empty local harness calibration”
viewer: select an independent trial, browse all samples in 250-row windows,
inspect recorded batch timers/counts and diagnostic nanoseconds per iteration,
and open its raw trial, exact configuration/policy and reproduction commands.
Failed or invalid samples retain their raw values but do not acquire normalized
calibration values. Sacrificial workload checks, if present, are labeled
separately. Calibration-only reports open the evidence panel and hide the
workload comparison graph; mixed reports exclude calibration from stage bars.

### Engine initialization

For core scalar workloads, wazero compiler/interpreter and Wasmtime
Cranelift/Winch support `engine-init`. Each operation times only construction of
a fresh configured runtime/engine. After stopping that timer, the adapter uses
that same engine to compile the resident workload, instantiate it, initialize
and apply inputs, invoke it, and check result/memory oracles. Verification and
release are untimed; successful construction alone is not verified usability.

```sh
./bin/wasmbench run --suite core \
  --runtimes wazero,wazero-interpreter,wasmtime,wasmtime-winch \
  --scenarios engine-init --profile timing --out runs/engine-init
WASMBENCH_ENGINE_INIT_TEST_RUNTIMES=wazero,wazero-interpreter,wasmtime,wasmtime-winch \
  go test ./experiment -run TestBuiltAdapterEngineInitOracles -count=1 -v
```

The effective configuration records this policy. Engines are fresh per
operation, but their process and filesystem caches are not reset per operation.
This is not cold-process latency. Wazero's Go allocation observations include
untimed compilation/usability verification and release, rather than isolating
constructor allocations. Wasmtime does not attribute an unrelated prepared
instance's logical memory to engine construction. Whole-process peak RSS still
includes setup and verification and is not an exact engine-init phase peak.

### AOT production and loading

Wago and Wasmtime (Cranelift/Winch) support `aot-produce` and `aot-load` for
core-module workloads. Production times resident Wasm bytes through compilation
(including mandatory validation) and serialization. Outside the timer, each
produced artifact is loaded, instantiated and checked against the workload's
result and memory oracles. Loading times the native-artifact load API alone;
behavior verification and resource release are outside that timer.

```sh
./bin/wasmbench run --suite core --runtimes wago,wasmtime,wasmtime-winch \
  --scenarios aot-produce,aot-load --profile timing --out runs/aot-timing
./bin/wasmbench run --suite core --runtimes wago,wasmtime,wasmtime-winch \
  --scenarios aot-produce,aot-load --profile memory --out runs/aot-memory
./bin/wasmbench report --run runs/aot-timing --memory-run runs/aot-memory \
  --out reports/aot
```

Load input is immutable resident bytes serialized locally by the same configured
engine before sampling, not a disk-read benchmark or externally supplied native
artifact. Wago uses `LoadTrustedArtifact`; Wasmtime's unsafe deserializer receives
only those locally produced bytes. Neither adapter accepts arbitrary native
artifacts through this workflow. The effective configuration records this AOT
policy. Wago's Go allocator observations cover the batch including untimed
verification and release, not just the timed API. Process peak RSS includes
startup/setup and is not an exact AOT-phase peak. Serialized artifact size is not
native instruction size.

The opt-in real-adapter regression checks both profiles and both AOT scenarios
with correct, wrong-result and wrong-memory oracles. CI runs it for the pinned
Wago, Cranelift and Winch builds:

```sh
WASMBENCH_AOT_TEST_RUNTIMES=wago,wasmtime,wasmtime-winch \
  go test ./experiment -run TestBuiltAdapterAOTOracles -count=1 -v
```

### Fully materialized compile diagnostics

Wasmtime 46.0.1 (Cranelift and Winch) supports `compile-materialized` in a
dedicated code pass. It times one synchronous `Module::new` call from resident
input bytes, including mandatory validation. Outside that timer it verifies
every defined function's retained native range, checks the counts against the
independent input analyzer, exports the image, and checks correctness.

```sh
./bin/wasmbench build --runtimes wasmtime,wasmtime-winch
./bin/wasmbench run --suite core --runtimes wasmtime,wasmtime-winch \
  --profile code --scenarios compile-materialized \
  --launches 1 --samples 1 --operations 1 --warmup 0 \
  --out runs/materialized
./bin/wasmbench export-code --run runs/materialized --out reports/materialized
./bin/wasmbench verify-report --dir reports/materialized
./bin/wasmbench serve --dir reports/materialized
```

The native viewer shows the completion proof, input coverage, diagnostic timer,
and function ranges. This is not instruction-only size, emitted-code lifetime,
creation/retirement evidence, or a headline timing result. Only stateless core
workloads with exact scalar results and no compound fixtures are supported;
other runtimes retain explicit unsupported outcomes. Each independent launch
performs exactly one measured compile, with no warmup or phase barriers.

### Latency and peak-RSS trade-offs

Paired reports include a collapsible **Latency × peak RSS trade-offs** view
under the main runtime graph. Choose a workload above and a single stage in the
trade-off view. Runtime filters apply to both graphs. The scatter plot places
stage latency per operation against whole-process peak RSS from its separate
matched memory pass; lower-left is better. The table retains configurations with
missing or unsupported measurements instead of silently dropping coverage.

Outlined points form the median-based Pareto frontier among the selected
configurations. A point is dominated only when another has no larger median on
either axis and a strictly smaller median on at least one. Exact ties both remain
on the frontier. This is a descriptive trade-off, **not** a significance test or
overall score. Filtering recomputes the frontier without changing saved evidence.

Both axes retain independent-launch counts and per-axis bootstrap intervals
where available; these are not a joint confidence region. Single-launch results
remain exploratory and have no invented intervals. Hover/focus or click a point
(or its table entry) for timing evidence, memory raw trials and both manifests.
RSS includes process startup and setup; it is not an isolated phase peak.
The versioned `pareto` dataset is recomputed from the copied independent passes
by `verify-report`, including frontier membership and unavailable outcomes.

### Sustained execution

The `sustained` suite runs an integer-sum workload on one retained instance in
wazero's compiler/interpreter or V8. The fixed sample budget is locked in advance;
`--sustained-duration` requires at least that much cumulative measured,
non-warmup API time. It is not a wall-clock or service-throughput target.

```sh
./bin/wasmbench run --suite sustained --runtimes wazero,wazero-interpreter,v8 \
  --scenarios sustained --sustained-duration 1s --profile timing \
  --launches 3 --samples 3000 --operations 1000 --warmup 10 --timeout 2m \
  --out runs/sustained-timing
./bin/wasmbench run --suite sustained --runtimes wazero,wazero-interpreter,v8 \
  --scenarios sustained --sustained-duration 1s --profile memory \
  --launches 3 --samples 3000 --operations 1000 --warmup 10 --timeout 2m \
  --out runs/sustained-memory
./bin/wasmbench report --run runs/sustained-timing \
  --memory-run runs/sustained-memory --out reports/sustained
./bin/wasmbench verify-report --dir reports/sustained
./bin/wasmbench serve --dir reports/sustained
```

Use the same binaries, host/resource policy, workload, duration, sample count,
operation count and warmup for both passes. Do not rebuild or run unrelated
work during measurement. A short session retains its samples with
`duration_budget_not_met` and does not enter headline latency estimates. Faster
runtimes may need larger fixed batches to reach the same duration target;
choose and lock the budget before the run, not after inspecting its results.

Under **Explore detailed evidence**, the sustained view plots monotonic elapsed
session time, preserves warmup and gaps, and exposes every sample window and
raw trial. Timing-pass batch latency is separate from memory-pass diagnostics.
Go allocation activity uses cumulative allocated-byte deltas over the observed
diagnostic brackets; heap snapshots and GC cycle deltas are distinct views.
Go counters cover the adapter's Go heap, not native or all process memory.
V8 reports distinct `process.memoryUsage().heapUsed` snapshots, not allocation
volume, Go heap or process RSS. Normal tiering remains enabled; tiers and
background compilation activity are not directly observed. V8 release drops
module/instance/export references, with `policy: js_references_dropped` and
`closed: false`: the process engine and internal caches are not disposed.
The final logical-close window is separate: no forced GC, physical-reclamation
claim, retained-live-heap claim, or leak qualification. Current workloads require
stateless core exports with exact scalar results; phase barriers are unsupported.
Raw JSON and nullable Parquet session clocks preserve the measurement boundary.

For an explicit post-collection diagnostic, add `--sustained-post-collection`
to the **memory** command. This performs one `runtime.GC` after logical close
and dropping runtime references. It is rejected for timing profiles and cannot
override an existing lock. V8 advertises this Go-only operation as unsupported.
The final raw sample contains
`sustained_release.post_collection`: the policy, monotonic bracket and seven
separately scoped allocator/heap/GC observations. `samples.parquet` v4 preserves
nullable collection clocks and policy; `observations.parquet` preserves the
metric definitions and has no guest-operation denominator for this window.
The paired timing pass does not force collection. All other sustained budgets
and identities must match. This is a post-Go-collection heap observation with
the adapter and sample buffers still alive, **not** reclaimed process RSS,
runtime-only retained memory, allocator purge, or leak qualification.

### Guest-state checkpoint lifecycle

The `checkpoints` suite measures four distinct operations in wazero's compiler
and interpreter: eager checkpoint creation, restoration into an already
instantiated target, the first guest write, and post-restore execution. It uses
an exact generated module with 1, 4, 16 or 64 fixed memory pages and one mutable
i32 global. These are **not whole-instance or copy-on-write snapshots**. Runtime
state, active stacks, files, tables, passive segments and WasmGC objects are not
supported. Generic `can_snapshot` stays false; the narrow capability is
`can_guest_checkpoint`.

```sh
./bin/wasmbench build --runtimes wazero,wazero-interpreter
./bin/wasmbench run --suite checkpoints --runtimes wazero,wazero-interpreter \
  --scenarios checkpoint-create,checkpoint-restore,checkpoint-first-write,checkpoint-execute \
  --launches 3 --samples 2 --operations 1 --warmup 0 --out runs/checkpoint-timing
./bin/wasmbench run --suite checkpoints --runtimes wazero,wazero-interpreter \
  --profile memory --phase-barriers \
  --scenarios checkpoint-create,checkpoint-restore,checkpoint-first-write,checkpoint-execute \
  --launches 3 --samples 2 --operations 1 --warmup 0 --out runs/checkpoint-memory
./bin/wasmbench report --run runs/checkpoint-timing \
  --memory-run runs/checkpoint-memory --out reports/checkpoints
./bin/wasmbench verify-report --dir reports/checkpoints
```

Creation includes allocation and copying; restoration excludes target
instantiation. First write measures an ordinary eager-copy guest call, not a
COW fault. Execution calls a full-memory checksum. Each sample has fresh source,
target and checkpoint state, while the engine and compiled module are retained.
Setup, complete state verification and release are outside the operation timer.
The source is mutated after saving to test independence; restored memory is
checked byte-for-byte, including its scalar global. Raw evidence contains state
hashes, global values, result checksums and exact payload bytes (memory + 4;
excludes host object overhead). Memory adds scoped Go allocator observations
and ordered boundary snapshots, without forced GC or reclamation claims.
Offline loading checks the exact fixture, phase-specific state evidence and
payload metric again. Unsupported runtimes stay visible rather than scoring zero.

### Fresh versus restored guest-state density

The `guest-density` suite compares fresh initialization with eager restoration
of fixed guest memory plus one scalar global. Wazero compiler and interpreter
hold 1, 4, 16 or 64 instances simultaneously, sharing one retained engine and
compiled module. Each policy has 1-page and 4-page fixtures, with either no
additional write or a first write to the last byte plus scalar. Initialization
and restoration both touch all memory: `unchanged` does **not** mean physically
untouched, lazily materialized or copy-on-write pages.

```sh
./bin/wasmbench build --runtimes wazero,wazero-interpreter
./bin/wasmbench run --suite guest-density --runtimes wazero,wazero-interpreter,v8 \
  --scenarios guest-density --launches 3 --samples 2 --operations 1 --warmup 0 \
  --out runs/guest-density-timing
./bin/wasmbench run --suite guest-density --runtimes wazero,wazero-interpreter,v8 \
  --profile memory --phase-barriers --scenarios guest-density \
  --launches 3 --samples 2 --operations 1 --warmup 0 --out runs/guest-density-memory
./bin/wasmbench report --run runs/guest-density-timing \
  --memory-run runs/guest-density-memory --out reports/guest-density
./bin/wasmbench verify-report --dir reports/guest-density
```

One timed operation provisions the entire fresh group: instantiate/start,
initialize or restore, and the optional first write. Compile, source setup,
checkpoint creation, complete state/checksum verification and release are
excluded. Restore uses one shared independently copied payload per request;
the source is mutated to prove independence and closed before measurements.
Every held instance is verified byte-for-byte and by its scalar and full-memory
checksum after timing. Memory barriers observe before provisioning, ready
before verification, and logically released while the engine/module/payload
remain held. Close is not physical reclamation, and no GC is forced.

Scaling curves keep policy, memory size and write state separate; show full and
marginal costs rather than assuming linear growth. Memory keeps logical group
bytes, Go allocation volume and heap/GC state in distinct domains.
Paired reports include separately labeled memory scaling curves and raw-memory
links for each point, including failed and unsupported launches. Scaling filters
select the measurement pass, runtime configuration and metric without removing
unavailable curves or changing the recorded estimates. Paired memory timelines
and density boundary changes are also accessible in the same report, with
source-qualified trial IDs and raw-memory links separate from primary trials.
A standalone report from the memory bundle remains available as well.
Separate `memory-samples.parquet` and `memory-observations.parquet` downloads
retain the complete raw memory pass, including sacrificial and unmatched cells;
they are never concatenated with timing exports. `verify-report` regenerates
those exports from copied raw memory evidence and compares their digests, in
addition to validating the report seal, recomputed dataset and rendered page.
Raw samples and nullable Parquet v6 evidence preserve every instance's state digest, scalar,
checksum, payload size and isolation flags. Offline validation rechecks the
canonical artifact, all instances, budgets, phase order and allocator domains.
The capability is `can_guest_density`; generic `can_snapshot` remains false.
Unsupported runtimes remain visible. Whole-instance/native/COW snapshot density
is not implemented by this experiment.

To show stage memory alongside timing, collect a separate `--profile memory`
run with the same workload artifacts and runtime binaries, then attach it:

```sh
./bin/wasmbench report --run runs/timing --memory-run runs/memory --out reports/paired
```

An optional sealed `--code-run runs/code` adds code-pass drilldowns to the same
report. The pairing requires the same host and resource policy, runtime binary
and effective configuration, and full workload contract. Clicking a result
then links to its native-output record under `code/`. Mixed native-image bytes
are labeled separately from guest instructions; unsupported exports remain
visible rather than becoming zero or disappearing. The nested code report and
copied source bundle are checked by `verify-report`.

The report joins only matching host identities, resource policies, protocol
versions, effective runtime configurations, runtime binaries, and complete
workload contracts (allowing the same artifact bytes at a restored path).
Its memory selector keeps Go allocation bytes per operation, cgroup phase peaks,
whole-process peak RSS, and endpoint heap/RSS snapshots in distinct measurement domains; unavailable
collectors remain unavailable. Memory values are not added to the timing total
or to each other.

Static reports are new-only sealed directories. `verify-report` checks every
file, reloads the copied raw bundle(s), recomputes the graph dataset, and
checks the page against its pinned renderer. Reports refuse an output path
inside either input bundle, including a
symlink alias. Checksums detect local modification, not producer authenticity.

When a Linux memory pass uses phase barriers in an adapter cgroup, the detailed
evidence view also shows total, user, and system process-tree CPU time for each
barrier window. These are diagnostic memory-pass observations including barrier
transport, not CPU time for just the compile or invocation API. The view keeps
launch coverage, unavailable values, and links to every raw trial; intervals
need at least three independent launches.

Open <http://127.0.0.1:8080>. The generated `index.html` also works directly from
disk. The landing view groups workloads and compares runtime configurations
within each one with a segmented bar per runtime configuration. The top toolbar
selects compile, instantiate, and one execution phase to show. Its configuration
filters, stage colors, coverage counts, and workload search provide a compact
overview; hover or focus a segment for its time and selected memory metric, and
click for intervals, launch counts, and raw evidence.
The large workload-selectable graph above the cards compares each runtime’s
selected stage medians in one segmented latency bar alongside whole-process
peak RSS for each selected stage. The graph follows the top stage selector;
latency shares one scale across runtimes, and all RSS bars share another scale.
The graph opens viewport-wide with taller stage bars. “Compact graph” restores
the normal report layout; “Expand graph” enlarges it again without changing selections.
Every selected stage also has a labeled value and evidence button beneath its
latency bar, so short stages remain readable even when compilation dominates.
Missing stages remain explicit; available stage values are still accessible when
the combined latency estimate is incomplete.
Peak RSS includes startup and setup and is not a phase-only peak. Clicking an RSS
value shows its independent memory-launch values, available bootstrap interval,
and links to every raw memory trial. Timing and memory runs must agree on host
identity, resource budget, runtime configuration, and workload contract before
their results can appear together.
Segment lengths sum separately measured stage medians for display, not an observed
end-to-end latency. Open “Explore detailed evidence” for the full tables, diagnostics, configuration,
and reproduction instructions. Reports include JSON, Parquet, the lock manifest,
and all raw trial files. There is no cross-workload overall score.

Paired passes and cross-run latency/native-code comparisons require matching
locked host baselines, CPU-partition and IRQ-affinity requirements, and every
resource-budget field. Equal host fingerprints alone do not make differently
controlled experiments comparable. Policy matching is separate from each run's
evidence eligibility and never certifies a dedicated measurement host.

The clicked result also shows a compact input-Wasm structure summary from the
sealed independent analyzer: file and section payload sizes, function-body
distribution, imports/exports, control depth, and frequent operators. Component
roots show their node count instead of inapplicable core-function statistics.
The full analyzer report remains linked, and its validator-feature policy is
not presented as a unique minimal requirement set.

### Packaged Linux workflow

The container includes the controller, independent analyzer, wazero compiler
and interpreter, Node/V8, and the separate Wasmtime/Winch native code-lifetime
diagnostic adapter. Go, Rust and network access are needed at image build time,
not during benchmark execution. Wago and ordinary Wasmtime timing adapters are
not included in this image.

```sh
docker build -t wasmbench:dev .
sh recipes/test-container.sh wasmbench:dev container-smoke
# Separate code-profile collection, locked replay and two sealed reports:
sh recipes/test-linux-code-lifetime.sh wasmbench:dev native-code-lifetime
```

The smoke recipe resolves the image to an immutable local ID and exercises
`doctor`, `check`, `run`, `reproduce`, `verify`, and `report`. Each container runs
as your UID/GID with networking disabled, no Linux capabilities, and a read-only
root filesystem. Only the new evidence directory and temporary state are
writable. It never mounts replacement executables from the host.

Open `runs/container-smoke/report/index.html`. The sibling `check`, `run` and
`reproduced` directories contain the sealed bundles. The `floats` and
`float-phases` bundles exercise all seven float fixtures, retained trajectory
warmups, teardown, and compile/instantiate/release barriers on the three packaged
configurations. The verifier checks the packaged V8 helper's locked hash, every
trial outcome, sample budget and phase boundary. The `density` bundle and
`density-report` exercise three independent density blocks, preserve V8's
unsupported separate-engine cases, require Linux RSS snapshots while groups are
held, and check logical-memory marginal costs and their paired-block intervals.
Choose a new directory
name for each invocation; existing evidence is never overwritten. Keep the
printed image ID available for exact replay. These shared-host container runs
are local functional checks, not official performance measurements or proof
of per-adapter cgroup isolation. Use the separate Linux phase recipe for that.

To include Wago and Wasmtime:

```sh
./bin/wasmbench build --runtimes wago --wago-source ../../Wago/wago
./bin/wasmbench build --runtimes wasmtime
./bin/wasmbench run --runtimes wago,wazero,wasmtime,v8 --out runs/four
```

Wago builds in this project's directory against your existing checkout. It does
not edit the Wago checkout. Its identity includes the commit and a digest of
tracked Go/assembly sources. Wasmtime pins its crate version and Cargo lockfile.
`wazero-interpreter` and `wasmtime-winch` are distinct runtime configurations.

### Controlled V8 compiler modes

`v8` keeps its production-default compiler behavior. Two separate optional
configurations provide eager controlled compilation: `v8-liftoff-only` and
`v8-optimizing-only`. Their exact flags disable lazy compilation and tier-up;
baseline-only additionally disallows optimizing fallback. These are not aliases
for production-default V8 or promises about equivalent feature coverage.

```sh
./bin/wasmbench build --runtimes v8,v8-liftoff-only,v8-optimizing-only
./bin/wasmbench run --suite core \
  --runtimes v8,v8-liftoff-only,v8-optimizing-only \
  --scenarios compile,instantiate,first-call --operations 1 --warmup 0 \
  --out runs/v8-controlled
```

Before workloads, each controlled adapter verifies the exact launch-flag sequence,
refuses `NODE_OPTIONS` overrides, and inspects an independent fixed calibration
module with V8's testing intrinsics `%IsLiftoffFunction` and `%IsTurboFanFunction`.
The probe checks code before its first call, records the module hash, observed
booleans, collector/V8 version and scope in the effective configuration, and
verifies its own result. It never invokes the benchmark workload. Missing
intrinsics or a contradictory result fail rather than trust a requested label.
Node, the adapter, the compiler-mode helper and native dependencies are pinned
under the existing runtime identity/archive contracts.

This calibration verifies the requested compiler mode on that installed build;
it is not per-workload/per-invocation tier coverage, a production tier trajectory,
background-compilation tracing, or proof that all module code is materialized.
`can_observe_tiers` therefore remains false. The standard optimizing-tier
classification does not identify a particular TurboFan/Turboshaft optimization
pipeline. These internal testing APIs are optional and can disappear; unsupported
builds fail explicitly. The normal `v8` configuration does not enable them.

V8's [compilation pipeline documentation](https://v8.dev/docs/wasm-compilation-pipeline)
describes controlled compiler modes. The
[V8 runtime inspection implementation](https://chromium.googlesource.com/v8/v8/+/2e56913c24effdce6a45fc66198e04eea16c87e6/src/runtime/runtime-test-wasm.cc)
defines the testing-intrinsic classification used by the probe. Actual capability
is always tested against the installed pinned binary, not inferred from those docs.

Run the real mode and no-hidden-workload-call integration checks with
`WASMBENCH_V8_COMPILER_MODE_TEST=1 go test ./experiment -run TestV8ControlledCompilerModes`.
The helper checks are `node --test adapters/v8/compiler-mode.test.mjs`.

### V8 workload tier diagnostics

`v8-tier-observed` is a separate diagnostic configuration. It enables only
`--allow-natives-syntax`, verifies installed code-tier inspection on a separate
calibration module, and leaves compiler, lazy-compilation and tier-up flags at
their production defaults. It refuses extra flags and `NODE_OPTIONS` overrides.
The normal `v8` timing configuration stays uninstrumented.

`v8-tier-traced` additionally collects native `v8.wasm` events through the
installed Node inspector, after a separate compilation-category calibration.
Use the same profiling commands below with `--runtimes v8-tier-traced` and new
output directories. The report's **Native Wasm engine events** section provides
all event windows, raw trial evidence and unmodified `.trace.json` downloads.
The trace preserves native PID/TID, phase, timestamps, durations and arguments,
including duplicate metadata and unknown event fields. Native event clocks have
microsecond resolution; collection bounds and the trajectory epoch are exact
decimal nanosecond strings, not floating-point timestamps.

Reports also include `engine-events.parquet` with schema
`native-engine-events-parquet-v1`. Filter `row_kind = 'trial_outcome'` for one
row per profiling trial and `row_kind = 'native_event'` for native events in
delivered order. Outcome rows preserve unavailable, incomplete, unsupported,
unrecorded and untraced-admission coverage. A completed empty collection has
event count zero; absent event evidence has a null count. Native duration zero
and absent duration remain distinct. Collection clocks use exact unsigned
64-bit nanoseconds; native clocks/durations use unsigned microseconds. Native
event JSON retains unknown fields and duplicate metadata. The experiment's
module hash does not attribute events to that module. Report verification
regenerates this versioned export from copied raw evidence, including for a
resealed report. Existing reports remain unchanged and can be verified with
their trusted archived builder.

The native-event viewer also aligns selected call windows, before/after code
inspection brackets and native thread events on one elapsed-time axis. It
subtracts exact clock origins before converting relative values to approximate
pixel coordinates. Calls retain invocation number, diagnostic latency, warmup
and failed outcomes; click or keyboard-activate a marker for exact brackets and
source details. Native phase `X` is an interval; other phases remain unpaired
point markers. Metadata stays in the event table and determines a displayed
thread name only when consistent. Missing metadata is not a background-worker
classification. Overlapping intervals get separate visual tracks without
inferring native nesting. Select later 500-call/event windows to reach all
delivered evidence; changing trials resets both windows. No durations are summed
and code snapshots are not joined into an inferred tier-transition line.

Tracing is process-wide and includes setup, explicit warmup, verification and
flush. It does not reliably attribute every event to the requested module or
function. Do not sum nested/overlapping durations or treat trace delivery as proof
of completed compilation, idle background jobs, code creation/retirement or the
tier executed by a call. A trace has an explicit `collected`, `incomplete` or
`unavailable` outcome. The collector retains a prefix within a 4 MiB byte budget;
traced runs allow at most 10,000 total calls, including warmup. Failures preserve
returned workload evidence; abrupt adapter death may prevent delivery. This
instrumentation is diagnostic and never enters headline performance estimates.

```sh
./bin/wasmbench build --runtimes v8-tier-observed
./bin/wasmbench run --suite core --runtimes v8-tier-observed \
  --profile profiling --scenarios trajectory --operations 1 \
  --samples 1000 --warmup 2 --launches 1 --out runs/v8-tiers
./bin/wasmbench report --run runs/v8-tiers --out reports/v8-tiers
./bin/wasmbench verify-report --dir reports/v8-tiers
./bin/wasmbench serve --dir reports/v8-tiers
```

The current contract supports import-free stateless core modules with i32
arguments and one exact i32 result, without explicit initialization or host,
memory-oracle, checkpoint or density fixtures. Other contracts are unsupported,
not zero. Sacrificial admission uses an uninstrumented first call in a separate
process. Measured trajectories retain one module, instance and exported function;
there is no hidden workload warmup, forced tier-up, or per-invocation RPC.

Each invocation records before/after code-state readings (`uncompiled`,
`liftoff`, `optimizing`, or `unavailable` with a reason), their monotonic clock
brackets, the call bracket, module/export identity, collector and pinned V8
version. Sequential intrinsic queries are **non-atomic**. These describe code
present at the exported entry around a call, not the tier executed throughout
that call, internal callees, background compiler activity, exact transition
times, code lifetime or completion of all machine code. Instrumentation affects
the process; call timers remain diagnostic and cannot enter headline rankings.
Absent optimization within a fixed budget is an observation, not a failed test.

Tier records v2 also retain `returned`, `oracle_mismatch`, or `guest_trap` as
the invocation outcome. A mismatch or unexpected trap stops the loop immediately
and returns the completed prefix plus the unverified failed call, its reason,
timer and before/after code readings. The controller validates the prefix against
the locked budget, order, warmup, oracle and failure status before retaining it
as diagnostic evidence. Malformed adapter payloads stay raw-only. Failed trials
never contribute their earlier successful calls to headline latency or throughput.
Abrupt process death or timeout can prevent delivery of an in-memory buffer;
missing observations are not reconstructed. Legacy v1 readings remain supported
without inventing invocation outcomes they did not record.

Open **Explore detailed evidence → Exported-entry code tier boundaries** for
before/after snapshot plots and paged exact tables, including warmup, unavailable
readings, verification/outcome labels, failed recorded traces and raw-trial links.
Red-outlined points identify unverified calls; colors still represent the observed
code state, not execution success. Raw JSON and nullable
`samples.parquet` v7 `tier_window_json` preserve all fields. The export version
changed because this adds a column; older sealed reports retain their original
exports through the explicit trusted `--recorded-builder` verification path.

Run real adapter, sacrificial/controller and resealed-evidence tests with
`WASMBENCH_V8_TIER_TEST=1 go test ./experiment -run Tier`. Protocol and Parquet
checks run in ordinary `go test ./...`; renderer checks use
`node --test publish/report-ui.test.mjs`.

For a Linux Wasmtime binary built on the Docker host architecture, run
`sh recipes/wasmtime-linux.sh`. Run the Linux adapter tests with
`sh recipes/wasmtime-linux.sh test`; these include command correctness and
per-sample descriptor-release checks and independent analyzer tests. The recipe pins its Rust container by digest,
mounts sources read-only, and uses the Cargo lockfile. Its output is
`.wasmbench/wasmtime-linux-target/release/adapter-wasmtime`; mount that binary
at the normal Wasmtime adapter path in a Linux measurement container. The recipe
also builds `wasm-analyze` in the same release directory; the Linux phase recipe
mounts both executables. Building
is separate from measurement; do not run this build on measurement cores during
a performance experiment.

## Expected invocation traps

The `traps` suite exercises unreachable, out-of-bounds memory, integer division
by zero and signed division overflow across all six configurations:

```sh
./bin/wasmbench run --suite traps \
  --runtimes wago,wazero,wazero-interpreter,wasmtime,wasmtime-winch,v8 \
  --scenarios first-call,steady,cold-process --out runs/traps
```

Its oracle is `{"kind":"expected_trap","expected_trap":"unreachable"}`
(or `memory_out_of_bounds`, `integer_divide_by_zero`, `integer_overflow`).
Contracts currently require bare core modules, a no-argument export and
`fresh_instance_per_sample`, without initializers, input writes, host profiles,
return-value/memory oracles, commands or vectors. Only the invocation may satisfy
the oracle: compilation, start-function and export-lookup failures do not.
Normal returns, wrong categories, unknown errors and deadlines never pass.

Each timing sample measures one embedding call through the trap return/unwind,
then classifies/verifies it outside the timer. The compiled module is reused but
the instance is fresh, including steady warmup samples; `--operations` does not
batch traps. Samples retain `trap_result` with category, classifier identity and
runtime diagnostic. Cold-process samples retain this evidence too. Wago and
Wasmtime use typed codes; wazero requires its pinned internal error type and
exact diagnostic; V8 requires `WebAssembly.RuntimeError` plus an exact diagnostic.
These diagnostic mappings must be reverified for new runtime builds.

Add `--profile memory` for trap diagnostics; adapters must advertise
`can_measure_invocation_traps` separately from timing support. Go adapters collect allocator and
GC counter deltas plus heap boundary snapshots over `*/trap_api_window`, after
instance/export preparation and before classification or release. V8 records
JS heap snapshots, not allocation volume. All adapters report guest logical
memory after the trap when observable (Wasmtime/V8 require an exported
`memory`). No GC is forced. Agent process-memory observations still cover the
declared whole request, not just the call. For cold-process runs, call-level
diagnostics remain in `adapter_samples` rather than being relabeled as process
measurements. Timing passes do not collect these memory snapshots.

Trap phase barriers, non-execution scenarios and other trap categories remain
explicitly unsupported. They are not recorded as zero cost.

## Teardown boundaries

`--scenarios teardown` runs one fresh, initialized instance per sample and
verifies its workload before the timed release. Setup, input preparation, and
verification are excluded; warmup and operation batching do not apply.
The runtime's `effective_configuration.teardown_policy` records the release:
wazero closes its runtime, Wasmtime drops its store/module/engine, Wago closes
the instance and compiled module while retaining its host runtime, and V8 only
drops JavaScript instance/module references. V8 does not expose synchronous
engine disposal. None forces GC or promises physical-memory reclamation.
Core scalar and ordered-vector workloads are supported. Vector teardown verifies
every case in order on a fresh instance before releasing its resources. WASI
command teardown is supported by wazero and Wasmtime, with fresh runtime/module
state per sample. It verifies termination and output before releasing remaining
resources. In wazero, `proc_exit` may already close the instance during execution;
this is recorded in `effective_configuration.command_teardown_policy`. Fixture
cleanup and output-buffer reclamation are excluded. Scalar, ordered-vector,
and supported command teardown accept `--profile memory --phase-barriers`:
`before_teardown` occurs after
verification, and `torn_down` occurs after release. OS/cgroup observations cover
that diagnostic window, including barrier transport; adapter timers exclude
the handshakes. Vector and command teardown-barrier capabilities are advertised
separately from compile-barrier support.
These policies must be considered
when comparing results.

For the opt-in Linux cgroup validation recipe, `WASMBENCH_PHASE_SCENARIO`
selects `compile`, `instantiate`, `teardown`, or `app-init`, and `WASMBENCH_PHASE_SUITE` can select a
built-in suite such as `lifecycle`. Otherwise the recipe uses the manifest
mounted from `WASMBENCH_PHASE_SUITE_FILE`. Build Linux adapters first; the
recipe runs only inside an ephemeral private-cgroup Docker container and
validates the saved evidence afterward.

## Build workloads from pinned source

Source builds are separate evidence bundles, not runtime timing runs. The
included freestanding C example uses Homebrew LLVM paths; edit the two tool
paths for your installation **before** locking. Source paths are relative to
the recipe file. Tools must support the recipe's Wasm target and arguments.

```sh
./bin/wasmbench source-lock --recipe recipes/source/xorshift-llvm.json \
  --validation-profile wasm1 --out source.lock.json
./bin/wasmbench source-build --lock source.lock.json --out builds/xorshift
./bin/wasmbench source-verify --bundle builds/xorshift
./bin/wasmbench source-rebuild --bundle builds/xorshift --out builds/xorshift-replay
./bin/wasmbench check --suite builds/xorshift/suite.json --runtimes wazero,v8
./bin/wasmbench run --suite builds/xorshift/suite.json --runtimes wazero,v8
```

The lock pins explicit source inputs, tool executable bytes and invocation
paths, arguments, environment, runner, independent analyzer and validation
policy, license, declared source revision and correctness contract. The source
revision is a supplied label; input hashes establish the actual source identity.
The output bundle retains source snapshots, compiler logs, the exact Wasm,
independent validation, build-host metadata and a runtime-ready suite. Runtime
run bundles retain the build lock as workload provenance; archive the source
bundle too to retain its source snapshots and compiler logs.

Replay uses saved source snapshots, not the original checkout, and requires
the exact runner/tool/analyzer pins at their recorded paths. It fails if the
generated Wasm differs. Offline `source-verify` does not require those tools.
Neither operation establishes workload correctness: `check` and measured runs
must still verify the oracle. A successful build is labeled
`validated_not_correctness_checked`, never a successful benchmark result.

Builds execute **trusted local tools**, without a sandbox. Only the declared
environment plus a private temporary directory and default C locale are passed;
there is no inherited PATH. Dynamic libraries, system headers, compiler resource
files and tool subprocesses are not automatically pinned, so these builds are
not claimed to be hermetic. Include required files explicitly and avoid ambient
configuration where the tool allows it. The example disables Clang's default
configuration and uses no headers or libc. Step wall times are operational
diagnostics, not repeated source-compiler performance measurements. Current
recipes accept core scalar-oracle workloads; broader source-toolchain coverage
and measured source-to-result end-to-end timing remain under development.

`source-compare` compares the runtime behavior of two source-build outputs:

```sh
./bin/wasmbench source-compare \
  --baseline-build builds/o0 --candidate-build builds/o2 \
  --baseline-run runs/o0 --candidate-run runs/o2 \
  --runtime wazero --out reports/source-comparison.json
```

Build each recipe first, then collect each emitted suite with identical runtime
and measurement settings. `recipes/source/xorshift-llvm-o0.json` and
`recipes/source/xorshift-llvm.json` are example `-O0`/`-O2` variants. This command
verifies all four bundles, binds each run to its exact build output and contract,
and requires identical source revision, staged source hashes, workload contract,
validation policy, measurement runner, runtime configuration and host. Tools,
build flags and build environment may differ and remain recorded in both locks.
Different Wasm hashes are allowed only in this explicit comparison track;
ordinary `compare` still requires identical Wasm bytes.

Use the distinct stack track when both the source-build configuration and the
runtime configuration change on the same task:

```sh
./bin/wasmbench stack-report \
  --baseline-build builds/o0 --candidate-build builds/o2 \
  --baseline-run runs/o0 --candidate-run runs/o2 \
  --baseline-runtime wago --candidate-runtime wasmtime \
  --out reports/stack
./bin/wasmbench verify-stack-report --report reports/stack
```

`stack-compare` accepts the same four inputs and runtime IDs for JSON output.
The offline report copies and verifies all four sealed bundles, then links exact
builds, runtime configurations, phase ratios, intervals, coverage and raw data.
It requires an unchanged source task, analyzer/correctness policy, measurement
protocol, runner and host. Build step durations stay separate operational
evidence: the ratios are runtime API measurements, not measured source-to-result
latency or attribution to either changed component.

For a fixed multi-workload source corpus, use `source-compare-set`. Build one
source bundle per workload and compiler variant, keep each workload ID stable
between variants, and collect each complete set into its own runtime run:

```sh
./bin/wasmbench source-compare-set \
  --baseline-builds builds/o0/hash-a,builds/o0/hash-b \
  --candidate-builds builds/o2/hash-a,builds/o2/hash-b \
  --baseline-run runs/o0 --candidate-run runs/o2 \
  --runtime wazero --out reports/source-set.json
```

The command matches source builds by workload ID, requires identical task
contracts for every pair, and rejects missing, duplicate, or extra run workloads.
Its output retains every verified source build and the per-workload comparison;
it does not collapse unlike workloads into a synthetic aggregate. Comma-separated
build paths must not themselves contain commas.

Use `source-compare-set-report` with the same inputs and `--out` to create a
portable offline site. It recomputes the comparison from sealed inputs, copies
and re-verifies each build and runtime bundle, and seals `index.html`, `data.json`
and their checksum manifest together. The page shows per-workload effects,
interval/coverage status, locked runtime configurations, and links to copied raw
evidence; no server or network is required.

Verify the report seal, all included build/run bundles, their exact workload
bindings, and analysis recomputed from those copied inputs with:

```sh
./bin/wasmbench verify-source-set-report --report reports/source-set
```

The JSON includes both source builds, runtime configurations, observed outcomes,
coverage, latency ratios and independent-launch bootstrap intervals. Matching
block numbers in separately collected runs are not treated as pairs. Failed or
unsupported outcomes remain visible; they are not scored as successful samples.
The single-workload command remains available. Neither command measures source
compiler speed or attributes a difference causally to a particular flag. Build
timings remain diagnostics. Dedicated source-comparison website views and
cross-workload aggregation are still under development.

For repeated **source compiler** wall-time measurements, use the separate
build benchmark rather than comparing one-off build diagnostics:

```sh
./bin/wasmbench source-lock --recipe recipes/source/xorshift-llvm-o0.json \
  --validation-profile wasm1 --out source-o0.lock.json
./bin/wasmbench source-lock --recipe recipes/source/xorshift-llvm.json \
  --validation-profile wasm1 --out source-o2.lock.json
./bin/wasmbench source-bench --locks source-o0.lock.json,source-o2.lock.json \
  --runtimes wazero,v8 --blocks 6 --warmup 1 --seed 42 --out builds/compiler-bench
./bin/wasmbench source-bench-report --bundle builds/compiler-bench \
  --out reports/compiler-bench.json
```

Each variant first gets a sacrificial build and correctness run on every
requested runtime. Unsupported or failed admission stops measured trials.
Variants then run sequentially in randomized blocks, with a fresh scratch
directory and fresh tool processes per build. Every output must match that
variant's correctness-admitted artifact exactly. Warmup blocks, failures,
source snapshots, logs, locks, validation and correctness bundles are retained.
The first declared variant is the comparison baseline.

The metric `source.build.tool_wall` is the sum of tool-step process wall times
per complete build recipe. It includes process startup, output capture and wait;
it excludes source staging, hashing, independent validation, correctness checks
and gaps between steps. This is neither compiler-internal CPU time nor whole
pipeline elapsed latency. Individual steps are not independent replicates.
The report includes build medians/means/dispersion and complete-build bootstrap
intervals, plus paired ratios for common successful randomized blocks. At least
three builds/pairs are required for intervals. Warmup observations are excluded
from these statistics but remain visible in the raw evidence and outcome counts.

`source-bench-report` verifies nested evidence, schedule, artifact identity,
correctness admission and timing arithmetic before analysis. Failed experiments
remain sealed and reportable, while the run command returns an error. Existing
bundle/report paths are never overwritten. The current build benchmark is
trusted-local and exploratory: no memory collector, enforced resource
budget, cache control, pilot qualification or official publication is implied.

Use `--profile cpu` for a separate compiler CPU accounting pass, with the same
source locks and correctness requirements:

```sh
./bin/wasmbench source-bench --locks source-o0.lock.json,source-o2.lock.json \
  --profile cpu --runtimes wazero,v8 --blocks 6 --warmup 1 --seed 42 \
  --out builds/compiler-cpu
./bin/wasmbench source-bench-report --bundle builds/compiler-cpu \
  --out reports/compiler-cpu.json
```

The CPU pass records user, system and combined CPU time for each exited tool
using [Go process-state accounting](https://pkg.go.dev/os#ProcessState.UserTime).
Its scope is **OS wait accounting**, not guaranteed complete process-tree or
cgroup coverage: descendant inclusion depends on OS accounting and child wait
behavior. It includes tool startup/shutdown but excludes the controller,
independent validation and runtime oracle processes. Values are serialized in
nanoseconds without claiming nanosecond accounting precision. Every reading
carries collector/version, scope, unit, accuracy and availability metadata.

CPU reports select `source.build.wait_cpu` and use complete-build CPU totals
for statistics and paired ratios. Wall durations remain raw diagnostics, not
headline latency from the CPU pass. Missing CPU values remain unavailable,
not zero; a real zero remains valid. Timing-only builds do not collect these
CPU fields. Replay preserves the selected profile. This is not compiler-pass
attribution, hardware-counter collection, CPU-affinity enforcement or a
replacement for full cgroup accounting.

Create an offline interactive source-compiler report from any sealed benchmark,
including failed-admission evidence:

```sh
./bin/wasmbench source-bench-html --bundle builds/compiler-cpu \
  --out reports/compiler-cpu
./bin/wasmbench serve --dir reports/compiler-cpu
```

The report selects wall time, OS wait CPU or maximum-step cgroup bytes according
to the collection profile. It includes intervals, paired comparisons, admission
outcomes, variant/warmup/failure filters, tool logs, pinned recipe details and
replay guidance. All raw evidence is copied and re-verified; the complete report
is sealed. Output must be a new directory outside the source bundle. It runs
offline without external assets. Failed builds display no performance sample;
failed admission does not imply any planned measurement blocks actually ran.

Source HTML reports also include `compiler-builds.parquet` and
`compiler-steps.parquet` (`source-build-tables-v1`). The build table has separate
nullable wall-time, wait-CPU and maximum-step-memory columns; group by
`config_sha256` and `profile` rather than pooling unrelated experiments.
Admission rows have `stage = 'admission'`, null block/aggregate values and
`not_a_measurement` availability. Trial rows retain warmups and every failure.
Step rows are diagnostics, not independent statistical samples. Their
`step_evidence_json` retains complete collector, scope, quality, availability,
resource-policy readback and context-error metadata. OOM is null when there is
no isolation evidence, rather than an invented false observation.

Run [analysis/compiler-builds.sql](../analysis/compiler-builds.sql) in DuckDB from
the report directory for profile-separated summaries that retain failure counts.
The query excludes warmups and admission, but does not filter out failed trials.

On Linux, tool steps can use delegated cgroup v2 resource limits. Add
`--cgroup-parent /absolute/delegated/path` and optional `--memory-max` (bytes),
`--no-swap`, `--cpu-quota-us` (per 100000 microseconds), `--cpus`, `--mems` and
`--pids-max` to `source-bench`. Each tool starts inside a fresh cgroup; setup
failure never falls back to an unrestricted launch. Limits apply to each tool
and its descendants, not to controller staging, validation or runtime oracles.
Ancestor limits still apply, and CPU affinity does not reserve exclusive CPUs.
Timeout and normal-exit cleanup kill remaining tool descendants.

Use `--profile memory` with `--cgroup-parent` for a dedicated compiler memory
pass. It records current charged memory and fresh-cgroup lifetime peak after
each tool exits, before descendant cleanup. Reports use
`source.build.max_step_cgroup_peak`: the **maximum of step peaks**, not their
sum, a simultaneous whole-build peak, process RSS or heap allocation volume.
File-cache charge ownership can differ between steps. All step peaks must be
available for a build summary; missing readings are not zero. Memory reports
use byte-valued fields, leaving nanosecond summary fields null. Wall durations
remain raw diagnostics. Replay preserves the resource policy and profile;
offline verification checks requested limits against saved local readbacks.
This profile requires Linux cgroup delegation; macOS does not silently substitute
another memory metric. The source compiler CPU pass above remains OS wait
accounting even when resource limits are enabled.

Replay a complete compiler experiment from its archived source snapshots:

```sh
./bin/wasmbench source-bench-replay --bundle builds/compiler-bench \
  --out builds/compiler-bench-replayed
./bin/wasmbench source-bench-report --bundle builds/compiler-bench-replayed \
  --out reports/compiler-bench-replayed.json
```

Replay preserves the configuration, variant order, seed, repetition budget,
warmup policy and timeouts. It requires the original pinned runner, tools,
analyzer and correctness adapters, plus a matching host fingerprint. It does
not read the original source checkout. Every variant is rebuilt from its saved
admission snapshots, must reproduce the admitted artifact exactly, and receives
new sacrificial runtime correctness checks before measurement. The new bundle
records the parent checksum-manifest hash, configuration hash and artifact
hashes; timing samples are newly collected and are not expected to match.
Failed correctness admission cannot be used as a replay baseline. There are no
flags to silently override locked replay settings, and existing output bundles
remain untouched.

All output paths must be new. Failed builds retain available logs and input
snapshots, remove their owned scratch directory, and remain unsealed. The total
tool/validation deadline defaults to one minute (`--timeout`); each output
stream is limited to 1 MiB. Linux and macOS are supported build hosts.

Source benchmark bundles additionally retain partial build results for failed
admissions, including attempted steps, CPU/resource observations and OOM flags.
The enclosing benchmark seal covers those diagnostics and logs; it does not
turn an incomplete build into a validated artifact or a performance sample.
Offline verification checks step order, log presence, profile metadata and OOM
classification. Earlier bundles without admission receipts remain readable,
but cannot supply observations that were never captured.

Failed tools with an observed expired deadline are classified as `timeout`;
explicit context cancellation is `canceled`. The final attempted step retains
`context_error` (`deadline_exceeded` or `canceled`). These labels describe the
context observed when the failed command returns, not proof that the context
was its sole failure cause. Kernel OOM evidence takes precedence. Ordinary
nonzero exits remain `build_failed`; unattempted measured trials remain
`not_run`. All these outcomes stay out of performance aggregates. This tool-step
classification does not relabel independent analyzer or runtime-oracle failures.

## Compare a runtime across recorded versions

```sh
./bin/wasmbench history --runs runs/baseline,runs/version-2,runs/version-3 \
  --runtime wazero --out reports/history.json
```

Inputs are checksum-verified bundles in your explicitly chosen order. The first
is the fixed baseline; timestamps and version labels do not imply commit ancestry.
The report retains absolute timing summaries, independent-launch comparison
intervals, outcome counts, manifests and checksum-manifest hashes. Different
hosts, protocols or profiles stay visible as incomparable points; changed or
missing workloads retain their per-cell reasons. No gap is interpolated and no
history-wide aggregate silently selects only successful workloads. The baseline
must be a timing measurement containing the requested runtime, and duplicate
run IDs are rejected. The output path must be new.

For a portable interactive report with workload/scenario selection and ratio
interval plots:

```sh
./bin/wasmbench history-report --runs runs/baseline,runs/version-2,runs/version-3 \
  --runtime wazero --out reports/history
./bin/wasmbench serve --dir reports/history
```

The report regenerates analysis from verified inputs and includes copies of all
raw bundles, trial JSON, configurations and checksum manifests. It re-verifies
copied bundles and seals the complete output. The output directory must be new
and outside every input bundle. It works offline without remote assets; plots
never connect missing values. Baseline self-comparison is not presented as a
measured interval. Each run provides absolute timing, outcomes, raw evidence
and reproduction guidance.

This is not an automatic regression bisector. Observed differences do not prove
a compiler change caused them; multiple-comparison correction and official
qualification are not supplied by these commands.

For a portable two-run comparison site, select the runtime configuration from
each run independently:

```sh
./bin/wasmbench compare-report \
  --baseline-run runs/baseline --candidate-run runs/candidate \
  --baseline-runtime wazero --candidate-runtime wasmtime \
  --out reports/runtime-comparison
./bin/wasmbench verify-compare-report --report reports/runtime-comparison
```

The report recomputes analysis from verified bundles, copies and verifies both
runs, and presents per-workload/scenario ratios, uncertainty, launch/outcome
coverage, locked configurations, raw manifests, and a reproduction command. It
does not invent a whole-suite score or hide failed/unsupported cells.

### Floating-point correctness

`float_bits_v1` compares `f32`/`f64` results numerically while retaining lossless
IEEE bit patterns in the existing decimal-string result encoding:

```json
{
  "kind": "float_bits_v1",
  "expected": ["4599075939470750515"],
  "float": {
    "types": ["f64"],
    "absolute_tolerance": 1e-15,
    "relative_tolerance": 1e-12,
    "nan": "reject",
    "signed_zero": "match"
  }
}
```

This example expects the bits of `0.3`. Finite values pass if the absolute
difference is within the absolute tolerance **or** the difference divided by
`max(abs(actual), abs(expected))` is within the relative tolerance. Tolerances
must be finite and nonnegative. F32 values are decoded as F32 and widened to F64
for comparison; their high 32 wire bits must be zero. Actual function result
types must match the declared types. All results must pass.

NaNs never match finite values. `nan: "any_nan"` permits any NaN payload only
where the expected value is NaN; `reject` disallows expected NaNs. Infinities
must match exactly, including sign. When both values are zero, `signed_zero`
must explicitly choose `match` or `ignore`; tolerance does not override `match`.
Use `exact_u64` for strict bitwise comparisons instead.

```sh
./bin/wasmbench check --suite floats --runtimes wago,wazero,wazero-interpreter,v8,wasmtime,wasmtime-winch
./bin/wasmbench run --suite floats --runtimes wago,wazero,wazero-interpreter,v8,wasmtime,wasmtime-winch \
  --scenarios compile,instantiate,first-call,steady --out runs/floats
```

All six runtime configurations advertise this contract for timing and
unbarriered memory passes (also cold-process first invocation). Stateless timing
trajectories are supported, including retained warmups and per-call verification.
Compile, instantiation and teardown also support `--profile memory --phase-barriers`,
advertised through `can_float_phases`. Teardown additionally advertises
`can_float_teardown` and supports timing and unbarriered memory passes. Compile/
instantiation verification follows the completed-phase snapshot; teardown verifies
before its release boundary. All verification remains outside the API timer.
Release policies remain engine-specific (V8 drops handles without forcing GC).
Other float lifecycle barriers are not supported yet. V8 extracts numeric function signatures
from the exact module bytes and decodes typed I32/I64/F32/F64 arguments; its
signature reader rejects GC type encodings and memory64 imports rather than
guessing. The helper source is included in the runtime's locked file hashes.
Observed JS Number results are re-encoded as IEEE bits outside timing; preserving
guest NaN payloads across that embedding boundary is not claimed. Unsupported
configurations stay visible. The built-in seven-case `floats` suite is a
correctness/mechanism fixture, not a representative numerical performance suite.

### Native code-image evidence

Wago's dedicated `code` pass preserves the complete native image returned by
`Compiled.WriteCodeTo`, separately from serialized artifact size:

```sh
./bin/wasmbench build --runtimes wago --wago-source ../../Wago/wago
./bin/wasmbench run --suite core --runtimes wago --profile code \
  --scenarios compile --launches 1 --out runs/native-code
./bin/wasmbench inspect --run runs/native-code --workload algorithms/sum \
  --artifact-evidence
./bin/wasmbench export-code --run runs/native-code --out reports/native-code
```

Each successful trial's optional `code_image` contains base64-encoded bytes,
the Wasm SHA-256, image SHA-256, architecture, backend, format and snapshot
semantics. The normal bundle seal protects the image; offline inspection checks
its digest and module identity. No adapter executable is needed to inspect it.
The image can include wrappers, padding and embedded data. Wago exports version 1
without function attribution. Wasmtime Cranelift and Winch export version 2 with
engine-reported function indices, names when available, text-relative ranges and
fixed backend generation zero. The offline report exposes those ranges in a
function table. `native.function_range_bytes` sums their lengths, which can include
constants and padding; it is not `native.guest_code` instruction-only size.
Where sealed `core-structure-v3` analyzer evidence provides exact full function
indices and body sizes, the report shows each native range's expansion relative
to its encoded Wasm body (locals and operators, excluding the size prefix).
`native-function-expansion-v1` preserves exact numerator/denominator bytes and
the derived ratio, counts functions/range bytes by fixed backend generation,
and links to the independent input evidence. Missing/legacy mappings stay
unavailable. The image remainder is unclassified—not inferred trampoline,
metadata or executable-mapping-capacity bytes. `verify-code` recomputes these
derived values from the copied sealed run.
Tier lifetime events, relocations and instruction-only attribution are **not**
inferred. This is a retained
compiled snapshot, not total code emitted over the process lifetime.

Images above 16 MiB are explicitly unavailable to leave headroom in the control
transport; they are never silently truncated. Existing adapters without code
export remain usable at their supported diagnostic level. These code-pass
records are not headline timing measurements.

`export-code` works entirely offline and writes a new sealed directory containing
a navigable `index.html`, `native-code.json`, non-executable `image-NNNNNN.bin` files and the complete
original bundle under `raw/`. The manifest maps every trial to its exact binary
and metadata, preserving failed, missing and uncollected exports without zero
placeholders. Binary filenames are generated independently of workload/trial
names. Existing output directories and paths inside the input bundle (including
symlink aliases) are rejected. The command does not load or execute native code.
These raw images are not object files; they lack symbols and relocation records.
Open the exported `index.html` or serve the directory with `wasmbench serve`.
Run `wasmbench verify-code --dir reports/native-code` to check the outer seal,
copied run, trial-to-image mapping, exact image bytes and rendered page. For a
disassembly export, this also checks synthetic ELF byte identity; the LLVM
listing itself remains a sealed tool-produced diagnostic, not independently
re-disassembled by the verifier.

For an offline assembly listing with recorded LLVM provenance:

```sh
./bin/wasmbench disassemble-code --run runs/native-code \
  --out reports/native-disassembly \
  --llvm-objcopy /path/to/llvm-objcopy --llvm-objdump /path/to/llvm-objdump
```

This creates synthetic ELF64 objects for ARM64 or AMD64, independently verifies
that each `.text` section is byte-identical to its raw image, and retains the
object, assembly text, tool logs, executable hashes, versions and argument lists.
The wrapper is not the runtime's original object file. Addresses are relative
to image offset zero; generated binary symbols are **not** Wasm function names.
The linear disassembler may decode embedded data as instructions. Version-2
disassembly reports also decode each engine-reported function range separately,
using the complete text object with explicit start/stop offsets rather than
rebasing slices. The report embeds expandable assembly by Wasm index and backend
generation, and saves each listing, exact argv and tool log beside the complete
image. Function listings have a cumulative 64 MiB budget per image. The verifier
checks range/argument/coverage identity and embedded listing byte equality; it
does not independently rerun LLVM. Legacy images retain only whole-image listings.
These listings do not recover relocations or infer instruction-only sizes,
tier transitions or spill counts. Details follow
LLVM's [binary input](https://llvm.org/docs/CommandGuide/llvm-objcopy.html#binary-input-and-output)
and [disassembly](https://llvm.org/docs/CommandGuide/llvm-objdump.html) interfaces.

Compare generated code from two verified exports (or two configurations in one):

```sh
wasmbench compare-code \
  --baseline-report reports/native-disassembly \
  --candidate-report reports/native-disassembly \
  --baseline-runtime wasmtime --candidate-runtime wasmtime-winch \
  --out reports/code-comparison
wasmbench verify-code-comparison --dir reports/code-comparison
```

The portable offline report copies both sealed exports and joins compile snapshots
by workload, block and full Wasm function index. Exact artifact/executable-contract,
host, resource and protocol identities must match for a code comparison; changed
workload contracts remain explicitly incomparable. Independent launches are never
collapsed into a representative snapshot. Missing exports and unmatched functions
remain visible. Expand a function for exact range-size delta, byte equality,
first differing function-relative byte offset and side-by-side assembly.
Offsets, backend/generation, range hashes and original evidence remain accessible.
The verifier recomputes each result and the rendered page from the copied reports.
This does not normalize relocations, infer semantic equivalence or predict a
performance effect, and no code-pass timer is treated as latency evidence.

Tool calls have a recorded deadline (`--tool-timeout`, default 30 seconds) and
64 MiB limit per output stream. A tool failure or overflow fails the command
without sealing a successful export; partial output remains for inspection.
The explicitly chosen local LLVM tools are trusted executable programs; their
dynamic libraries are not automatically pinned. No native image is executed.

### Within-launch timing stability

Timing summaries include per-trial `warmup_diagnostics` for `steady` and
`trajectory`. The versioned `post-warmup-thirds-v1` screen uses **all** declared
post-warmup observations, split into three chronological, approximately equal
windows (at least 15 observations total). Median spread above 10% of the full
sequence median flags drift; median absolute deviation above 10% flags high
variability. These are descriptive thresholds, not significance tests or proof
of convergence. Quiet launch medians cannot override detected within-launch drift.

Short sequences, failed trials, invalid samples and zero timer medians receive
explicit unavailable/insufficient statuses. Warmup observations remain in the
raw evidence. No samples are removed and no faster interval is selected for the
reported performance estimate. `no_drift_detected` does not establish a steady
tier, rule out later transitions, or detect every periodic/late change. Batch
observations describe batch averages, not individual-request latency.

## Experimental counters

For sampled interpreter CPU stacks and raw pprof downloads, see
[the profiling workflow](../docs/PROFILING.md). Profiling is a separate diagnostic
pass; its timers do not enter headline latency summaries.

For experimental Linux perf collection, see [the counters workflow](../docs/PERF.md).
Wago, wazero, Wasmtime and V8 compile/instantiate passes support sealed per-CPU evidence with explicit
unavailable and multiplexed statuses. Successful workload execution does not
imply counters were available; positive PMU collection remains unqualified.

## Instance density

```sh
./bin/wasmbench run --suite density --runtimes wago,wazero,wazero-interpreter \
  --scenarios density --launches 3 --samples 3 --out runs/density-timing
./bin/wasmbench run --suite density --runtimes wago,wazero,wazero-interpreter \
  --scenarios density --profile memory --phase-barriers \
  --launches 3 --samples 3 --out runs/density-memory
```

The suite holds 1, 4, 16 or 64 fresh instances simultaneously, comparing one
shared compiled module against independently constructed engines/modules.
Separate curves cover untouched guest memory and 65,536 written/read bytes per
instance. Each sample is one complete group, with no warmup (the ordinary
operation/warmup flags do not multiply density groups). Admission verifies a
complete group in a sacrificial process.

Wago uses its runtime-aware `Runtime.Compile`/`Runtime.Instantiate` API for this
scenario, recorded in the adapter's density configuration. Release waits for
`Runtime.CloseContext` and closes compiled modules. Wazero closes each runtime.
These are explicit embedding/lifecycle choices, not identical internal engines.
Wasmtime Cranelift and Winch also support this suite (`--runtimes
wasmtime,wasmtime-winch`). Each instance owns a Store; release drops all Stores,
then Modules, then Engines. Separate-engine mode constructs and compiles once
per instance. Neither Wasmtime mode forces allocator reclamation.
`wasmtime-pooling` adds a distinct Cranelift configuration with Wasmtime's pooled
allocator. Build it with `wasmbench build --runtimes wasmtime-pooling`, then use
`--runtimes wasmtime,wasmtime-pooling` for a density comparison. Its fixed pool
has 128 instance/memory slots, 16 table slots, a 16 MiB per-memory limit and
reservation, and 64 KiB memory guards. The exact policy is locked in runtime
configuration. These limits and code-generation settings differ from the
on-demand defaults, so differences cannot be attributed solely to pooling.
Density samples include fresh pool/engine construction; they are not a warmed
pool-reuse latency measurement. Logical instances remain fresh, not reused
guest state. General workloads exceeding pool limits can fail admission.

Use `--scenarios density-cycle --samples 100` with Wasmtime configurations to
retain engines/modules while repeatedly allocating fresh instance groups.
Each sample times Store/instance construction, initialization and workload calls;
compilation, verification and Store release are excluded. The first allocation
cycle is retained—there is no hidden pool prewarm. Memory barriers observe each
group and its release while the engines/modules remain held. In a cycle-only
run, sacrificial admission checks two cycles. Other adapters explicitly reject
this scenario. It tests allocator-resource reuse, not reuse of guest state or
snapshot restoration; pool resources are constructed afresh for each launch.

Report `data.json` includes `memory_timelines`, one ordered snapshot series per
trial and exact measurement domain. It retains warmup labels, unavailable
points and phase distinctions. `first_last_snapshot_change_bytes` is emitted
only for complete, successful sequences: it includes any retained warmup and is
an endpoint footprint change, not allocation volume or proof of a leak.
Sample indices are ordinal positions, not equally spaced wall-clock times.
Allocation counters and observed/kernel peaks are excluded. The report's memory
snapshot view separates trials and measurement domains, plots gaps without
connecting them, highlights warmup, and links to raw trial evidence. Long
sequences use selectable 500-point windows; every point remains accessible in
the table and JSON rather than being silently downsampled.

The same view includes within-cycle footprint changes for density and
density-cycle runs with phase barriers. `density_footprints` in `data.json`
joins before/ready/released snapshots only within one launch and exact collector,
scope, quality and metric definition. It reports signed ready-minus-before,
released-minus-ready and released-minus-before bytes. Missing, ambiguous or
unverified boundaries leave all three changes unavailable for that cycle;
unsupported collectors are not zero-filled. Negative changes mean smaller
observed footprints, not proof of allocator reclamation. These differences are
neither allocation volume nor rates, and no forced GC is implied. The report
retains the phase names, analysis version and links to raw trial evidence.

V8 (`--runtimes v8`) supports shared-module groups only. Its existing process
engine is reused, internal code caching is uncontrolled, and release drops JS
references without forcing GC. These differences are recorded in its density
policy; its timer does not include engine construction. Separate-engine cases
remain explicitly unsupported rather than becoming repeated compilation in
the same engine.

Timing includes engine construction, compilation, instantiation/start,
initialization/input and workload calls; verification and release are excluded.
Memory boundaries are before provisioning, while the verified group is held,
and after engine closure. Closure does not imply physical-memory reclamation.
Group logical memory is not resident memory or runtime overhead. Linux is
required for the existing process/cgroup collectors; unavailable metrics remain
explicit. Other runtime adapters and pooled/restored instances are not implemented
yet. The report's scaling section shows the full curve and signed marginal-cost
plot, with intervals, paired-block counts and missing-data statuses. Its curve
selector includes the generator policy to distinguish sharing and touch modes.
Report `data.json` includes
`scaling[].marginal_costs`: finite differences between adjacent declared sizes,
paired by run block after reducing each launch to its sample median. Values are
per added instance (or other scaling-axis unit), not allocation volume or a
linear extrapolation. Negative differences remain visible; missing endpoints
are not bridged. Confidence intervals require at least three paired blocks.

## Use Wago's corpus

Headline latency is restricted to measurement bundles locked to the `timing`
profile, with matching trial labels, no phase barriers, and satisfied locked
host requirements. Memory, code, counter, and profiling passes retain their raw
timers and execution outcomes but do not produce headline latency estimates or
paired latency ratios. Successful launches and latency-eligible launches are
reported separately. Scaling wall-time and break-even analysis use the same
eligibility policy.

Analysis `cluster-median-bootstrap-v5` and sample export
`sample-evidence-parquet-v2` make this distinction explicit. Parquet rows include
`latency_eligible`, `latency_status`, and `latency_reason`; use the eligibility
column for latency queries, not merely a successful execution status. Regenerate
reports into a new output directory to apply this policy to an existing sealed
bundle without changing its raw evidence or previous report.

Workload details also report **useful-work throughput** for first-call, steady,
and trajectory timing scenarios. Each launch's rate is its total declared work
units divided by its total measured duration; the report summarizes independent
launch rates with a median and (from three launches) a bootstrap interval.
This is throughput over the timed regions, excluding setup, verification, and
inter-sample gaps—not sustained service capacity. Units come from the locked
workload contract, never from an inferred input size. Invalid measured samples,
duplicate blocks, missing work contracts, and zero total duration withhold rates.
Raw integer totals are accumulated without overflow before conversion to a
display rate. `data.json` includes the versioned `throughput` dataset; there is
no cross-workload aggregate combining unlike work units.

`throughput.parquet` contains one row per measured trial, including excluded
trials with explicit statuses and null rates. Version 2 of the throughput
analysis also retains exact decimal operation, work-unit, and elapsed-nanosecond
totals in JSON. They are strings because totals may exceed 64-bit integers;
do not round them through JavaScript numbers or SQL DOUBLE for exact checks.
From a generated report directory, run DuckDB with
`.read /path/to/wasm-bench/analysis/throughput.sql` to summarize launch rates and
display exclusion coverage separately. The query does not compute intervals;
those are supplied by the versioned JSON analysis.

For pilot-selected fixed repetition budgets, see [the pilot workflow](../docs/PILOTS.md).
`pilot-plan` preserves all cells and uncertainty; `pilot-run` executes a separate
locked confirmation run. Neither command confers official publication status.

```sh
./bin/wasmbench import-wago --source ../../Wago/wago \
  --ids tiny,fib_rec,dispatch,matmul,sha256 --out wago-suite.json
./bin/wasmbench check --suite wago-suite.json --runtimes wago,wazero,v8
./bin/wasmbench run --suite wago-suite.json --runtimes wago,wazero,v8
```

The importer verifies catalog SHA-256 hashes, retains the original contracts and
source metadata, and uses fresh instances per sample. Direct return-value and
absolute-memory and exported-output-pointer oracles are supported. The output
pointer is resolved after invocation outside timing, with checked wasm32 offset
arithmetic. Input bytes can be written to absolute or exported-pointer addresses
after initialization and before timing, once per fresh instance. Supported WASI
`_start` commands retain exact exit/output oracles; the versioned
`llvm-ir-preds` stdout normalizer preserves both raw and normalized digests.
Unknown normalizers and unimplemented command/custom-host contracts remain
explicitly unsupported, never reduced to a successful-return check. `--ids all`
imports the complete inventory, including unsupported entries. Artifacts are
copied into the run bundle when a run starts. The source checkout is read-only.

BLAKE3 ordered vectors (`--ids blake3`) run on Wago, V8, Wasmtime Cranelift/Winch
and wazero's compiler/interpreter in the timing profile. Each sample uses a fresh instance and checks
every case in order. Execution samples are `sequence_call_sum`: the sum of
timed guest calls for one complete sequence, excluding input writes, pointer
resolution and digest checks—not an individual request latency. Compilation and
instantiation time their respective APIs and verify the sequence afterward.
Sacrificial checks repeat the sequence on two fresh instances. `cold-process`
instead measures launch through the first correct complete sequence, including
setup, input generation, verification and protocol transport. Its raw adapter
sample is retained separately from the end-to-end process sample. Vector memory
passes collect logical guest memory and available process diagnostics. Go
allocation counters cover the sample lifecycle including instance setup,
verification and release; V8 heap snapshots end with the verified instance
still held. These are not narrow phase allocations or exact memory peaks.
Vector compile-phase barriers are available on all six configurations. They
hold the compiled module before verification and release handles before the
final boundary. Go allocation counters and V8 heap snapshots in these phased
runs cover the compile API window only. V8 garbage collection and engine-cache
reclamation remain uncontrolled; dropping handles does not imply reclaimed bytes.

Plain WASI Preview 1 `_start` commands with exact stdout/stderr hashes run on
wazero's compiler/interpreter and Wasmtime's Cranelift/Winch backends. Import, for example:

```sh
./bin/wasmbench import-wago --source ../../Wago/wago \
  --ids coreutils-sort,icepack-pack,icepll-clock,json2csv-people --out commands.json
./bin/wasmbench check --suite commands.json --runtimes wazero,wazero-interpreter
./bin/wasmbench run --suite commands.json --runtimes wazero,wazero-interpreter \
  --scenarios compile,instantiate,first-call,steady,cold-process
```

After `./bin/wasmbench build --runtimes wasmtime`, the same suite can use
`--runtimes wasmtime,wasmtime-winch`. Wasmtime uses pinned `wasmtime-wasi`
46.0.1, with a new owned WASI context for each sample; dropping its Store releases
the guest filesystem handles. Both implementations enforce the output limit
even when a guest ignores a failed write. Output capture is included in call
timing, while hashing and oracle checks are outside it. Compile and instantiate
scenarios verify the command afterward, outside their respective API windows.
Wasmtime command memory passes support compile barriers and OS collectors;
they do not claim native allocator allocation counts or bytes.

The `wasi-preview1-readonly-v1` host profile pins fixtures by hash and size,
copies referenced files into the run bundle, stages them in a private read-only guest root, passes explicit
argv, and starts each sample with fresh stdin and guest state. It exposes no
ambient environment or directories. Random bytes are zero; synthetic clock
reads advance by 1 ms (wall clock starts at 2022-01-01 UTC). This is an explicit deterministic test environment,
not production entropy. Inline input/metadata is bounded to 1 MiB and referenced
fixtures to 256 MiB per workload; imported stream capture is
bounded to 16 MiB per stream. Exit status and declared stream digests must match.
Stream hashes and byte counts remain in raw samples, including cold-process
adapter samples. Hashing occurs after timing; stream capture is part of the
embedding call. `steady` repeats fresh commands, not a persistent process.
Memory-profile Go counters cover setup, capture, verification and release as a
lifecycle window. Wago/V8 command adapters and writable-file oracles remain
open; only allowlisted output normalization is supported on wazero and Wasmtime.
Wasmtime supports WASI Preview 2
component commands for compile, instantiate and first-call timing and separate
memory passes under explicit read-only or disposable
temporary-filesystem profiles; general component export calls and P2 resource
quotas remain open. See [Component Model support](../docs/COMPONENTS.md).
The separate host-import-free `component-u64-v1` profile supports named exports
with up to sixteen typed `u64` parameters and one exact `u64` result on both
Wasmtime backends. Compile, instantiate and first-call are single-operation,
fresh-instance scenarios with separate timing and memory passes. Memory-only
before/returned/released barriers retain external boundary snapshots and
whole-process peak RSS without instrumenting headline timing. Other WIT types
remain open.

The Wago importer also recognizes Emscripten `main(argc, argv)` command
contracts. Wazero and both Wasmtime backends implement the explicit
`emscripten-stdio-v1` profile: pinned
argv/stdin, bounded stdout/stderr, deterministic clocks and random bytes, and a
fail-closed set of filesystem syscalls. `main` is invoked with marshaled argv
after constructors; Emscripten `exit` is translated into the adapter's exit
status. This profile does not grant a general Emscripten filesystem or browser
environment. Wago does not advertise it yet. All eight Emscripten workloads in
the current Wago catalog pass their exact sacrificial checks on wazero in
`runs/emscripten-wago-v1`; this is correctness evidence, not performance data.
The same eight also passed on Wasmtime Cranelift and Winch in a verified
correctness-only check. Reproduce that ABI slice with:

```sh
./bin/wasmbench import-wago --source ../../Wago/wago \
  --ids sed-records,seqtk-fastq-to-fasta,lcs-substrings,needleman-wunsch-dna,smith-waterman-dna,fasttree-phylogeny,gnu-seq-range,gnu-tr-uppercase \
  --out emscripten-suite.json
./bin/wasmbench check --suite emscripten-suite.json \
  --runtimes wazero,wasmtime,wasmtime-winch
```

Imported command manifests use file references, including `stdin_file` when
stdin comes from a fixture. A run copies them into deduplicated `inputs/<sha256>`
files and records bundle-relative paths. Reproduction reads the bundle rather
than the original corpus checkout. Hashing and staging are streamed and happen
outside invocation timing; cold-process timing includes this preparation.

Both wazero and both Wasmtime backends support command compile barriers with
`--profile memory --phase-barriers --scenarios compile`. Fixture staging and
host import/linker setup precede `before_compile`; `compiled` holds the measured module before
command execution; `released` follows exact result verification and closure of
the instance, compiled module and per-sample stdin handle. In this mode Go
allocation counters cover only the compile API window. The engine and staged
fixtures remain alive across samples, and no forced GC is performed.

Wazero compiler/interpreter and Wasmtime Cranelift/Winch also support command
`instantiate` and `first-call` memory barriers, for both WASI Preview 1 and
`emscripten-stdio-v1`. Instantiation snapshots bracket the instantiate API,
including Wasm start but excluding Emscripten constructors and argv marshaling.
First-call snapshots bracket `_start` or `main`, including host calls and bounded
output capture, before exit/output oracle checking. Prepared imports, WASI
context, capture buffers and stdin handles are outside these API windows.
Each sample uses a fresh instance. The final snapshot follows verification and
instance/Store plus stdin-handle closure, with shared module/engine, staging
root and capture buffers retained. WASI exit may close guest resources before
the returned snapshot; neither closure nor a footprint decrease proves
physical reclamation. Go allocation deltas use the matching API window;
process snapshots and cgroup barrier windows retain their separate scopes.
Unsupported OS collectors remain explicit, never zero.

```sh
./bin/wasmbench run --suite emscripten-suite.json \
  --runtimes wazero,wazero-interpreter,wasmtime,wasmtime-winch \
  --scenarios instantiate,first-call --profile memory --phase-barriers \
  --launches 3 --samples 2 --operations 1 --warmup 0 \
  --out runs/command-lifecycle-memory
WASMBENCH_COMMAND_LIFECYCLE_TEST_RUNTIMES=wazero,wazero-interpreter,wasmtime,wasmtime-winch \
  go test ./experiment -run TestBuiltAdapterCommandLifecyclePhases -count=1 -v
```

For native Linux procfs qualification and both ordinary/archived replay, build
the Linux tools first, then run the network-disabled, unprivileged recipe:

```sh
sh recipes/wasmtime-linux.sh build
# On a native Linux host; for cross-building, use the Docker daemon's GOARCH.
CGO_ENABLED=0 GOOS=linux go build -trimpath \
  -o .wasmbench/wasmbench-linux-command ./cmd/wasmbench
CGO_ENABLED=0 GOOS=linux go build -trimpath \
  -o .wasmbench/adapter-wazero-linux-command ./adapters/wazero
sh recipes/test-linux-command-lifecycle.sh wasmbench:ci linux-command-lifecycle
```

The image must already exist and match the Docker daemon's native architecture.
The recipe pins its immutable image digest, preserves any existing output,
copies the WASI/Emscripten fixtures and source, collects all four configurations
sequentially, and verifies exact boundary/sample coverage and available
RSS/PSS/private/virtual procfs readings. It retains process-lifetime peak RSS
separately and checks Go allocation-window domains. The original run, ordinary
reproduction and relocated archived-tool replay each retain their own seals;
the report is checked with current and archived builders. The wrapper retains
fixture/recipe/image receipts and checksums. No host cgroup mutation, credentials
or network access is granted. This is memory diagnostics, not phase-exact peaks,
cgroup qualification, controlled timing, dedicated-host evidence or a claim of
coverage on an architecture not actually tested.

## Measurement contract

The `assemblyscript-abort-v1` host profile supplies only
`env.abort(i32, i32, i32, i32) -> ()`. Calling it fails the workload; it is not a
no-op compatibility stub. Diagnostics retain unsigned message/file pointers and
line/column values without reading guest strings or accessing host files.
`import-wago` selects this profile for its `workloads/assemblyscript/` family.
Other imports are not supplied, and an initializer name alone does not select
this host profile. The selected profile remains part of the locked workload.

Explicit core-module initialization is a separate `app-init` scenario on all six
runtime configurations:

```sh
./bin/wasmbench run --suite lifecycle --runtimes wago,wazero,wasmtime,v8 \
  --scenarios instantiate,app-init,first-call
```

Each `app-init` sample creates a fresh instance, resolves the declared
`initialize` export, and times one no-argument, void initialization call. The
Wasm start function belongs to instantiation, not this window. Inputs are
installed afterward and the workload's return/memory oracle is checked outside
timing. Warmup is not used, and `--operations` does not turn initialization into
a loop on reused state. Workloads without an initializer are `not_applicable`.
Current support covers core scalar contracts, not command or vector contracts.
Memory passes expose narrow Go allocation windows, V8 heap boundary snapshots,
or Wasmtime logical-memory snapshots without conflating their domains.
Add `--profile memory --phase-barriers` for `before_app_init`,
`app_initialized`, and `app_released` snapshots. The first two bracket the
initializer, before input writes or workload verification. The final snapshot
follows verified execution and instance release, retaining the compiled module
and engine. Each adapter records `app_init_release_policy`; V8 drops references
without forcing GC. Linux peak/CPU observations cover the initialization
barrier window (including diagnostics and transport), not release or execution.

The `lifecycle` suite includes a stateful ordering fixture. Its source is
`corpus/testdata/app-init.wat`; the checked-in binary was generated with WABT
1.0.41 using `wat2wasm corpus/testdata/app-init.wat -o corpus/testdata/app-init.wasm`.

The `reactors` suite adds [WASI Preview 1 reactor experiments](../docs/WASI-REACTORS.md)
on wazero compiler/interpreter and Wasmtime Cranelift/Winch/pooling configurations, including separately timed
`_initialize`, fresh-instance calls and stateless steady batches. Its deterministic
no-I/O host profile is explicit; other adapters retain unsupported coverage.

Core scalar instantiation also supports `--scenarios instantiate --profile memory
--phase-barriers` on all six configurations. `before_instantiate` follows module
compilation and import preparation; `instantiated` follows construction,
including the Wasm start function but excluding explicit initialization and
input writes. After initialization and workload verification, the instance is
released and `instance_released` is recorded. Compiled modules and engines are
retained. Each sample uses one fresh instance, with no warmup or batching.
`instantiate_release_policy` records runtime-specific ownership. V8 drops
references without forcing GC. Wasmtime prepares its store/imports outside the
instantiation window; Wago prepares its host-runtime module handle outside it.
Ordered-vector instantiation barriers are supported by Wago, wazero (compiler and
interpreter), Wasmtime (Cranelift and Winch), and V8. Each sample uses a fresh
instance with the prepared compiled module retained. The API window includes
the Wasm start function but excludes explicit initialization, vector input
setup, ordered calls, output verification, and release. All vector outputs must
verify before the `instance_released` boundary and successful sample are emitted.
Go allocator and V8 heap observations cover the API window, not the full vector
lifecycle. V8 release drops references; it does not establish physical reclamation.
Command lifecycle barriers are described separately above. Native adapter tests
also use `corpus/testdata/vector-initialization.wat` to require exactly one Wasm
start, exactly one explicit initializer, and two ordered vector calls on each
fresh instance. Its binary is generated with WABT 1.0.41:
`wat2wasm corpus/testdata/vector-initialization.wat -o corpus/testdata/vector-initialization.wasm`.

Ordered-vector first-call memory barriers are also available on these six
configurations with `--scenarios first-call --profile memory --phase-barriers`.
`before_first_call` follows initialization, pointer resolution and the first
input write. `first_call_returned` follows the last ordered call, before its
output check; `first_call_released` requires every output to verify and the
instance to be released. The contiguous memory window includes intermediate
output checks and later input writes. It is deliberately broader than the
`sequence_call_sum` timer, which sums only individual calls. Go allocator and
V8 heap observations use `first-call/vector_sequence_call_window` and an explicit
ordered-sequence denominator; they are not labeled as API-only allocations.
Wrong intermediate outputs stop the sequence without a returned boundary;
a wrong final output retains the returned boundary but never qualifies a sample.
V8 reference release does not establish physical reclamation. OS phase collectors
may additionally include diagnostic snapshots and barrier transport.

For native Linux collector qualification, build the tools before measurement
on a Linux host using the image's native architecture:

```sh
sh recipes/wasmtime-linux.sh build
CGO_ENABLED=0 go build -trimpath -o .wasmbench/wasmbench-linux-vector ./cmd/wasmbench
CGO_ENABLED=0 go build -trimpath -o .wasmbench/adapter-wazero-linux-vector ./adapters/wazero
CGO_ENABLED=0 ./.wasmbench/wasmbench-linux-vector build --runtimes wago --wago-source ../../Wago/wago
cp bin/adapter-wago .wasmbench/adapter-wago-linux-vector
sh recipes/test-linux-vector-lifecycle.sh wasmbench:ci linux-vector-lifecycle
```

Use an existing native Linux image with Node and the adapter's required native
libraries; the recipe pins its image ID and refuses emulation. It never
overwrites existing evidence, mounts tools and fixtures read-only, disables
network and privilege escalation, and preserves original/replayed/archived
bundles. Its gate requires both initialization fixtures, all six configurations,
verified ordered outputs, exact boundary attachment, available Linux procfs
snapshots, allocator-domain labels and process-lifetime peak RSS. Reports are
verified with current and archived builders after collection is terminal.
This qualifies procfs boundary snapshots, not isolated cgroup phase peaks,
physical reclamation or publication on a dedicated host.

For separate private-cgroup qualification, build a matching native Linux agent
test executable before collecting, then explicitly opt in to the disposable
privileged-container recipe:

```sh
CGO_ENABLED=0 go test -c -o .wasmbench/agent-cgroup-vector.test ./agent
WASMBENCH_EPHEMERAL_CGROUP_TEST=1 sh recipes/test-linux-vector-cgroup.sh \
  wasmbench:ci linux-vector-cgroup
```

This additional recipe checks same-file-descriptor phase peaks, process-tree
CPU deltas, atomic worker placement and exact resource-limit readbacks across
original, ordinary replay and archived-tool replay. It never binds host cgroups
or credentials, rejects emulated images, preserves existing evidence and verifies
both report builders after collection. Cgroup memory is not RSS; these diagnostic
barrier windows include transport and do not certify exclusive measurement CPUs
or official publication. See [resource qualification](../docs/RESOURCE-VERIFICATION.md)
for privileges, build-host architecture settings and evidence limits.

For native Windows runtime collection/replay, see the separate
[Windows profile](../docs/WINDOWS.md). It preserves explicit Linux collector gaps
instead of substituting zero footprints. Native Windows qualification still
requires successful Windows-host evidence; cross-compilation alone is not proof.

On Linux, use a writable delegated cgroup v2 parent for atomic adapter isolation:

```sh
./bin/wasmbench run --cgroup-parent /sys/fs/cgroup/wasmbench-workers \
  --memory-max 536870912 --no-swap --cpu-quota-us 100000 --pids-max 128
```

The operator must provision the parent and enable the required controllers.
The runner never changes parent controls or moves the controller process.
Each adapter is created directly in a new child cgroup; requested isolation
never falls back to an ordinary launch. `--cpus 2-3` selects allowed CPUs but
does not reserve them exclusively. `--mems 0` requests allowed NUMA allocation nodes before
spawn; it does not measure physical page placement. CPU and NUMA lists inherit
their parent allowances when omitted. CPU quota is per 100000 microseconds, not a
thread-count setting. Memory and swap limits are separate. Requests and local
controller settings are recorded; ancestor limits still apply.

Explicit controls must match kernel readback before spawn and at the final
boundary. Missing or mismatched readback fails the launch or trial, with retained
evidence. See [resource verification](../docs/RESOURCE-VERIFICATION.md) for exact
scope, CPU-set matching, memory alignment and limitations.

Timeout cleanup kills the adapter cgroup including descendants. Kernel OOM
events produce an explicit `out_of_memory` outcome. Memory passes include
cgroup current memory and the peak since cgroup creation, including startup;
these are neither RSS nor phase-reset peaks. Without `--cgroup-parent`, isolation is
explicitly uncontrolled. Cgroup requests on other platforms fail closed.

For compile diagnostics, add `--phase-barriers --profile memory
--scenarios compile`. This uses one operation per sample and pauses before
compile, while the compiled module remains alive, and after verification/release.
Procfs snapshots and same-descriptor cgroup phase peaks are attached to the
sample; the raw phase event sequence is retained even if the trial fails.
The peak window includes barrier transport and background runtime work, but not
verification/release. Go allocation counters use a narrower compile API window,
excluding barrier transport and release. Post-release snapshots do not imply
forced collection or complete reclamation. Missing reset support remains
unavailable, never a lifetime peak substituted as a phase peak. Wago, wazero,
Wasmtime (Cranelift and Winch), and V8 support compile barriers. Other lifecycle
scenarios currently report this mode as unsupported. Timing passes reject
phase barriers. Each adapter describes its release policy: in particular, V8
drops JS handles without claiming that GC or native-code reclamation occurred.

With cgroup isolation, compile barriers also collect total/user/system CPU time
for the adapter process tree and local CPU-quota throttling counters. These
diagnostic windows include barrier transport, not just the compile API; kernel
microsecond counters are converted to nanoseconds without implying finer
precision. Memory boundaries include anonymous, file, kernel, socket, page-table
and slab accounting. These categories overlap and must not be summed. Missing
fields and regressed counters remain unavailable, while genuine zeroes stay zero.

The default quick profile uses six independent launches, ten batches per launch,
100 operations per batch, and three retained steady-state warmup batches.
Override these with `--launches`, `--samples`, `--operations`, and `--warmup`.
Fresh-instance workloads use one operation per sample and record that actual
count. These defaults are a convenience, not an official statistical guarantee.

```sh
./bin/wasmbench run --scenarios compile,instantiate,first-call,steady,cold-process
./bin/wasmbench run --profile memory --scenarios compile,steady
./bin/wasmbench run --profile code --scenarios compile
./bin/wasmbench run --suite scaling --profile timing
./bin/wasmbench metrics
```

The `scaling` suite contains 40 deterministic fixtures: five sizes (1, 16, 64,
256, 1024) for function count, body size, locals, nesting, branch-table entries,
distinct types, data segments, and imports. The imports dimension repeats a
bounded `wasmbench.identity(i32) -> i32` binding; it measures import count, not
host-function diversity. Type fixtures use distinct fixed-arity signatures.
The pinned analyzer independently checks the advertised structural dimensions.
These inputs support scaling experiments; their existence does not establish
an asymptotic performance claim.

Inspect an artifact independently of the runtime under test:

```sh
cargo build --release --locked --manifest-path adapters/wasmtime/Cargo.toml --bin wasm-analyze
./bin/wasmbench analyze --artifact corpus/testdata/app-init.wasm --features wasm1
```

The pinned wasmparser 0.251.0 analyzer emits schema 2: actual imports and their
index spaces, exports, start function, flattened type indices, and each defined
function's module index, parameters, results, locals and body statistics.
`--features` selects `default`, `wasm1`, `wasm2`, or `wasm3`; validation failures
exit nonzero. The output records the selected policy and enabled feature flags.
These flags describe accepted features, not inferred minimal requirements or
proof that a runtime supports the artifact. Component artifacts emit a separate
parent-linked `component-structure-v1` report; see [Component Model support](../docs/COMPONENTS.md)
for the compile-only Wasmtime workload boundary.

Analysis version `core-structure-v3` also records `feature_probes`: for each
enumerated enabled flag, the pinned validator rechecks the artifact with that
flag removed and all others unchanged. Each result includes the removed and
remaining bit masks, success/failure, and the failure message/byte offset.
This is necessity evidence under the selected policy, not a unique minimal
proposal set. Composite flags can overlap, and switches such as `FLOATS` and
`GC_TYPES` are not standalone proposals. Successful removal does not establish
that several flags can all be removed together. Probes run outside measurements.

New `plan`, `run`, and `check` commands require independent validation before
any adapter starts. The default profile is the pinned validator's `default`;
use `--validation-profile wasm1`, `wasm2`, or `wasm3` to select another policy:

```sh
./bin/wasmbench plan --suite core --validation-profile wasm1 --out suite.lock
./bin/wasmbench run --lock suite.lock --out runs/admitted
```

The lock pins the analyzer executable SHA-256, analysis version and policy.
Each distinct artifact gets a complete report under `validation/<sha256>.json`,
covered by the bundle checksums. Loading verifies report identity and policy
without needing the analyzer installed; reproduction requires its exact binary.
Admission failures exit nonzero before adapter startup and may leave an unsealed
partial directory, which cannot be loaded as a completed result. A missing
analyzer fails with build instructions; it is never built during a run. Existing
locks retain their recorded policy, including legacy locks without independent
admission. No `--validation-profile` override is accepted with `--lock`.
For v3 reports, necessary validator flags are matched against explicit adapter
declarations in the same pinned validator vocabulary. A known-disabled flag
produces a visible `unsupported` result for every requested cell; absent
declarations remain unknown and proceed through sacrificial correctness checks.
The wazero compiler and interpreter explicitly pin `api.CoreFeaturesV2` and
declare five excluded experimental feature flags. Wasmtime explicitly applies
and advertises a subset of its pinned default policy, including tail calls and
relaxed SIMD enabled in Cranelift but disabled in Winch. Enabled validation flags
do not promise complete backend instruction support; correctness checks still
run. Wago and V8 retain correctness-based admission without declared feature
policies.
This does not infer support from the older, non-exhaustive `features` list.

Reports expose each workload's admission status, conditional feature requirements,
and a link to its complete sealed analyzer report (types, signatures, imports,
opcode counts and feature-probe witnesses). Legacy locks show `not_recorded`;
v2 analysis remains validated but explicitly lacks feature probes. The same
evidence can be inspected offline without installed runtime/analyzer binaries:

```sh
./bin/wasmbench inspect --run runs/feature-admission \
  --workload features/tail-call --artifact-evidence
```

This output includes the workload contract, analyzer identity, full analysis
and matching trials. Without `--artifact-evidence`, workload inspection retains
the existing trials-only output. All evidence reads verify bundle checksums.

```sh
./bin/wasmbench run --suite corpus/testdata/feature-admission.json \
  --runtimes wazero,wazero-interpreter,v8 --validation-profile default \
  --scenarios compile,first-call --launches 2 --samples 3 --operations 1 \
  --out runs/feature-admission
```

Reports for scaling runs include a curve selector, exact measurement provenance,
launch coverage, raw-trial links, and bootstrap intervals over launch medians.
Different allocator domains, collectors, phases, and batch sizes stay separate.
Descriptive log–log fits require at least three complete positive size points
and three successful launches per point; they are not complexity proofs.

For import fixtures, Wago uses an owned public host-function reference and its
runtime module-binding API. The host runtime/reference is prepared outside the
timed window; module binding and wrapper release are included in instantiation.
Import-free Wago fixtures use the low-level compiled-module API.

Every cell receives a separate sacrificial correctness process before measured
launches. Runtime order is randomized within workload/scenario/launch blocks
using the recorded `--seed`. Each trial has a process deadline (`--timeout`).
Timeouts, incorrect results, unsupported capabilities and preflight failures
remain in the evidence. `check` bundles cannot produce performance reports.

Timing is monotonic elapsed time around embedding operations, including their
API overhead. It is not guest-only execution time. Compilation includes API
validation. Instantiation includes a Wasm start section, while explicit
application initialization is untimed. V8's standard API cannot disable its
engine cache or observe tier changes; that limitation is recorded in its
effective configuration. Do not interpret its compile return as complete
optimized-code materialization.

Memory passes expose Go allocator counters, JS heap snapshots, and logical guest
memory as different metrics. Go counters cover the Go allocator only. The Go
adapters also record completed GC cycles, application-forced GC cycles, and
`host.gc.pause_time` (the `PauseTotalNs` delta in nanoseconds). Pause accounting
is not concurrent GC CPU, request latency, or a pause percentile. No collection
is forced by the collector; a GC spanning a boundary is not clipped to the API
timer. Definitions follow [Go's MemStats contract](https://pkg.go.dev/runtime#MemStats).
OS
collectors currently use Linux `/proc` and are explicitly unsupported on other
hosts. The sampler observes the batch RPC envelope, including preparation and
verification inside the adapter; it is not an exact phase peak. Allocation
counters include untimed verification/release work where labeled. Cgroup phase
barriers and narrow allocator windows remain in the acceptance ledger.

Code passes expose Wago's complete native-code image and serialized artifact as
different quantities. Neither is mislabeled as guest-function-only instruction
bytes. Wasmtime also retains engine-reported compiled function ranges as a separate
measurement, without inferring instruction-only bytes or trampoline sizes from
unattributed text. Missing diagnostics carry a reason rather than a fabricated zero.

## Evidence, analysis and jobs

### Linux host facts

`doctor --irq-cpus 2-3` provides a bounded, read-only
[IRQ-affinity readiness probe](../docs/IRQ-AFFINITY.md). It retains default,
requested and effective device-IRQ masks at two observations. Missing evidence
is explicit; passing does not certify interrupt delivery or dedicated-host control.
Runtime `run`, `check`, and `plan` can lock `--require-irq-affinity` with an explicit
`--cgroup-parent` and `--cpus` budget. Runs refuse non-ready start evidence and
seal diagnostics but withhold estimates when the end observation fails. Replay
preserves and reprobes the requirement; existing locks cannot be overridden.
`source-bench --require-irq-affinity` records and enforces the same outer-boundary
contract for compiler timing, CPU and memory passes and their replay.

`doctor --cpu-partition /sys/fs/cgroup/measurement --measurement-cpus 2-3`
provides a [read-only partition-readiness probe](../docs/CPU-PARTITIONS.md), including
all controller threads and online SMT siblings. It changes no host settings and
does not certify continuous isolation or official publication readiness.
`--require-isolated-cpu-partition` pins start/end readiness as a requirement for
runtime runs and source benchmarks, using their `--cgroup-parent` and `--cpus`.
Runtime trials also retain read-only occupied-partition samples while the adapter
runs. Failed observations retain diagnostic evidence but exclude performance
samples; sampling does not prove uninterrupted isolation or dedicated-host control.

Official results require a separate [publication evidence audit](../docs/PUBLICATION.md).
Run `wasmbench publication-check --run runs/ID` to inspect each requirement.
An `official` manifest label never overrides missing evidence; dedicated-machine
qualification now has an operator-signed, separately trusted verification path.
`publish --qualification operator-signed.json --qualification-key /trusted/operator-public.json`
archives the sealed pilot, confirmation, signed statement and recomputable audit.
Readers authenticate with `verify-report --dir report --qualification-key /trusted/operator-public.json`;
ordinary checksum verification does not establish independent operator trust.
Local synthetic acceptance tests do not certify a dedicated host.

Use `wasmbench host-policy --out host.json`, then add `--host-policy host.json`
to `run`, `check`, or `plan` to pin an observed host baseline. Both run boundaries
must match for measurements to remain eligible. This does not certify machine
control; see [observed host baselines](../docs/HOST-BASELINE.md) for scope and limits.
`source-bench --host-policy host.json` applies the same boundary checks to source
compilation benchmarks, and source benchmark replay preserves the pinned policy.

`doctor` and new run manifests include a versioned `host.fingerprint` on Linux.
It records source paths, availability and raw values for per-CPU identity,
features/microcode and topology; CPU online/isolated sets; CPUFreq policy drivers,
governors and limits; SMT settings; NUMA CPU lists/distances; controller-allowed
CPU/node lists; total memory/swap; and VM/huge-page policies, including exposed
per-size THP settings. CPU feature lists remain per-record to preserve
heterogeneous cores. Values retain kernel units/formats rather than guessing
normalized meanings. Missing files and denied reads are explicit, not zero.

These are read-only settings observed in the **controller's namespace at
manifest creation**, not proof of enforced policy, actual NUMA placement,
adapter affinity, constant frequency or host isolation. Containers may hide
settings. Requested/effective adapter cgroup limits remain separate. This does
not enable official publication or establish that settings remain unchanged
during the run. Frequency samples, free memory, PID and other transient counters
are excluded from identity; the CPU description no longer embeds raw cpuinfo.

Cross-run comparisons require matching fingerprints, including collector
version and availability. Legacy bundles still load, but their missing host
facts are not backfilled or treated as equivalent to a newly collected Linux
fingerprint. Other operating systems do not receive Linux-derived facts.

Kernel interfaces: [CPU topology](https://docs.kernel.org/admin-guide/cputopology.html),
[CPUFreq policy](https://docs.kernel.org/admin-guide/pm/cpufreq.html), and
[transparent huge pages](https://docs.kernel.org/admin-guide/mm/transhuge.html).

### Fixed workload-set aggregates

```sh
# Create an explicit proposal from a reference bundle, then review and freeze it.
./bin/wasmbench aggregate-set --run runs/reference --id my-suite-v1 \
  --scenario compile > my-suite-v1.json
./bin/wasmbench aggregate --run runs/comparison --set my-suite-v1.json \
  --baseline wazero --candidate v8 > aggregate.json
./bin/wasmbench aggregate-report --run runs/comparison --set my-suite-v1.json \
  --baseline wazero --candidate v8 --out reports/aggregate
```

The proposal includes every workload, groups by corpus family, and assigns equal
category weights. Edit the categories/positive weights deliberately before using
the set for comparisons; workloads divide their category's weight equally. Each
member pins its complete workload contract except the artifact's transport path.
Inputs, result oracle, artifact hash, reset policy and provenance remain pinned.
Generating a set from already observed results does not establish preregistration.

For each complete block, analysis computes a weighted geometric mean of
candidate/baseline launch-median ratios, then reports the median block aggregate
and a 4,000-resample block-bootstrap interval (at least three complete blocks).
Only one scenario and timing profile may be aggregated. Ratios below 1 favor the
candidate for elapsed time; they are not a universal runtime score.

Every required workload has visible paired-block coverage, outcomes and stability
labels. Missing, changed, unsupported, or zero-timer workloads cannot be silently
removed or cause weights to be redistributed. A whole-set result requires a
common complete block across every member; fewer than three blocks have no CI.
Category results may be available while the whole-set ratio is null. The output
includes the set and its digest, source lock digest and both full configurations.
These commands do not authorize official publication. `aggregate-report` produces
a portable, sealed HTML report with interval plots, fixed weights, coverage and
stability, configuration details, raw trials, and reproduction commands. It copies
and verifies the source bundle, then recomputes the analysis from that copy; it
does not accept a precomputed result as evidence. Missing ratios have no plotted
point. Filtering coverage never changes the fixed set. Output must be a new
directory outside the source bundle. Copy the entire report directory for offline
use, including `raw/`, `set.json`, `data.json`, and checksums.

### Observed execution trajectories and break-even curves

```sh
./bin/wasmbench run --suite core --runtimes wazero,wasmtime,v8 \
  --scenarios compile,instantiate,trajectory --launches 6 \
  --samples 100 --warmup 10 --operations 1 --out runs/trajectory
./bin/wasmbench break-even --run runs/trajectory \
  --baseline wazero --candidate v8
./bin/wasmbench report --run runs/trajectory --out reports/trajectory
```

`trajectory` starts with invocation 1 after declared initialization, on one
freshly compiled module and one reused instance per request. It does not make
an untimed benchmark verification call before sampling. Each sample measures
one invocation regardless of `--operations`; all outputs are checked outside
their timers, and the first `--warmup` calls remain labeled in the evidence.
This timing scenario requires stateless core scalar workloads with `exact_u64`
or `float_bits_v1` oracles. Float trajectories require the separately advertised
`can_float_trajectory` capability. Fresh-instance,
command, vector, trap and diagnostic-profile contracts remain unsupported.
Initialization and verification can affect engine state; tier and background
compilation events are not inferred from timing changes.

Break-even analysis adds selected setup-phase medians to cumulative observed
call times, **including warmup**. It never substitutes a steady-state slope or
extrapolates beyond recorded calls. In the report, click a curve point (or focus
it and press Enter/Space) for its synthetic total, pointwise interval, complete
block coverage, exact runtime configuration, observed sample prefix and raw
setup/trajectory trials. N=0 contains only setup cost; warmup calls are explicit.
Changing workload/runtime filters clears stale point details; resizing retains
the selected evidence. Each detail also
links to the report's reproduction instructions.
`--setup auto` (default) selects compile and
instantiate, plus app-init when the workload declares an initializer. Use
`--setup compile` for a specifically compile-only setup comparison, or list
other distinct setup phases explicitly. Process/cold-start scenarios are not
accepted as setup components. Missing phases yield unavailable curves rather
than a zero setup cost.

These are synthetic totals from separate phase trials, not measured end-to-end
latency: process launch, unselected setup, verification, input preparation and
inter-call gaps are excluded. Complete blocks supply pointwise bootstrap
intervals; paired runtime deltas use only common complete blocks. Coverage and
failures remain visible. A crossover at one count is not a permanent winner,
and intervals do not correct for simultaneous comparisons. The offline report
plots each runtime through observed counts and links its contributing trials.
Counts use a 1/2/5 logarithmic grid plus the final recorded invocation.

```sh
./bin/wasmbench plan --out suite.lock
./bin/wasmbench fetch --lock suite.lock
./bin/wasmbench run --lock suite.lock --out runs/locked
./bin/wasmbench verify --run runs/locked
./bin/wasmbench inspect --run runs/locked --workload algorithms/sum
./bin/wasmbench compare --run runs/locked --baseline wazero --candidate v8
./bin/wasmbench compare runs/baseline runs/candidate --baseline wazero --candidate wazero
./bin/wasmbench reproduce runs/locked --out runs/reproduced
./bin/wasmbench reproduce-report --dir reports/paired --out runs/reproduced-paired
./bin/wasmbench list
./bin/wasmbench queue --lock suite.lock --out runs/queued
./bin/wasmbench worker
./bin/wasmbench queue
```

`fetch` currently verifies local pinned files; remote acquisition recipes are not
yet implemented. `reproduce` requires the exact runner executable, adapter files
at their recorded paths, and an appropriate host. It refuses changed runner or
runtime files. Completed
bundles and reports are never overwritten. Checksums detect accidental changes;
they are not cryptographic signatures from a trusted publisher.

`reproduce-report` verifies a lifecycle, runtime comparison, history, aggregate,
source-build benchmark, source-output set, stack, native-code export, disassembly
or native-code comparison report and preflights
**all** its locked passes. It runs each recorded input sequentially into a
separate new bundle, then generates and verifies a new report
at `<out>/report`. Each pass uses its exact recorded runner: the current binary
when its hash matches, otherwise `tools/runner/wasmbench` from that pass's sealed
archive. Adapter/analyzer files must still match at their recorded paths. It does
not silently relocate adapters or relax identities. Archived runners are staged
as executable, hash-checked copies under the new output's `runners/` directory;
the non-executable source archive is never modified. The command executes these
verified runner binaries; inspecting or verifying a report alone never does.

Missing or changed tools fail before any output or measurement starts. A failure
after execution starts preserves completed/partial evidence and does not run the
remaining passes or publish a partial comparison. Existing outputs are refused;
use a fresh directory to retry. Reproduction uses the current report analysis and
renderer and does not imply identical performance or official qualification.

Comparison inputs remain separate baseline/candidate passes; history retains
its input order; aggregates keep their exact versioned workload set and weights.
Native-code replay collects new code-pass evidence and creates new exports and,
where originally collected, new LLVM listings. It never substitutes old code
bytes or disassembly for replayed outputs. Exact LLVM executables must still be
available at their recorded paths, with recorded hashes; all such tools are
checked before any measurements begin and checked again before/after each tool
invocation. Native comparisons preserve each side's export/disassembly policy.
LLVM diagnostics run after all measurement passes, not concurrently with them.
Saved LLVM versions and arguments remain explicit; dynamic dependencies are
not automatically pinned.

Lifecycle reports now preserve their exact builder executable as a non-executable
`builder/wasmbench` archive. `builder.json` records its hash, OS/architecture,
dataset hash, analysis version and renderer hash, covered by the report seal.
Default `verify-report` remains read-only and recomputes using the current build.
It can therefore reject a historical report after an analysis/renderer update.

For a **trusted** report, explicitly execute its archived builder to preserve the
original analysis rather than weakening verification:

```sh
./bin/wasmbench verify-report --dir reports/old --recorded-builder
./bin/wasmbench reproduce-report --dir reports/old --out runs/original-analysis \
  --recorded-builder
./bin/wasmbench verify-report --dir runs/original-analysis/report --recorded-builder
```

This executes code from the report. Checksums establish consistency, not a trusted
publisher identity; never use the option for an archive you do not trust to run.
The command validates the complete seal, archive receipt and current platform,
then runs a hash-checked executable copy in its own temporary directory. It does
not change the archived executable's permissions or source files. Temporary
staging is removed afterward. Recorded-builder reproduction retains the original
report analysis/renderer while rerunning the independent locked measurement passes.
Archives generated before this feature cannot be retroactively supplied with their
original builder; recover that executable separately.

Builder archival also covers runtime comparison, history, aggregate, source-build,
source-set, stack, native-code export, disassembly and native-code comparison
reports via `sealed-report-family-builder-v2`. The same
`verify-report --dir REPORT` command detects these families and regenerates their
derived files from copied evidence in a temporary directory. It compares the exact
file set and hashes—including HTML, JSON, Parquet and copied raw bundles—excluding
only the top-level builder archive/receipt that can differ between analysis builds.
Nested input archives remain part of the comparison. Verification does not execute
compilers, adapters or benchmarks. `--recorded-builder` is still the explicit
trusted-code option for retaining historical analysis.

Source report replay reruns the original compiler benchmark schedule, including
separate correctness admission, warmup and measurement builds. Source-set and
stack replay rebuild every source artifact before running either runtime side.
Rebuilt Wasm bytes and source contracts must match the originals exactly; runtime
inputs are new copies containing those rebuilt artifacts, with the original locks
and seals still valid. Changed outputs fail rather than silently become a new
experiment. Compiler/tool/analyzer/runner checks for every pass happen before any
compiler measurement begins. Build and runtime passes are never concurrent.

New source builds retain their exact runner as nonexecutable
`tools/runner/wasmbench`, record `runner_archive_version` and the total
build/validation `tool_timeout_ns`, and include both in the source bundle seal.
Source benchmarks use the archived runner in their admitted builds. Report replay
checks the recorded host fingerprint/environment, reuses saved source snapshots,
and preserves source collection profiles, resource budgets and build timeouts.
Earlier standalone source builds without timeout evidence cannot support exact
report replay; recover that evidence or make a new experiment. Pre-archive builds
need their exact original runner installed separately. Neither builds nor replay
become hermetic: unlisted native tool libraries/system dependencies are not pinned.

With the installed LLVM tools used by `recipes/source/xorshift-llvm*.json`, the
independent analyzer, and both wazero adapters built, run the real source replay
integration checks with:

```sh
WASMBENCH_SOURCE_LLVM_TEST=1 go test -p 1 ./publish ./sourcebuild
```

These include a controller different from the locked CLI, executable copies of
archived runners, fresh compiler/runtime subprocesses, all three source-report
families, and a two-workload source set. They use disposable run indexes. These
are correctness/reproduction checks, not statistical performance measurements.

Parquet exports pin their writer metadata to `parquet-export-v1` instead of
depending on the executable's Go build-info availability. This leaves row schemas
and measurement semantics unchanged, but changes file bytes from earlier exports.
Preserve those earlier reports and use their trusted recorded builders for exact
historical verification; regenerate into a new directory for current exports.

Original archived builders may predate a family's measurement-replay support;
executing an older builder does not add capabilities it never had. Native reports
retain their established `native-code.json` payload; its hash is bound by the
builder receipt. Native images, function ranges and expansion analysis are
recomputed from raw evidence. Disassembly verification retains sealed LLVM
objects, listings and logs as diagnostics and validates their byte/range mapping;
it does not independently redisassemble instructions or execute LLVM. Original
LLVM paths need not be installed for offline report verification. Tool hashes and
versions remain provenance, not a claim that their dynamic dependencies are pinned.
`aggregate-set --out FILE.json` now writes a new set file exclusively; without
`--out` it prints JSON. Existing files and output inside a sealed input are refused.

New CLI plans preserve exact runner, adapter and analyzer files in sealed tool
archives by default. [Tool restoration](../docs/TOOL-ARCHIVES.md) can export a new
replay directory without overwriting installed tools:

```sh
./bin/wasmbench restore-tools --run runs/locked --out .wasmbench/replay/locked
./.wasmbench/replay/locked/tools/runner/wasmbench run \
  --lock .wasmbench/replay/locked/suite.lock --out runs/restored
```

Restoration changes path-bound configuration identity; it is not hermetic.
Mach-O third-party libraries and supported Linux ELF startup libraries are pinned.
ELF runs recheck actual loader resolution before measurement; native loader/OS
prerequisites remain explicit. See the linked platform limits before sharing
archives. Use `--archive-tools=false` when creating a plan to opt out of copies.

The SQLite database at `.wasmbench/index.sqlite` indexes runs and stores local
jobs. A worker atomically claims pending jobs. A running job is never
automatically restarted merely because another worker cannot observe it.
Use `index --run <bundle>` to rebuild an index entry from verified evidence.

Analysis computes medians per independent process, then bootstrap intervals
over those process medians. Paired comparisons use launch blocks. Warmup samples
stay in the raw data but are excluded from steady-state summaries. No request
p99 is inferred from batch averages. Fewer than three launches are labeled
insufficient; low observed dispersion does not establish VM convergence.

Cross-run comparisons use independent bootstrap samples on each side; matching
launch numbers in separate runs are not treated as pairs. They require matching
host/environment fingerprints and timing protocols. Changed artifact hashes or
correctness/reset contracts remain visible as incomparable cells, and missing
workloads retain null ratios. The output records both runtime configurations,
whether the runner changed, and the exact common workload/scenario subset.

For DuckDB, run [analysis/launches.sql](../analysis/launches.sql) from the report
directory. It reads `samples.parquet`, preserves process replication, and emits
coverage separately. The exported Parquet keeps missing observations null.
Reports also export `observations.parquet` for sample-level and trial-level
diagnostics, including collector/version, scope, quality, phase, denominator,
availability and reason. Use [analysis/observations.sql](../analysis/observations.sql)
to aggregate those observations without mixing measurement domains or giving
sample-heavy processes extra weight. Footprint values are not divided by the
operation count. Raw evidence retains failed and warmup observations.

Analysis `cluster-median-bootstrap-v2` uses JSON null for unavailable summary
statistics and for intervals with fewer than three independent launches/pairs.
A measured zero remains zero; it is not usable as a latency-ratio denominator.

## Development

```sh
go test ./...
go vet ./...
make build
```

The default Go suite skips opt-in runtime/platform acceptance tests. A package
PASS does not prove those tests executed. See the [acceptance map](../docs/ACCEPTANCE.md)
and required-test coverage gate for live adapter and archived-replay qualification.

The controller imports no runtime implementation. Adapters use dedicated JSON
control messages; guest output goes to stderr. Integer bits are decimal strings
on the wire to avoid JavaScript rounding 64-bit values. See
[protocol/types.go](../protocol/types.go) and [metrics/registry.go](../metrics/registry.go).

This is a trusted local experiment runner, not yet an untrusted-submission
sandbox. Use separately qualified hosts for official experiments; the
[publication gates](../docs/PUBLICATION.md) do not certify dedication by themselves.
