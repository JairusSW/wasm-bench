# Component Model support status

Independent structural inspection and sealed experiment admission are implemented.
Wasmtime supports component compilation and lifecycle command workloads for
WASI Preview 2 components. Compile-only trials require
`abi: component`, `oracle.kind: component_compile_only`, one operation per sample,
and the minimally instrumented timing profile. Each sample compiles
the component with the pinned Wasmtime engine and releases it. The `verified`
field means compilation succeeded; it makes no claim about guest behavior.

The command path requires `oracle.kind: exact_command`, `reset:
fresh_instance_per_sample`, `host_profile: wasi-preview2-readonly-v1` or
`wasi-preview2-temporary-filesystem-v1`, and a pinned command contract with exact
stdout and/or stderr hashes. It supports compile, instantiate and first-call
scenarios in separate timing and memory passes, with one operation per sample.
Memory passes optionally use the standard ordered before/returned/released
barriers. Timing passes reject barriers; these scenarios have no warmup samples.
Every sample gets a fresh WASI
context, component instance, deterministic random source and synthetic clocks;
the environment is empty and stream capture is bounded by the command contract.
The read-only profile grants no filesystem preopen when no fixture files exist,
and grants a read-only root when they do. The temporary-filesystem profile
copies fixtures into a fresh private directory for every sample and grants
read/write access only to that disposable directory; it is deleted after the
sample's instance and store are released. Fixture staging and cleanup are outside
the first-call timer. Guest-created files and input mutations do not persist
between samples, including sacrificial correctness samples. No host directory
or network permission is inherited. This host does not yet expose configurable
per-workload WASI resource quotas.

The Rust smoke and filesystem fixtures in `../../Wago/wasi/p2/testdata` were run
through the adapter protocol with matching output hashes. The smoke fixture
verified argument/stdin handling, empty environment, deterministic clocks and
randomness. The filesystem fixture read pinned input and wrote only inside the
disposable staging directory. The sibling socket-denial fixture trapped under
Wasmtime 46.0.1 and is not admitted as a supported oracle yet. Other P2 worlds,
general WIT export calls, counter/profiling profiles, and WASI resource quotas remain
unsupported.

## Typed host-import-free exports

The separate `component-u64-v1` profile executes a named export with zero to
sixteen Component Model `u64` parameters and exactly one `u64` result. It requires
`oracle.kind: exact_u64`, `reset: fresh_instance_per_sample`, and an explicit work
denominator. The adapter checks the actual signature before invocation; it does
not reinterpret core-Wasm bits as arbitrary WIT values. Values stay decimal
strings on the protocol wire, including values above JavaScript's exact-number
range. An empty linker supplies no ambient host capabilities; unresolved imports
fail rather than receiving a WASI context.

Both Cranelift and Winch advertise `can_component_u64_calls_v1` and the separate
`can_component_u64_memory_v1` capability. Compile, instantiate and first-call
use one operation per sample, no warmup, and a fresh Store/instance. Timing has
no barriers; memory optionally uses before/returned/released barriers.
Engine/linker creation and argument buffers
are prepared untimed. Compile measures `Component::new`, including validation;
instantiate measures `Linker::instantiate`, including nested core starts;
first-call measures the checked function call and post-return API. Export lookup,
signature checking, exact oracle verification, and Store/component release are
outside those timers. The compiled component is shared between samples except
in compile. Every stage verifies guest behavior after the measured API.

Memory barriers use the standard lifecycle stage names. The returned boundary
precedes oracle verification; the released boundary follows successful
verification and drops the Store and any per-sample compiled Component. The
engine, linker, and shared compiled Component remain alive. No forced allocator
reclamation is performed. Boundary residency is an OS snapshot; process peak RSS
includes startup and setup, not just the selected phase. Linux cgroup phase
peaks require the separately probed cgroup collector. Missing collectors retain
their explicit availability status. Memory-pass elapsed times are diagnostic,
not headline latency. A failed barrier or oracle never produces a verified
sample or a false released boundary.

Build the pinned stateful fixture and use the ordinary product CLI:

