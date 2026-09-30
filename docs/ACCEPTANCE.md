# Product acceptance and evidence boundaries

The original twelve-section specification remains the acceptance scope. This
map identifies implementation seams and verification routes. File presence,
passing unit tests, and a configured CI job are not proof of live acceptance.
Do not mark the entire product complete from this map alone.

| Specification | Authoritative implementation | Acceptance evidence needed |
| --- | --- | --- |
| 1. Configuration identity, source/runtime/stack tracks and scopes | Protocol types, experiment locks, source/stack tracks, metric registry | Lossless values, locked identities, host-policy and paired comparison tests; separate embedding/guest/process boundaries. Runtime configurations, not names alone, are graph rows. |
| 2. Planner, controller, subprocess adapters and collectors | CLI, experiment, agent, protocol and Wago/wazero/Wasmtime/V8 adapters | Real adapter lifecycle and incorrect-oracle tests, opt-in built-adapter tests, sealed CLI runs and replay. Capability declarations alone are insufficient. |
| 3. Lifecycle, coldness, tiering and CPU | Metric scenarios and adapter compile/instantiate/init/first-call/steady/cold-process/AOT/teardown paths | Built lifecycle, engine-init, AOT, trajectory, materialization and tier tests. Runtime validation, starts, initialization and release are explicit. CPU needs a live compatible collector. |
| 4. Residency, accounting, allocations, guest and code memory | Collectors, observations, allocator diagnostics, ordered barriers and memory analysis | Native procfs/cgroup/allocator/guest tests and Linux recipes. Before/returned/released snapshots, phase peaks and sample attachments. Sampled peaks and lifetime RSS are not phase-exact peaks; unsupported instrumentation stays unavailable. |
| 5. Input structure, generated code and lifetime | Pinned wasm-analyze, code records/export, native comparisons and lifetime paths | Analyzer fixtures, LLVM tests, sealed code exports and archived-tool replay. Serialized size, mapping capacity and instructions stay distinct. Exact compiler-internal statistics are not inferred. |
| 6. Corpus families, ABI, correctness and reset | Corpus, workload contracts, sacrificial checks, floating/trap/output oracles | Corpus/scaling and built core/command/reactor/Emscripten tests; Preview 2 and typed-component bundle tests. Components are bounded by advertised profiles, not arbitrary WIT worlds. |
| 7. Five separate passes, calibration, counters and warmup | Locked profiles, local adapter loops, perf/Go/V8 collectors | Latency eligibility/calibration, counters, profiling and warmup/tier tests. Batch averages are not request percentiles; diagnostic clocks cannot become headline timing. Denied counters remain gaps. |
| 8. Machine policy, pilot and independent statistics | Host/resource/IRQ/partition receipts, qualification and analysis | Live collectors, host-policy/resource tests, pilot/statistics/aggregate tests and signed publication archives. Synthetic evidence never certifies a real dedicated host. |
| 9. Immutable bundles, SQLite, Parquet and reproduction | Experiment, storage, typed exporters, tool/report-builder archives | Seal/tamper/new-only/path tests, exclusive queue claims, Parquet round trips, exact replay and report verification. Checksums establish integrity, not authenticity. |
| 10. Break-even, density, scaling, sustained release and snapshots | Analysis families and native continuation/process snapshot workers | Family-specific analysis and native execution tests, linked trials, common coverage, uncertainty and curves. Fits are diagnostic; non-atomic PSS/private sums are not physical peaks; stack restoration is not whole-instance restoration. |
| 11. CLI, doctor, check, run, compare, inspect, report and reproduce | CLI, README workflows and pinned recipes | CLI tests and production commands. Correctness-only runs cannot qualify for publication. The trusted local runner is not an untrusted-submission sandbox. |
| 12. Six delivery stages and additional platforms | Above seams, platform collectors and native recipes | Local Linux and Darwin functional evidence exists. Native Windows execution and real dedicated-host publication still need live evidence; workflows and cross-builds cannot replace it. |

## Required tests must execute

`go test ./...` deliberately skips opt-in adapters, fixtures and platform workers.
A package PASS does not establish their acceptance. Set the variables documented
in [components](COMPONENTS.md), then require the named tests:

```sh
go test ./experiment -run '^TestComponentU64(Bundle|MemoryBundle)$' -count=1 -json |
  node recipes/verify-go-test-coverage.mjs \
    --package github.com/wasmbench/wasmbench/experiment \
    --require TestComponentU64Bundle,TestComponentU64MemoryBundle
```

The gate requires execution and PASS for every named top-level test, package
start/completion, no failures in the stream, and no skipped subtests underneath
required tests. It rejects duplicate/missing tests, truncation, malformed JSON,
wrong packages and human-readable PASS text. Check the producer's exit status
too (`set -o pipefail` where supported). This proves test execution coverage,
not the correctness of assertions, host dedication, or performance quality.
The gate itself executes no downloaded or archived binaries.

Wasmtime CI uses the gate for typed calls and Preview 2 reset/lifecycle tests.
That configuration does not prove remote CI ran or passed.

## Evidence refreshed locally on 2026-09-30

- The default Go suite passed all thirteen packages, with dozens of opt-in tests
  skipped. This is default-path/unit evidence only.
- Both real Cranelift/Winch typed-component timing and memory bundle tests passed
  with required execution coverage, including archived replay.
- Preview 2 filesystem reset, command lifecycle, negative lifecycle contracts and
  controller capability gates all passed with required execution coverage.
- Six built-adapter groups (float, calibration, application initialization,
  teardown, expected traps and execution trajectory) passed across Wago, wazero
  compiler/interpreter, Wasmtime Cranelift/Winch and V8, with no skipped subtests.
  Five additional groups passed on their supported configurations: engine
  usability, AOT oracles, command lifecycle, reactors and exact tool replay.
- Production `check` and `reproduce` retained twelve verified correctness trials
  each in `runs/acceptance-core-six-runtime-v1` and
  `runs/acceptance-core-six-runtime-replayed-v1`. Both seals verified. Performance
  report creation rejected this correctness-only input before creating output;
  its separate publication audit is blocked as required.
- Both real LLVM source-build/source-benchmark tests and six live LLVM
  disassembly/builder/source/stack report-replay tests executed and passed.
- `reports/runtime-big-graph-preview-v2`: seal and derived dataset verified;
  browser hover and RSS evidence clicks checked.
- `reports/synthetic-qualified-publication-cli-v1`: production publication and
  reader-key verification passed. Its host/kernel/analyzer assertions are fake,
  explicitly synthetic acceptance fixtures, not host certification.
- DuckDB 1.5.6 executed `analysis/launches.sql` against the graph's Parquet:
  all 100 launch-count/median cells matched its independently verified dataset.
  `analysis/compiler-builds.sql` executed against retained source Parquet and
  returned both source-variant groups. This does not qualify either host.

These local adapter tests are functional checks, not official performance runs.
Pending independent evidence remains native Windows execution and an actually
qualified dedicated host. Additional adapters, browser/device profiles and
arbitrary component worlds are extension candidates, not advertised as complete.
New capabilities need their own contracts and acceptance tests, without
redefining existing measurements.
