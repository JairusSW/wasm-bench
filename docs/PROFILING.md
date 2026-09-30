# CPU profiling

The experimental `profiling` pass currently supports wazero-interpreter's
stateless core scalar `steady` workloads. It collects unmodified gzip pprof
evidence with Go's `runtime/pprof`. V8 supports the same workload contract using
its native inspector profiler, described below. Other native-code adapters remain
unsupported: their stack unwinding and guest symbol attribution are not qualified.

## V8 native profiles

Use `--runtimes v8` with the profiling command below. The collector uses a local
`node:inspector/promises` session, without opening a listening port, and requests
a 1,000 microsecond sampling interval. It retains native inspector JSON as
`profiles/<sha256>.cpuprofile`, suitable for compatible DevTools profile viewers.
The locked runtime includes the profiling helper's hash and Node/V8 versions.

Scope is `adapter_v8_isolate_sampled_stacks`: this is not all-process/thread CPU,
guest-only attribution, or proof of optimized-tier coverage. Collection surrounds
the complete run request, including setup, warmup and verification. Native time
deltas are not converted into Go process CPU nanoseconds. The offline flamegraph
uses recorded isolate sample counts, with an explicit unit in its context,
tooltips and exact frame table; it also links the unchanged native download.

Analysis version `v8-isolate-sample-tree-v1` counts each entry in the native
`samples` array once at its referenced node, then sums inclusive counts up the
tree. It does not substitute `hitCount`, infer duration from the requested
interval, or weight by `timeDeltas`. Missing samples are unsupported; an explicit
empty array remains empty. Idle, program, GC, anonymous and unresolved frames
stay visible; samples are not invocation counts or an all-thread CPU metric.
Raw node/script identity and source positions are retained without merging
same-named frames, including recursive occurrences. Displayed lines are 1-based
(zero means unavailable); raw columns remain zero-based in location metadata.

The decoder rejects duplicate IDs, unknown sample/child references, repeated
edges, multiple parents/roots, cycles, disconnected nodes and invalid source
coordinates. Budgets are 100,000 nodes/edges, 1,000,000 samples, depth 1,024,
4,096 bytes per label and 8 MiB combined labels. Exceeded budgets retain the raw
download and expose no partial tree. Native JSON parsing remains subject to the
16 MiB transport bound, not an OS memory sandbox. The shared chart supports
zoom/reset and a paged frame table with the same rendering limits as pprof.

