# Linux perf cgroup collector

Status: experimental counters runner implemented for Wago, wazero, Wasmtime and V8
compile/instantiate (including interpreter, Winch and pooling configurations).
Positive hardware counting is not yet qualified; unavailable/permission-denied
paths and protocol integration are tested. Profiling remains unconfigured.

```sh
./bin/wasmbench run --suite core --runtimes wazero,wazero-interpreter \
  --profile counters --phase-barriers --operations 1 --warmup 0 \
  --scenarios compile,instantiate --launches 3 --samples 10 \
  --out runs/counters
```

On Linux, add `--cgroup-parent /absolute/delegated/parent` to create an isolated
cgroup from process start. This needs appropriate host delegation and perf
permissions; the runner never changes host security settings. Without isolation,
or on non-Linux systems, windows record explicit unavailability, not zero counts.
CLI progress separates workload success from counter-window availability.

### First-invocation counters

All seven current configurations also support `--scenarios first-call`. Each
sample gets a fresh instance, including its start function, explicit initializer,
input installation and export resolution before the counter window. Exactly one
requested invocation is counted; result copying, oracle verification and instance
release follow the end barrier. The engine and compiled module are retained.
There is no hidden workload pre-call and no allocator snapshot or forced GC.

```sh
./bin/wasmbench run --suite lifecycle --runtimes wago,wazero,wazero-interpreter \
  --profile counters --phase-barriers --operations 1 --warmup 0 \
  --scenarios first-call --launches 3 --samples 10 --out runs/first-counters
```

The protocol stages are `before_first_call`, `first_call_returned` and
`first_call_released`. A returned call that traps still ends the counter window,
but fails the trial; its readings are diagnostic, never successful workload
measurements. An initialization failure occurs before any counter window.
Wasmtime uses a fresh store and drops it after verification, including in pooling
mode. V8 resolves arguments before collection and normalizes results afterward;
release drops references, not a guarantee of reclamation. V8's background compiler
work can share the window and its code cache remains uncontrolled.

Run the shared first-call conformance suite after building the adapters:

```sh
WASMBENCH_FIRST_COUNTER_TEST_RUNTIMES=wago,wazero,wazero-interpreter,wasmtime,wasmtime-winch,wasmtime-pooling,v8 \
  go test -race ./experiment -run TestBuiltAdapterFirstCallCounters -count=1
```

### Steady-batch counters

All seven current configurations support stateless exact-scalar steady batches. A
request creates one fresh initialized instance, then retains it across all
explicit warmup and measured batches. No hidden pre-call runs. Warmup is counted,
verified and retained as separate marked samples, not silently discarded.

```sh
./bin/wasmbench run --suite core --runtimes wago,wazero,wazero-interpreter \
  --profile counters --phase-barriers --scenarios steady \
  --operations 100 --warmup 2 --samples 10 --launches 3 --out runs/steady-counters
```

Each window spans the requested batch, including embedding API allocation/work
and barrier transport, not guest-only execution. Result containers are allocated
before collection. Every returned result is verified after the end barrier;
failed warmup stops the request. Raw counts are batch totals, never automatically
divided by the operation count. Memory-oracle checks observe the stateless
instance after the batch, not intermediate per-invocation memory snapshots.
No claim of warmup convergence is inferred. Reinitializing state per sample and
other ABI/oracle profiles remain unsupported for this scenario.
Budgets allow 1..100,000 measured batches, 0..100,000 warmup batches and
1..1,000,000 operations per batch; workload timeout/resource limits still apply.
Compile/instantiate/first-call retain one operation and zero warmup.

Wago returns an instance-owned result slice that the next invocation overwrites.
Its adapter copies each result into preallocated storage during the batch;
capture cost belongs to the measurement, while oracle checks remain outside.
This also applies to Wago timing/trajectory/first-call result capture.
Wasmtime prepares separate result buffers before each batch and retains its
store across batches, including pooling mode. V8 retains raw JS results until
post-batch normalization/verification; process engine, code cache and GC remain
uncontrolled, and background compiler work can contribute to the window. Neither
adapter adds forced GC, allocator purges or hidden warmup.

Conformance uses `WASMBENCH_STEADY_COUNTER_TEST_RUNTIMES=wago,wazero,wazero-interpreter,wasmtime,wasmtime-winch,wasmtime-pooling,v8`
with `go test -race ./experiment -run TestBuiltAdapterSteadyCounters -count=1`.

## Doctor probe

`wasmbench doctor` includes `perf_cgroup_probe`. It reports `unsupported` outside
Linux and `not_requested` on Linux until an existing cgroup is explicitly selected:

```sh
./bin/wasmbench doctor --perf-cgroup /absolute/existing/cgroup
```