```sh
cargo build --locked --manifest-path adapters/wasmtime/Cargo.toml \
  --features component-fixtures --bin component-u64-fixture
adapters/wasmtime/target/debug/component-u64-fixture recipes/fixtures/component-u64.wasm
make build
./bin/wasmbench build --runtimes wasmtime,wasmtime-winch
./bin/wasmbench run --suite recipes/fixtures/component-u64.json \
  --runtimes wasmtime,wasmtime-winch --scenarios compile,instantiate,first-call \
  --operations 1 --warmup 0 --out runs/component-u64
./bin/wasmbench run --suite recipes/fixtures/component-u64.json \
  --runtimes wasmtime,wasmtime-winch --scenarios compile,instantiate,first-call \
  --operations 1 --warmup 0 --profile memory --phase-barriers \
  --out runs/component-u64-memory
./bin/wasmbench report --run runs/component-u64 \
  --memory-run runs/component-u64-memory --out reports/component-u64
```

The fixture generator refuses existing output. The manifest pins the resulting
bytes. Its guest changes a global on every call, so repeated samples verify fresh
instance reset. Run the opt-in sealed-bundle/replay qualification with
`WASMBENCH_COMPONENT_U64_ARTIFACT`, `WASMBENCH_COMPONENT_ADAPTER` and
`WASMBENCH_COMPONENT_ANALYZER` set to absolute paths, then
`go test ./experiment -run 'TestComponentU64(Bundle|MemoryBundle)' -count=1 -v`.
`WASMBENCH_COMPONENT_U64_EVIDENCE_DIR` optionally selects a new evidence prefix;
the test retains original/replayed bundles, restored tools and negative-contract
bundles. CI builds the fixture and runs the same qualification.

After `wasmbench verify --run RUN` has checked the seal, run
`node recipes/verify-component-u64.mjs RUN` to check the full backend/stage/block
matrix, exact results, boundary order, sample attachments, whole-process RSS and
platform-specific collector availability. This supplements integrity verification;
it does not establish dedicated-host publication qualification.

Strings, lists, records, variants, resources, nested interface exports and other
scalar types are not supported by this profile. Counter/profiling passes,
batching, steady reuse and arbitrary component worlds remain unsupported, not
inferred from the generic Component Model ABI. `can_execute_components` remains
false for general execution; the specific capability defines this bounded path.

Command lifecycle boundaries are explicit:

- Compile measures resident bytes through `Component::new`, including mandatory
  validation. The engine and WASI linker are prepared before the timer. Each
  compiled component is subsequently instantiated, run and verified untimed,
  then released.
- Instantiate measures typed `Command::instantiate` with a prepared Store, WASI
  context and linker. Nested core start functions are included; `wasi:cli/run`
  execution and behavioral verification happen afterward, outside the timer.
  The compiled component and engine remain alive between samples.
- First-call measures only `wasi:cli/run`, with fresh prepared instance and host
  state. Output capture is part of execution; digest/exit verification is outside
  the timer. The compiled component and engine remain alive between samples.

Every scenario verifies exact command output, not merely successful compilation
or instantiation. The release boundary follows verified execution, instance/Store
drop, and staging-directory removal. It retains the engine/linker and, except in
compile, the shared compiled component. Capture buffers remain alive through
sample evidence construction; no forced collection/reclamation is implied.

The external memory pass records boundary snapshots, sampled observations and a
separately labelled process-lifetime peak RSS. Process high-water RSS includes
setup and verification; it is not an exact API-phase peak. Unsupported host
collectors remain explicit unavailable observations. The capability flags
`can_component_command_lifecycle` and `can_component_command_phases` gate the
expanded contract; old adapters retain only their existing first-call timing
support. General typed component exports remain unsupported.

The opt-in regression test also exercises a complete planned run: independent
component admission, sacrificial fresh-instance correctness, measured first-call
trials, tool archiving, bundle sealing, offline reload, and exact output evidence.
After building the debug binaries, run it from the repository root with:

```sh
cargo build --locked --manifest-path adapters/wasmtime/Cargo.toml --bins
WASMBENCH_COMPONENT_ARTIFACT="$(pwd)/../../Wago/wasi/p2/testdata/rust_smoke.component.wasm" \
WASMBENCH_COMPONENT_ADAPTER="$(pwd)/adapters/wasmtime/target/debug/adapter-wasmtime" \
WASMBENCH_COMPONENT_ANALYZER="$(pwd)/adapters/wasmtime/target/debug/wasm-analyze" \
go test ./experiment -run TestComponentCommandBundle -count=1
```

The standalone filesystem-reset regression needs no sibling checkout. Its guest
requires pristine input, creates a file with exclusive creation, and mutates the
input on every invocation. Both Cranelift and Winch must pass single invocations
and multi-sample sacrificial/measured trials with exact output verification:

```sh
./recipes/build-p2-reset-fixture.sh
cargo build --locked --manifest-path adapters/wasmtime/Cargo.toml --bins
WASMBENCH_COMPONENT_RESET_ARTIFACT="$PWD/.wasmbench/p2-reset-fixture/p2-filesystem-reset.component.wasm" \
WASMBENCH_COMPONENT_ADAPTER="$PWD/adapters/wasmtime/target/debug/adapter-wasmtime" \
WASMBENCH_COMPONENT_ANALYZER="$PWD/adapters/wasmtime/target/debug/wasm-analyze" \
go test ./experiment -run TestComponentFilesystemResetBundle -count=1 -v
```

The build recipe pins Rust and its container image; CI builds the same guest
using its pinned Rust toolchain and runs the regression against release adapters.
Set `WASMBENCH_COMPONENT_RESET_EVIDENCE_DIR` to a new directory to retain the
sealed six-trial bundle (four sacrificial and twelve measured samples), restored
tools in the sibling `-restored-tools` directory, and a second sealed bundle in
the sibling `-replayed` directory. The replay uses archived adapter/analyzer
executables and workload bytes. These are functional qualification runs, not
official performance results.

To qualify all three lifecycle stages, both measurement passes, negative
contracts and archived-tool replay using that same fixture and adapter setup:

```sh
WASMBENCH_COMPONENT_RESET_ARTIFACT="$PWD/.wasmbench/p2-reset-fixture/p2-filesystem-reset.component.wasm" \
WASMBENCH_COMPONENT_ADAPTER="$PWD/adapters/wasmtime/target/debug/adapter-wasmtime" \
WASMBENCH_COMPONENT_ANALYZER="$PWD/adapters/wasmtime/target/debug/wasm-analyze" \
go test ./experiment -run 'TestComponentCommand(Lifecycle|ControllerCapabilityGate)' -count=1 -v
```

Set `WASMBENCH_COMPONENT_LIFECYCLE_EVIDENCE_DIR` to a new path prefix to retain
`-timing` and `-memory` bundles, their `-replayed` bundles and `-restored-tools`.
Each bundle has two sacrificial checks and six measured runtime/scenario cells,
with sixteen exact-output-verified samples. Each measured memory trial has six
ordered boundary events; minimally instrumented timing trials have none.

The production CLI can be qualified on native Linux with procfs boundary
readings, normal reproduction, explicitly restored-tool replay and a paired
static report. Finish all builds before collection:

```sh
sh recipes/build-p2-reset-fixture.sh
sh recipes/wasmtime-linux.sh build
# On a Linux host use its native architecture. From macOS, cross-build for
# the Docker daemon architecture (arm64 or amd64).
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o .wasmbench/wasmbench-linux-p2 ./cmd/wasmbench
sh recipes/test-linux-p2.sh wasmbench:dev NEW_EVIDENCE_NAME
```

Use an existing native Linux image containing Node and compatible shared
libraries. The recipe rejects emulation and existing evidence directories. It
uses a read-only, unprivileged, network-disabled container with only explicit
fixture/tool/evidence mounts. Original, reproduced and restored-tool runs execute
sequentially before report generation. The coverage gate requires available
RSS/PSS/private/virtual boundary readings attached to their exact samples and
keeps process-lifetime peak RSS separate. Source/compiler hashes are recorded in
the workload provenance. No delegated cgroup, phase cgroup peak, dedicated CPU
allocation or official publication is implied. Packaged-container CI runs this
qualification and retains its evidence; local proof does not prove remote CI.

