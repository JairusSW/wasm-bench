# Changelog

Notable user-facing changes to Wasmbench are recorded here. No versioned release
has been published yet; development changes belong under Unreleased.

## [Unreleased]

### Added

- Direct V8 shell, SpiderMonkey, raw JavaScriptCore, Deno, wasmi, WAVM, wasm3
  and WasmEdge adapters for bounded core integer scalar timing/memory contracts.
  Includes stage barriers, exact i64 results, input/memory verification, explicit
  SDK selection and unsupported-stage reporting for wasm3 instantiation.
- Native dependency pinning for universal Mach-O engines and their dylibs.
- Reproducible experiment planning, locked inputs, subprocess adapters, immutable
  run bundles, correctness oracles and exact-tool replay.
- Wago, wazero compiler/interpreter, Wasmtime Cranelift/Winch and Node.js V8
  configurations, with explicit capability and unsupported-result reporting.
- Separate timing, memory, counters, native-code and profiling passes; lifecycle
  scenarios, tier/warmup diagnostics and independent Wasm artifact analysis.
- Linux resource and phase collectors, managed/native allocator diagnostics,
  code export/disassembly and typed memory evidence.
- Source-to-Wasm and compiler/runtime stack tracks; paired statistics, history,
  aggregates, break-even, scaling, density, sustained execution and bounded
  continuation/process-snapshot workflows.
- Static offline reports with selectable segmented lifecycle bars, per-stage
  peak RSS, compact hover details and links to measurements and configuration.
- SQLite run indexing/work queues, Parquet exports and DuckDB analysis queries.
- Publication audit, pilot planning, operator-signed qualification, sealed pilot
  archival and independent reader-key verification.
- Native-platform recipes and fail-closed acceptance-test execution gates.
- Apache-2.0 project license, NOTICE, contribution/versioning guides and a
  concise README with a separate workflow reference.

### Known limitations

- Native Windows acceptance and actual dedicated-host publication qualification
  still need live evidence. Cross-builds and synthetic fixtures do not prove them.
- Capabilities vary by runtime, ABI, profile and host. General WIT worlds and
  untrusted-submission sandboxing are not advertised as supported.
- No installer, prebuilt release artifacts or official performance dataset has
  been published. See docs/ACCEPTANCE.md for verification scope.
