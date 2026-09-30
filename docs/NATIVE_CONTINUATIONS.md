# Native execution-stack continuations

The `continuations` suite measures native execution-stack capture, guest
resumption, first write and subsequent execution as separate scenarios. It is
not a whole-instance snapshot implementation. The ordinary adapter continues
to advertise `can_snapshot: false`, alongside the narrower
`can_native_continuation` compiler capability. Existing eager guest-memory
checkpoints retain their previous definitions.

The pinned wazero 1.12.0 experimental interface captures execution stacks from
an enabled host callback. The new measurement module uses the actual
`Snapshotter.Snapshot` and `Snapshot.Restore` methods, without copying guest
memory to simulate that facility. Compiler and interpreter method availability
does not imply they satisfy the same correctness contract.

## Artifact and oracle

`corpus.ContinuationModule(depth)` produces deterministic core Wasm bytes at
depths 0 through 128. Depth counts recursive ancestors above the capture frame,
not native stack bytes or engine-internal frame counts. Qualification covers
0, 1, 8, 32 and 128, with three fresh instances per compiler depth.

Each ancestor retains a distinct parameter. After restoration, the stack result
must equal `7 + depth * (depth + 1) / 2`, and the first resumed-guest host marker
must have run exactly once. Before restoring, guest code changes memory byte 0
to 11 and the mutable scalar global to 99. Those values MUST remain changed:
execution-stack restoration is not linear-memory or global restoration.

After the first post-restore write, byte 65535 must be 22 and the scalar 100.
Verification checks all 65536 memory bytes, records full-memory SHA-256 at both
points, and requires the subsequent full-memory-plus-global checksum to be 133.
Verification is outside all four clocks. Corrupting an unrelated byte fails the
oracle. Failed verification, cancellation or instance release returns no
successful sample; earlier verified samples may remain diagnostic evidence in
a failed batch and never enter headline analysis.

## Clock definitions

| Clock | Start | End |
| --- | --- | --- |
| Creation | Immediately before native `Snapshot()` | Native method returns |
| Restore-resumption | Immediately before native `Restore([1])` | Entry to first resumed-guest host marker |
| First write | Embedding call to `first_write` | That call returns |
| Post-restore execution | Embedding call to full-memory `benchmark` | That call returns |

Restore never returns normally. Restore-resumption includes engine control
transfer and guest-to-host marker transition; it is not restore API-return
latency. First write is an embedding operation, not an attributed COW fault.
Snapshot storage size is unavailable through the pinned public interface; it is
not estimated from a Go interface header or mislabeled as checkpoint payload.
Memory passes expose seven separately scoped Go allocator/heap/GC observations
around the selected stage, plus optional OS boundary snapshots and cgroup
accounting outcomes. The Go domain excludes mmap and foreign allocation.
No forced collection or allocator purge is included in these measurements.

The measurement module owns compiled and host modules, while callers own the
engine. Each measurement uses a fresh instance and a new capture restored once
inside its original invocation. The capture is discarded when that invocation
returns. Close and measurement are serialized. Cross-instance captures are
rejected by the pinned engines; invalid restore can propagate a native panic.

## Run and inspect

Build adapters/analyzer before measurement, then run the passes sequentially:

```sh
make build
./bin/wasmbench run --suite continuations --runtimes wazero,wazero-interpreter \
  --scenarios continuation-create,continuation-resume,continuation-first-write,continuation-execute \
  --profile timing --launches 3 --samples 2 --operations 1 --warmup 0 \
  --out runs/continuations-timing
./bin/wasmbench run --suite continuations --runtimes wazero,wazero-interpreter \
  --scenarios continuation-create,continuation-resume,continuation-first-write,continuation-execute \
  --profile memory --phase-barriers --launches 3 --samples 2 --operations 1 --warmup 0 \
  --out runs/continuations-memory
./bin/wasmbench report --run runs/continuations-timing \
  --memory-run runs/continuations-memory --out reports/continuations
./bin/wasmbench verify-report --dir reports/continuations
./bin/wasmbench serve --dir reports/continuations
```

The controller admits exact canonical artifact bytes with its pinned independent
analyzer, uses two sacrificial restore/resumption samples before measurement,
and gates the runtime build/backend/protocol. Batches have one operation per
sample and no warmup. Unsupported profiles, resets, arbitrary artifacts and
whole-instance contracts are rejected, not silently converted.

The memory barriers are before selected stage, selected stage completed and
verified instance released. For resumption the middle barrier is inside the
resumed host marker, outside its measured timestamp. Instance release follows
all state verification; the shared module/engine remain retained until batch
close. Barriers and Go snapshots are outside latency clocks, but can perturb
memory runs; their timestamps/peaks are diagnostic, not timing-pass evidence.