A runnable smoke workload uses the existing command contract and component
artifact encoding. `command.stdin` is JSON base64 for its `[]byte` field; fixture
file `data/input.txt` maps to `/data/input.txt` when the staged root is mounted.

```json
{
  "schema": 1,
  "id": "p2/rust-smoke",
  "family": "applications",
  "artifact": "rust_smoke.component.wasm",
  "sha256": "c6979faf9c5dff8b07f2015ea2c7e96273b281f946f09752ebc201ec6d8b2d6e",
  "abi": "component",
  "host_profile": "wasi-preview2-readonly-v1",
  "features": ["component-model"],
  "export": "_start",
  "args": [],
  "command": {
    "argv": ["wasi-p2-smoke", "alpha", "beta"],
    "stdin": "ZnJvbS1ydXN0LXN0ZGluCg==",
    "exit_code": 0,
    "stdout_sha256": "a947d801afe54d4c2d2c048ee853cca4ab4a5f08921698063ba6d287b524b5a8",
    "stderr_sha256": "f7718a0427ded6fc8775c92ab79be7a8f6673fcf9a3c6b85274087c5838281dc",
    "output_limit_bytes": 4096
  },
  "work_unit": "command",
  "units_per_invocation": 1,
  "reset": "fresh_instance_per_sample",
  "oracle": {"kind": "exact_command", "expected": []},
  "license": "Apache-2.0",
  "source": "Wago wasi/p2 Rust smoke fixture",
  "generator": "rustc wasm32-wasip2"
}
```

Save the matching artifact beside the workload, then admit and run it using the
standard lock flow: `wasmbench plan --suite component.json --runtimes wasmtime
--out component.lock`, followed by `wasmbench run --lock component.lock
--profile timing --scenarios first-call --out component-run`.

Build and inspect without running guest code:

```sh
cargo build --locked --manifest-path adapters/wasmtime/Cargo.toml --bin wasm-analyze
adapters/wasmtime/target/debug/wasm-analyze artifact.component.wasm default
```

The analyzer validates the whole artifact with pinned wasmparser 0.251.0 and
emits `component-structure-v1`. Core-module inspection remains
`core-structure-v3`, preserving its existing fields and interpretation.

Component evidence contains a flat, parent-linked `nodes` tree. Every component
or nested core module has a distinct node ID, encoding, end-exclusive absolute
byte range, size, and SHA-256. Imports, exports, type references, instance and
canonical-function counts, aliases, start metadata, and core function-body
records stay with their owning node. Defined-function indices are local to that
core module; they do not include imported functions or represent a global
component function index. Type references use the pinned parser's representation,
not a complete WIT rendering or resolved host contract.

Container sizes and section payloads include nested bytes. Do not add parent and
child sizes to estimate artifact size. Nested component validation occurs in the
whole-artifact context, since outer aliases can depend on enclosing types.
Feature-removal probes apply to the whole artifact and selected policy, not to
each nested node independently. They do not establish runtime compatibility.

New analyzer locks use `artifact-structure-v1`, accepting exactly core-module
`core-structure-v3` or component `component-structure-v1` evidence. Existing
core-only locks keep their original accepted version and encoding. Component
workloads require `abi: component`, an independent analyzer lock, and a rebuilt
analyzer executable; old analyzer binaries still work for core-only workloads.
Run admission and offline loading check the encoding against every workload,
including aliases sharing artifact bytes. Saved component hierarchy checks reject
invalid parent relationships, overlapping sibling ranges, and inconsistent
root/child identities. Structure validation is not behavioral correctness.

General-purpose component worlds remain unsupported. Typed `u64` exports follow
the explicit policy above. Component command execution is restricted to the P2 policy above;
structural inspection alone grants no host capabilities.