The probe opens disabled descriptors for all seven event definitions across the
target's online effective cpuset, then closes them. It never enables events,
collects counts, creates cgroups, moves tasks, changes permissions or requests
privilege escalation. Each event/CPU preserves its open outcome and reason;
the overall outcome distinguishes `open_succeeded`, `partial_open`, unavailable
and cleanup failure. `perf_event_paranoid` is reported as context when readable,
not interpreted as proof of effective permission. Existing `perf` executable
discovery is separate: the syscall collector does not require that executable.

Open success does not prove PMU scheduling, successful enable/read, useful counts,
stable topology, workload isolation or publication readiness. Actual experiment
collection still performs its own checks. Root-cgroup probes include its full
CPU scope but never count root-cgroup activity.

## Trial evidence

Every trial's `counter_phases` records the sample, diagnostic phase, collector
version and complete raw per-CPU event results. These fields are checksum-sealed
with the trial and retained in report JSON and raw downloads. Failed-call records
remain diagnostic only. Sacrificial correctness admission runs in a separate
process with the timing preparation profile and no counters. Unsupported adapters
and contracts stay visible. Counter/profiling-pass elapsed timers are excluded
from latency summaries (analysis version 4), while raw sample timers remain
available for diagnostics.

The offline report's **Counter evidence** view shows every trial outcome and
per-CPU reading, including unavailable, failed and admission records. Select a
trial and a 500-row window to inspect all evidence without downsampling. Counts,
event configurations and enabled/running times use exact decimal strings in the
display dataset (`raw-counter-decimal-display-v1`); JavaScript never converts
these 64-bit values to floating-point numbers. Zero and unavailable remain
distinct. Each row exposes its complete definition/provenance, and the view links
to the sealed raw trial and typed Parquet download. This is a diagnostic evidence
browser, not a normalized counter ranking or guest-only attribution.

Reports include `counters.parquet` (export version `raw-perf-counters-parquet-v2`).
Its nullable unsigned integer columns preserve the full raw count/config/time
range without floating-point conversion. Each per-CPU reading carries trial,
window and reading status, source order, sample verification, event definition,
scope and privilege provenance. No status is silently filtered and no scaled or
aggregated count is invented. `row_kind=window_outcome` preserves windows with
no readings; `trial_outcome` preserves trials with no windows, including unsupported
and admission trials. Use `block >= 0` to select measured launches. Missing or
ambiguous sample matches have null verification, not inferred correctness.
Version 2 adds nullable `sample_warmup` and `sample_operations` from unique raw
sample matches; absent/ambiguous samples remain null, and nonpositive operation
counts are not exposed as valid denominators. The UI and SQL preserve this context.
Failed/coverage-invalidated raw readings remain diagnostic only.

Run [analysis/counters.sql](../analysis/counters.sql) in DuckDB from the report
directory for coverage and raw per-CPU views. Do not treat an event row as an
independent replicate or sum incomplete CPU coverage. Reports for non-counter
runs contain a typed empty counter table.

`collectors.OpenPerfCgroup` accepts an already-open cgroup-v2 directory and an
explicit nonempty CPU list. It opens one disabled event descriptor per event/CPU,
using `PERF_FLAG_PID_CGROUP | PERF_FLAG_FD_CLOEXEC`. It never silently switches to
calling-thread collection. The caller owns the cgroup directory; the collector
owns and closes every event descriptor. The caller must keep the cgroup directory
open through Finish. The selected CPUs must exactly match the intersection of
`cpuset.cpus.effective` and the host online CPU list; missing files, empty sets,
overlapping/malformed ranges and partial coverage fail closed. Effective cpuset
reads are relative to the supplied cgroup descriptor, not a reconstructed path.
Lists are bounded to 4,096 CPUs to prevent unbounded descriptor allocation.

The collector rechecks effective and online sets immediately before enabling
and after disabling/reading. A changed set before Start closes descriptors
without enabling; a changed or unreadable set after collection returns an error
and marks every reading `coverage_changed`, retaining raw values and prior event
status for diagnostics only. These are boundary consistency checks, not proof
against transient hotplug or cpuset changes that occur and revert between reads.
Official controlled hosts must prohibit such concurrent reconfiguration.

Generic event definitions are returned by `PerfEvents`: cycles, instructions,
branches, branch misses, page faults, context switches and CPU migrations.
Records preserve numeric Linux event type/config, CPU, collector version and
privilege scope. User and kernel execution are included; hypervisor execution
is excluded. These are cgroup-thread counts on the recorded CPU, not guest-only
work. Event support and privileges are discovered independently for each event.

