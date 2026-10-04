# Runtime adapters

Adapters embed a runtime in the measured process. They do not time a command-line
launcher and relabel it as compilation, instantiation or execution. Missing
engines fail with installation/path guidance; unsupported contracts stay
unsupported, not zero-duration successes.

## Configurations

| Configuration | Embedding | Initial contract |
| --- | --- | --- |
| `wago` | Go embedding | Existing capabilities |
| `wazero`, `wazero-interpreter` | Go embedding | Existing capabilities |
| `wasmtime`, `wasmtime-winch` | Rust embedding | Existing capabilities |
| `v8` | Node.js WebAssembly API | Existing capabilities |
| `v8-shell` | Direct `d8` executable | Import-free core integer scalar |
| `spidermonkey` | Direct Mozilla JS shell | Import-free core integer scalar |
| `jsc` | Raw JavaScriptCore C API host on macOS | Import-free core integer scalar |
| `deno` | Deno WebAssembly API | Import-free core integer scalar |
| `wasmi` | Rust, pinned wasmi 2.0.0 | Import-free core integer scalar |
| `wavm` | WAVM's own C embedding API | Import-free core integer scalar |
| `wasm3` | wasm3 v0.5.0 C API | Import-free core integer scalar |
| `wasmedge` | WasmEdge 0.17.1 C interpreter API | Import-free core integer scalar |

The new adapters support `compile`, `instantiate`, `first-call` and `steady`,
except wasm3's `instantiate`: its loaded/compiled module belongs to one runtime,
so reusable compiled-module instantiation is not advertised. Initializers,
input installation, integer argument/results and memory-output checks are
verified outside the timed API window. i64 values remain decimal strings on
the wire, including values above JavaScript's exact Number range.

The initial new-adapter scope excludes WASI, Emscripten hosts, components,
imports, float/trap/vector oracles, native-code export, counters and CPU
profiling. Only `mvp` is advertised conservatively. Runtime acceptance of an
additional proposal does not constitute a tested proposal capability.

## Direct JS engines

No jsvu, npm package, browser or Node compatibility shim is required.
Install an engine directly or point to an existing binary:

```sh
export WASMBENCH_V8_SHELL=/path/to/d8
export WASMBENCH_SPIDERMONKEY=/path/to/mozilla/js
export WASMBENCH_DENO=/path/to/deno
./bin/wasmbench build --runtimes v8-shell,spidermonkey,deno,jsc
```

Without overrides, the controller searches PATH for `d8`, `spidermonkey` and
`deno`. It intentionally does not assume a generic executable named `js` is
SpiderMonkey. `build` performs an actual JSON describe/close protocol probe.
The binary, script and discoverable native startup libraries are hashed and
retained by the existing exact-tool archive. Binary hashes are the identity
fallback where the shell has no reliable version API.

On macOS, `build --runtimes jsc` compiles a small C++ host against the system
JavaScriptCore framework. It supplies a monotonic `steady_clock`, binary reads
and JSON-line stdin/stdout to the same shared script. System framework versions
remain host/OS prerequisites, not distributable SDK artifacts. The ordinary
`jsc` shell's `preciseTime` is wall time and is never used as a latency timer.
Other platforms can set `WASMBENCH_JSC` to a shell/host exposing the script's
required I/O and monotonic `performance.now` or `benchNow`; those paths are not
yet qualified. Tiering, lazy compilation, engine caches and GC remain at engine
defaults and are not claimed to be controlled.

Deno uses `--no-prompt --allow-read`; no network, write, subprocess or FFI
permission is granted. Read access is broad because exact-tool replay changes
artifact paths; this is not an untrusted-guest sandbox claim.

