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
Unmodified SDK detection also passed (capability false). Linux verification and
full corpus collection remain required.
