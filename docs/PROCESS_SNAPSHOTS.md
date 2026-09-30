# Linux process-level snapshot qualification

This is a standalone functional qualification for Wasmtime 46.0.1, using
Cranelift and Winch. It is not a benchmark scenario, engine snapshot API,
serialized checkpoint, or official performance result. Ordinary adapters still
advertise `can_snapshot: false`.

The scope is `linux_process_cow_clone`. A dedicated single-threaded executable
creates a quiescent core instance, forks a retained template process, mutates
and releases the source Store, Module and Engine, then asks the template to fork
two independent restored children. Each child verifies guest state, modifies it,
and exits. The template remains available until both restorations complete.

Linux implements fork with copy-on-write pages, but inherited file descriptors
can share offsets and other state. Forking a multithreaded embedding also imposes
child safety restrictions. See the [Linux fork contract](https://man7.org/linux/man-pages/man2/fork.2.html).
The qualifier therefore has no guest imports, disables parallel compilation,
and requires exactly one procfs thread immediately before every fork. Its guard
test creates a second thread and proves rejection without actually forking.
This check is not a general guarantee against foreign thread-creation races.
Do not move this implementation into a Go process, Rust test harness, or another
multithreaded embedding.

## Run the qualification

Build everything before qualification. From the repository root:

```sh
docker build -t wasmbench:local .
sh recipes/wasmtime-linux.sh build-process-snapshot
sh recipes/qualify-linux-process-snapshots.sh wasmbench:local process-snapshot-local-v1
```

Use a new evidence name for each run. The recipe refuses existing directories,
dangling aliases, traversal, non-Linux images and emulation before creating
evidence. It resolves the immutable image ID, then runs without network,
capabilities or writable root filesystem, as the current user, with a 16-process
limit and an init process. Only the new evidence directory is writable. The
standalone qualifier and verifier are mounted read-only; the product image
provides the independent analyzer and Node verifier environment. The qualifier
is not included in the ordinary product image or enabled by ordinary builds.

The retained files are:

- `fixture.wasm`: exact fixed qualification module, created without overwrite.
- `structure.json`: independently validated input structure.
- `guard.json`: multithreaded rejection test outcome.
- `cleanup.json`: positive control, six rejected/reaped child failures and
  verified parent-death termination and reaping.
- `qualifier`: the exact executable used, copied before any qualification runs.
- `qualification.json`: backend-specific state and restoration proofs.
- `container.json`: immutable image ID and native architecture.
- `receipt.json`: SHA-256 hashes of these files and the actual qualifier binary.

The version-3 receipt checks consistency and binds the archived executable to
the mounted build; it is neither a signature nor a sealed benchmark bundle.
Earlier receipt versions do not establish this stronger contract. The image
and its dynamic dependencies are not archived in this qualification bundle.
The command accepts no arbitrary guest module or user-supplied runtime program.

## What the fixture proves

Independent admission requires eight defined functions, one hidden mutable
global, one memory, one hidden table, one passive data segment, two element
segments, and zero imports. Seeding grows memory and table to three units and
changes the table's indirect-call target. The seeded oracle is 64.

After capture, source mutation changes memory, global, table and grown sizes,
drops passive segments, and produces oracle 125. Source access to the dropped
segments must trap. The source Store, Module and Engine are then dropped before
either restoration. Both restored children must have oracle 64, three memory
pages and three table elements, successfully initialize the retained passive
segments with probe result 127, and produce oracle 125 after independent mutation.
This distinguishes the proof from copying exported memory or re-instantiating a
fresh module. It is broader than the separately scoped
[native stack continuations](NATIVE_CONTINUATIONS.md) and guest-copy workflows.

Every child must exit successfully before its proof reaches the root process.
Root proof reads have five-second timeouts; template proof reads and child-exit
polling each allow two seconds. Errors terminate and reap the owned child before
returning failure. Waits retry interruptions and relinquish ownership on ECHILD
instead of signaling a potentially reused PID. Cleanup never targets another
service or process group. These deadlines bound polling, not kernel scheduling
or a guarantee against uninterruptible kernel stalls.

Each fork child arms SIGKILL on parent death and then rechecks parent identity,
closing the race where the parent dies before setup. The setting must be armed
again in every descendant because Linux clears it at fork; see
[PR_SET_PDEATHSIG](https://man7.org/linux/man-pages/man2/PR_SET_PDEATHSIG.2const.html).
The dedicated failure probe uses a test-only subreaper to verify termination and
reap the adopted grandchild after its direct parent exits. Fork children use
`_exit` rather than unwinding inherited parent resources. Container init remains
the fallback orphan reaper.

`--test-failure-cleanup` exercises the same proof receiver and ownership guard
as real restoration, with one successful positive control and rejection of:
no proof/nonzero exit, partial proof, wrong state, complete proof/nonzero exit,
complete proof followed by a stall, and a stall without proof. All test children
must be reaped; complete bytes alone never mean success. These are supervisor
fault-injection tests, not arbitrary-guest failure or kernel-stall qualification.

Local Linux/arm64 evidence at
`runs/linux-process-snapshot-qualification-v1` passed both backends, two restores
each, with source release and the thread guard. A fresh run at
`runs/linux-process-snapshot-qualification-v2` produced byte-identical fixture,
structure, guard, qualification, container metadata and receipt. This is repeat
qualification, not locked benchmark replay. The input digest is
`7a828c361cd0790044ac23e97c5c93838605c8f202088cabe990c4c1eb6d3381`.
The first qualifier binary digest is
`b4a8a9cbe15a05e8394dc92dc0f0b35aeaa2ea995326971274b60d78899d1169`.
Native AMD64 and remote CI are not locally verified. These are functional
shared-VM observations, not dedicated-host performance data.

The stronger cleanup/archival workflow passed at
`runs/linux-process-snapshot-cleanup-v3` and `-v4`. All eight retained files,
including the executable and version-3 receipt, are byte-identical between the
fresh runs. Offline receipt verification also passed with the archived binary.
These runs require cleanup evidence version `linux-process-snapshot-cleanup-v2`;
earlier development cleanup runs lack this final versioned contract. The final
archived qualifier digest is
`c920c0e18d1af4396ed01efc1227fc0534fd384b44c5aa8bb70d25e814177772`.

## Replay retained qualification evidence

For a trusted locally generated version-3 bundle, replay uses only its archived
qualifier and exact retained local image ID, not the current build or an image
tag:

```sh
sh recipes/replay-linux-process-snapshots.sh process-snapshot-local-v1 process-snapshot-replayed-v1
```

The same native architecture and the exact image must be available locally;
the recipe does not pull a replacement image or accept emulation. Node is
required on the controller host for read-only preflight. The output name must
be new. Source directory aliases, member symlinks, missing files and inconsistent
receipts are rejected before container execution. The restricted container
rechecks the source receipt and copied executable before running the qualifier.
It regenerates the fixed fixture, independent analysis, guard, cleanup and
backend proofs, verifies the output receipt, and requires byte-identical
receipts. Only then is `replay.json` written with both receipt digests.

`runs/linux-process-snapshot-replay-v1` successfully replayed
`runs/linux-process-snapshot-cleanup-v3` on Linux/arm64. A fresh replay at
`runs/linux-process-snapshot-replay-v2` retained byte-identical qualification and
replay provenance files. All retained qualification
members matched through the receipt; the source/replayed receipt SHA-256 is
`11b28591db42e14f7f6750859cf18a7f733fca5f3cd1afd2d0a3b287d206057f`.
This is exact functional qualification replay, not a timed benchmark replay.

Checksums do not authenticate an executable. Do not replay untrusted or resealed
third-party bundles. This recipe does not archive the container image, provide a
dependency-independent portable bundle, or authorize official publication. Its
trusted current verifier is intentionally used for both preflight and output
validation; older contracts are not silently upgraded.

## Product contract and evidence export

The `process-snapshots` corpus generator retains the exact 343-byte qualified
fixture. The versioned contract is `linux-process-cow-clone-v1`, distinct from
engine serialization, guest-memory copying and execution-stack continuations.
Live execution and offline bundle loading share typed validation of the fixture,
configuration, sample sequence, process lineage, clock ownership and state proof.

| Scenario | Timed operation | Clock owner |
| --- | --- | --- |
| `process-snapshot-capture` | Source fork until template-ready acknowledgment | Source adapter |
| `process-snapshot-restore` | Restore request until child-ready acknowledgment | Source adapter |
| `process-snapshot-first-write` | One embedding memory write in the restored child | Restored child |
| `process-snapshot-execute` | Typed `check` call in the restored child | Restored child |

Capture and restore include process-control transport and scheduling; they are
not fork API-return latency. First write stores byte 22 at offset 65535, leaving
the typed result at 64. It is not an isolated COW-fault measurement. Full
three-page memory hashes are checked before the passive-segment probe, which
returns 127. Source mutation returns 125, and its Store, Module and Engine must
be released before restoration. The source adapter process itself remains alive.
The standalone qualifier's restored mutation to 125 does not constitute this
new timing proof.

Each sample requires a fresh template and two independent restorations with
single-threaded pre-fork evidence and completed child reaping. Process identities
include PID and canonical decimal `/proc/PID/stat` birth ticks; ticks remain
strings to avoid JavaScript precision loss. A batch retains the source adapter
identity and rejects reused captured/restored identities. Live execution also
checks the source PID against the launched adapter. Raw OS lineage attestation
and child-inclusive collectors remain open work.

Only timing is admitted by this initial contract, with one operation per sample,
no warmup and no phase barriers. Memory is explicitly unsupported until restored
children are included. Dedicated `wasmtime-process-snapshot` and
`wasmtime-winch-process-snapshot` configurations advertise
`can_linux_process_snapshot`; ordinary runtime cells remain unsupported.
Qualified adapters must be native Linux Wasmtime 46.0.1, Cranelift or
Winch, with parallel compilation and code caching disabled. The generic
`can_snapshot` capability must remain false.

Sample Parquet export version `sample-evidence-parquet-v9` adds nullable
`process_snapshot_result_json`, retaining the complete typed proof and exact
birth strings. Report verification regenerates the export from sealed raw
evidence; absent proofs remain null. This is export support, not evidence that
a native timed adapter exists.

The current development wiring can be exercised with:

```sh
wasmbench run --suite process-snapshots --runtimes wazero,wazero-interpreter,v8 \
  --scenarios process-snapshot-capture,process-snapshot-restore,process-snapshot-first-write,process-snapshot-execute \
  --launches 1 --samples 1 --operations 1 --warmup 0 --out runs/snapshot-contract
wasmbench verify --run runs/snapshot-contract
wasmbench report --run runs/snapshot-contract --out reports/snapshot-contract
wasmbench verify-report --dir reports/snapshot-contract
```

This deliberately records unsupported outcomes, not performance measurements.
Local `runs/process-snapshot-contract-coverage-v1` contains three unsupported
sacrificial checks and 12 unsupported runtime/stage cells, with zero samples.
Its sealed bundle and derived report verified successfully. The updated export
report at `reports/process-snapshot-contract-coverage-v2` also verified using
its exact archived report builder. Guest-fixture
behavior is separately exercised in wazero; synthetic protocol/export tests
are not native process-snapshot qualification.

## Native development timing worker

The qualifier now accepts `--timed-stage BACKEND STAGE SAMPLES`. It uses the
same owned-child guard and parent-death cleanup as qualification, with a fresh
template per sample and two restored children. Capture and restoration end at
ready acknowledgments; restored-state checks, complete memory hashes, passive
segment probes and JSON proof encoding follow readiness, outside those parent
timers. Child-local timers enclose only the embedding memory write or prepared
typed call. Readiness includes the child's process identity and handle setup.
A post-timer release handshake prevents child proof work from starting while
the source is still receiving the ready acknowledgment.

Builds and tests must not overlap the worker measurements:

```sh
sh recipes/wasmtime-linux.sh build-process-snapshot
sh recipes/measure-linux-process-snapshot-worker.sh wasmbench:code-lifetime-package-v2 snapshot-worker-local
WASMBENCH_SNAPSHOT_WORKER_EVIDENCE="$PWD/runs/snapshot-worker-local" \
  go test ./protocol -run TestNativeProcessSnapshotWorkerEvidence -v
```

The recipe requires an existing native Linux image and a new output name. It
archives the exact worker, image identity, guard/cleanup/functional qualification
proofs and eight timing outputs. SHA-256 checksums provide consistency, not
authentication. It runs without network, capabilities or a writable root, with
a PID limit and the caller's UID/GID. The Go evidence test requires both
backends, all four stages and two samples per record, then applies the product
protocol's complete sample-sequence checks.

`runs/linux-process-snapshot-timing-worker-v2` passed on native Linux/arm64 in
the shared Docker VM. All 16 samples passed typed protocol verification; guard,
cleanup and functional proofs also passed their validators. Archived worker
SHA-256 is `25d75dd1b2e00d98b9cb901cd91a1c0ba11559a17b61b0e9134d71726fbbcf72`.
The earlier `-v1` run is preserved but lacks the post-timer release handshake;
it is not evidence of the final timer-isolation implementation.
This is development timing evidence, not an independent-launch performance
comparison, a registered adapter, a sealed experiment bundle or locked replay.
No native AMD64 verification or child-inclusive memory claim is made.

## Product adapter and locked replay

The dedicated configurations invoke the archived qualifier executable with
`--adapter=cranelift` or `--adapter=winch`. It implements version-1
describe/prepare/run/close messages on the ordinary control channel. Preparation
admits only the exact canonical artifact and timing contract; invalid preparation
clears prior state. One run request collects the complete batch locally through
the same timing implementation as the standalone worker. Unsupported methods,
foreign workloads, warmup, barriers and memory requests cannot produce samples.

On native Linux, build and use it through the normal CLI:

```sh
wasmbench build --runtimes wasmtime-process-snapshot,wasmtime-winch-process-snapshot
wasmbench run --suite process-snapshots \
  --runtimes wasmtime-process-snapshot,wasmtime-winch-process-snapshot \
  --scenarios process-snapshot-capture,process-snapshot-restore,process-snapshot-first-write,process-snapshot-execute \
  --launches 2 --samples 2 --operations 1 --warmup 0 --out runs/process-snapshot-native
wasmbench verify --run runs/process-snapshot-native
wasmbench reproduce runs/process-snapshot-native --out runs/process-snapshot-replayed
```

For the local native Docker qualification, build all tools before collection:

```sh
sh recipes/wasmtime-linux.sh build-process-snapshot
GOOS=linux GOARCH=arm64 go build -trimpath \
  -o .wasmbench/wasmbench-linux-process-snapshot ./cmd/wasmbench
sh recipes/test-linux-process-snapshot-adapter.sh wasmbench:code-lifetime-package-v2 snapshot-adapter-local
```

Use the native architecture (`arm64` above), never emulation. The recipe retains
the exact image identity, original/reproduced sealed bundles and verified report
in a new directory. Native dependencies and the exact runner/adapter/analyzer
are retained by the normal tool-archive workflow; replay uses those tools rather
than replacing them with the current build. The container image is not archived
as a portable dependency closure. Only trusted local archives may be replayed.

`runs/linux-process-snapshot-adapter-v2` passed on Linux/arm64: original and
reproduced bundles each contain two successful sacrificial checks and 16
successful timing trials, with 36 verified samples total per bundle. All four
stages and both backends are covered at two independent launches. Typed bundle
validation, coverage/configuration equality across replay, derived report
verification and exact archived report-builder verification passed. Adapter
digest is `e68f967029dde1d7e44e13005d40180a073650d156c4ce2f65fefb614de98150`.
The earlier `-v1` original also passed collection and bundle validation, but its
recipe stopped at a coverage-verifier field-name error before replay. It remains
preserved; it is not claimed as completed replay evidence.

These are shared-VM integration measurements, not official publication or
precision-qualified performance comparisons. No child-inclusive RSS or COW
fault-volume measurement is inferred from the latency samples.

## Independent procfs collector foundation

`collectors.CollectSnapshotProcess` now collects a declared live PID/birth/parent
identity independently of adapter claims. It brackets status and smaps reads
with `/proc/PID/stat`, rejects changed births/parents, zombies, exited processes,
malformed counters and integer overflow, and retains both raw stat readings.
The shared validator re-derives identities and footprints from raw evidence for
offline checks. Birth ticks never pass through floating-point numbers. The
collector reads only the declared process; it does not discover, signal or
acquire ownership of arbitrary processes.

RSS and virtual size come from status; PSS and `Private_Clean + Private_Dirty`
come separately from smaps_rollup. Permission-denied or missing smaps retain a
reason and null PSS/private values, without discarding available status RSS.
These are non-atomic boundary readings, not phase peaks, process-tree totals,
guest-memory decomposition, COW-fault volume or physical reclamation evidence.

Functional qualification with owned Go helpers, not Wasmtime clones:

```sh
GOOS=linux GOARCH=arm64 go test -c \
  -o .wasmbench/snapshot-process-collector.test ./collectors
sh recipes/test-linux-snapshot-process-collector.sh wasmbench:code-lifetime-package-v2 snapshot-proc-local
WASMBENCH_SNAPSHOT_PROC_EVIDENCE_DIR="$PWD/runs/snapshot-proc-local" \
  go test ./collectors -run TestRetainedSnapshotProcessEvidence -v
```

`runs/linux-snapshot-process-collector-v1` retains the native Linux/arm64 test
executable, image identity, three raw process readings, test log and checksums.
Live source/template/restored parent links and birth identities passed, wrong
live parents/births were rejected, and owned descendants were released/reaped.
Offline raw-evidence validation and checksums also passed. Test executable
digest is `f8c91526f14c8888cb830a3f5a220637e4b25ee34c500ba5d70f80e1b4cd28fe`.
Checksums establish consistency, not authenticity. Native AMD64 and remote CI
remain unverified locally.

Timing responses arrive after restored children are reaped; they cannot be used
to collect those children's live OS evidence retroactively. The diagnostic
inspection workflow below holds processes alive and binds readings to proofs.
Product memory-run storage, metric exports and report views remain open work.

## Held-live native diagnostic inspection

The dedicated adapter now advertises `can_inspect_linux_snapshot_lineage`.
Prepare the canonical workload with profile `memory`, then send `inspect` with
an explicit phase-barrier request. This is a separate control flow, not a normal
timing `run`. `agent.Client.CallSnapshotInspection` refuses timing/unprepared
calls and requires an observer; ordinary calls reject snapshot barrier messages.
The inspector reuses the qualified fork/cleanup implementation.

Each sample has seven ordered barriers: template after source resource release,
then restore readiness, first-write completion and execution completion for each
of two sequential restorations. The relevant child waits for controller release
before progressing. The controller reads source, template and restored-child
procfs evidence independently, checks actual source PID/controller parent,
birth identities, parent links and one live thread, and binds all events to the
completed snapshot proof. Missing, duplicated, reordered or mismatched events
fail validation.

Inspection returns `snapshot_diagnostics`, explicitly profile `memory` and
`latency_eligible: false`; it never populates the ordinary response sample array.
Local diagnostic operation durations are retained for provenance, not headline
timing. Parent/child transport, controller pauses and instrumentation can affect
these operations. Memory changes between barriers include validation, hashing,
adapter work and the guest operation; they do not isolate COW page-copy costs.
The readings are individual, non-atomic process boundaries, not a simultaneous
process-tree peak or density measurement.

```sh
sh recipes/wasmtime-linux.sh build-process-snapshot
GOOS=linux GOARCH=arm64 go test -c \
  -o .wasmbench/snapshot-live-inspection.test ./experiment
sh recipes/test-linux-snapshot-live-inspection.sh wasmbench:code-lifetime-package-v2 snapshot-live-local
WASMBENCH_SNAPSHOT_RETAINED_LIVE_EVIDENCE_DIR="$PWD/runs/snapshot-live-local" \
  go test ./experiment -run TestRetainedNativeSnapshotInspection -v
```

Native Linux/arm64 `runs/linux-snapshot-live-inspection-v2` passed both backends,
two samples each: 28 barriers and 80 live procfs readings. Complete state proofs,
live parent/birth/thread checks, archived raw-evidence validation and checksums
passed. Exact test/adapter binaries and raw records are retained. Adapter SHA-256
is `62770d91da0e66adf6bce02691168c66acb3ebc6fd3a3b87059415a4280392a5`.
The earlier `-v1` test stopped before inspection due to a duplicated absolute
artifact path in the test harness; it is preserved, not successful evidence.

The updated ordinary timing adapter also passed a fresh sealed collection,
archived-tool replay and both report verifiers at
`runs/linux-process-snapshot-adapter-v3`. No diagnostic barrier entered its
timing channel. CI now runs live inspection and retained-evidence validation,
but remote CI and native AMD64 remain unexecuted locally. This is scoped native
inspection qualification, not yet the CLI's sealed memory experiment workflow.

## Sealed memory runs

The dedicated native Linux configurations now support `--profile memory
--phase-barriers`. The controller collects the source, template and restored
child separately while the adapter holds them live. Each sample retains seven
boundaries and twenty bracketed procfs readings, including both independent
restorations. The sealed loader revalidates raw process birth/parent links,
thread count, ordered collector windows and the completed guest-state proofs.
RSS and virtual size remain available when smaps access is denied; PSS/private
memory remain unavailable with the raw reason. These are non-atomic boundary
snapshots, not process-tree or stage-only peaks.

Memory trials store `snapshot_memory`, not ordinary timing samples. Nested
diagnostic timings are latency-ineligible. The sacrificial admission still uses
two fresh timing restorations in a separate process. The root-only memory
sampler and process high-water RSS are deliberately bypassed. If configured,
whole-tree cgroup lifetime observations remain a separately labeled accounting
domain; the local restricted Docker qualification did not provide cgroups.

Parquet version `sample-evidence-parquet-v10` adds nullable
`snapshot_memory_evidence_json`, preserving raw readings, exact decimal process
births, denied-smaps reasons and completed diagnostic proofs in a trial row.
No top-level elapsed time, operation count or sample index is invented for that
row. Static reports retain the same typed evidence and copied raw trial JSON.

```sh
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build \
  -o .wasmbench/wasmbench-linux-process-snapshot ./cmd/wasmbench
sh recipes/test-linux-snapshot-memory-adapter.sh \
  wasmbench:code-lifetime-package-v2 snapshot-memory-local
```

Native Linux/arm64 `runs/linux-snapshot-memory-adapter-v2` passed original and
exact-tool replay: eighteen successful trials per run (two sacrificial and
sixteen memory trials), 224 boundaries and 640 readings per run. All four stages
and both backends passed typed bundle verification, coverage checks and current
and archived report-builder verification. This is exploratory shared-VM
qualification, not dedicated-host performance or physical reclamation evidence.
The earlier `-v1` also passed before the Parquet v10 extension. Fresh timing
regression `runs/linux-process-snapshot-adapter-v4` passed all four stages,
exact-tool replay and current/archived report validation without memory barriers.

## Interactive process-footprint report

Reports now include versioned `snapshot_memory_views` derived from validated
raw process readings. The viewer works in standalone memory reports and in
timing reports with a strictly matched `--memory-run`. Source, template and
restored child stay separate, including both restorations. Trial, sample,
process role and RSS/PSS/private/virtual controls make every retained sample
reachable. A common scale spans all roles within the selected sample; filters
do not rescale one process to conceal the others. RSS is never summed across
processes or substituted for a peak measurement.

Birth ticks, raw collector brackets and byte counts remain exact decimal
strings in derived data. Denied smaps keeps the status and reason rather than
zero-valued PSS/private bars. Failed/unsupported trials link to raw evidence,
but failed prefixes do not supply successful chart values. Click or keyboard
activation opens identity, bracket and raw record/reading coordinates. Paired
views link to `raw-memory`, not the timing pass's trial JSON.

```sh
sh recipes/test-linux-snapshot-memory-adapter.sh \
  wasmbench:code-lifetime-package-v2 snapshot-paired-local paired
```

The optional `paired` recipe collects timing and memory sequentially in one
container before report generation, preserving the same recorded host identity.
Earlier independent containers had different hostnames and were correctly
refused by the join gate; that gate was not weakened. Native Linux/arm64
`runs/linux-snapshot-memory-paired-view-v1` passed timing collection, memory
collection, exact-tool memory replay, coverage and standalone/paired report
validation with both current and archived builders. The paired report retains
sixteen memory trials and 640 separate process readings.

Browser QA covered real desktop/mobile rendering, role/sample/metric changes,
click and keyboard evidence, exact raw-memory links and no console errors.
Screenshots are under `output/playwright/`. This remains exploratory shared-VM
evidence, not official comparative performance or physical COW accounting.
Latest native-rendered report `runs/linux-snapshot-memory-report-view-v1/report`
was rebuilt offline from that same matched evidence with the final role colors
and mobile width constraints. Both current and archived native builder checks
passed; no measurement values were overwritten or recollected for styling.

## Simultaneously held restored groups

The standalone `--density-worker BACKEND INSTANCES` qualification now holds
1–32 restored processes concurrently. The source drops its instance, module
and engine before restoration. Four explicit inspection barriers retain the
source/template and the entire declared group: template after source release,
idle restored children, touched children, and executed children. Every child
PID/birth remains identical across the three group barriers; each procfs reading
is independently bracketed and parent-bound. This is not simultaneous atomic
sampling, a summed RSS value, or a process-tree peak.

Each child writes its own marker at offsets 65535, 131071 and 196607, the ends
of three 64-KiB Wasm pages. Full-memory SHA-256 proofs are checked before writes,
after writes, after execution/passive-segment use, and again after every sibling
has executed. The template retains its original full-memory hash and hidden
state. These writes are not physical-page COW fault counts or copied-byte
measurements. Touched/executed footprints also include verification, embedding
and control-channel work; they are not isolated guest-memory costs.

Provisioning time spans the source's restore request through receipt of all
live-ready identity frames. Touch/execution clocks belong to individual child
processes. All are memory-profile diagnostics, not headline timing. Prefix
validation checks ordered membership and raw readings but cannot establish
completed state proofs or cleanup. Full validation additionally requires the
completed child/template proofs and explicit reaping results.

Build the Linux qualifier and a native Linux Go test executable first, then run:

```sh
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go test -c \
  -o .wasmbench/snapshot-density.test ./experiment
sh recipes/test-linux-snapshot-density.sh \
  wasmbench:code-lifetime-package-v2 snapshot-density-local
WASMBENCH_SNAPSHOT_RETAINED_DENSITY_EVIDENCE_DIR="$PWD/runs/snapshot-density-local" \
  go test ./experiment -run TestRetainedSnapshotDensity -count=1 -v
```

Use `GOARCH=amd64` on an AMD64 host. The recipe refuses image/daemon architecture
mismatch, preserves existing evidence directories, archives both executables,
and runs read-only/network-disabled/capability-dropped with an init reaper.
Its checksums bind ten successful groups, ten cleanup prefixes, logs and image
identity. `runs/linux-snapshot-density-v3` passed native Linux/ARM64 counts
1, 2, 4, 8 and 32 for both Cranelift and Winch. All four invalid-continuation
barriers and owned-source death at the eight-child idle barrier passed cleanup,
including absence of declared zombies. Offline native and macOS validators
passed. Cleanup assertions attest that test execution; raw prefixes alone cannot
independently prove processes are gone later. AMD64/remote CI remain unverified.

This closes bounded fixed-fixture simultaneous-group qualification. The private
worker alone is not a generic snapshot capability. Density/marginal-cost reports, partial-provisioning fault
qualification and broader guest/ABI support are still required. A single fresh
qualification per count in a shared VM is not comparative performance evidence
or qualification for thousands of instances.

## Sealed restored-density memory runs

The dedicated Linux adapters now expose `can_inspect_linux_snapshot_density`
and the separate `process-snapshot-density` scenario. The versioned corpus locks
counts 1, 2, 4, 8 and 32 against the same canonical import-free Wasm bytes. Each
batch inspects 1–32 fresh concurrent groups sequentially; each group has the
complete template/idle/touched/executed barriers and completed qualification
proof. The nested worker proof remains explicitly qualification-scoped rather
than asserting support for arbitrary snapshot workloads.

```sh
wasmbench run --suite process-snapshot-density --profile memory --phase-barriers \
  --runtimes wasmtime-process-snapshot,wasmtime-winch-process-snapshot \
  --scenarios process-snapshot-density --launches 2 --samples 2 \
  --operations 1 --warmup 0 --out runs/restored-density
wasmbench reproduce runs/restored-density --out runs/restored-density-replay
wasmbench report --run runs/restored-density --out reports/restored-density
```

Sacrificial correctness admission uses two separate fresh groups in a separate
memory-profile process. Admission is excluded from analysis. The controller
collects source/template/every restored child while held at explicit barriers;
root-only samplers and process high-water RSS are bypassed. Cgroup lifetime
accounting, where delegated, stays separate. Online and sealed-loader validation
enforce count, backend, sample order, canonical artifact, exact ancestry and
native Linux architecture. Failed prefixes remain raw diagnostics.

`samples.parquet` v11 retains nullable `snapshot_density_evidence_json`, including
all groups/proofs and exact raw readings. This does not populate top-level
sample/elapsed/operation fields or headline latency eligibility. Reproduction
archives the runner, adapter and analyzer normally. For a build-first restricted
native qualification and complete CLI/replay/report check, use:

```sh
sh recipes/test-linux-snapshot-density.sh \
  wasmbench:code-lifetime-package-v2 restored-density-product product
```

Native ARM64 `runs/linux-snapshot-density-product-v2` completed original and
exact-tool replay with 30 successful trials per run: ten sacrificial admissions
and twenty measured trials, forty measured groups and 1,448 raw readings.
Both backends and all counts passed current and archived report verification.
Report curves and a dedicated interactive density viewer derive idle, touched
and executed group PSS/private sums from the complete raw readings. Each group
includes source, template and all restored children; sums are non-atomic
boundary accounting, not physical peaks or summed RSS. Missing smaps excludes
the whole affected launch rather than selecting readable groups. Inner groups
reduce to launch medians; adjacent-count marginal costs pair randomized blocks.
Two launches provide medians but no confidence intervals. Raw-launch links
retain exact identities, state proofs and collector brackets. This shared-VM check is
functional qualification, not official comparative performance.

The density viewer also exposes source-clock provisioning latency and the
arithmetic mean of child-local touch/execution regions, each derived from the
validated proof. These are instrumented memory-pass diagnostics, never headline
timing or summed group wall time. Children are not independent replications.
Adjacent-count clock finite differences retain the region's denominator; they
do not estimate total execution cost of additional children. The metric registry
defines each scope and boundary. Missing or inexact clocks fail validation.
Both point and marginal intervals are withheld below three independent launches
or paired blocks, respectively.

Each density point also links to an exact per-launch process explorer. Choose
the group, boundary stage and source/template/restored role to inspect individual
RSS, PSS, private and virtual-memory readings. Child indices and PID/birth pairs
identify every restored process; collector brackets and byte values serialize as
decimal strings. Each row links its group/record/reading coordinates to the raw
trial. Denied smaps remains unavailable without suppressing recorded RSS; failed
prefixes retain boundary counts and raw evidence only. No sum of individual RSS
or peak/physical-reclamation claim is derived from this view.

## Integration gates still open

### Partial provisioning cleanup qualification

The separate qualifier CLI `--density-failure-worker BACKEND INSTANCES
FAIL_AFTER` pauses after an incomplete group is held ready, emits a distinct
`linux-process-snapshot-density-partial-v1` frame, then injects an error through
the template's existing owned-child cleanup path. Normal adapter inspection has
no fault selector. Partial frames cannot pass the complete-group boundary,
prefix or proof validators and never produce headline samples or a completion
proof.

Native ARM64 `runs/linux-snapshot-density-partial-product-v1` checks both backends
after 1, 4 and 7 of 8 children and after 31 of 32 children. Raw procfs readings
bind each simultaneously held incarnation; after failure, read-only PID/birth
checks require source, template and every declared child to disappear, including
zombies. Source exit must be through the normal error path, not an outer timeout
or test signal. Eight retained cases reject missing cleanup, altered membership,
false identities, incomplete readings and clock overlap. The recipe archives the
worker/test binaries and includes every partial-failure file in its checksum
gate. Its ordinary product path also passed original/replay and current/archived
report verification with the same rebuilt worker.

An initial qualification directory `runs/linux-snapshot-density-partial-v1`
remains preserved: the new eight cases passed, but an existing source-death probe
hit procfs `ESRCH` while reading a disappearing process. The checker now accepts
ENOENT/ESRCH as disappearance, without masking permission/I/O failures; a
portable regression test captures that distinction. The complete rerun in
`runs/linux-snapshot-density-partial-v2` passed. This injection qualifies shared
ownership cleanup, not all real fork, FD exhaustion, OOM or malformed-ready
failure modes; AMD64 and broader native failure qualification remain open.

Only the dedicated native Linux configurations supply process-COW timing.
No physical COW fault count, copied-page volume, snapshot size, instance-only
RSS, physical-memory density, reclamation or leak claim follows from this proof.
Host files/resources, WASI, shared memory, WasmGC, guest threads and an active
guest execution stack are outside the qualified fixture.

Product integration still requires dedicated-host cgroup qualification;
broader simultaneous restored-instance density metrics and diagnostic clocks; broader
guest/runtime failure qualification; broader snapshot ABI/platform support.
Parent adapter RSS cannot stand in for the footprint of its restored children.
Do not enable a generic snapshot capability or join these proofs to headline
latency or memory graphs before those gates pass.
