# Native executable-code publication and retirement

The opt-in Wasmtime `native-code-lifetime` feature supports the Linux
`code-lifetime` diagnostic scenario through the separate
`wasmtime-code-lifetime` and `wasmtime-winch-code-lifetime` configurations.
The controller validates and seals its events, exports typed Parquet, and
renders an ownership timeline with links to each raw trial. Ordinary adapter
configurations and timing paths do not install this collector.

## What is observed

Wasmtime 46.0.1's public `CustomCodeMemory` hooks publish a region for execution
and unpublish it when the engine no longer permits execution from that region.
The collector records ordered events with a monotonic relative clock, unique
publication identity, address, capacity, active published capacity and cumulative
published capacity. A reused address receives a new publication identity.

The domain is page-aligned executable text, including padding, constants and
wrappers. It is not instruction-only function size, all native allocations,
process RSS, physical residency or mapping reclamation. Unpublication changes
RX pages back to RW; it does not itself prove `munmap`, physical page release,
absence of leaks, or allocator reclamation. Cumulative publication is not total
compiler emission: precompiled images can be republished without generating
new code. No graph should label it emitted bytes.

The version-1 wire contract binds the publication to the exact Wasm digest,
native image digest, backend, architecture, host page size and image offset.
Addresses are canonical unsigned decimal strings, preserving values above
JavaScript's exact integer range. Relative diagnostic clocks and capacities
are bounded to exact integers; they are not headline latency samples.

One fresh module produces one publication and one retirement event. Five
checkpoints record compilation, verified instantiation, dropping all Module
handles, dropping the owning Store, and dropping the engine. The workload is
called and checked against its exact oracle before and after Module handles
drop. The Store keeps its code callable; retirement follows Store release.
Independent input analysis establishes complete import-free function coverage,
and exported function ranges remain bound to the recorded native image.

This scenario accepts core, stateless, import-free workloads with exact-result
oracles, one sample and one operation, without warmup, phase barriers, host
profiles, continuations, initialization, input injection or sustained options.
Sacrificial first-call verification uses an ordinary engine in a separate
process. The diagnostic binary rejects other measurement scenarios and cannot
be combined with the native allocator feature.

## Protection and platform policy

The implementation is Linux-only for arm64/amd64. It uses `rustix` mprotect,
enforces RW to RX (never RWX), clears the instruction cache before protection,
and flushes pipelines before execution. Cache maintenance uses the exact pinned
`wasmtime-internal-jit-icache-coherence` 46.0.1 dependency. That crate explicitly
does not support use outside Wasmtime; engine upgrades require requalification.

On arm64, attachment pins the compiler's `use_bti` setting to detected host BTI
and applies matching Linux `PROT_BTI`. Directly attaching the publisher without
its configuration method is not supported. An earlier qualification correctly
rejected BTI hosts before this matching policy was implemented. Cranelift and
Winch now both execute correctly with that policy on the local BTI-capable
Linux/arm64 VM. Native amd64 and other platforms remain unverified here.

Invalid, overlapping and unknown nonempty ranges fail without changing event
evidence or permissions. Wasmtime 46 drops an empty text image through a
zero-length unpublish callback without having called publish; that is a no-op,
not a zero-valued code publication. Errors do not produce successful lifecycle
evidence. The diagnostic ledger serializes callbacks and snapshot reads.

## Collect and view on Linux

From the repository root on a native Linux host:

```sh
./bin/wasmbench build --runtimes wasmtime-code-lifetime,wasmtime-winch-code-lifetime
./bin/wasmbench run --suite core \
  --runtimes wasmtime-code-lifetime,wasmtime-winch-code-lifetime \
  --scenarios code-lifetime --profile code \
  --launches 3 --samples 1 --operations 1 --warmup 0 \
  --out runs/code-lifetime
./bin/wasmbench verify --run runs/code-lifetime
./bin/wasmbench reproduce runs/code-lifetime --out runs/code-lifetime-replayed
./bin/wasmbench report --run runs/code-lifetime --out reports/code-lifetime
./bin/wasmbench verify-report --dir reports/code-lifetime
./bin/wasmbench serve --dir reports/code-lifetime
```

Build the CLI first with `make build` if it is absent. Use new output directories;
existing sealed evidence is not overwritten. The diagnostic executable is built
under `adapters/wasmtime/target/code-lifetime/release`, separate from ordinary
and allocator adapters. The independent analyzer is also required.

The report's native code-lifetime panel shows active and cumulative published
capacity on a diagnostic clock, the two publication events and all ownership
checkpoints. Zero active capacity after retirement is a real observation;
missing evidence is not zero. `code-lifetimes.parquet` preserves trial outcomes,
nullable event/checkpoint fields and exact address strings. Report verification
recomputes this export from the copied raw bundle, detecting altered derived
data even if someone regenerates file checksums.