The single-use lifecycle is open → Start → Finish; Close handles cancellation
or failure at any point. Start enables descriptors sequentially; Finish disables
all of them before reading. Cross-event/CPU boundaries are therefore not atomic.
Callers must hold the guest at barriers and label the resulting diagnostic
window, including transport/background activity, rather than claiming exact
embedding-API or guest-only boundaries. No counter reset/reuse ambiguity exists:
each window owns fresh descriptors.

Each record retains unsigned 64-bit raw count, enabled nanoseconds and running
nanoseconds. No floating-point conversion or scaling is performed. A shorter
running interval is labeled `multiplexed`, and zero running time is
`not_running`, not a measured zero-event claim. Genuine zero with positive,
equal running/enabled times remains available. Inconsistent accounting and
short reads cannot produce usable values. Permission and other syscall errors
retain their reason and null counts. Never sum partial CPU coverage or treat
per-CPU records as independent process replications. JSON consumers must use a
lossless integer parser before numerical work on counts above 2^53.

The ABI/read format follows the Linux
[perf_event_open documentation](https://man7.org/linux/man-pages/man2/perf_event_open.2.html).
No estimated scaled count is substituted for raw multiplexed evidence.

## Verification

`go test -race ./collectors` tests lifecycle ordering, exact integers, genuine
zero, multiplexing, invalid accounting, both byte orders, unavailable events,
partial event support and closure on every tested path. Linux-only tests check
the definitions against x/sys constants and reject non-cgroup targets.

On a Linux cgroup-v2 host, the opt-in kernel smoke test is:

```sh
WASMBENCH_PERF_KERNEL_TEST=1 go test -v ./collectors -run TestPerf
```

It uses the visible cgroup root and its complete online effective cpuset without
changing host policy or moving processes; it tests syscall/read plumbing and
boundary coverage checks, not isolated benchmark-worker placement.
Permission denial passes as an honest unavailable result, not positive PMU
qualification. On the local Linux ARM64 Docker host with capabilities dropped,
all seven events returned `permission_denied`. Both Linux ARM64 and AMD64 test
binaries cross-compile; AMD64 execution and successful hardware counting remain
unverified. The restricted Linux ARM64 test suite and full root Go tests/vet pass.

## Remaining integration

`agent.Client.CallCounterPhases` now provides opt-in phase collection and requires
a successful counters-profile preparation. Failed preparation clears the tracked
profile. Ordinary calls never open counters. Each supported lifecycle sample gets
a fresh perf window: start before acknowledging its initial barrier, finish
before acknowledging its measured-end barrier, and exclude the trailing release
barrier where applicable. Strict sample/stage ordering, final sample count,
one-operation identity and verification are required. Active windows close on
protocol errors, interrupted responses and collection failures. Unavailable
cgroups/events remain explicit; partial/multiplexed event evidence is not promoted
to fully available. Completed records returned alongside a failed call remain
diagnostic evidence, not publishable successful measurements.

The runner uses this API for counters passes. Wago, wazero compiler/interpreter,
Wasmtime Cranelift/Winch/pooling and V8
advertise `can_counter_compile` and `can_counter_instantiate` for core scalar
`exact_u64` contracts, with explicit counters-only handshakes. Both scenarios require
phase barriers, one operation, zero warmup and 1..100,000 samples. Memory
snapshots are disabled in this profile; verification follows the compile or
instantiation end barrier and precedes release. Unsupported scenarios/contracts
fail before emitting barriers. Memory passes cannot be relabeled as counter passes.
V8 retains production-default tiering, uncontrolled internal caching and GC.
Its counter window includes adapter transport and any background compiler work
in the selected cgroup during that window; compile API return is not evidence
of final-tier completion or complete compilation work. Reference release does
not guarantee reclamation, and no forced GC is introduced.

Built-adapter conformance runs with:

```sh
go build -o bin/adapter-wazero ./adapters/wazero
./bin/wasmbench build --runtimes wago --wago-source ../../Wago/wago
./bin/wasmbench build --runtimes wasmtime,wasmtime-winch,wasmtime-pooling
WASMBENCH_COUNTER_TEST_RUNTIMES=wago,wazero,wazero-interpreter,wasmtime,wasmtime-winch,wasmtime-pooling,v8 \
  go test -race ./experiment -run TestBuiltAdapterCounters -count=1
```

This exercises real subprocess handshakes, initialization/input, verification,
invalid requests and absent memory observations. Clients intentionally have no
cgroup, so it verifies unavailable perf evidence rather than positive PMU counts.

Add counter comparison views, enforce stable host topology throughout collection,
and publish event coverage/multiplexing without hiding failures. Qualify positive
counts on a permitted Linux host before treating the
experimental counters profile as hardware-qualified. Profiling stacks/flamegraphs
are a separate subsystem.
