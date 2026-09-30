# WASI Preview 1 reactors

The `reactors` suite exercises a separate `wasi-reactor` ABI on wazero's compiler
and interpreter configurations, plus Wasmtime Cranelift, Winch and pooling.
Other adapters remain explicitly unsupported;
they are not silently treated as core modules or WASI commands.

```sh
wasmbench run --suite reactors --runtimes wazero,wazero-interpreter,v8 \
  --scenarios compile,instantiate,app-init,first-call,steady,cold-process,teardown \
  --launches 6 --samples 10 --operations 100 --warmup 3 --out runs/reactors
wasmbench run --suite reactors --runtimes wazero,wazero-interpreter \
  --profile memory --scenarios compile,instantiate,app-init,first-call,steady,teardown \
  --out runs/reactor-memory
```

## Host and correctness contract

The versioned `wasi-preview1-reactor-noio-v1` host profile requires an explicit
`_initialize: () -> ()`, called exactly once for every fresh instance before
input installation or workload calls. `_start` exports are rejected as an
ambiguous command/reactor contract. The fixture also has a Wasm start function,
which executes as part of instantiation before explicit initialization.
The once-only initializer convention is consistent with
[wasi-libc's reactor entry point](https://github.com/WebAssembly/wasi-libc/blob/main/libc-bottom-half/crt/crt1-reactor.c).
This initial profile requires an initializer even though some reactor modules
can omit one.

No host files are preopened; argv/environment are empty, stdin is empty, random
bytes are zero, and clocks use the recorded adapter-specific synthetic behavior.
Wasmtime wall time starts at 1640995200000000000ns and monotonic time at zero;
both advance 1ms per read. Wazero uses its default synthetic clocks. No network
capability is supplied. Nonempty stdout/stderr writes are rejected, including
writes made by initialization or oracle exports. This is a deterministic
benchmark host profile, not a production WASI environment or a security
certification. Stream-producing or filesystem-dependent reactors need a future
explicit host profile, not ambient access added to this one.

The existing exact scalar result and memory oracles apply. Every result in a
steady batch is verified, including warmup results. Memory checks describe the
post-batch state; they are not a history of every intermediate memory mutation.
The two fixtures check real Preview 1 argument/environment/random imports,
initialization order, input installation after initialization, and either
stateless reuse or fresh state per sample. A stateful fixture traps on a second
call, catching accidental pre-calls or reuse.

## Measurement boundaries

| Scenario | Timed operation | Untimed preparation and verification |
| --- | --- | --- |
| Compile | Guest compile API including mandatory validation | Engine/WASI host creation; instantiate, initialize, call, verify, release |
| Instantiate | Guest instance creation including Wasm start | Engine/module/WASI host creation; explicit initialize, input, call, verify, release |
| App-init | `_initialize` call only | Fresh instance/start before; input installation and verified workload call after |
| First-call | One workload call | Fresh initialized instance and input before; verification and release after |
| Steady, stateless | Declared local batch of workload calls | One initialized instance per request; buffers allocated before timer, every result checked after |
| Steady, fresh | One workload call per sample | Fresh initialized instance and input for each sample |
| Teardown | Close guest instance, compiled module, and runtime (including host module) | Fresh runtime/module/instance, initialization and verified workload call before |
| Cold process | Controller's process-to-correct-result boundary | Sacrificial correctness uses a separate process |

Compile, instantiate, app-init, first-call and teardown use one operation per
sample without warmup. Steady retains configured warmup observations. Stateless
steady has no hidden workload pre-call. Engine/WASI host and guest module are
retained where the table says so; no cache subtraction or forced GC occurs.

On wazero, the memory pass records Go-managed allocation/GC observations around the same
API window, plus guest logical memory after verification where an instance is
still alive. Wasmtime reports post-verification guest logical memory, with OS
collectors supplied by the controller; it does not claim native allocator counts.
Wasmtime teardown drops the Store, module, linker and engine without forced
allocator reclamation. Pooling uses the separately recorded fixed pool policy;
fresh instances do not inherit guest state. Logical memory is not RSS, and teardown does not invent a zero
guest footprint after release. Phase barriers, hardware counters, profiling,
native code export and trajectory/engine-init scenarios remain unsupported for
this reactor profile. Memory-pass timers remain instrumented diagnostics in the
memory-labelled report; do not compare them with minimally instrumented timing
passes. The official publication gate rejects memory profiles as headline timing.

`corpus/testdata/wasi-reactor.wasm` was generated from its MIT-licensed WAT source
with WABT 1.0.41:

```sh
wat2wasm corpus/testdata/wasi-reactor.wat -o corpus/testdata/wasi-reactor.wasm
```