## Build and qualify using Docker

```sh
sh recipes/wasmtime-linux.sh test-code-lifetime
sh recipes/wasmtime-linux.sh build-code-lifetime
```

The pinned Docker build mounts source read-only and runs six real-engine tests
for both backends. These verify:

- code stays callable after every Module handle drops while a Store owns it;
- the actual unpublish callback occurs when that Store drops;
- Module clones do not create extra publication events;
- simultaneous modules have distinct identities and correct active totals;
- repeated compile/drop increases cumulative publication but returns active
  capacity to zero each time;
- `/proc/self/maps` shows published code as private RX, never writable/executable;
- invalid callbacks, invalid Wasm and empty modules invent no events.
- the diagnostic wire output binds the native image, records verified ownership
  and rejects incorrect results and timing/batch/barrier requests.

The Docker recipe writes the Linux diagnostic binary to
`.wasmbench/wasmtime-linux-target/code-lifetime/release/adapter-wasmtime`.
This is not the native registry path and a Linux binary cannot run natively on
macOS. Container collection must mount that binary at the registry path and
mount the matching Linux analyzer; the build recipe itself does not collect a
run. It does not overwrite ordinary adapter binaries.

The Ubuntu CI job invokes the feature-qualified tests, builds the separate
diagnostic adapter, and runs a controller/sealed-report round trip. Remote CI
execution is not claimed. Local qualification is not performance data.

## Packaged Linux collection and replay

The product image includes the independent analyzer and separate diagnostic
adapter. Build first, then collect; do not build or run tests alongside a live
measurement:

```sh
docker build -t wasmbench:local .
sh recipes/test-linux-code-lifetime.sh wasmbench:local code-lifetime-container
./bin/wasmbench serve --dir runs/code-lifetime-container/reports/original
```

The recipe resolves the image to an immutable local ID, rejects emulated image
architectures and existing evidence paths, and uses an unprivileged,
network-disabled, read-only-root container. Its only writable host mount is the
new evidence directory. All collection and replay finish before report
generation. Both runs must pass sealed validation plus a separate twelve-cell
coverage gate; both reports must verify with current and archived builders.
Original and replay run inside one container to preserve observed host identity.
The coverage gate is additional qualification, not a replacement for seal or
protocol validation. Missing or unsupported measured cells fail the recipe.

The locally built `wasmbench:code-lifetime-package-v2` image passed this recipe
without host-binary overrides on Linux/arm64. Its evidence lives at
`runs/linux-code-lifetime-packaged-v2`, with original and replay reports under
`reports/`. Each run has twelve successful measured trials, 24 publication
events and 60 ownership checkpoints. Both reports verified with current and
archived builders. The original report is previewed locally on port 8162.
The same image also passed the existing core/float/density packaged regression
recipe. Native AMD64 and remote CI are still unverified.

For an existing base image, an optional third argument supplies a directory
containing already-built native Linux `wasmbench`, `wasm-analyze` and
`adapter-wasmtime-code-lifetime` binaries. They are mounted read-only at the
registry paths. This mode is useful while developing the adapter; it does not
build binaries or establish their correctness. Preserve a failed recipe's
partial evidence and choose a new output name for the next attempt.

## Verified evidence and remaining scope

The local Linux/arm64 bundles at `runs/linux-code-lifetime-v1/original` and
`runs/linux-code-lifetime-v1/reproduced` each contain twelve successful
diagnostic trials: two core workloads, two backends and three independent
launches. Each retains 24 callback events and 60 ownership checkpoints, without
timing samples. Collection and replay ran sequentially in one restricted
container; builds, tests and browser checks ran after collection finished.
The controller/report smoke test also passed in Linux/arm64. The sealed original
report is under `reports/linux-code-lifetime-v3` (local preview port 8161).

These are exploratory shared-VM diagnostics, not dedicated-host performance or
publication qualification. Native AMD64 and remote CI remain unverified.
Multiple-module generations, imported host bridges and AOT republishing need
separate scenario attribution before collection can expand beyond this contract.
Total compiler emission requires its own exposed observation; it cannot be
inferred from publications. Physical reclamation, leak qualification,
whole-instance/COW snapshots and restored whole-instance density remain separate
unfinished product requirements.

Sources: pinned engine `runtime/code_memory.rs` and
`runtime/vm/sys/unix/mmap.rs`; public
[CustomCodeMemory documentation](https://docs.wasmtime.dev/api/wasmtime/trait.CustomCodeMemory.html).
