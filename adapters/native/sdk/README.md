# Wasmer native-size bridge

The adapter detects `wasmbench_module_native_function_size` in the selected SDK
at runtime. Without the getter it does not advertise native-size support. The
getter uses public `Module::sys_artifact()` and `Artifact::finished_function_extents()`
APIs, requires complete defined-function coverage, checks addition overflow, and
exports a byte count. It never exports host addresses or counts serialized
artifact metadata as machine code. Call trampolines are excluded and the report
records that scope.

The patch changes the C API measurement surface only. Engine and compiler code
are unchanged. Keep this SDK in a shared cache and select it through
`WASMBENCH_WASMER_SDK`; reports retain the exact binary/library identities.

The verified build uses Wasmer **v7.3.0** at
`35c10644f7b0aad6fd9458624ceb8429fe7413c4` and its pinned NAPI submodule
`0b6cbe9c4a90d0f002c772ed4ce714107592a37c`. Use an isolated release checkout:

```sh
git clone --branch v7.3.0 --recurse-submodules https://github.com/wasmerio/wasmer source
git -C source apply /absolute/path/to/wasmer-native-size.patch
CARGO_BUILD_JOBS=1 cargo build --manifest-path source/lib/c-api/Cargo.toml \
  --release --locked --no-default-features --features sys,singlepass,compiler \
  --target aarch64-apple-darwin --target-dir target
```

The explicit target is required by Wasmer's C API build script, including native
builds. Use `x86_64-unknown-linux-gnu` for Hub. Install the resulting `libwasmer`
shared library under a cache prefix's `lib/` alongside matching release headers
under `include/`. Keep a receipt with release/source and submodule revisions,
patch and library SHA-256 values, target, and features. Do not replace an SDK
being used by an active collection.

Then build the native adapter with `--features wasmer_singlepass` and run:

```sh
python3 ../tests/wasmer-native-size-protocol.py /path/to/adapter-native
```

Verified on macOS arm64: one defined return-42 function is 92 bytes; adding an
unexported second function yields 184 bytes. Repeated inspections match exactly.
On Linux amd64 the same checks yield 81 and 162 bytes. Unmodified SDK detection
also passed (capability false). Both hosts have sealed, verified core-corpus
reports: 145 measured workloads and two SIMD workloads rejected by the selected
Singlepass policy. Compilation, instantiation and steady execution each contain
three measured samples; first call, memory and native size contain one. The
Linux code pass uses the same CPU affinity as its matched timing and memory
passes. WASI command collection through Wasmer remains a separate binding task.

## Wasmer 7.5 WASI stream bridge

`wasmer-bounded-streams.rs` adds adapter-only exports to the C API WASI module.
It uses the public WASI builder and virtual-file interfaces. It does not change
engine or compiler code. The bridge provides finite stdin, bounded stdout and
stderr with sticky overflow detection, read-only preopens, typed `proc_exit`
detection, guarded instance initialization, and a runtime with one worker.

Use the full-source release archive for **v7.5.0**, source commit
`82ff099e082da586b38af79986abbb8b677f8baf`. The verified archive SHA-256 is
`5504da6c260ab320e768376bf8eea3c409d53e28da035c6281360067a721a672`.
Apply `wasmer-native-size.patch`, copy the stream bridge into
`lib/c-api/src/wasm_c_api/wasi/wasmbench_streams.rs`, and add
`mod wasmbench_streams;` to that directory's `mod.rs`. Build with the explicit
`sys,singlepass,compiler,wasi` features and a target, using the shared SDK cache.

Run the focused bridge tests before installing the SDK:

```sh
CARGO_BUILD_JOBS=1 cargo test --manifest-path source/lib/c-api/Cargo.toml \
  --release --locked --no-default-features \
  --features sys,singlepass,compiler,wasi --target aarch64-apple-darwin \
  --target-dir target --lib wasmbench_streams
```

Four focused tests passed on macOS arm64: finite stdin and write rejection,
output bounds and sticky overflow, one runtime worker and null-name rejection,
and typed exits distinguished from ordinary guest traps. These checks validate
the bridge; end-to-end command collection requires separate workload evidence.