Upstream entry points: [V8 d8](https://v8.dev/docs/d8),
[Mozilla shell](https://firefox-source-docs.mozilla.org/js/),
[JavaScriptCore API](https://developer.apple.com/documentation/javascriptcore),
[Deno](https://docs.deno.com/).

## Native embeddings

```sh
./bin/wasmbench build --runtimes wasmi

export WASMBENCH_WAVM_SDK=/path/to/wavm-sdk
export WASMBENCH_WAVM_VERSION=selected-version-or-commit
./bin/wasmbench build --runtimes wavm

export WASMBENCH_WASM3_SDK=/path/to/wasm3-sdk
export WASMBENCH_WASM3_VERSION=0.5.0
./bin/wasmbench build --runtimes wasm3

export WASMBENCH_WASMEDGE_SDK=/path/to/wasmedge-sdk
./bin/wasmbench build --runtimes wasmedge
```

Builds never download an SDK automatically. Each native SDK prefix needs:

| Runtime | Header | Library |
| --- | --- | --- |
| WAVM | `include/WAVM/wavm-c/wavm-c.h` | `lib/libWAVM` |
| wasm3 | `include/wasm3.h`, `include/wasm3_defs.h` | `lib/libm3` |
| WasmEdge | `include/wasmedge/wasmedge.h` and included headers | `lib/libwasmedge` |

Library suffixes follow the platform. A static wasm3 library works. A separate
Cargo feature/target directory is used per engine; selecting multiple features
in one binary is rejected. SDK version labels for WAVM/wasm3 are operator
supplied; exact linked library/executable hashes are recorded independently.
WasmEdge reports its linked library version. Existing native dependency closure
rules apply: some libraries remain exact host-path prerequisites after replay.

wasmi uses eager bytecode compilation and portable dispatch with no fuel.
wasm3 uses a 1 MiB runtime stack and eagerly compiles bytecode; its compile
window includes parsing, runtime allocation and module loading. A fresh wasm3
runtime is prepared outside each first-call timer. WasmEdge's `compile` window
is loader parsing plus validation, **not native code generation**; this adapter
is the interpreter configuration, not WasmEdge AOT. Native invocation windows
include embedding export lookup, integer marshalling and result allocation;
they are not guest-only CPU time. Reports preserve these configurations.

Upstream APIs: [wasmi](https://docs.rs/wasmi/2.0.0/wasmi/),
[WAVM C API](https://github.com/WAVM/WAVM/blob/master/Include/WAVM/wavm-c/wavm-c.h),
[wasm3 v0.5.0](https://github.com/wasm3/wasm3/blob/v0.5.0/source/wasm3.h),
[WasmEdge C API](https://wasmedge.org/docs/embed/c/).

## Run and inspect

```sh
./bin/wasmbench check --suite core \
  --runtimes wasmi,wavm,wasm3,wasmedge,spidermonkey,jsc,v8-shell,deno
./bin/wasmbench run --suite core \
  --runtimes wasmi,wavm,wasm3,wasmedge,spidermonkey,jsc,v8-shell,deno \
  --launches 3 --samples 5 --operations 1 --warmup 0 --out runs/extra-timing
./bin/wasmbench run --suite core \
  --runtimes wasmi,wavm,wasm3,wasmedge,spidermonkey,jsc,v8-shell,deno \
  --profile memory --phase-barriers \
  --launches 3 --samples 5 --operations 1 --warmup 0 --out runs/extra-memory
./bin/wasmbench report --run runs/extra-timing --memory-run runs/extra-memory \
  --out reports/extra
```

RSS comes from the controller's platform collector over the actual engine PID;
barriers expose each supported stage. Guest logical memory is a separate
runtime observation, not process RSS. Release barriers mean native object
disposal or JS reference release, not proven physical reclamation. The missing
wasm3 instantiation cell remains unsupported in reports.

## Verification

```sh
node --test adapters/js-shell/*.test.mjs
WASMBENCH_EXTRA_TEST_RUNTIMES=wasmi,wavm,wasm3,wasmedge,spidermonkey,jsc,v8-shell,deno \
  go test ./experiment -run '^TestBuiltExtraAdapter$' -count=1 -json | \
  node recipes/verify-go-test-coverage.mjs \
    --package github.com/wasmbench/wasmbench/experiment --require TestBuiltExtraAdapter
```

All eight adapters were functionally exercised on native macOS/arm64 during
implementation, including lifecycle timing, memory barriers, digest rejection,
wrong scalar/memory oracles, exact i32/i64 boundaries and initialization/input
order. Exact-tool archive restoration and replay also passed for all eight.
That is bounded core-contract evidence, not full conformance, Linux/Windows qualification or
dedicated-host performance evidence. An unset opt-in variable skips the real
engine test; the structured gate above must accompany acceptance claims.

## Wazevo native image capture

The Wazero compiler adapter exports the native executable segment serialized by
Wazevo 1.12.0 into a fresh temporary compilation cache during the code-only pass.
It verifies the cache version, function offsets, segment length and Castagnoli
CRC, and removes the temporary cache afterward. Timing and memory compilation
still use uncached runtimes. The image excludes cache metadata, separate shared
helpers and entry preambles. Its bytes may include padding and embedded data;
instruction-only function attribution is unavailable. Interpreters report N/A.
Oversize segments retain an exact verified byte count but are never truncated
into a pretend complete inspectable image.