Format reference: [DevTools Profiler protocol](https://chromedevtools.github.io/devtools-protocol/tot/Profiler/).

Successful and incorrect-result runs can both retain collected profiles. Invalid
contracts are rejected before collection. Empty samples are not zero CPU proof.
Native JSON output has the same 16 MiB retained-byte limit, not an internal V8
profiler memory limit. Envelope validation checks digest, size, JSON nodes-array
and nonnegative ordered timestamps; it does not establish graph/symbol validity.

Tests: `node --test adapters/v8/profiling.test.mjs` and
`WASMBENCH_V8_PROFILE_TEST=1 go test ./experiment -run TestBuiltAdapterCPUProfiling`.

```sh
./bin/wasmbench build --runtimes wazero-interpreter
./bin/wasmbench run --suite core --runtimes wazero-interpreter \
  --profile profiling --scenarios steady --operations 10000 \
  --warmup 1 --samples 3 --launches 1 --out runs/profile
./bin/wasmbench verify --run runs/profile
./bin/wasmbench report --run runs/profile --out reports/profile
```

The report links each collected profile under `profiles/<sha256>.pprof` and its
raw trial, including failed trial diagnostics. Unsupported and unavailable
outcomes remain visible. Keep the exact locked adapter binary for symbolization.

```sh
go tool pprof -top reports/profile/profiles/<sha256>.pprof
go tool pprof -http=127.0.0.1:8080 reports/profile/profiles/<sha256>.pprof
```

The second command starts pprof's local interactive analysis UI. The offline
report also provides a built-in sampled-CPU flamegraph and exact frame table.

## Offline stack analysis

Report generation decodes pprof using a pinned `github.com/google/pprof` revision
(`v0.0.0-20251114195745-4902fdda35c8`, also vendored by the local Go 1.26.5 tools).
Derived data uses analysis version `pprof-cpu-stack-tree-v1`. It accepts exactly
one `cpu/nanoseconds` sample type; unsupported units, negative weights, malformed
protobuf, ambiguous types and overflowing totals receive explicit outcomes.
It never rewrites the sealed raw profile.

The tree preserves inline frames, recursive occurrences, source locations and
unresolved stacks. It aggregates all profile labels, not a chosen thread or
goroutine. Locations at different addresses are not silently merged into one
function. Inclusive and self weights are exact decimal integers. Flamegraph
width uses rounded geometry; it is not call order or elapsed wall time. Empty
profiles remain empty, not evidence of zero CPU work.

Select a trial, click or keyboard-activate a frame to zoom, and use Reset zoom to
return to the full tree. The chart draws at most 2,000 frames per zoom and marks
that limit. Every analyzed frame remains in the 500-row paged table, including
zero/tiny-width frames; table buttons can zoom directly to any branch. The view
links to raw trial and pprof evidence, decoder version and workload outcome.

Analysis refuses profiles exceeding 100,000 sample records, locations, functions
or tree nodes, depth 1,024, one million expanded frames, 8 MiB of tree labels or
4,096 bytes per symbol/file label. These outcomes retain raw downloads and never
present a partial tree as complete. The gzip envelope is limited to 16 MiB
compressed / 64 MiB decoded; parsing itself is not an OS memory sandbox.

## Scope and interpretation

- The window surrounds the entire adapter run request: setup, explicit warmup,
  normal steady-path verification calls, measured batches and their verification.
  Profiler startup/stop and runtime background activity can contribute overhead.
- Scope is sampled Go-process CPU, including runtime and interpreter stacks—not
  guest-only execution, exact instructions, a phase counter or per-Wasm-function
  attribution. Profiler overhead can materially perturb the workload.
- `collected` means a raw artifact was produced, not that it contains samples.
  Very short workloads can yield an empty sample set. Do not infer zero CPU work.
- Instrumented sample timers remain raw diagnostic evidence; latency summaries,
  confidence intervals and performance comparisons exclude profiling passes.
- Correctness admission runs separately with the timing preparation profile.
  Incorrect-result profiles can be retained but never become successful results.
- No forced GC, privilege changes, perf permissions or host policy changes occur.

The controller verifies module identity, declared collector/scope, content digest,
compressed size and bounded gzip integrity. This is transport validation, not a
claim to validate all protobuf semantics or symbol completeness. `go tool pprof`
independently interprets the retained format. The sealed trial contains exact
bytes and collector/toolchain identity; reports write the same bytes, not a
re-encoded profile.

Output is capped at 16 MiB. Overflow records `unavailable` with a reason and no
partial artifact. This limits retained output, not all internal memory used by
the Go profiler. Trial timeouts/resource limits still apply. If the process is
terminated before Stop flushes, there may be no completed profile. A failed
Start never stops a profiler owned by another caller; Stop flushes and releases
only this collector's successful Start.

## Verification and remaining work

`go test -race ./collectors` checks bounded output, process-global profiler
ownership, idempotent Stop, restart and independent parsing by `go tool pprof`.
Protocol tests cover envelope integrity and supported requests. Storage tests
reject resealed mismatched identities and misplaced profiling evidence. Publisher
tests preserve bytes, deduplicate downloads and refuse overwrites/unsafe digests.

```sh
WASMBENCH_GO_PROFILE_TEST=1 go test -race ./experiment \
  -run TestBuiltAdapterCPUProfiling -count=1
```

This tests the built interpreter, native-backend refusal, invalid phase barriers,
correctness failures with retained diagnostics and absent memory instrumentation.
The live macOS `runs/go-cpu-profile-v1` has two collected interpreter profiles and
four unsupported native/V8 outcomes. The sum profile parses into real sampled
interpreter/runtime stacks and is dominated by runtime scheduling activity; it
is not a guest performance claim. Its profiling latency summaries remain null.

Linux-native/perf stack collection, JIT symbols, guest attribution and additional
adapters remain unfinished. Hosted CI has not yet been observed for this collector.