Raw trial JSON contains `continuation_result`, full-memory digests, surviving
globals and stack checksum. Offline validation rechecks canonical bytes,
qualified identity, request/sequence, proof values, memory-window provenance,
ordered barriers and exact binding of four OS snapshot outcomes at every
boundary. Sample Parquet v8 retains nullable `continuation_result_json`.
The report labels native stack capture and guest resumption separately, links
raw timing and memory evidence, preserves interpreter unsupported outcomes and
never adds memory to latency. Whole-process peak RSS from a stage-focused
memory launch remains distinct from a phase-only peak.

## Current qualification and remaining work

```sh
go test ./adapters/wazero -run NativeContinuation -count=1 -v
go test ./corpus -run Continuation -count=1
```

On the current Darwin/arm64 host, the pinned compiler passes every qualified
depth. The pinned interpreter returns without reaching the guest-resumption
marker and therefore fails qualification at every tested depth. Tests preserve
that failure rather than accepting API availability or weakening the oracle.
This does not establish whether every possible interpreter snapshot use fails.

Fresh paired evidence at `runs/native-continuation-v1/` covers five depths, four
scenarios, three independent launches and two samples per launch: 60 successful
measured trials and 120 samples in each pass. Interpreter cells remain visibly
unsupported. `reports/native-continuation-v3` is the final sealed paired report
with all four stages selected by default (v1/v2 are preserved). Mobile QA fixed
an unavailable-RSS rendering bug; missing cells now have no colored value fill.
The macOS host reports procfs boundary residency as unsupported, not zero.
Builds/tests finished before either pass; this exploratory host is not official
publication-qualified.

## Reproducible Linux qualification

Build the packaged image before collection, then use a new evidence name:

```sh
docker build -t wasmbench:continuations .
sh recipes/test-linux-continuations.sh wasmbench:continuations continuations-linux
./bin/wasmbench serve --dir runs/continuations-linux/reports/original
```

The recipe resolves the image to its content-addressed ID, runs timing, memory
and their locked reproductions sequentially inside one container, validates all
four sealed bundles, and generates and verifies separate original/replay reports
with current and recorded builders. One container preserves the observed host
identity across passes; separate random container hostnames are not silently
accepted as a paired host. Existing evidence names fail instead of overwriting.
An optional third argument names a directory containing prebuilt native Linux
`wasmbench`, `adapter-wazero` and `wasm-analyze` binaries, mounted read-only over
the packaged paths. This is useful for focused qualification without rebuilding
unrelated adapters. The sealed bundles retain the actual binary identities.

Containers are unprivileged, network-disabled and read-only except for scoped
evidence and temporary storage. `/tmp` permits execution so the trusted archived
report builder can be verified; it still uses `nosuid,nodev`. Do not verify
untrusted builder archives. This does not establish dedicated CPUs or a
per-adapter cgroup. The gate requires available procfs residency snapshots and
explicitly unavailable kernel phase peaks; neither is substituted for the other.
All builds must finish before running the recipe. Do not run browser QA, tests
or other workloads on the measurement host during collection.

Verified Linux/arm64 evidence is in `runs/linux-native-continuation-v3/`, including
`reproduced/`. Each timing/memory pair retains 60 successful compiler trials,
60 unsupported interpreter trials and 120 successful samples per profile.
Each memory pass has 360 boundaries and 1440 available RSS/PSS/private/virtual
snapshots; all 120 cgroup phase peaks are unavailable. Original and replay host
identities match. The paired original report is
`reports/linux-native-continuation-v1`; the earlier Linux v1/v2 run directories
remain preserved but their differently named containers are not pairable.
Original/replay reports under `runs/linux-native-continuation-v3/reports/`
also passed both builder verification modes inside the restricted Linux
container after measurement had finished.

The packaged CI workflow invokes this recipe and uploads its evidence and
reports. Remote CI execution and native Linux/amd64 qualification have not been
verified here. Docker's shared Linux VM is exploratory, not an official host.

The optional Go evidence gates can recheck either pair without recollection:

```sh
WASMBENCH_CONTINUATION_BUNDLE=../runs/linux-native-continuation-v3 \
WASMBENCH_CONTINUATION_LINUX_BUNDLE=../runs/linux-native-continuation-v3 \
  go test ./experiment -run Continuation -count=1
```

Paths above are relative to the `experiment` test package. Substitute
`../runs/linux-native-continuation-v3/reproduced` to check the replay pair.

Whole-instance restoration, restored instance density, snapshot storage size
and COW snapshots remain separate unfinished requirements. This family does
not replace them or establish physical reclamation.
