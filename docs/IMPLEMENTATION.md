# Product acceptance ledger

## Self-verifying static runtime reports

The ordinary `report` command now rejects output paths inside either timing or
paired memory input before creating files, including symlink aliases. It seals
the generated HTML, JSON, Parquet and copied raw bundles, then verifies the
complete report before returning success. `verify-report --dir` checks the outer
seal, reloads the copied bundle(s), recomputes the versioned dataset from
those copied inputs, and compares the page with the pinned renderer hash.
This detects a changed dataset or page even if someone writes a new local
checksum file. Tests cover nested outputs, symlink aliases, intact inputs,
tampering and recomputation; the paired five-runtime report passed live
verification. Local checksums are not signatures or producer authentication.

Older ordinary reports generated before this seal cannot pass `verify-report`;
regenerate them into new directories from their sealed raw runs. Future analysis
or renderer versions will need an explicit compatibility policy rather than
silently rewriting old evidence. This does not qualify official publication.

## Phase CPU-work report

The memory-pass report now summarizes Linux cgroup barrier-window total, user,
and system CPU work separately from wall latency. It accepts only the versioned
process-tree `cpu.stat` delta contract, computes a median within each complete
verified launch, then a median and optional 95% bootstrap interval across
independent launches. Locked host-baseline/CPU-partition failures withhold
derived values but retain raw trial links. Failed and incomplete launches
remain in coverage; zero
does not become unavailable. The detailed report displays all three domains,
coverage, interval availability and raw trial links, with a warning that the
diagnostic window includes barrier transport and background work. Its data
version is `cgroup-phase-cpu-launch-median-v1`.

Synthetic tests cover collector/version/verification/operation-shape rejection,
host-policy withholding, missing versus zero, failure counts and launch-level
provenance. A sealed Linux cgroup run reanalyzed into
`reports/linux-phase-cpu-v2` has 30 metric rows and positive
total/user/system observations for compile barriers. Browser inspection verified
values and raw links, a 390px page without overflow, and no console errors.
This is not pure compile-API CPU time or official publication qualification.

## Paired memory graph evidence

The lifecycle graph now follows the selected timing stages. Its peak-RSS cell
opens all contributing memory-profile launches, their raw trial links, and a
95% percentile-bootstrap interval over independent launch values when at least
three are available. One- and two-launch reports explicitly withhold that
interval. The RSS metric is a whole-process high-water mark, not a stage-only
peak; the selected scenario identifies the separate launch that produced it.

Paired reports now reject differences in host identity, observed policy,
protocol, resource budget, host policy, effective runtime configuration, native
dependencies, or the full workload contract. Exact artifact bytes may move to a
different path without changing the contract. The primary bundle must be a
timing-profile measurement and the paired bundle a memory-profile measurement.
Tests cover mismatched identities, path-only relocation, launch-value provenance,
and interval withholding. The existing five-runtime timing/memory bundles still
join. Browser QA of the regenerated report verified the RSS panel, raw link,
390px layout without page overflow, and no console errors. The live example has
one memory launch per cell, so it does not demonstrate a multi-launch interval
in the browser. This does not qualify official publication or complete the
broader product requirements.

## Registry contract validation

Metric and lifecycle-scenario definitions are validated for unique IDs and
complete version, unit, scope, measurement boundary, and missing-data policy
before CLI operations or static reports can consume them. Negative tests cover
duplicate IDs and incomplete contracts; the checked-in registry validates.

## Component evidence admission and offline inspection

New independent analyzer locks use `artifact-structure-v1`: exactly core v3 or
component v1 evidence is accepted according to encoding. Legacy core v2/v3 locks
remain core-only. Component admission uses the independent hierarchy rather than
the core-only section parser. Component bytes require `abi: component` and the
new lock contract; each alias is checked, not merely the first workload sharing
a digest. Offline loading validates encoding, root identity, parent ordering,
range bounds and sibling non-overlap, and retains conditional feature evidence.

Opt-in `TestComponentAdmissionBundle` passed against the rebuilt debug analyzer
and real wazero adapter: an empty component fixture is admitted and sealed,
both sacrificial and measured cells are unsupported with no samples, and offline
inspection returns its component report. A resealed mixed-ABI alias and a fresh
run with that alias are rejected. The fixture has no workload export and does not
claim behavioral correctness. The test is wired into CI after `make build` via
`WASMBENCH_COMPONENT_ANALYZER`; that variable is relative to the Go package's
working directory.

The full Go suite, renderer tests, vet and workflow lint passed. Additional unit
tests check malformed hierarchy and exact analyzer encoding/version pairs,
including rejection of component evidence by legacy locks. General component
export-call contracts and execution remain unfinished.

## Component structure inspection prerequisite

The independent analyzer now emits `component-structure-v1` for Component Model
artifacts instead of rejecting them or flattening their nested core modules.
Whole-artifact validation precedes traversal. Parent-linked nodes retain local
index spaces, hashes/ranges, imports/exports, canonical/type/instance/alias counts,
start metadata and core function-body records. Core-module output remains v3;
the planner's existing core-only analyzer evidence contract remains unchanged.
See `docs/COMPONENTS.md` for the exact supported boundary and remaining execution
work. This is not a claim of component benchmark support.

All 13 Wasmtime adapter tests, six independent analyzer tests, and the full Go
suite passed. New tests cover nested component/core relationships, duplicate
module bytes at distinct ranges, local function indices, nonempty imports and
exports, exact hashes/ranges, truncation, and feature-policy rejection.

Read-only live inspection of three sibling WASI P2 artifacts passed:

| Artifact | SHA-256 | Structure |
| --- | --- | --- |
| `../../Wago/wasi/p2/testdata/rust_smoke.component.wasm` | `c6979faf9c5dff8b07f2015ea2c7e96273b281f946f09752ebc201ec6d8b2d6e` | 2 components, 3 core modules, 238 core function bodies |
| `../../Wago/wasi/p2/testdata/rust_filesystem.component.wasm` | `090acf5a4cefbb980b9713c999d85b95545576be30adbc0949b94819063fbe38` | 2 components, 3 core modules, 314 core function bodies |
| `../../Wago/wasi/p2/testdata/rust_sockets.component.wasm` | `f40c5a186d9129bee23acb8c4aab1470665a4c6150e3101105e0e157c42239c0` | 2 components, 3 core modules, 417 core function bodies |

All three expose `wasi:cli/run@0.2.0` at the root, with their actual named WASI
0.2.9 imports retained. During that structural-only inspection no guest code was
executed or host capabilities supplied.
The debug analyzer was rebuilt; pinned release analyzer/adapter files were not
overwritten. Component planner admission remains unfinished. Wasmtime compile-only
trials and a first-call WASI P2 command path are implemented. Direct adapter
protocol runs of the Rust smoke fixture and temporary-filesystem fixture returned
the expected pinned stdout/stderr hashes. An opt-in sealed-bundle test passes
component admission, sacrificial correctness execution, measured samples, tool
archiving, sealing, and offline reload against the real Rust smoke fixture. The
Rust sockets fixture trapped and remains unsupported. These results are
engine-host-specific and do not establish general component API execution support.

## Throughput offline evidence

Throughput analysis v2 adds per-trial evidence and exact decimal totals to JSON.
Every measured trial receives an entry, including profile/host exclusions,
failed/invalid trials and duplicate blocks. Raw trial outcomes and failure reasons
remain distinct from throughput eligibility. Excluded values are null, not zero;
sacrificial trials and warmup samples remain in the raw sample export.

Reports now write `throughput.parquet` (`timed-work-evidence-parquet-v1`), linked
from workload details. Tests round-trip totals exceeding uint64 and signed int64,
check diagnostic/invalid exclusions and original failure reasons, and reject
overwriting an existing export. `analysis/throughput.sql` summarizes equally
weighted independent-launch rates with work-unit/profile/run grouping, then
prints exclusion coverage separately. It does not claim bootstrap intervals.

Full Go tests, vet and renderer tests passed. DuckDB 1.4.3 executed the actual SQL
against `reports/work-throughput-parquet-v1` and
`reports/work-throughput-parquet-memory-v1`: timing produced two rate groups with
four eligible launch totals matching JSON and recomputed rates; memory produced
zero rate groups while retaining all 36 coverage groups. Original sealed bundles
and earlier reports were not overwritten. The final additive raw failure-reason
column is separately covered by the Parquet round-trip test.

## Useful-work throughput

`analysis.Throughput` and report dataset `timed-work-launch-rates-v1` now expose
declared work units per second for first-call, steady and trajectory call regions.
Each launch sums measured operations and elapsed nanoseconds with arbitrary-size
integers, multiplies operations by the locked workload's units per invocation,
and converts the resulting rational rate to a display number. Independent-launch
rates receive equal weight; their median and 95% bootstrap interval (at least
three launches) are reported. Warmup is excluded from the rate but retained raw.
This does not invert the median sample latency or average per-sample rates.

The timing-pass eligibility policy applies. Missing work contracts, zero total
duration, failed trials, unknown sample kinds, invalid measured samples and
duplicate blocks withhold rates. Setup/teardown scenarios are not applicable.
The metric registry defines `work.throughput` v1; workload details display the
rate, units, eligible launch count, interval when available, and explicit
exclusion of setup/verification/inter-sample gaps. This is not sustained service
capacity, end-to-end throughput, or an aggregate over unlike workload units.

Validation: full `go test ./...`, `go vet ./...`, and Node report-renderer tests
passed. Tests cover unequal sample counts, warmup exclusion, three-launch
intervals, two forms of integer-overflow risk, diagnostic and malformed evidence,
duplicate blocks, and unchanged raw inputs. Reports regenerated from sealed
bundles into new directories: `reports/work-throughput-v1` has two eligible
launches per workload and correctly null intervals; all 36 cells in
`reports/work-throughput-memory-v1` have null rates and `not_timing_pass` status.
Chromium detail-view/mobile QA passed with no console errors; screenshot at
`output/playwright/work-throughput-mobile.png`. These remain exploratory host
measurements, not publication-qualified performance claims.

## Timing-pass eligibility and diagnostic evidence separation

Headline summaries and within-run comparisons now require an eligible locked
timing measurement and matching trial profile. Correctness-only runs, diagnostic
profiles, phase barriers, and unsatisfied host requirements withhold estimates.
Execution outcomes, successful launch counts, and raw samples remain separate
from eligible latency launch counts. Parquet v2 retains diagnostic timer values
with explicit eligibility fields; the example DuckDB query filters these fields.
Scaling synthetic wall-time curves and break-even analysis use the same policy.
Analysis versions are summary v5, scaling v3, and break-even v2.

Verification: `go test ./...` and `go vet ./...` passed. Added matrix tests cover
profile mismatches, all diagnostic profiles, check/barrier/host exclusions,
unchanged raw evidence, paired withholding, derived latency eligibility, and
Parquet row preservation including warmup, failed, and sacrificial trials.
New reports generated from existing sealed reactor bundles:

- `reports/profile-separated-memory-v1`: 36 successful launches, no headline
  medians, 84 retained raw samples, and 30 memory timelines.
- `reports/profile-separated-timing-v1`: 70 successful launches and headline
  medians, with 220 retained raw samples.

Both generated report scripts pass JavaScript syntax checks. Local Chromium
browser QA verified desktop and 390px mobile rendering: memory-pass rows say
Withheld, their details contain no latency trajectory, and memory snapshots remain
available. A timing steady-state detail shows five verified samples, including
two amber warmup points, for its selected independent launch. Unsupported V8
reactor rows display unavailable latency and no fabricated plot. No browser
console errors were observed. Screenshots are in `output/playwright/`.

The browser check found stale detail content after filter changes. The report
now clears that panel on filtering and labels the detail's scenario explicitly.
`reports/profile-separated-timing-v2` was regenerated without overwriting v1;
browser checks verified the compile label and empty hidden detail after changing
to steady. `go test ./publish` passed. These live fixtures have one launch per
cell and fewer than 500 samples, so they do not establish multi-launch switching
or multi-window browser coverage. This increment does not establish complete
product acceptance.

Repeatable renderer coverage now lives in `publish/report-ui.test.mjs` and runs
under `make test` and the Node-enabled CI job. It evaluates the actual renderer
functions from `report.html` against a small DOM contract double: 1,001 samples
across three reachable windows, switching between two launches and resetting the
window, preserving warmup and raw evidence, omitting invalid samples without
closing index gaps, excluding failed/diagnostic launches, and missing-evidence
messages. Table tests distinguish measured zero from unavailable and withheld
values, preserve diagnostic success counts, and verify filter changes clear stale
details. All four Node tests and the full Go suite passed via `make test`. This
is state/rendering logic coverage, not a substitute for real-browser layout and
multi-window interaction checks.

The subsequent real-run check closes that specific browser gap.
`runs/report-windows-v1` contains two successful sacrificial checks and four
successful measured launches (core identity/sum, wazero, two launches each).
Every measured launch retains 1,003 verified samples: two warmup plus 1,001
measured. The exact runner is `bin/wasmbench-ui-windows`, also archived in the
sealed bundle; `verify --run runs/report-windows-v1` passed. Reproduce the
functional scenario with:

```sh
./bin/wasmbench-ui-windows run --suite core --runtimes wazero \
  --profile timing --scenarios steady --launches 2 --samples 1001 \
  --operations 1 --warmup 2 --out runs/report-windows-new
```

`reports/report-windows-v1` was inspected in local Chromium. The third window
contained exactly indices 1000–1002; selecting the second launch reset the
window to zero with 500 dots; its middle window contained exactly 500 dots
at indices 500–999. Trial labels changed from `trial-000000` to `trial-000002`.
No console errors were observed. Mobile 390px visual QA passed; screenshot:
`output/playwright/timing-second-launch-window-mobile.png`. The browser and
local server were closed afterward. This is functional report evidence on an
uncontrolled development host, not publication-quality performance data.

## Portable fixed-set aggregate report

`aggregate-report` now copies and verifies a source bundle into `raw/`, checks
the copied receipt against the original, and computes aggregate analysis from
the copied evidence. It emits a self-contained HTML page, data/set/trial JSON and
a sealed checksum manifest, refusing overwrite and nested/symlink-aliased input
outputs. The page shows whole-set and category ratios with block-bootstrap
intervals, category weights, required/covered counts, complete block IDs,
per-workload outcomes and stability, and links to all evidence/configuration
files. Its filter changes only the coverage table, never the analysis.

Tests verify portable evidence, missing-result nulls, input preservation, overwrite
and nested-output rejection, invalid configuration rejection, evidence links and
HTML-safe JSON embedding. `reports/core-aggregate-html-v1` and the intentionally
incomplete-set UI fixture `reports/aggregate-incomplete-html-v1` passed independent
checksum checks. Playwright verified desktop and 390px layouts, unchanged aggregate
under filtering, filtering with the network disabled, reproduction commands,
and absent whole-set plot/ratio for a missing required workload. No page-width
overflow occurred; the wide coverage table scrolls locally. The browser tool
blocks direct file URLs, so local-file navigation itself was not verified; the
page uses only embedded data and relative evidence links. Go race tests including
native/source LLVM integrations and vet passed. This delivers an offline aggregate
view, not official publication policy or the remaining experiment subsystems.

## Fixed-set weighted aggregate analysis

`aggregate-set` emits an explicit schema-1 proposal with a versioned ID, one
scenario, categories, positive weights and full workload-contract digests (only
the artifact transport path is excluded). `aggregate` consumes a verified timing
bundle and the reviewed set, preserving all required members and category
weights. It emits per-workload outcomes, positive paired blocks and stability,
category coverage/results, and a whole-set result. Missing/changed contracts,
unsupported members and zero-timer cells cannot silently shrink the set. Duplicate
runtime identities/trial cells/members, invalid weights and malformed sets fail.

`fixed-set-paired-geomean-v1` first computes weighted geometric means of paired
launch-median ratios per complete block, with equal member shares within each
category. It then takes the median of block aggregates and bootstraps complete
blocks 4,000 times; fewer than three blocks have no interval. All members must
share each included block, and coverage exposes omitted/incomplete blocks. The
JSON includes the full set, its SHA-256, source lock SHA-256 and configurations.
It explicitly does not establish preregistration, official publication or causal
attribution. The portable aggregate report now exposes this analysis visually.

Tests cover unequal weights, inner-sample imbalance, warmup exclusion, missing
workloads, changed contracts, unsupported cases, zero timers, disjoint block sets,
insufficient replication and duplicate identities. The CLI analyzed the sealed
`runs/trajectory-breakeven-v3` into `reports/core-aggregate-set-v1.json` and
`reports/core-aggregate-v1.json` (two required workloads, two categories, three
complete blocks). An independent Node calculation from raw samples exactly
matched its point estimate. This is functional smoke evidence, not a meaningful
runtime ranking. Full Go race tests and vet passed.

## Packaged float regressions and CI coverage

The runtime image now includes `adapters/v8/floats.mjs`; omitting this imported
helper prevented the packaged V8 adapter from starting after float support was
added. The smoke recipe now exercises seven float fixtures across wazero compiler,
wazero interpreter and V8: compile/instantiate/trajectory/teardown timing and
compile/instantiate/teardown memory barriers. Its independent verifier checks
every trial, sample count, retained warmups, release boundaries and the helper's
locked content hash. Doctor output is retained as `doctor.json`.

Fresh Linux/arm64 image
`sha256:3fc2b0a5f36b152c820f32846043b2fc8e1de31f1bf7cd5a5eb86d8e98c8e1c6`
passed `recipes/test-container.sh` into `runs/container-floats-v2`: core check,
24 measured core trials and exact replay, report, 84 timing float trials and 63
float phase trials, and checksum validation of all five bundles. An additional
read-only check confirmed 336 positive Linux RSS boundary snapshots. These are
process snapshots, not isolated cgroup peaks or official performance results.
The first smoke directory is partial: editing the executing shell script to
capture doctor output disrupted its read offset; the untouched second execution
passed. No sealed evidence was overwritten.

## Full Wago catalog admission check

On 2026-09-29, `import-wago --ids all` read the adjacent
`../../Wago/wago` checkout without modifying it. It imported all 132 catalog
contracts and verified artifact and fixture hashes (catalog SHA-256
`e02b20f2ccf9db3bc4649270c8d8eeff2b29bdb6acb6ecb47fab2697cd811a9f`). A
correctness-only `check` against wazero initially produced 103 successful
sacrificial checks and retained 29 unsupported outcomes. After adding the
catalog's `llvm-ir-preds` output normalizer, the sealed rerun
`/tmp/wasmbench-wago-check-all-v2` verifies with 105 successful checks and 27
unsupported outcomes. At that point, unsupported cells were 18 unimplemented
command-host contracts, eight Emscripten ABIs, and one artifact requiring an
unadvertised validator feature. A separate sealed check of both Clang workloads across
wazero and Wasmtime passed all four cells; each retains its raw stdout digest
and normalized-oracle digest. Check bundles have publication state `prohibited`
and cannot be mistaken for performance measurements. This is the pre-Emscripten
baseline; the eight Emscripten entries were subsequently enabled for wazero as
described below. The catalog evidence does not establish complete cross-runtime
support or performance qualification.

### Emscripten stdio command profile

`import-wago` now admits the catalog's Emscripten `main(argc, argv)` contracts
under the explicit `emscripten-stdio-v1` host profile. The wazero adapter
marshals argv through the module's `stackAlloc`, invokes constructors once, and
maps `exit` to wazero's process-exit error convention. It uses bounded stream
capture, pinned stdin fixtures, deterministic clock/random sources, and a
fail-closed syscall shim; this is not general Emscripten filesystem support.
Protocol and importer tests cover profile/export pairing, and an adapter fixture
checks argv handling plus exit-status propagation. Wasmtime Cranelift and Winch
now use the same explicit profile and fail-closed Emscripten syscall set. Both
adapters prepare constructors and argv before timing the `main` call.

`runs/emscripten-wago-v1` is a checksum-verified correctness-only bundle for the
eight Emscripten applications in the adjacent Wago catalog. All eight sacrificial
checks passed on wazero with their pinned stdout hashes. The bundle is prohibited
from publication as performance evidence. After the call-timing boundary
adjustment, a controller check of the same eight contracts passed all 24 cells
on wazero, Wasmtime Cranelift and Wasmtime Winch. The correctness-only bundle
at `/tmp/wasmbench-emscripten-three-check-v5` passed checksum verification and
has publication state `prohibited`. Wago does not yet advertise this ABI
profile, and broader Emscripten filesystem semantics remain unsupported.

CI now enables float, teardown, application-init and AssemblyScript tests for
Go/V8 and Wasmtime configurations, Node float tests, and Rust adapter/analyzer
tests. A separate Wago job checks out pinned source revision
`49acf64a3800d3a00ab4ff0a69e5772d6f293215` alongside wasmbench and runs adapter and
feature conformance. The same new test selections passed locally, as did all Go
tests, Rust/Node tests, shell/JS syntax checks and actionlint. Hosted Actions have
not been run; no remote CI success is claimed.

## Floating-point oracle contract

Wasmtime Cranelift/Winch and V8 now implement the same numeric policy as the Go
adapters. The V8 helper is included in locked runtime file hashes and extracts
numeric signatures from exact artifact bytes, rejecting unsupported GC and wide
import encodings rather than guessing. Result re-encoding occurs outside timing;
guest NaN payload preservation through JS Number is not claimed. The suite now
also exercises mixed typed arguments and F32/F64 multi-results. Shared built-
adapter tests pass on all six configurations for timing/memory compile,
instantiate, first-call and steady scenarios, including negative value/type/
signed-zero/NaN cases. Node and Rust edge-case tests and the full Go race suite
pass. Float compile and instantiation phase barriers are now supported on all
six configurations through `can_float_phases`. Float teardown additionally uses
`can_float_teardown` for timing, memory and memory-barrier passes. Other float
lifecycle barriers remain unfinished. Wago verification now receives the actual compiled module
when checking temporary compile outputs, rather than assuming a retained module.
Tests cover all seven float fixtures, exact barrier order, wrong values/types,
and rejecting timing-profile barriers. A runner regression test ensures the
separate sacrificial first-call check does not inherit measurement barriers.

`runs/float-phases-v2` verifies 42 admissions and 84 measured compile/instantiate
trials with 168 verified samples. The initial `float-phases-v1` bundle preserves
the planner admission rejection discovered and corrected during integration.
This validation ran on macOS: Linux process/cgroup metrics remain explicitly
unavailable here, so this evidence proves barrier wiring and available allocator
observations, not Linux kernel-accounted float memory peaks or CPU totals.
Full Go race tests with float, trajectory, compile-phase and native/source LLVM
integrations, Go vet, Node float tests and Rust adapter tests pass.

Float teardown verification runs before `before_teardown`, with one timed release
per sample and no warmup calls. Wrong values/types cannot reach the release
boundary. Existing runtime-specific release policies are unchanged; notably V8
drops handles and does not claim immediate GC or physical-memory reclamation.
`runs/float-teardown-timing-v1` and `runs/float-teardown-phases-v1` each verify 42
admissions and 42 measured trials, retaining 126 single-release samples. The phase
bundle retains both release boundaries for every sample. All seven float fixtures
pass across six configurations, including mixed typed arguments and multi-results.
Local macOS evidence does not establish Linux kernel memory/CPU values. Full Go
race tests with float/teardown and native/source LLVM integrations, Go vet, Node
and Rust tests pass.

Float trajectories now run on all six configurations with a separate
`can_float_trajectory` capability. They retain invocation 1 and warmup calls,
verify every result outside timing, and reject reset-per-sample, memory-profile
and barrier contracts. A stateful adversarial fixture returns the expected F64
value for exactly five calls and an incorrect sixth value: cross-adapter tests
prove there is no hidden pre-call or per-sample reset, each new request starts
fresh, and the sixth incorrect result fails the request. Seven normal float
fixtures also pass trajectory tests, including typed arguments and multi-results.

`runs/float-trajectory-v1` contains 378 successful measured cells (seven fixtures,
six configurations, three scenarios, three launches). Its 126 trajectories retain
all 3,150 calls, including five warmups and 20 measured calls per trajectory.
Bundle verification passes. `reports/float-break-even-v1.json` has 42 available
curves with three complete blocks and prefixes through all 25 calls, plus seven
paired wazero/V8 comparisons. These are functional shared-host checks, not an
official performance ranking. Full Go race tests with all six adapter float and
trajectory tests, native/source LLVM integrations, Go vet, Node tests and Rust
adapter tests pass.

Fresh sealed evidence: `runs/float-all-six-v3` passes 210 timing cells (seven
fixtures, six configurations, five scenarios including cold-process), and
`runs/float-memory-all-six-v3` passes 168 memory cells (four scenarios), each
with 42 passing sacrificial admission cells. Bundle verification and an
independent check of the V8 helper's locked hash pass. These one-launch runs
establish functional coverage only, not performance rankings or uncertainty.

`float_bits_v1` adds explicit absolute/relative tolerances, result types, NaN
and signed-zero policies while preserving observed IEEE result bits. Finite comparisons
use F64 arithmetic with overflow-safe relative distance; infinities require
exact sign, NaN expectations explicitly allow any NaN payload, and both-zero
comparisons honor the declared sign policy. Wago and wazero compiler/interpreter
validate actual result types outside measured calls. A capability gate retains
unsupported coverage for older adapter builds instead of accepting an unimplemented oracle.

The `floats` suite includes F64 addition, F32 square root, signed zero, NaN and
infinity. Built-adapter tests pass all five fixtures across compile, instantiate,
first-call and steady scenarios, and reject wrong values/types/zero signs and
finite-for-NaN results. `runs/float-oracles-v1` has 15 successful measured cells
and 15 explicitly unsupported cells across all six configurations. These are
functional checks, not numerical performance claims. This initial bundle predates
Wasmtime/V8 support; trajectory/barrier support and broader numerical corpora remain open.

The first float memory smoke exposed a pre-existing wazero typed-nil memory
interface panic for modules without linear memory (`runs/float-memory-v1`).
Memory-presence checks now normalize that case across sampling, input, vector
and trap paths; an unconditional adapter regression test covers it. The new
`runs/float-memory-v2` passes all 60 measured cells (five fixtures, three Go
configurations, four scenarios). Built-adapter tests now exercise both timing
and memory profiles. Failed v1 evidence remains untouched.

## Native image export evidence

`disassemble-code` adds synthetic ARM64/AMD64 ELF wrappers and LLVM linear
disassembly. Go's independent ELF reader checks machine/class/type, zero section
address, size and byte identity before disassembly. Tool executable hashes are
checked before and after each invocation; version strings, argv, logs, timeout
and output limits are retained. Failure/overflow leaves an unsealed partial
directory, never a successful-looking sealed result. Tests exercise ARM64 and
AMD64 RET fixtures through installed LLVM, ELF machine rejection, preserved
missing exports, input/output immutability, output limits and tool deadlines.
This supplies mixed-image listings, not per-function/tier attribution or the
original compiler's object/relocation information.

Validation passed: full race suite with native/source LLVM integration enabled,
vet, current CLI build, and explicit failed-tool log retention without a seal.
`reports/native-disassembly-v2` contains two real Wago ARM64 listings with LLVM
22.1.8 executable provenance and independently verified export checksums. The
AMD64 fixture is cross-disassembled on the local ARM64 host, not executed.

`export-code --run CODE_BUNDLE --out NEW_DIRECTORY` now extracts verified raw
images offline into a separately sealed evidence directory. It retains the
complete source bundle and checksum identity, writes numbered non-executable
binary files, and records every trial's export availability and failure reason
in `native-code.json`. Tests cover binary fidelity, missing/error/admission/size
limit outcomes, path-like trial identifiers, input immutability, nested symlink
outputs, wrong profiles and overwrite refusal. The live export
`reports/native-code-export-v1` reproduced both native-image-v1 hashes and exact
76/136-byte lengths. Per-function/tier attribution remains separate work.

The Wago code pass now preserves an optional version-1 raw native code image in
the sealed trial JSON. The contract records module/image digests, architecture,
backend, mixed section kind and compile-snapshot event. Offline loading rejects
wrong module identities, corrupt images, unsupported contracts and images in
failed, admission or non-code trials. Code images over 16 MiB are explicitly
unavailable, not truncated. No guest-only instruction size or function boundaries
are inferred from the mixed image. Per-function/tier exports and disassembly
remain unfinished.

Live evidence: `runs/native-image-v1` passed sacrificial correctness and two
Wago code trials. Independent Node decoding/hash checks matched the reported
native image sizes: identity 76 bytes, sum 136 bytes on ARM64. Offline
`inspect --artifact-evidence` succeeded. These are diagnostic artifacts, not
performance claims; the Wago source checkout was not edited.

The original specification is the product scope. This ledger distinguishes
implemented behavior from planned work; a working quick run is not proof that
the entire specification has been delivered.

## Contract

- [ ] Versioned protocol, metric registry, missing-data semantics, phase boundaries.
- [ ] Fully resolved artifact, input, build, configuration, host, resource and protocol identities.
- [ ] Distinct runtime, source-toolchain and end-to-end tracks.
- [ ] All lifecycle scenarios, multidimensional coldness and optional capabilities.

## Runner and correctness

- [x] Independent Wago, wazero, Wasmtime and V8 subprocess adapters.
- [x] Dedicated control transport, separate guest output, local measurement batches.
- [ ] Versioned corpus with mechanisms, algorithms, applications and scaling generators.
- [ ] Core, WASI command/reactor, component and Emscripten ABI profiles.
- [ ] Sacrificial validation, measured-result verification, reset and tolerance/trap policies.
- [ ] Immutable bundles with exact artifacts, environment, raw samples, logs and reproduction.
- [ ] Doctor, fetch, build, check, run, compare, inspect, report and reproduce commands.
- [ ] Timeouts, incorrect results, unsupported configurations and OOM remain visible.

## Collection

- [ ] Linux cgroups before adapter startup; phase barriers and same-FD memory.peak probing.
- [ ] RSS/PSS/private/virtual snapshots and honestly labeled sampled peaks.
- [ ] Allocation activity, retained heap and logical guest memory kept distinct.
- [ ] Timing, memory, counters, code and profiling as separate passes.
- [ ] Counter thread/cgroup coverage, event definitions and multiplexing metadata.
- [ ] Independent pinned Wasm structure analyzer and feature admission.
- [ ] Native code export, function/tier disassembly, compiler diagnostic namespaces.

## Analysis and publication

- [x] Cluster-aware intervals and randomized paired/interleaved launch ordering.
- [ ] Coverage, instability, warmup trajectories and common-subset comparisons.
- [x] SQLite index/work queue, Parquet export and DuckDB analysis.
- [x] Static comparison sites for cross-run runtime comparisons and source-output comparisons, with uncertainty, coverage, configurations, raw data and reproduction links.
- [ ] Official publication gates, pilot budgets, machine policies and immutable analysis versions.

## Advanced tracks

- [ ] Compile/execution break-even, including measured tier trajectories.
- [ ] Shared/separate module density, idle/touched memory and marginal costs.
- [ ] Scaling of functions, body size, locals, nesting, branch tables, types, data and imports.
- [ ] Sustained execution, reclamation and separate snapshot lifecycle observations.
- [ ] Source-toolchain builds with recipes, provenance, licenses and correctness.
- [ ] Additional platform profiles without Linux semantic substitutions.

Every item requires tests or live evidence at the scope it claims before it is
marked complete. Optional runtime capabilities may be unsupported; a subsystem
that merely labels everything unsupported is not a delivered subsystem.

## Verified development evidence (2026-09-28)

- `runs/five-configurations`: 300 measured trials and 25 sacrificial checks,
  all correct, on five Wago workloads, across Wago, wazero, Wasmtime Cranelift,
  Wasmtime Winch and V8 on Darwin/arm64. Three independent launches per cell.
- `runs/semantic-check` and `runs/wasmtime-semantic-check`: Wago memory_tree's
  64-bit result and CoreMark's return/memory oracle passed across those adapters.
- `runs/memory-live`: 24 successful trials with Go/JS/logical-memory diagnostics;
  Linux procfs metrics correctly unavailable on Darwin.
- `runs/code-live`: Wago code-image and serialized-artifact sizes are separate;
  unsupported native-code diagnostics remain explicit for other adapters.
- `runs/queued-live`: SQLite-queued experiment completed and indexed; its Parquet
  was read and aggregated with DuckDB using `analysis/launches.sql`.
- `runs/linux-memory`: 16 successful Linux/arm64 trials in Docker; RSS, PSS,
  private residency and virtual-memory boundary snapshots and sampled peaks
  are available. This is procfs evidence, not cgroup phase-peak evidence.
- `runs/reproduction-source-v2` and `runs/reproduction-verified-v2`: rerun using
  the exact runner and adapter executable hashes. Earlier `reproduced-live`
  predates enforcement of the runner hash and is not exact-runner evidence.
- Go unit/integration tests and race tests cover reusable locks, wrong-oracle
  rejection, lifecycle batches, timeout termination, checksum tampering,
  lossless 64-bit values, bootstrap replication, Parquet nulls and queue claims.
- Playwright checked desktop/mobile reports, scenario/runtime/text filters and
  raw-evidence drilldowns. Screenshots are in `output/playwright/`.
- `wasmbench analyze` validates with pinned wasmparser 0.251.0 and reports opcode
  histograms, body sizes, locals, control depth, branch tables and section sizes.
  Automatic analyzer admission and complete feature/type metadata remain open.

These are exploratory functional tests, not controlled performance claims.
Evidence directories are local build outputs excluded from Git.

### Cross-run comparison verification

`compare run-a run-b` now checks host/environment and timing-protocol identity,
retains the union of workload/scenario cells, rejects changed artifact/oracle
contracts per cell, and reports the common successful subset. Its independent
bootstrap resamples each run's process medians separately; unrelated block
numbers are never paired. Missing ratios and insufficient-sample intervals are
null. Both runtime configurations and runner-change status are included.

`runs/regression-baseline` and `runs/regression-candidate` completed 12 trials
each (three launches per cell) and were compared through the CLI. The same-build
comparison also demonstrated ordinary run-to-run drift: statuses describe
observed faster/slower latency, not proof of a compiler-caused regression.
Unit tests cover host/protocol mismatches, changed/missing workloads, independent
versus paired estimators, and insufficient launch counts. Static version-history
views and controlled regression qualification remain unfinished.

## Next implementation work

1. Complete Wago semantic vector contracts and real command ABIs;
   preserve licenses, fixtures and source-build recipes in portable bundles.
2. Complete coldness/resource policies, cgroup phase barriers and reset probing,
   CPU time and narrow allocator windows.
3. Finish counters/profiling, native function export/disassembly, and analyzer
   feature admission. Current counters/profiling requests remain unsupported.
4. Add density, snapshots, sustained/reclamation, scaling analysis/plots,
   source-toolchain/end-to-end tracks, break-even/history views.
5. Complete official machine/pilot/publication gates and remote fetch/build
   recipes; verify full acceptance on the reference platform.

### Wasmtime WASI command integration

Wasmtime Cranelift and Winch now run the pinned read-only Preview 1 command
contract through `wasmtime-wasi` 46.0.1. Every sample owns its WASI context in a
Store; dropping the Store releases the fixture and stdin descriptors. Staging,
argv setup and host binding occur outside the measured compile/instantiate/call
API as appropriate. Output capture remains inside invocation timing; stream
hashes, exit-code validation and cleanup are outside it. Compile barriers hold
the measured module until verification and release. No native allocator
accounting is claimed.

Release Rust tests cover both backends and all four adapter command scenarios,
fresh-instance stdin/guest-state resets, changed fixture digests, wrong stream
hashes/exit codes, overflow, mixed contracts and unsupported barrier profiles.
A separate Wasm fixture verifies read-only preopens, denied write rights,
parent traversal rejection, zero random bytes and advancing synthetic clocks.
The opt-in subprocess command tests now exercise Wasmtime's three compile
barriers, including failure paths without a successful release event. The
all-six-configuration built-adapter suite, root race tests and vet pass.

Darwin/arm64 functional evidence (not performance qualification):

- `runs/wasmtime-command-timing-v1`: 50 successful measured trials of coreutils
  sort, icepll and json2csv across five scenarios and two launches. Ten Winch
  coreutils cells retain `preflight_failed`: the pinned backend fails compilation
  with `Unimplemented Wasm load kind`. Five sacrificial checks pass and one
  fails. The run intentionally exits nonzero; its sealed evidence verifies and
  `reports/wasmtime-command-timing-v1` preserves the failure coverage.
- `runs/wasmtime-command-files-check-v1`: Brotli and esbuild pass on both
  backends with their pinned 2,160,055-byte input. Coreutils passes on Cranelift
  and retains the same Winch compile failure. This is correctness-only evidence.
- `runs/wasmtime-command-phases-v1`: Cranelift passes three real command compile
  trials, two samples each, with all 18 ordered barrier events. This validates
  protocol boundaries on Darwin, not Linux kernel phase peaks.

All three bundles pass checksum verification. The Linux command phase evidence
for Wasmtime is recorded below; the earlier `linux-command-phases-v1` evidence
applies only to wazero.

### Scaling fixture verification

Generator `wasmbench-core-v2` supplies all eight requested structural dimensions
at five sizes each. `TestScalingIndependentAnalyzer` verifies all 40 modules
with pinned wasmparser and checks actual function/operator/local/depth/table/
type/data/import counts against the manifest. Execution oracles run in the Go
tests as well. The analyzer test runs in the Rust CI job after building it.

`runs/scaling-eight-dimensions-check-v3` passed all 200 correctness checks across
Wago, wazero, Wasmtime Cranelift, Wasmtime Winch, and V8 on Darwin/arm64. The two
earlier check bundles retain the Wago host-binding errors found during adapter
development; they are not successful evidence. Imports now use Wago's owned
public host-function reference with a signed i32 callback. Retained-memory
collection and controlled complexity qualification remain open, so the
advanced-track acceptance item is not yet checked.

`runs/scaling-eight-dimensions-lifecycle` exercised compile, instantiate, steady,
and teardown with two independent launches per cell: 1,440 successful trials,
160 explicitly unsupported Wasmtime teardown trials, and no failed trials.
Both scaling bundles passed checksum verification. Race tests and vet passed.
This run overlapped test activity and is functional evidence only, not a
controlled timing comparison.

`scaling-launch-medians-v1` now exports measurement-specific curves in report
datasets. It separates collector/version, scope, phase, quality, profile,
denominator, and batch operation count; missing points and short-run confidence
intervals are null. Fits require a complete positive series and three launches
per point. Tests cover a known quadratic fixture, missing/zero measurements,
collector separation, warmup exclusion, and unequal within-launch sample counts.
The static report adds curve selection, uncertainty, coverage, provenance and
raw-evidence drilldowns. `reports/scaling-curves-v2` contains 160 timing curves;
its two-launch evidence correctly produces no fitted growth rates.
Retained-memory collection and controlled growth experiments remain unfinished.
Playwright verified curve selection, five plotted size points, raw-trial links,
and a mobile layout repair; screenshots are under `output/playwright/scaling-*`.

### Nullable statistics and diagnostic exports

`cluster-median-bootstrap-v2` makes unavailable summary statistics and
underpowered paired intervals null. It rejects negative elapsed samples from
analysis while retaining measured zeroes. Regression tests cover failed,
unverified, invalid, zero, and single-launch inputs. Existing evidence and
older report directories are not overwritten.

Reports now include `observations.parquet`, with metric definitions, collector
identity, phase/scope/quality, denominator, sample identity, availability, and
reasons. Trial-level diagnostics keep sample identity null. Tests round-trip
metadata, preserve measured zeroes, null unsupported values, reject malformed
available values, and reject overwriting existing exports.

DuckDB executed `analysis/observations.sql` against
`reports/diagnostic-parquet-v1`: 532 available and 192 unsupported observations,
with no numeric values in unsupported rows; 112 launch-level metric groups.
`reports/nullable-statistics-v2` has 80 unavailable timing cells with null
statistics and no intervals for fewer than three launches. Full Go race tests
and vet passed after these changes.

### Linux cgroup startup isolation

Adapters can now be spawned into a fresh cgroup v2 child using Go's
`UseCgroupFD`/`CLONE_INTO_CGROUP` path, including description and sacrificial
processes. Requested RAM, swap-disable, CPU quota, CPU-set and task limits are
locked. Startup fails closed when delegation, controllers, or atomic spawn are
unavailable. Local settings and ancestor-limit caveats are retained per trial.
The runner does not mutate parent controller settings.

Linux/arm64 tests in an ephemeral, private-cgroup-namespace Docker container
verified child membership, controller exclusion, CPU selection, RAM/CPU/task
limits, descendant termination on deadline, cgroup OOM accounting, and leaf
cleanup. The OOM test explicitly disables swap. The opt-in harness is
`agent/test-cgroup-container.sh`; never run its delegation setup on the host.
`runs/linux-cgroup-v1` completed eight measured trials and four sacrificial
checks with wazero and V8; checksums verified. The memory observations are
whole-cgroup lifetime peak/current values, not phase peaks.

The kernel semantics are documented in the
[cgroup v2 guide](https://cdn.kernel.org/doc/html/latest/admin-guide/cgroup-v2.html).
Dedicated-host policy and broad phase coverage remain open; the full cgroup
collection acceptance item is not yet complete.

### Compile phase barriers

The optional protocol extension sends bounded phase events and waits for a
matching request/version acknowledgement. The runner validates sample identity,
stage order and completeness, retaining raw phase records even on failure.
The extension is restricted to memory passes and advertised per scenario.

Wazero's compile path now holds the freshly compiled module for retained-state
snapshots, verifies that module's result, releases it, and takes a post-release
snapshot. It reports the actual one operation per sample. Allocation counters
cover the compile API window rather than verification or barrier transport.
The cgroup peak covers the wider acknowledged barrier window and is labeled
accordingly. No post-GC live-memory claim is made.

Linux/arm64 `TestCgroupPeakResetUsesSameDescriptor` established a 32 MiB
historical peak, reset one descriptor, performed a smaller allocation, and
verified the reset descriptor's peak decreased while the other retained its
historical value. `runs/linux-cgroup-phase-v1` passed four wazero measured trials
with twelve samples and 36 phase events; all twelve phase peaks were available.
Four V8 cells remain explicitly unsupported. The bundle passed checksum
verification and generated `reports/linux-cgroup-phase-v1`. Unit tests cover
bad/missing acknowledgements, invalid phase ordering, incomplete sequences,
actual operation counts, incorrect measured results, and timing-pass rejection.
Other adapters/scenarios and retained-state attribution remain unfinished.

### Cross-adapter compile barriers

Wago, Wasmtime Cranelift/Winch, and V8 now implement the same three-stage
compile barrier contract. All initial adapters describe their release policy.
Wago reports narrow Go allocation deltas; V8 reports JS heap snapshots rather
than allocation volume; Wasmtime does not fabricate allocator diagnostics.
V8 reference release does not claim GC or native-code reclamation.

Opt-in conformance tests exercise correct results, wrong return values, wrong
memory bytes, imports, repeated samples, operation counts, stage ordering, and
timing rejection across all five configurations. CI now runs these tests for
the adapters each job builds. `runs/compile-phases-five-configurations` passed
200 scaling trials and 200 sacrificial checks on Darwin/arm64, retaining 1,200
phase events. Linux-specific collectors there are correctly unavailable.

`runs/linux-compile-phases-three-v1` passed twelve measured trials across Wago,
wazero and V8 on Linux/arm64, with 108 phase events and 36 available phase peaks.
Both bundles passed checksum verification and generated local reports. These
are functional diagnostics, not controlled performance comparisons.
The digest-pinned `recipes/wasmtime-linux.sh` built Wasmtime for Linux/arm64
with read-only sources and Cargo.lock. `runs/linux-compile-phases-five-v1` then
passed twenty measured trials across all five configurations: sixty samples,
180 phase events, and sixty available phase peaks. Checksums verified and a
report was generated. Other lifecycle phase barriers remain open.

### Compile CPU and memory accounting

Compile barriers now retain cgroup total/user/system CPU deltas, local bandwidth
periods/throttled periods/throttled time, and six memory.stat boundary fields.
CPU usage includes adapter descendants; bandwidth counters describe the local
cgroup, not ancestor throttling. Diagnostic windows include barrier transport.
Memory categories overlap (kernel includes page tables and slab), so no additive
decomposition is claimed. Missing fields, read errors and counter regressions
produce unavailable observations; genuine zeroes are retained.

`runs/linux-compile-accounting-v1` passed twenty measured trials across Wago,
wazero, Wasmtime Cranelift/Winch and V8 on Linux/arm64. Its sixty samples contain
sixty available observations for each of the six CPU/bandwidth metrics and
1,080 available memory-breakdown observations across 180 boundaries. Bundle
checksums verified and `reports/linux-compile-accounting-v1` was generated.
The Linux integration suite verifies CPU accounting includes child-process
work. Parser tests cover field ordering, future fields, malformed counters,
unit conversion, zeroes, absent values and regressions; root race tests pass.
These are functional checks, not official performance results. CPU collection
for other lifecycle phases remains open.

### Exported output-buffer oracles

The importer now supports Wago's `output_ptr_export` contract. Every adapter
resolves the zero-argument pointer export after the measured invocation and
checks the exact memory bytes relative to that address, outside timing. Missing
exports, wrong bytes and overflowing offsets fail correctness. Existing absolute
offset contracts keep a zero base. Input injection and vector contracts remain
explicitly unsupported.

`runs/output-pointer-check-v1` passed QOI encoding and LZ4 compression on Wago,
wazero, Wasmtime Cranelift/Winch and V8 (ten checks). QOI decoding and LZ4
decompression remain ten visible unsupported checks. `runs/output-pointer-timing-v1`
exercises compile, instantiate, first-call and steady scenarios with two launches
and two samples each: eighty successful measured trials and eighty unsupported
ones. These are functional measurements, not official performance comparisons.
The generated-module conformance test covers four scenarios, four correctness
cases and six configurations including wazero's interpreter (96 cases). CI runs
it alongside phase tests for the adapters built by each job. Root race tests and
vet pass.

### Input-buffer preparation

Workloads can now embed exact input bytes and an absolute or exported-pointer
wasm32 address. All adapters write the bytes after initialization, before the
timed invocation, once per fresh instance. Fresh-instance reset covers these
inputs; stateless reuse still requires an explicitly stateless workload contract.
Malformed hex, missing pointers, invalid pointer arity, address overflow and
out-of-bounds writes fail rather than truncate. Embedded input is part of the
locked workload and cross-run contract identity.

`runs/input-buffer-check-v1` passed all thirty checks for QOI encode/decode,
LZ4 compress/decompress, zlib inflate and Zstandard decompress across five
runtime configurations. `runs/input-buffer-timing-v1` passed 240 measured trials
covering compile, instantiate, first-call and steady with two independent
launches and two samples. The cross-adapter pointer/input suite now contains
240 cases including wazero's interpreter and malformed-input tests. These are
functional validation, not official performance results.

Wazero timing-mode compile and instantiate samples now verify the actual newly
created module/instance outside the timer, including input setup and exact
memory checks, instead of relying solely on the prepared-instance precheck.
Vector and command ABI support remain open.

`runs/input-buffer-phases-v1` additionally passed thirty compile-barrier trials
with sixty verified samples across the same workloads/configurations on
Darwin/arm64. All three input-buffer bundles passed checksum verification.
Root race tests and vet passed after the implementation changes.

### Ordered vector executor (partial integration)

`protocol.PrepareVectors` validates the exact output lengths, decodes expected
digests, and generates all input patterns before measurement under an explicit
byte budget. Its executor preserves ordered cases on one guest instance,
resolves pointer exports once (overriding absolute offsets), and checks each
output before the next invocation. Pointer resolution, writes, reads and digest
comparison are separate from the invocation callback so adapters can time only
guest calls. Bounds errors, traps and digest mismatches fail closed.

The optional `WASMBENCH_WAGO_CORPUS=/path/to/wago go test ./corpus -run
TestWagoPublishedVectors -v` passed against the hash-verified local Wago artifact:
all 35 cases for each BLAKE3 mode (hash, keyed hash, derive key), repeated on two
fresh wazero instances, totaling 210 exact digest checks. Unit tests cover
ordered execution, pointer precedence, zero-length input, pattern generation,
malformed contracts, allocation budgets, bounds failures and wrong digests.
This is executor evidence only. CLI/adapter-protocol integration and equivalent
cross-adapter validation remain open; imported vector workloads remain visibly
unsupported until that integration exists.

### Vector CLI and Go-adapter integration

The importer now admits strictly decoded ordered vector contracts with no mixed
single-call oracle. It locks the 64 MiB preparation byte budget and preserves
all cases under the `vector_sequence` work unit. Wago and both wazero backends
advertise vector timing support for compile, instantiate, first-call and steady.
Each sample owns a fresh instance; input generation occurs before measurement,
and execution samples sum guest-call intervals with `sequence_call_sum` type.
Pointer resolution, writes and digest checking stay outside those intervals.
The result is one whole sequence, not one request or a batch average of cases.
Compile/instantiate verify every vector on the newly measured object afterward.
Sacrificial admission runs two sequences on separate fresh instances.

`runs/vector-check-v1` passed nine checks (three modes across Wago and two wazero
backends), retaining six unsupported Wasmtime/V8 checks. `runs/vector-timing-v1`
passed 72 measured trials and retained 48 unsupported trials over four lifecycle
scenarios, two launches and two samples. Cross-adapter generated fixtures cover
all four scenarios and both correct and later-case wrong digests (24 tests).
Wasmtime/V8 adapters, memory profiles, barriers and cold-process vector timing
remain open and explicitly unsupported. Root race tests and vet pass.

### V8 vector integration

V8 now implements the same ordered vector timing contract, including checked
input generation under the locked byte budget, pointer precedence, exact
per-case digest checks, and fresh instances. Memory views are reacquired after
guest calls so memory growth does not leave a stale buffer. Engine tiering and
cache behavior retain their existing uncontrolled/production-default labels.
`runs/vector-v8-v1` passed all 24 measured BLAKE3 trials across four scenarios,
three modes and two launches. The cross-adapter vector suite now exercises ten
positive/negative cases per scenario across four configurations (160 cases),
covering malformed hex, empty cases, output lengths, admission budgets, missing
pointers, input/output bounds and mixed contracts in addition to wrong digests.
Wasmtime vector integration and vector diagnostic profiles remain open.

V8's non-vector compile timing path also now verifies the actual newly compiled
module, outside the timer, instead of relying only on the setup-module precheck.

### Wasmtime vector integration

Wasmtime Cranelift and Winch now implement ordered vector timing with the same
fresh-instance policy and explicit byte budget. Input generation, pointer
resolution, argument/result buffer preparation and exact digest checks remain
outside guest-call intervals. Compiled modules and instances used in timed setup
operations are the ones subsequently verified. Every sequence has one operation
and execution uses `sequence_call_sum`, matching the other adapters.

`runs/vector-wasmtime-v1` passed 48 measured trials for all three BLAKE3 modes
across compile, instantiate, first-call and steady, with two launches and two
samples per trial. Both Wasmtime configurations passed the ten-case generated
fixture matrix, including later-case digest corruption and malformed/bounds
contracts. The complete vector matrix now covers six runtime configurations
(240 positive/negative cases). Vector diagnostics, barriers and cold-process
timing remain explicitly unsupported; this does not close those requirements.

### Cold-process vector measurements

Cold-process vector timing now stops after the first complete verified sequence
in a new adapter process. It does not repeat the two-instance sacrificial gate
inside the measured process. The end-to-end interval includes initialization,
input generation, all cases, exact digest checking and control transport; it is
not the sum of guest-call intervals. The inner adapter sample is retained as
`adapter_samples` alongside the `individual_process` headline sample.

`runs/vector-cold-process-v1` passed 36 measurements across three BLAKE3 modes,
six configurations and two independent launches. The requested seven samples,
99 operations and three warmups correctly produced only one measured sequence
per process. Its eighteen sacrificial checks each retained two fresh-instance
samples. Request-budget tests distinguish these paths, and controller response
validation now rejects extra/missing samples, wrong indices and wrong warmup
flags, retaining invalid responses as raw adapter samples. Root race tests and
vet passed. Vector memory diagnostics and barriers remain open.

### Vector memory passes

Vector memory profiles now collect logical guest memory on all six
configurations and use the existing controller process-boundary/sampling
collectors where available. Wago and wazero add Go allocation volume/count,
heap boundaries and GC-cycle deltas over the complete per-sample lifecycle:
instance setup, verification and release are included. Input-pattern preparation
and shared module compilation precede this window; compile scenarios include
the measured module compilation within it. V8 captures JS heap boundaries with
the verified instance still held at the end. Wasmtime reports logical guest
memory without fabricating native allocator statistics. These distinct domains
and boundaries are explicit; no narrow-phase allocation or exact peak is claimed.

Cross-adapter conformance now includes a memory case for each scenario (264
cases total). Tests require logical memory observations in memory passes and
no diagnostic observations in timing passes. Shared hook tests verify collection
order and that timing never calls diagnostic hooks. Vector barriers and narrow
phase diagnostics remain open.

`runs/vector-memory-v1` passed 72 measured trials on Darwin/arm64 across all
six configurations, retaining 162 available logical-memory snapshots including
warmups. Linux procfs/cgroup metrics on this host remain unavailable, not zero.
The bundle passed checksum verification and generated `reports/vector-memory-v1`.
Root race tests and vet passed; these runs establish functional collection, not
controlled performance comparisons or Linux vector-footprint validation.

### Go-adapter vector compile barriers

Wago and both wazero backends now advertise the optional vector compile-barrier
capability. They hold the measured module at the compiled boundary, verify the
entire ordered vector sequence afterward, then release it before the final
boundary. Narrow Go allocation counters exclude vector input generation,
verification and release. A malformed/mixed vector contract is rejected through
the same shared admission checks as ordinary vector runs.

`runs/vector-go-phases-v1` passed eighteen measured trials on Darwin/arm64,
covering three BLAKE3 modes, three configurations and two launches, with 36
samples and 108 phase events. Linux-only observations on this host remain
unavailable. Cross-adapter tests exercise ordered success barriers and wrong
digests after the compiled boundary; a failed digest must not produce a successful
released event. Wasmtime/V8 vector barriers and Linux vector-specific collection
validation remain open.

### All-adapter vector compile barriers

V8 and Wasmtime Cranelift/Winch now implement the vector compile barriers,
completing support across all six runtime configurations. Vector input preparation
precedes the measured compile phase; ordered digest verification follows the
compiled boundary. Wasmtime drops the store and measured module before release.
V8 drops instance, function and module references without forcing or claiming
garbage collection. Its phased JS heap observations cover the compile API window,
not the verification lifecycle. A wrong digest preserves the first two events
and never emits a successful release boundary.

The all-adapter conformance suite, root tests, race tests and vet passed.
`runs/vector-all-phases-v1` passed 36 measured BLAKE3 trials on Darwin/arm64
(three modes, six configurations, two launches), with two samples per trial.
Bundle checksums verified and `reports/vector-all-phases-v1` generated.
These are functional checks, not official performance evidence; Linux-specific
vector phase collection still needs live validation. Other lifecycle barriers
and the remaining unchecked product requirements remain open.

### Linux vector evidence gate

`TestLinuxPhaseEvidence` loads and checksum-verifies an existing bundle, then
requires successful isolated compile trials, verified individual samples,
ordered barriers, available procfs/cgroup boundary snapshots, kernel-accounted
phase peaks, all-tree CPU deltas and local bandwidth counters. It passes on
`runs/linux-compile-accounting-v1` and rejects the Darwin vector bundle.

`recipes/test-linux-vectors.sh` is an opt-in integration recipe using the existing
ephemeral private-cgroup test setup. It requires `wasmbench:dev`, current native
Linux controller/adapter/test binaries under `.wasmbench`, the imported
`.wasmbench/vector-suite.json`, and the Wago checkout used by that suite. Example:

```sh
WASMBENCH_EPHEMERAL_CGROUP_TEST=1 sh recipes/test-linux-vectors.sh \
  ../../Wago/wago linux-vector-phases-v1
```

It mounts sources read-only, writes a new bundle under `runs/`, runs the existing
Linux cgroup integration tests, measures all six configurations, verifies bundle
checksums and runs the evidence gate. The recipe refuses an existing output
bundle. The gate also requires the complete runtime/workload/launch grid from
the locked manifest and the exact sample budget, rejecting missing or duplicate
cells.

`runs/linux-vector-phases-v1` completed on Linux/arm64 with all 36 measured
trials passing (three BLAKE3 modes, six configurations, two launches). Its 108
samples contain 324 ordered boundary events, 108 available kernel-accounted
phase peaks and 108 available all-tree CPU-total deltas, plus user/system CPU,
bandwidth counters and boundary memory breakdowns. The evidence gate and bundle
checksums passed; `reports/linux-vector-phases-v1` was generated. The same
container passed real-kernel peak-reset, descendant CPU, OOM, deadline and
isolation cleanup tests before measurement. A first container launch failed
before any trial due to Docker bind-mount visibility; the source file was
confirmed present and the subsequent launch succeeded.

This verifies Linux vector measurement wiring and scopes, not controlled
performance: Docker shares a development host with unrelated services. Official
machine qualification, other lifecycle barriers and remaining original product
requirements are still open.

# Packaged Linux density validation

The existing hardened container smoke workflow now runs three independent
density blocks on the packaged wazero compiler/interpreter and V8 adapters,
then generates a density report. The verifier checks all group budgets and
logical-memory sums, preserves V8's unsupported separate-engine cases, and
requires positive Linux process RSS boundary snapshots with the correct
`boundary_snapshot_only` quality. It checks ten logical-memory curves and their
30 paired marginal intervals, each exactly 65,536 bytes per added instance.
This extends the existing packaged core replay and 147 float-trial checks.

The live Linux/arm64 run `runs/container-density-v1` completed successfully:
120 successful density cells, 24 unsupported cells, 240 live-group RSS snapshots
(720 total density boundary snapshots), and three paired blocks per marginal
point. Every bundle passed checksum verification and both reports were generated.
The image ID was
`sha256:154dfebd9313195b136c38c22397c5b6cf0f693d068d92fab527ac6bd2a8335c`
(`wasmbench:density-package`). Container image-build Go tests, shell/Node syntax
checks and Actionlint passed. The CI container job invokes this same updated
recipe; hosted execution remains unverified.

The run used packaged binaries only, networking disabled, a read-only root,
the caller's UID/GID and no Linux capabilities. Its RSS observations are process
snapshots, not exact phase peaks, guest-only residency or evidence of independent
adapter cgroups. Three blocks establish the analysis plumbing, not publication
readiness or a performance ranking. Wago/Wasmtime are still outside this small
packaged image; their native conformance was verified separately.

# Wasmtime pooled allocation configuration

`wasmtime-pooling` is a separate Cranelift runtime identity selecting the native
pooling allocator. Its CLI flag, binary digest and explicit `instance_allocation`
policy are locked. Policy v1 fixes 128 core-instance/memory slots, 16 table slots
of up to 10,000 elements, 16 MiB maximum/reserved memory, 64 KiB guards, 128 warm
slots, zero keep-resident linear-memory bytes, and decommit batch size one.
Memory-protection-keys is not compiled in. Other settings use pinned Wasmtime
46.0.1 defaults. The allocator feature is enabled in the pinned dependency;
default Wasmtime/Winch continue to use on-demand allocation.

Density can now compare pooled and on-demand fresh instance groups, including
pool construction cost per fresh engine. This is not warmed pool-reuse latency
and is not an allocator-only controlled comparison: memory limits/reservation/
guards differ. Runtime capacity is explicit, so general workloads exceeding it
may fail admission. No reuse of live guest state is implied.

Rust tests fill all 128 slots, reject the 129th simultaneous instance, release
the group, then refill all slots simultaneously and verify both zeroed memory
and reset mutable globals. Nine adapter tests and four analyzer tests pass.
Built-adapter density race conformance, full Go tests/vet and Actionlint pass.
The Wasmtime CI job now includes pooled density conformance; hosted CI remains
unverified. An initial build exposed an unavailable feature-gated MPK setter;
the final implementation correctly records the disabled build feature instead.

`runs/density-pooling-memory-v1` and `reports/density-pooling-memory-v1` contain
32 successful measured cells across on-demand and pooled Cranelift. The bundle
passes checksum verification; independent checks confirm allocator identities,
the pooling CLI flag, sample/phase counts and group logical-memory sums. This
macOS functional run does not provide Linux residency evidence or performance
rankings. Sustained warm-pool reuse, snapshots and other product requirements
remain open.

# Retained-engine density cycles

The distinct `density-cycle` scenario retains engines/compiled modules for a
batch while creating and dropping fresh simultaneous Store/instance groups per
sample. Wasmtime Cranelift, Winch and pooled Cranelift support it. Engine/module
setup is untimed; group construction, initialization/input and invocation are
timed; verification and Store release are outside timing. The initial allocation
cycle is retained without hidden instance prewarm. Memory stages are
`before_density_cycle`, `density_cycle_ready`, and `density_cycle_released`.
The last release snapshot still holds engines/modules; their final destruction
happens afterward. This is not a claim of complete memory reclamation.

Cycle-only experiments use two verified sacrificial cycles. Mixed-scenario
experiments retain ordinary density admission and verify every measured cycle.
Unsupported adapters fail explicitly; ordinary density validation does not accept
cycles and silently rebuild resources. Protocol and driver tests pin the
one-group/no-warmup budgets and distinct phase contract. Rust tests check no
prewarmed instances, retained engine/module objects, and reset-sensitive results
across repeated refills. Cross-adapter race conformance passes across seven
configurations, exercising both supported and rejected cycles. Full Go tests,
vet, Actionlint, ten Rust adapter tests and four analyzer tests pass.

`runs/density-cycle-timing-v1` and `runs/density-cycle-memory-v1` each contain 32
successful measured cells across on-demand/pooled Cranelift and 32 two-cycle
admissions. Timing retains 20 cycles per cell; memory retains four cycles and
12 boundary events per cell. Checksums and independent Node checks verified
these budgets, order, first-cycle retention and phase stages. These short macOS
functional runs are not sustained-reclamation evidence or performance rankings;
long-run analysis, broader cycle adapters, snapshots and remaining requirements
are still open.

# Memory snapshot sequence analysis

`memory-snapshot-sequence-v1` emits per-trial ordered snapshot series in report
JSON, separated by the complete metric/phase/collector/quality/denominator
identity. It preserves warmup, zeros, negative endpoint changes and missing-data
reasons. Missing/ambiguous/unverified points or invalid sample order/boundaries
prevent endpoint summaries; unsuccessful trials retain diagnostic points but
cannot produce a summary. Independent launches are never joined. The coordinate
is sample index, not elapsed time or a claim of regular sampling intervals.

The snapshot metric allowlist and quality filter exclude allocation-volume/GC
counters and sampled/kernel peaks, even when a peak uses the same metric name
as RSS snapshots. First-to-last changes require a complete successful sequence
and include warmup when present. They are descriptive footprint differences,
not allocation volume, causal retention or a leak test. No confidence interval
or long-run trend claim is inferred from two endpoints.

Scoped analysis/publishing race tests cover negative changes, zero, nonlinear
intermediate points, holes, unavailable/nil/NaN/negative values, duplicates,
unverified/failed trials, changing sample boundaries and independent domains.
`reports/density-linux-timelines-v2` contains 3,960 series from sealed Linux
evidence. Independent Node checks validated 360 RSS series and 120 logical-memory
series and confirmed exclusion of peaks/allocation counters. The earlier v1
report is preserved; v2 explicitly filters peak quality. Dedicated HTML memory
timeline rendering and sustained-run qualification remain pending.

# Memory timeline report view

The offline report now has independent trial, snapshot-domain and sample-window
selectors. Charts preserve missing-point gaps, warmup color and real zero values;
they never join independent launches. The complete sequence's endpoint change
is explicitly separate from the selected display window. Tables expose statuses
and reasons, and each series links to its sealed raw trial and exact measurement
provenance. Windows contain at most 500 points to bound rendering cost without
discarding evidence; all windows remain selectable.

`reports/density-linux-timeline-html-v1` renders the existing sealed Linux run.
Playwright checked a real two-point RSS series, then a temporary 501-point
synthetic series to verify a missing-point gap, warmup marker, two windows and
reachability of the final sample, restoring the original data afterward.
Desktop and 390×844 mobile screenshots were inspected; document width remained
390px on mobile. Console had no errors or warnings. The screenshots are
`output/playwright/memory-timeline-desktop.png` and
`output/playwright/memory-timeline-mobile.png`. The local HTTP server and browser
session were closed after verification. Direct file-URL behavior was not tested.
This supersedes the JSON-only limitation above; sustained-run qualification and
the remaining product requirements are still open.

### Initial WASI command execution

The `exact_command` contract now carries explicit argv (including argv0), stdin,
hash-verified embedded fixtures, expected exit code and stream digests, and
bounded output capture. `wasi-preview1-readonly-v1` exposes only a staged private
read-only fixture tree, empty environment, zero-filled randomness and wazero's
synthetic clock defaults. It is deliberately identified separately from core
modules and from any future production host profile. The initial inline input
budget is 1 MiB; larger inputs retain an unsupported contract rather than being
dropped from inventory. Portable external fixture references remain open.

Wazero compiler/interpreter run `_start` on a fresh instance for every sample.
Compile and instantiate time only their respective APIs, then execute and verify
the command outside timing. First-call/steady time `_start`, including host I/O
capture but excluding digest calculation. Steady repeats fresh commands, not a
long-lived guest. Cold-process retains its separately scoped end-to-end sample
and raw command result. Sacrificial admission checks two fresh instances.
Go memory observations describe the whole sample lifecycle, not narrow API
allocations. Exit codes, stream hashes and stream byte counts survive in raw
sample records; the controller independently checks command result evidence.

Tests on both backends cover stdin replay, explicit argv, empty environment,
fresh globals, nonzero expected exit, wrong stdout/stderr/exit, ignored output
overflow, invalid stdin, mixed contracts and unsupported phase requests.
Importer tests reject changed fixture hashes, traversal, escaping symlinks and
unpinned stdin; unknown output semantics remain unsupported. The full Wago
catalog imports 132 expanded contracts, including unsupported entries.

The current sibling checkout was re-imported with `--ids all`: all 132
artifacts matched their catalog hashes. The 103 contracts with supported exact
oracles (72 core workloads and 31 WASI Preview 1 commands; 69 scalar/memory,
three ordered-vector, and 31 command oracles) passed sacrificial correctness
checks on wazero. The sealed `correctness_only` bundle passed `verify`; all 103
trials were `ok`, and every retained sample was verified. This is real-corpus
correctness evidence on one adapter, not timing or cross-runtime qualification.

Other command adapters, writable-file oracles, output normalization, self-check
admission, WASI reactors/components, Emscripten and command phase barriers remain
unfinished. `runs/command-timing-v1` is not exact-build evidence: an adapter was
rebuilt during that run.

The stable-binary rerun `runs/command-timing-v2` passed all 80 measured trials:
coreutils-sort, icepack-pack, icepll-clock and json2csv-people, across compiler
and interpreter, five lifecycle scenarios and two launches. The bundle retains
160 command result records including warmups and cold-process adapter samples.
Its recorded adapter digest matches the fixed executable; bundle checksums
passed and `reports/command-timing-v2` was generated. `runs/command-memory-v1`
also passed eight measured first-call trials with lifecycle Go observations
and verified bundle checksums.
These Darwin/arm64 runs establish correctness and functional collection, not
official performance comparisons or WASI coverage for every imported command.

### Portable command fixture files

Command files now support pinned `path`, `size`, and SHA-256 references in
addition to inline data. `stdin_file` aliases a named fixture without duplicating
it in the control message. Import verifies regular-file sizes and hashes using
the corpus root boundary. Runs stream-verify and copy references into deduplicated
`inputs/<sha256>` objects, then lock bundle-relative paths. Adapter preparation
resolves a separate copy of those paths, leaving the manifest portable. Wazero
stages verified bytes into its private read-only fixture root and reopens stdin
for every fresh instance. Inline metadata retains its 1 MiB bound; file contents
have a separate 256 MiB aggregate bound. Bundle hashing and copying now stream
instead of allocating a buffer equal to the entire file size.

Tests cover files larger than the former inline budget, missing/changed files,
size and digest mismatches, mixed inline/reference data, path traversal,
relocation after deleting the source input, sealed-bundle tampering, and
non-mutating preparation path resolution. Both wazero backends test referenced
stdin replay and reject changed referenced bytes.

Wago command integration remains pending: the inspected raw WASI imports API
does not expose an owned close handle for its filesystem state; lifecycle-managed
provider configuration must be integrated without leaking state between samples.
No Wago or WASI source checkout was changed to work around this.

`runs/command-files-v1` and `runs/command-files-reproduced-v1` both passed all
three measured workloads (coreutils-sort, brotli-compress and esbuild-minify)
on wazero compiler/Darwin arm64, with two samples and two fresh-instance
sacrificial checks per workload. Brotli and esbuild use the same 2,160,055-byte
stdin fixture: each bundle stores one shared content-addressed copy. The initial
bundle passed checksum verification and generated `reports/command-files-v1`;
the reproduction uses the exact runner/adapter hashes and bundle-relative input
paths. These are functional portability checks, not performance qualification.

### WASI command compile-phase collection

Wazero compiler/interpreter now advertise `can_command_compile_phases`.
Fixture staging and WASI setup precede the first barrier. The newly compiled
module remains held at `compiled`; command execution and hash verification
follow outside the measured compile region. Instance/module/stdin handles close
before `released`. Go allocator snapshots use `compile/api_window` and exclude
verification, capture and release. The engine and staged fixtures remain alive
for the batch; release does not imply collection of all host heap or page cache.

Real subprocess protocol tests pass for both backends: success yields three
events per sample; wrong exit, wrong stream digest and output overflow yield no
successful release event; changed fixture bytes fail before the first barrier.
The Linux evidence gate now checks retained command exit/output evidence and
narrow allocator labels as well as complete trial coverage and OS observations.

`runs/linux-command-phases-v1` passed twelve measured trials across coreutils-sort,
icepll-clock and json2csv-people, two backends and two launches. Thirty-six samples
retained 108 boundary events, 36 available kernel-accounted phase peaks and 36
all-tree CPU-total deltas. The container's cgroup integration tests, checksums
and strengthened evidence gate passed, and `reports/linux-command-phases-v1`
was generated. The first Docker launch failed before measurement because of
transient single-file mount visibility; a read-only parent-mount check succeeded
and the retry completed. This remains functional Linux/arm64 evidence on a shared
development host, not official performance qualification.

The existing Linux recipe accepts `WASMBENCH_PHASE_SUITE_FILE`,
`WASMBENCH_PHASE_RUNTIMES` and `WASMBENCH_PHASE_TIMEOUT` overrides for command
validation. Wago/V8 command support and non-compile lifecycle barriers remain open.

### Linux Wasmtime command validation

The pinned Linux recipe now accepts `sh recipes/wasmtime-linux.sh test`. It
mounts the shared command fixture read-only and runs the release tests inside
the same digest-pinned Rust image used to build the adapter. All five tests
passed on Linux/arm64. In addition to command oracles, file rights, clocks and
overflow, the Linux-only descriptor test checks 32 successive `released`
barriers on each backend, rejecting accumulating stdin/preopen descriptors.
Runtime-using unit tests are serialized so parallel tests cannot distort those
process-local descriptor counts.

After build/test completion, two sequential, isolated cgroup runs passed:

- `runs/linux-wasmtime-command-phases-v1`: coreutils-sort, icepll-clock and
  json2csv-people on Cranelift, two launches and three samples per trial:
  six measured trials, 18 samples and 54 barrier events.
- `runs/linux-winch-command-phases-v1`: icepll-clock and json2csv-people on
  Winch, the same launch/sample budget: four measured trials, 12 samples and
  36 barrier events. Coreutils is deliberately not in this phase-validation
  subset; its pinned Winch compilation failure remains in the earlier timing
  and correctness bundles. This subset is not a common-corpus performance score.

Both runs passed the cgroup spawn/cleanup, descendant timeout, OOM accounting,
same-descriptor peak-reset and child-CPU tests before measurement. The evidence
gate checked every declared measured cell, exact command results, all ordered
barriers, RSS/PSS/private/virtual snapshots, cgroup accounting breakdown,
kernel-accounted phase peaks, process-tree CPU deltas and bandwidth counters.
Both sealed bundles passed checksum verification and have matching reports
under `reports/`. These are functional Linux/arm64 checks on a shared Docker
development host, not controlled performance qualification.

### Explicit application initialization

All six runtime configurations now expose `app-init` for core scalar workload
contracts with an explicit `initialize` export. Each sample has fresh instance
state even for a workload otherwise labeled stateless. Compilation,
instantiation (including the Wasm start function), export lookup, input writes,
workload execution/oracle checks and release are outside the initialization
timer. Exactly one no-argument, void call is measured; warmup is not performed.
The controller rejects batched/warmup app-init evidence and records workloads
without initializers as `not_applicable`, with no latency samples.

The built-in `lifecycle` suite's stateful fixture traps on missing/repeated
initialization, missing start-function effects, input installation before init,
or reused workload state. Cross-runtime subprocess tests passed all 66 cases,
including wrong signatures, traps, result/memory mismatches, missing inputs and
unsupported phase requests. Default Go tests additionally exercise both wazero
backends and controller boundary validation. Root race tests, vet, Rust release
tests and the combined existing built-adapter conformance tests passed.

In memory passes the Go adapters snapshot allocation activity only around the
initializer, V8 records heap boundaries around it, and Wasmtime records logical
guest memory immediately after it. These domains remain distinct. Command and
vector app-init contracts are not yet implemented. App-init phase barriers
were subsequently added; see the initialization-boundary evidence below.

Darwin/arm64 functional evidence:

- `runs/app-init-timing-v1`: 36 successful trials across six configurations,
  two launches, and instantiate/app-init/first-call, three samples each.
- `runs/app-init-memory-v1`: six successful app-init trials, three samples each,
  with the declared engine-specific observations.
- `runs/app-init-wago-v1`: BLAKE and UTF initializers passed 24 measured trials
  across all six configurations and two launches. The 24 JSON-AS cells retain
  `preflight_failed` because its `env.abort` host import is not yet implemented;
  the run correctly exits nonzero rather than hiding the failed coverage.
- `runs/app-init-not-applicable-v1`: both core workloads retain
  `not_applicable` and have no measured samples; the CLI reports the incomplete
  requested experiment with a nonzero exit.

The timing, memory and imported-Wago bundles passed checksum verification.
Reports for timing and imported-Wago coverage are under `reports/`. These are
functional checks, not controlled performance claims or Linux app-init evidence.

### AssemblyScript abort host profile

All six configurations now implement `assemblyscript-abort-v1`: the sole host
function is `env.abort(i32,i32,i32,i32)->()`, and every call fails the workload.
Errors retain unsigned message/file pointers and line/column values without
dereferencing guest strings or reading host files. Wago uses its owned public
HostFuncRef with a typed HostCall callback and HostTrap; the other adapters use
their native host-error mechanism. No source changes were made to Wago.

The importer selects this explicit profile for Wago's named
`workloads/assemblyscript/` family, preserving the original catalog contract and
artifact digest. An initializer name alone never selects it. Unknown imports
remain failures. V8 import objects have null prototypes, preventing inherited
JavaScript functions such as `valueOf` from satisfying undeclared imports.

The 216-case cross-runtime test matrix passes compile, instantiate, app-init,
first-call, steady and phased compile for correct fixtures, aborting initializers
and calls, missing host profiles, wrong oracles and unknown imports. Invalid
message pointers still produce abort evidence; aborted phase runs never emit a
successful release event. Default Go tests cover both wazero backends and
importer selection. Combined built-adapter conformance, root race tests, vet and
Rust release tests passed.

`runs/assemblyscript-host-timing-v2` passed 144 measured trials: JSON-AS
serialization/deserialization, BLAKE and UTF across six configurations and six
lifecycle scenarios, with two samples (plus steady warmup). The v1 timing run
also passed but predates the V8 null-prototype hardening; v2 is the final-source
rerun. The earlier `app-init-wago-v1` failure evidence remains unchanged.

`runs/assemblyscript-host-phases-v1` passed 24 compile trials with 48 samples and
144 barrier events across the same four workloads and six configurations. Both
final bundles passed checksum verification and have corresponding reports.
These are Darwin/arm64 functional results with one independent launch per cell,
not confidence-qualified performance comparisons or Linux phase-peak evidence.

## Core teardown verification and Wasmtime support

All six configurations now support core scalar teardown. Each sample creates
fresh state, initializes and applies input, executes and verifies the workload
outside timing, then measures one release with no warmup or batching. Fixed
Wago's missing teardown execution and wazero/V8's verification of only the
initial instance. Samples retain the verified return value before release.
Wasmtime drops the store, compiled module, and engine in its timed region.
Effective configuration records each adapter's distinct release policy; V8
only releases references, Wago retains its host runtime, and no adapter forces
collection or claims allocator reclamation. The controller rejects batched or
warmup teardown samples.

The 42-case teardown integration matrix covers fresh state, memory passes,
wrong return/memory oracles, missing inputs, initializer traps, and unsupported
barriers on all six configurations. AssemblyScript tests now also exercise
teardown, including aborting calls and initializers. Default tests cover the
controller boundary and wazero abort behavior.

`runs/teardown-timing-v1` passed 12 trials (two launches per configuration,
three samples each); `runs/teardown-memory-v1` passed six trials with three
samples each. Both bundles passed checksum verification; the timing bundle has
a report at `reports/teardown-timing-v1`. These are Darwin/arm64 functional
checks, not controlled performance comparisons. Command/vector teardown,
teardown phase barriers, Linux teardown evidence, and explicit reclamation
diagnostics remain unfinished.

## Ordered-vector teardown

Ordered-vector teardown is now supported by all six configurations and admitted
by the controller. Each sample compiles and initializes fresh state, executes
and verifies every vector in order outside timing, and measures resource
release under the adapter's declared policy. Wasmtime releases a per-sample
engine as well as its store/module; wazero closes the compiled module and
runtime; Wago closes the instance/module; V8 drops its handles without GC.
There is no warmup or batching. Go allocator and V8 heap observations use the
release window; logical guest memory is captured before release.

The cross-runtime vector suite now includes teardown and a stateful fixture
that rejects sequence replay or reused instances. It verifies malformed
digests, budgets, pointers, bounds, and ambiguous contracts. A protocol test
asserts setup, invocation, oracle checking, and the logical snapshot precede
the release-only diagnostic window.

`runs/vector-teardown-timing-v1` passed 36 trials (three BLAKE3 sequences,
six configurations, two launches, three samples per trial).
`runs/vector-teardown-memory-v1` passed 18 trials with three samples each.
Both bundles passed checksum verification and have matching reports. Root
race tests, vet, Rust release tests, and built-adapter integration tests passed.
This is Darwin/arm64 functional evidence, not qualified performance or Linux
reclamation evidence. Command teardown and teardown phase barriers remain open.

## WASI command teardown

The four command-capable configurations (wazero compiler/interpreter and
Wasmtime Cranelift/Winch) now implement teardown with fresh runtime/module
state per sample. Command execution, exact stream/exit verification, fixture
staging, and fixture cleanup are excluded from release timing. Wasmtime drops
its store (including WASI context and descriptors), module, linker, and engine.
wazero closes remaining instance/module/stdin/runtime resources. Its
`proc_exit` implementation can already close the instance during execution;
the explicit `command_teardown_policy` records this distinction. Capture-buffer
reclamation and forced GC are not included or claimed. Go memory diagnostics
cover the release window only; Wasmtime does not invent native allocator data.

The 32-case built-adapter matrix verifies both ordinary return and `proc_exit`,
fresh stdin/instances across three samples, inline/file inputs, bad output,
wrong exit, output overflow, and rejected teardown barriers. Default Go and
Rust command lifecycle tests also include teardown. Race tests, vet, Rust
release tests, and command integration tests passed.

`runs/command-teardown-timing-v1` records 22 successful trials and two retained
Winch/coreutils preflight compile failures. `runs/command-teardown-memory-v1`
records 11 successful trials and one identical preflight failure. These cover
coreutils sort, icepll, and json2csv, three samples per successful trial. Both
bundles passed checksum verification and have matching reports. This is local
Darwin/arm64 functional evidence, not qualified performance data. Teardown
phase barriers, Linux release evidence, and reclamation diagnostics remain open.

## Core scalar teardown barriers and Linux evidence

All six configurations now support a two-barrier scalar teardown protocol:
`before_teardown` after verified workload execution, and `torn_down` after
release. Compile keeps its original three barriers. The controller enforces
scenario-specific ordering/counts and rejects incomplete or duplicate events.
Incorrect workloads never reach the first teardown barrier. Adapter-local
timing and memory snapshots exclude handshakes; OS/cgroup observations retain
their explicit barrier-window scope and do not imply allocator reclamation.

Linux collection resets/reads memory.peak through the same descriptor for the
release window, labels snapshots `teardown/<stage>`, and reports process-tree
CPU deltas and local bandwidth counters under `teardown/barrier_window`.
Unavailable collectors preserve teardown labels and null values. The Linux
evidence gate now validates both compile and teardown stage schemas.

`runs/teardown-phases-v1` passed 12 Darwin/arm64 trials across all six
configurations (36 samples, 72 events). `runs/linux-teardown-phases-v1` passed
six Linux/arm64 trials for wazero compiler/interpreter and V8 (18 samples,
36 events), with atomic cgroup isolation, RSS/PSS/private/virtual snapshots,
same-FD peak, memory accounting, whole-tree CPU, and bandwidth observations.
The container cgroup tests and full evidence gate passed; both bundles passed
checksum verification and have matching reports. This is functional evidence
on a shared Docker host, not official performance data. Wago/Wasmtime Linux
teardown evidence, vector/command teardown barriers, and reclamation diagnostics
remain unfinished.

## Vector and command teardown barriers

Ordered-vector teardown supports the release handshakes on all six
configurations; WASI command teardown supports them on wazero compiler and
interpreter plus Wasmtime Cranelift and Winch. Separate
`can_vector_teardown_phases` and `can_command_teardown_phases` capabilities
prevent treating compile support as release support. Every vector case and
command output/exit oracle is verified before `before_teardown`; wrong results
emit no teardown events. `torn_down` follows actual resource release and local
diagnostic snapshots, outside the adapter's release timer. Go tests also prove
that a failed handshake closes its owned vector instance and returns no sample.

The built-adapter tests cover both correct and incorrect phased vectors and
commands. Root race tests, vet, and Rust release tests passed. Local evidence:

- `runs/vector-teardown-phases-v1`: 18 successful trials, three BLAKE3
  sequences across six configurations, 54 samples and 108 events.
- `runs/command-teardown-phases-v1`: eight successful trials, icepll and
  json2csv across four configurations, 24 samples and 48 events.
- `runs/linux-vector-teardown-phases-v1`: 18 successful trials for wazero
  compiler/interpreter and V8, 54 samples and 108 events.
- `runs/linux-command-teardown-phases-v1`: eight successful trials for wazero
  compiler/interpreter, 24 samples and 48 events.

All bundles passed checksum verification and have corresponding reports.
Both Linux bundles passed the full cgroup/process/peak/CPU evidence gate in
ephemeral private-cgroup containers. These are functional checks, not official
performance data. The two-command subset avoids the already-recorded pinned
Winch/coreutils compile failure; earlier failure bundles remain unchanged.
Wago/Wasmtime Linux teardown evidence and explicit reclamation diagnostics
remain unfinished.

## Linux teardown coverage across all supported configurations

Rebuilt Wago for Linux/arm64 with commit plus tracked Go/assembly source digest,
without modifying its checkout. Rebuilt Wasmtime using the pinned Linux Rust
image and Cargo lock; all five Linux Rust tests passed, including descriptor
release between command samples. Builds and test linking finished before any
measurement began. The recipe now accepts built-in suites via
`WASMBENCH_PHASE_SUITE`, and the evidence gate additionally checks scalar
teardown return values and individual-operation sample types.

Verified Linux/arm64 bundles (two launches, three samples per trial):

- `runs/linux-all-scalar-teardown-phases-v1`: 12 successful trials on the
  stateful lifecycle fixture across all six configurations.
- `runs/linux-all-vector-teardown-phases-v1`: 36 successful trials for the
  three BLAKE3 sequences across all six configurations.
- `runs/linux-all-command-teardown-phases-v1`: 16 successful trials for
  icepll/json2csv across all four command-capable configurations.
- `runs/linux-assemblyscript-teardown-phases-v1`: 48 successful trials for
  JSON-AS serialization/deserialization, BLAKE, and UTF across all six
  configurations, exercising initialized real modules and imported host state.

All 112 trials (336 samples, 672 release events) passed the Linux evidence
gate: exact sample/launch coverage, correctness evidence, atomic cgroup
isolation, process snapshots, same-descriptor release-window peaks, memory
accounting, whole-tree CPU, and local bandwidth observations. Checksums passed
and matching reports were generated. Existing known Winch/coreutils failure
evidence is unchanged; that command is not in this two-command fixture subset.
These are shared-Docker functional checks, not official performance data or
proof of physical-memory reclamation. Explicit reclamation, the remaining
lifecycle collection windows, and the broader acceptance ledger remain open.

## Application-initialization phase boundaries

All six configurations now support memory-profile `app-init` barriers:
`before_app_init`, `app_initialized`, and `app_released`. Compilation,
instantiation/start, and initializer lookup precede the first boundary. The
middle boundary follows the initializer and local memory diagnostics, before
input installation or workload execution/verification. Only this initialization
window receives Linux cgroup peak and CPU deltas. The final boundary follows
verified execution and instance release, retaining the compiled module and
engine; `app_init_release_policy` records each adapter's behavior. V8 drops
references without forced GC. These snapshots do not prove reclamation.

Tests reject timing-profile barriers, incorrect ordering and incomplete release
evidence. Initializer traps stop after the first boundary; workload/oracle
failures stop after the second. The built-adapter initialization and
AssemblyScript host matrices passed across all six configurations, including
the new abort boundary cases. Root race tests, vet, and local Rust release tests
passed.

Verified bundles (two launches, three samples per trial):

- `runs/app-init-phases-v1`: 12 successful Darwin/arm64 lifecycle trials
  across all six configurations.
- `runs/assemblyscript-app-init-phases-v1`: 48 successful Darwin/arm64 trials
  for Wago's JSON-AS serialize/deserialize, BLAKE, and UTF workloads across all
  six configurations.
- `runs/linux-app-init-phases-v1`: six successful lifecycle trials for wazero
  compiler/interpreter and V8.
- `runs/linux-assemblyscript-app-init-phases-v1`: 24 successful trials for
  those four imported workloads on the same three Linux configurations.

All bundles passed checksum verification and have matching reports. Both Linux
bundles passed the complete cgroup/process/peak/CPU evidence gate, including
retained scalar oracle results and exact sample/launch coverage. These are
functional checks on shared development hosts, not official performance data.
Wago/Wasmtime Linux app-init phase verification, command/vector initialization
contracts, and the broader acceptance ledger remain unfinished. The Wago
checkout was read-only throughout this work.

### Linux initialization coverage across all six configurations

Rebuilt Linux/arm64 Wago with its commit plus tracked-source digest and
Wasmtime with the pinned Rust image/Cargo lock. All builds and test linking
finished before measurements. The Linux evidence gate now also requires the
initialization capability/release policy and checks Go diagnostic windows.

- `runs/linux-all-app-init-phases-v1`: 12 successful lifecycle trials across
  all six runtime configurations, two independent launches and three samples.
- `runs/linux-all-assemblyscript-app-init-phases-v1`: 48 successful trials for
  JSON-AS serialization/deserialization, BLAKE, and UTF on all six configurations
  with the same launch/sample budgets.

All 60 measured trials, 180 samples, and 540 boundary events passed the complete
Linux evidence gate and bundle checksum verification. Matching reports are in
`reports/`. Root race tests and vet passed. Wago's existing unrelated changes
were preserved. This closes the Wago/Wasmtime Linux initialization coverage gap
above, not the broader lifecycle/ABI or product acceptance requirements. These
shared-container runs remain functional evidence, not official performance
qualification.

## Go GC pause accounting

Go memory collection now consistently emits `host.gc.pause_time` in nanoseconds
and `host.gc.forced_cycles` alongside total cycles, allocation volume/count,
and start/end HeapAlloc. A shared snapshot normalizer replaces four duplicated
Wago/wazero paths while preserving their capture positions and declared
denominators. It also covers existing initializer, vector, and WASI command
windows through `GoMemoryWindow`.

Pause accounting is the cumulative `runtime.MemStats.PauseTotalNs` delta,
not concurrent GC CPU work, API elapsed time, or a latency percentile. No
collection is forced in production. Boundary-straddling GC accounting is not
clipped to the operation timer; broad batch windows retain their existing
explicit verification/release inclusion labels. The definitions follow
[Go's runtime contract](https://pkg.go.dev/runtime#MemStats), also checked
against the installed Go 1.26.5 documentation.

Deterministic tests verify snapshot deltas, units and registry coverage,
including decreasing HeapAlloc alongside positive allocation volume. A live
test explicitly forces GC only in its fixture to prove cycle/pause capture.
Root race tests, vet, and rebuilt Wago/wazero compiler/interpreter subprocess
tests passed (initialization, compile phases, teardown, vectors and commands).

Darwin/arm64 evidence:

- `runs/go-gc-memory-v1`: 30 successful trials across compile, instantiate,
  app-init, first-call and teardown on the three Go configurations.
- `runs/go-gc-compile-phases-v1`: 24 successful compile-barrier trials using
  JSON-AS serialization/deserialization, BLAKE and UTF on those configurations.

Both bundles passed checksum verification and have matching reports. All 162
measured samples contain pause observations with nanosecond units; 22 observed
positive pause deltas. These are collection checks, not performance claims.
GC CPU estimates, per-pause distributions, explicit reclamation experiments,
and the broader memory acceptance requirements remain unfinished.

## Core scalar instantiation boundaries

All six runtime configurations implement memory-profile instantiation barriers:
`before_instantiate`, `instantiated`, and `instance_released`. Engine/module
compilation and import preparation precede the first boundary. The timed API
constructs exactly one fresh instance, including its Wasm start function.
Explicit initialization, input writes and workload verification follow the
second boundary. The last boundary follows verified instance release, retaining
the compiled module and engine. Runtime-specific ownership is recorded in
`instantiate_release_policy`; no adapter forces GC or claims reclamation.
Wasmtime store/import preparation and Wago's host-runtime module-handle
preparation are outside this window. V8 releases JavaScript references only.

Go allocation/GC observations use the narrow instantiation window. V8 records
heap snapshots and Wasmtime records logical guest memory. Linux process/cgroup
snapshots occur at all three boundaries; only the first-to-second interval
gets phase-peak and CPU deltas. The metric registry now describes all supported
phase windows instead of describing compilation alone.

Cross-runtime tests cover missing and optional initializers, wrong results,
missing inputs, start-function traps and initializer traps. A start trap emits
only the first boundary; later initialization/oracle failures never emit the
release boundary. The stateful lifecycle fixture verifies start/init/input
ordering and fresh instance state. AssemblyScript abort tests exercise the
new phased scenario. Root race tests, vet, and local Rust release tests passed.
Command/vector instantiation barriers remain explicitly unsupported; other
lifecycle windows and the broader acceptance ledger remain unfinished.

Verified instantiation bundles (two launches, three samples per trial):

- `runs/instantiate-phases-v1`: 12 Darwin/arm64 lifecycle trials on all six
  configurations.
- `runs/assemblyscript-instantiate-phases-v1`: 48 Darwin/arm64 trials on all six
  configurations, JSON-AS serialize/deserialize, BLAKE and UTF.
- `runs/linux-instantiate-phases-v1`: the same 12 lifecycle cells on Linux/arm64.
- `runs/linux-assemblyscript-instantiate-phases-v1`: the same 48 real-workload
  cells on Linux/arm64.

All 120 measured trials passed correctness (360 samples, 1,080 boundary events).
Both Linux bundles passed the strengthened process/cgroup/peak/CPU evidence
gate, including scalar result retention, release policy and allocator labels.
Checksums passed and matching reports were generated. Builds finished before
measurement; the sibling Wago checkout remained unchanged. These shared-host
runs are functional evidence, not official performance qualification.

## Independent analyzer structure and explicit feature policies

`wasm-analyze` now emits schema 2 / `core-structure-v2`, with wasmparser pinned
to 0.251.0. It distinguishes actual imports from import groups, records import
module/name/kind/index/type references, export names/indices, the start function,
flattened type indices, and defined-function parameters/results. Function indices
include the imported-function offset; defined indices remain separately named.
Imported functions, memories, tables and globals are counted separately from
definitions. Recursive groups no longer stand in for individual type indices.

`wasmbench analyze --features default|wasm1|wasm2|wasm3` enforces the selected
validator policy, recording the profile and enabled flag names. These are
accepted features, not inferred minimal requirements. Policy definitions come
from the [pinned wasmparser API](https://docs.rs/wasmparser/0.251.0/wasmparser/struct.WasmFeatures.html).
Components are explicitly rejected rather than silently flattening nested
module statistics. Component structure support remains unfinished.

Three Rust tests cover import offsets/signatures, feature-policy enforcement,
start indices, malformed input and component rejection. The mixed-import
multi-value fixture is `corpus/testdata/analyzer-signatures.wat` and its generated
Wasm. Root race tests and vet passed, including independent analysis of all 40
scaling fixtures. CLI checks accepted the fixture under wasm2 and rejected it
under wasm1 with a multi-value validation error. A read-only sweep passed all
118 distinct artifacts in Wago's 125-entry benchmark catalog, checking exact
artifact hashes, import counts, function-index offsets and signature fields.

This delivers explicit independent validation and richer structural facts,
not automatic admission. Pinning analyzer identity/policy into run locks,
storing its complete output with each artifact, checking declared feature
requirements, and runtime feature-capability admission remain open.

## Locked independent admission

`plan`, `run`, and `check` now accept `--validation-profile` to pin an independent
wasmparser executable by absolute path and SHA-256, schema/analysis version and
feature policy. Existing locks cannot be overridden with this flag. Run/fetch
verification checks the pin; execution checks it again around analyzer use.
Each distinct copied artifact is independently validated before any adapter
starts, with a 30-second deadline and bounded stdout/stderr. The full JSON is
stored once per digest under `validation/`, separate from the existing light
structural summary, and sealed with the bundle.

Loading checks the saved report's schema, analyzer identity, analysis version,
artifact hash, encoding, success status and policy against the lock. It never
executes the analyzer, so offline reports remain usable without a local binary.
Reproduction reruns validation using the exact pinned executable. Old locks
without an analyzer remain readable and make no independent-admission claim.
Failed admission returns an error before adapter startup and leaves at most an
unsealed partial directory, not successful measurement evidence.

Tests cover missing/mismatched report identity, binary changes, invalid policy,
output bounds, offline evidence mismatch even with recomputed checksums,
saved-lock reproduction, and multi-value rejection under wasm1 before any
adapter log is created. Root race tests and vet passed.

Live Darwin/arm64 evidence:

- `runs/admission-wasm1-v1`: 16 successful measured trials, core workloads on
  wazero/V8 with compile and first-call, two launches and three samples.
- `runs/admission-wasm1-reproduced-v1`: all 16 repeated trials passed; validation
  reports are byte-identical to the source run.
- `runs/admission-assemblyscript-v1`: 48 successful first-call trials for the
  four Wago workloads on all six configurations, with wasm2 admission. Three
  distinct artifacts produce three saved validation reports.

All three bundles passed verification and have reports. Wago was unchanged.
This is opt-in policy admission, not inferred minimal feature requirements,
runtime feature-capability matching, component admission, or official host
qualification. Those requirements and the broader product ledger remain open.

## Artifact-specific validator feature probes

The independent analyzer now emits `core-structure-v3` with
`single-flag-removal-v1` evidence. After validating under the selected policy,
it validates again for each enumerated enabled flag with only that flag's bits
removed. Results retain the exact policy/disabled/remaining masks, boolean
outcome and failure message/byte offset. This proves conditional necessity
under the pinned validator policy, not a unique minimal proposal set. Composite
flags overlap, some switches are not proposals, and individually removable
flags are not necessarily jointly removable. Runtime support is not inferred.

New analyzer locks require v3 evidence. Saved v2 locks/reports remain readable;
reproduction still requires their original executable hash. Admission and
offline loading reject missing/duplicate probes, mismatched masks, missing
outcomes, absent failure witnesses and contradictory success/failure fields.

Rust tests distinguish a multi-value signature from unused SIMD support and
detect SIMD/bulk-memory instruction requirements in
`corpus/testdata/analyzer-features.wat` (WABT 1.0.41 generated Wasm). Go tests
exercise evidence consistency and old-version loading. Root race tests, vet,
and all four analyzer tests passed. A read-only sweep analyzed all 118 distinct
Wago benchmark artifacts with 2,242 removal probes and no failures. Five
artifacts failed without SIMD and 61 without bulk-memory under the default
policy; these are admission facts, not performance results.

`runs/admission-feature-probes-v1` passed 48 first-call trials across all six
configurations on the four AssemblyScript workloads (two launches, three
samples). Its three distinct artifacts have complete sealed v3 probe reports.
Bundle verification passed and `reports/admission-feature-probes-v1` was
generated. Both earlier v2 admission bundles still pass offline verification.
Wago remained unchanged. Default mandatory admission, component support and
the broader acceptance ledger remain open.

### Explicit validator-policy compatibility

Adapter descriptions now optionally expose `validator_features` with a pinned
validator namespace, configuration evidence, and explicit boolean declarations.
The runner derives conditional requirements from validated v3 removal probes,
keyed by artifact hash in private run state. Requirements are never loaded from
user JSON; replay recomputes them from newly validated artifact bytes. Missing
declarations, absent policy evidence, older analysis and different namespaces
do not infer compatibility or incompatibility. Ordinary correctness admission
still applies. Only a known-disabled required flag skips execution and records
`unsupported` throughout the requested matrix, with the flag and policy reason.

Both wazero configurations now explicitly select `api.CoreFeaturesV2`. Their
pinned source defines five excluded experimental flags (threads, tail calls,
extended constants, exceptions, typed function references), which are declared
disabled in wasmparser 0.251.0 vocabulary. No exhaustive support claim is made.
Other adapters do not yet provide equivalent feature policy declarations.

Unit tests cover unknown/absent/positive/negative declarations and namespace
boundaries. Built-adapter tests verify actual rejection of a tail-call module
and acceptance of SIMD/bulk-memory instructions by both wazero backends. A
bundle integration test covers duplicate artifacts, all requested cells, and
replay of the serialized lock. Root race tests and vet passed.

`runs/feature-capability-v1` exercises the documented fixture suite with wazero,
wazero-interpreter and V8: 24 measured cells, 16 successful and eight explicitly
unsupported, plus six sacrificial checks. V8 executes the tail-call oracle;
all three configurations execute the SIMD/bulk-memory oracle. Bundle checksums
verified and `reports/feature-capability-v1` was generated. These are local
functional checks, not official performance evidence. Wago was untouched.

### Default independent admission in the CLI

New CLI `plan`, `run` and `check` requests now pin independent validation with
the `default` policy unless a named wasm1/2/3 policy is selected. Empty and
unknown policies are rejected. Missing analyzer binaries fail with build
instructions; no builds happen during measurements. Existing locks are not
silently migrated: their pinned policy, including legacy absence of admission,
survives loading/replay. Every explicit policy override with `--lock` is rejected.
The lower-level `NewLock` API still supports legacy/analyzer-free fixtures.

`make build` and CLI adapter builds include the analyzer. Doctor reports its
identity/hash or missing-build instruction. Docker now includes a pinned Rust
analyzer stage, CI sets the tested Rust toolchain, and the Linux build/test
recipe builds both executables and mounts the analyzer fixtures read-only.
The phase recipe mounts the Linux analyzer alongside adapters.

CLI tests cover all four policies, omitted-default behavior, invalid/empty
policy rejection, immutable locked policies, legacy locks, and missing binary
failure without writing a plan. Root race tests, vet and `make build` passed.
Linux release builds and all five adapter/four analyzer Rust tests passed.
The revised Dockerfile and hosted CI workflow have not yet been built/run as
complete workflows; the local Linux recipe is independently verified.

`runs/default-admission-v1` and its `default-admission-reproduced-v1` replay
each passed 16 measured trials across wazero/V8 without a validation flag.
`runs/linux-default-admission-v1` passed 24 instantiation memory trials across
all six configurations, including the full Linux phase-evidence gate. All
three bundles contain two pinned v3 validation reports; checksums verified and
local reports were generated for the native and Linux originals. These are
functional evidence, not official performance measurements. Wago was untouched.

### Packaged container verification

The updated Dockerfile built successfully on Linux/arm64 using all three pinned
base images. Its build stage passed the Go tests and produced the controller,
wazero adapter and pinned independent analyzer. No host-built executables were
substituted during the subsequent test.

`recipes/test-container.sh` resolves a supplied image tag to an immutable local
ID and runs doctor/check/run/reproduce/verify/report with no network, a read-only
root filesystem, all capabilities dropped, no-new-privileges, and the invoking
user's UID/GID. Only a newly created evidence directory and temporary state
are writable. The recipe refuses an existing evidence directory. Its Node
verifier checks full trial coverage and statuses, result verification, default
analyzer reports, exact replay executable/configuration identities and identical
analysis evidence, correctness-only publication prohibition, and report output.

`runs/docker-admission-v1` contains six successful correctness-only checks,
24 successful measured cells plus six sacrificial checks in the original run,
and the same coverage in replay. All three bundles verified. Report generation
and the recipe's evidence assertions passed using packaged wazero compiler,
wazero interpreter and V8. Image ID:
`sha256:9dc451ac2776de6a160a936779dcf9e362ab86055947980fc122b1f506b36b64`.
This is functional packaging evidence on a shared Docker host, not controlled
performance data. Docker/AMD64 and the new hosted CI packaging job remain
unverified until run on those systems.

### Wasmtime validator policy evidence

Wasmtime's engine configuration and description now share one explicit subset
of its pinned 46.0.1 default policy, using the wasmparser 0.251.0 vocabulary.
Cranelift enables tail calls and relaxed SIMD; Winch disables them. Both disable
GC, function references, current/legacy exceptions; Winch additionally disables
GC types and stack switching, plus threads on AArch64. These choices follow
the pinned `config.rs` defaults/backend exclusions and are explicitly applied
through `Config::wasm_features`. Unlisted flags stay unknown to the controller.
Positive flags describe validator policy, not complete backend instruction
support. In particular, Winch's partially implemented instructions still go
through correctness checks and can fail visibly.

Rust tests compile/execute the tail-call oracle with Cranelift, establish its
validation rejection with Winch, and compile/execute the SIMD/bulk-memory
oracle with both. Root race tests and vet passed. The configurable built-adapter
feature test passed for wazero compiler/interpreter, Wasmtime Cranelift/Winch
and V8, including duplicate artifacts and replay. CI now requests this coverage.

`runs/wasmtime-feature-policies-v1` (Darwin/arm64) and
`runs/linux-feature-policies-v1` each preserve all 40 measured cells: 28 correct,
12 explicitly unsupported, plus ten sacrificial checks. Cranelift/V8 execute
tail calls; Winch/wazero reject them through declared policy. All five execute
SIMD/bulk memory. Both sealed bundles verified and their local reports were
generated. The Linux adapter was rebuilt from current source; these are local
functional results, not official performance evidence. Wago and V8 policy
declarations, component admission and broader product requirements remain open.

### Offline artifact evidence inspection and report links

Verified bundle loading now derives one compact admission summary per distinct
artifact. It records validated/not-recorded status and conditional necessary
flags only for v3 evidence. Legacy locks and v2 reports do not acquire invented
feature claims. These summaries are derived views, not changes to sealed files.

`inspect --run ... --workload ... --artifact-evidence` verifies the bundle and
returns the contract, analyzer pin, summary, complete analyzer JSON and matching
trials without executing an analyzer or adapter. Unknown workloads and checksum
tampering fail. The existing trials-only inspection output is unchanged.

Reports now render an artifact-admission section with exact artifact identities,
policy/version context and links into the copied raw evidence. Full reports
remain external rather than duplicating potentially large opcode/type data in
the embedded page. Mobile rows are stacked with labels and wrapped hashes.

Root race tests and vet passed; deterministic inspection tests cover v2/v3,
legacy absence, artifact deduplication, trial filtering, missing IDs, offline
operation and tampering. Focused tests passed after the responsive refinement.
Live CLI inspection returned the TAIL_CALL witness and 25 matching trials from
the saved Wasmtime feature matrix. Playwright verified v3 links resolve to the
exact report digest, v2 says probes not recorded, and legacy runs say independent
admission was not recorded. Desktop/mobile screenshots are under
`output/playwright/admission-*`; final report is `reports/admission-inspection-v3`.
The first mobile rendering required horizontal scrolling; browser QA prompted
the stacked-row correction. Full product acceptance remains open.

### Expected invocation-trap contracts

Added `expected_trap` oracles and per-sample `trap_result` evidence with four
normalized categories: unreachable, memory out of bounds, integer divide by
zero and integer overflow. The contract is intentionally phase-specific:
bare-core, no-argument exports on fresh instances with no initialization/input/
host/return-value/memory/command/vector contract. Only an invocation error can
satisfy it. Factory/start/lookup errors, normal returns, mismatched and unknown
errors fail. The controller independently checks each sample's category,
classifier/message presence and single-operation shape before accepting it;
cold-process promotion preserves the trap evidence.

All six configurations advertise this capability for timing first-call, steady
and cold-process. Go adapters share the sample loop. Wago and Wasmtime classify
typed runtime trap codes; wazero checks its pinned internal error type before
matching an exact diagnostic, while V8 requires WebAssembly.RuntimeError and
an exact message. Classification/verification is outside the call timer. Other
profiles/scenarios and trap categories remain unsupported, not silently mapped
to successful returns or arbitrary exceptions.

The new embedded `traps` suite has four exports in one WABT-1.0.41-built module.
A mutable global makes a reused instance return normally instead of trapping,
so repeated success proves the prescribed reset. Unit tests cover ambiguity,
missing/wrong evidence, setup-vs-invocation boundaries, cleanup and warmup.
All 288 live adapter cases passed across six configurations, two scenarios,
four categories, and correct/normal-return/wrong-trap/missing-export/start-trap/
unsupported-profile modes. CI requests the applicable configurations.

Root race tests and vet passed. `runs/invocation-traps-v1` on Darwin/arm64 and
`runs/linux-invocation-traps-v1` each passed 144 measured trials plus 24
sacrificial checks, with 480 verified trap samples including warmup and cold
process evidence. All six Linux adapters were rebuilt from current sources.
Both bundles verified and local reports were generated. A separate memory-pass
request retained all four workloads as unsupported in
`runs/trap-memory-unsupported-v1`, which also verified. These are local functional
results, not official performance evidence. Additional trap categories,
initialization traps, trap diagnostics/memory passes, floating tolerances and
the broader correctness/product requirements remain open.

### Expected invocation-trap memory passes

All six runtime configurations now separately advertise
`can_measure_invocation_traps` and support memory first-call, steady and
cold-process requests. The controller still rejects trap phase barriers,
non-execution scenarios and other diagnostic profiles. Older adapters that
advertise only timing are not assumed to support memory collection.

The Go trap loop has an explicit per-instance diagnostic-window callback.
It starts after construction/export lookup, snapshots immediately after the
call timer stops, then classifies/verifies the error and releases the instance.
Go's seven allocation/heap/GC observations use `*/trap_api_window` and the
denominator `trapping_invocation_excluding_classification_release`; they are
process-Go-heap observations, not guest-only or native allocations. V8 records
two JS heap snapshots, not allocation deltas. Guest logical memory is observed
before release; Wasmtime/V8 require an exported `memory`. The fixture now exports
its memory and still detects accidental instance reuse through a mutable global.
Its changed bytes receive a new artifact digest; prior sealed runs are untouched.

Unit tests establish setup/begin/invoke/snapshot/classify/release ordering,
timing-mode absence of diagnostics, and cleanup without invocation when memory
callbacks are missing. The built-adapter matrix passed all 432 cases over six
configurations, two scenarios, four trap categories and nine success/failure
modes. Root race tests and vet passed; native Wasmtime's five tests passed.

`runs/invocation-trap-memory-v1` (Darwin/arm64) and
`runs/linux-invocation-trap-memory-v1` each contain 144 successful measured
trials plus 24 sacrificial checks and 480 verified samples including warmup.
All adapters were rebuilt before collection. Both bundles verified, their
reports were generated, and saved-sample audits checked trap identity, exact
one-operation shape, 64 KiB guest snapshots and allocator denominators.
Linux measured trials also retain available RSS/PSS/private/virtual observations.
These OS observations cover the declared request, not the narrow trap API window.
Cold-process call diagnostics remain in `adapter_samples`; promoted process
samples do not relabel them. These are functional shared-host results, not
official performance evidence. Trap phase barriers, additional trap categories,
floating tolerances and the rest of the product acceptance ledger remain open.

### First-invocation trajectories and observed break-even models

Added a timing `trajectory` scenario to all six configurations. It creates a
fresh compiled-module/instance state per request, then measures sequential
individual calls on that same instance without an untimed benchmark pre-call.
Declared initialization and per-call verification remain outside timers.
Requested operations do not batch this scenario; retained warmup flags label
the initial calls without removing them from raw evidence. Stateless scalar
core contracts are required. Other reset/ABI/oracle/profiling contracts remain
explicitly unsupported. Existing steady semantics are not reinterpreted.

The adversarial five-call fixture establishes no hidden pre-call (five samples
succeed), same-instance reuse (sixth call traps), and fresh request state (a
second five-call request succeeds). All six built adapters passed, including
invalid profile/reset/negative-warmup rejection. Controller/protocol unit tests
check individual-operation shape, ordering, warmup and invalid contracts. CI
requests the corresponding built-adapter tests. Root race tests and vet passed;
the 432 trap cases and five native Wasmtime tests still passed.

`break-even --run ...` and report datasets now offer versioned observed-prefix
models. Default setup is compile + instantiate, plus declared app-init; users
can explicitly choose other distinct setup phases. Within each complete block,
the model sums setup-phase sample medians and cumulative trajectory call times,
including warmup. Curves use block medians/pointwise bootstrap intervals;
optional paired comparisons use common complete blocks and signed total-time
deltas. Missing, failed, duplicate, malformed and incompatible evidence cannot
supply a zero cost. All workload/configuration coverage remains visible.
There is no extrapolation or invented constant slope for tiered engines.

These synthetic totals combine separately collected phases, not an observed
end-to-end execution. They exclude unselected setup, input preparation,
verification and inter-call gaps. Tier/background events are still unobserved,
and one pointwise crossover is not a permanent or simultaneous-confidence win.
The report shows logarithmic observed counts, total-time axes, pointwise
intervals, setup/coverage/outcomes and contributing raw-trial links.

Final `runs/trajectory-breakeven-v3`, its exact-build replay
`runs/trajectory-breakeven-reproduced-v1`, and Linux/arm64
`runs/linux-trajectory-breakeven-v2` each verified 108 measured trials plus 12
sacrificial checks. Each has 900 ordered trajectory samples and 12 complete
curves through invocation 25. An independent Node audit recomputed every curve
median from raw setup samples and cumulative calls. Runtime configurations,
runner hash and collection options matched on replay. Reports were generated
for all three. These are shared-host functional checks, not official rankings.

Playwright checked workload switching, six plotted configurations, mobile width
390 without horizontal page overflow, uncertainty tables and resolving raw
trial links. Visual QA prompted responsive SVG dimensions/readable time-axis
labels. Screenshots are `output/playwright/breakeven-*-v2.png`; browser and local
servers were closed. Broader trajectories (commands/vectors/fresh state),
observed tier events, optional constant-cost forecasts and the full remaining
product acceptance scope are not claimed complete.

### Versioned Linux host-setting evidence

`doctor` and new Linux manifests now contain `linux-host-facts-v1`, with raw
source paths, values and explicit availability per setting. Sources include
per-record CPU identity/features/microcode, per-CPU topology, online/isolated
CPU sets, CPUFreq policy drivers/governors/limits, SMT, NUMA CPU lists/distances,
controller allowed CPU/node sets, total memory/swap and VM/huge-page policy.
Per-size THP and hugetlb settings are retained when exposed. CPU descriptions
are concise model names; transient raw cpuinfo MHz/BogoMIPS fields are excluded.
Free-memory, current-frequency and process-ID observations are also excluded
from identity. Heterogeneous CPU feature sets are not collapsed into a union.

The collector is read-only, bounded and filesystem-injected. Tests exercise
x86/ARM fixtures, heterogeneous cores, missing/denied/oversized sources, empty
but valid settings, feature ordering, transient-counter stability and changed
configuration sensitivity. Cross-run comparison tests reject changes in facts,
availability and fingerprint version, including legacy-vs-new evidence. Legacy
JSON loading does not invent new fields. Root race tests and vet passed; the
collector fixtures also passed in a Linux/arm64 container.

Live Linux collection observed 322 facts (191 available, 131 not exposed).
Two `doctor` calls and both `runs/linux-host-facts-v1` and
`runs/linux-host-facts-reproduced-v1` had identical host evidence in one container.
Each sealed run passed 16 measured wazero/V8 core trials plus four sacrificial
checks. Exact-build replay and matched-host cross-run comparison passed. Both
reports retain the identical host evidence. Older Linux trajectory and Darwin
bundles still verify; Darwin does not acquire Linux-derived host facts.

Scope is the controller namespace at manifest creation. This is not enforcement
of machine policy, actual NUMA placement or proof of fixed clock speed, and
containers may conceal host settings. Adapter cgroup effective limits remain
separate. Mid-run policy stability, dedicated-host guarantees, machine-policy
enforcement, richer non-Linux fingerprints and official publication remain open.

### Source-to-Wasm build evidence and snapshot replay

Added `source-lock`, `source-build`, `source-verify` and `source-rebuild`.
Recipes pin explicit input files, tool executable bytes and invocation paths,
arguments, environment, runner, independent analyzer policy, license, declared
source revision and a core scalar correctness contract. Sealed bundles retain
source snapshots, step logs, Wasm output, independent validation and a suite
usable by the existing runtime runner. The suite carries the entire source lock
as provenance. Snapshot replay works after removal of original source files,
requires the exact pinned executables, and rejects different output bytes.
Offline verification checks checksums plus lock/artifact/source/analyzer/suite
and step-evidence consistency, without requiring tool executables.

Tests cover source changes before execution, recipe traversal/reserved paths,
reserved environment, invalid analyzer policy, existing-output preservation,
failed compiler exit, missing output, symlink output, timeout, excessive logs,
scratch cleanup, snapshot tampering and resealed artifact inconsistency.
The real LLVM test exposed multi-call driver name loss when resolving
`wasm-ld` to `lld`; a regression now preserves invocation paths while still
checking resolved executable bytes. macOS system temporary-directory symlinks
are canonicalized before checking generated output paths.

`WASMBENCH_SOURCE_LLVM_TEST=1 go test -race ./...` and `go vet ./...` passed.
The source-build unit tests also passed in Linux/arm64 Docker (the Homebrew LLVM
integration test is explicitly skipped there). Two native LLVM builds passed
independent wasm1 validation and produced identical bytes. Final-CLI evidence:
`builds/source-xorshift-v2` and `builds/source-xorshift-replayed-v2`, both verified,
artifact SHA-256 `c492c2b2667c2166db9756ac7f17c2488f73535f0d1fecc61d66f05a9d067f18`.

`runs/source-xorshift-timing-v2` passed six sacrificial correctness checks and
54 measured trials across Wago, wazero compiler/interpreter, Wasmtime
Cranelift/Winch and V8: compile, first-call and trajectory, three launches each.
Its exact source-lock provenance was checked against the source build bundle;
the runtime bundle verified and its report is in `reports/source-xorshift-timing-v2`.
These are shared-host functional observations, not official performance data.

This is a build/reproduction foundation, not completion of source-toolchain
comparison or end-to-end analysis. Builds are trusted local processes, not a
sandbox, and do not automatically pin dynamic libraries, compiler resource files
or transitive tools. Source revision is a declared label; file hashes determine
actual source identity. Step durations are operational diagnostics, not repeated
compiler-performance samples. Non-scalar source contracts, controlled repeated
compiler comparisons, multi-toolchain comparison policy and source-aware report
drilldowns remain open. Source snapshots remain in the separate source bundle;
runtime bundles retain its lock, not a redundant copy of all source-build files.

### Explicit source-toolchain output comparisons

Added `source-compare` over two verified source bundles and two verified runtime
measurement bundles. The source track holds source revision, license, staged
input names/hashes, workload correctness contract, independent analyzer and
feature policy fixed. Each run must bind to its own exact source-build artifact
and provenance. Runtime configuration, measurement runner, protocol, resources
and host must match. Compiler executables, flags and build environment remain
variable and fully represented in the source locks.

The existing independent-launch ratio bootstrap is reused without treating
cross-run block numbers as pairs. Unsupported/failed outcomes and the common
successful subset remain visible. Ordinary runtime comparison keeps its exact
Wasm identity requirement. Tests reject source, oracle, policy, analyzer,
artifact, provenance, runtime configuration, runner, host and protocol changes,
and accidental additional workloads. A regression test explicitly proves that
different source outputs are accepted only by the source comparison track.

Root race tests and vet passed. Live LLVM `-O0` and `-O2` recipes produced
different valid artifacts (310 and 385 bytes respectively) for the same 1,000-step
xorshift oracle. `runs/source-compare-o0-v1` and `runs/source-compare-o2-v1`
each passed 54 measured trials and six sacrificial checks across all six
configurations, with three launches for compile, first-call and steady execution.
Both bundles verified. Six `reports/source-compare-<runtime>-v1.json` files
contain three valid ratio/interval cells each, both complete source-build
identities and runtime configurations. These are exploratory shared-host results,
not official performance comparisons.

This compares runtime behavior of source-compiler outputs, not source-compiler
speed. Repeated/interleaved source compilation measurements, cross-toolchain
aggregate policies, generic runtime-comparison website views, and the distinct
end-to-end stack track remain unfinished.

`source-compare-set` now compares a fixed multi-workload source corpus. It binds
each runtime workload to exactly one verified source-build result by workload
ID, requires the same workload-ID set on both sides, applies `SameTask` to every
baseline/candidate build pair, and rejects missing, duplicate or extra runtime
workloads before calculating the usual per-workload independent-launch ratios.
The result retains all source-build evidence in deterministic workload-ID order;
it does not combine unlike work into a single score. A two-workload fixture
confirms both ratios, ordering and fail-closed handling of incomplete build/run
sets. The CLI, usage text and README document the workflow. Cross-workload
aggregate policy and end-to-end stack analysis remain open.

`source-compare-set-report` now generates a portable, offline page from that
fixed source workload set. It recomputes from verified inputs, copies and
revalidates every source-build and runtime bundle, preserves workload-level
effects/intervals/outcomes/configurations and links to raw evidence, and emits a
reproduction command. `verify-source-set-report` verifies the outer seal,
included bundle seals, exact build-to-run bindings, and analysis recomputed from
the copied evidence. The renderer uses DOM text nodes for result data and embeds
JSON with HTML escaping. A real report generated from the existing LLVM `-O0`/
`-O2` xorshift builds and runtime runs passed report verification and browser
smoke checks on desktop and mobile with the page loaded before disconnecting the
browser from the network; there were no console errors. Screenshots are
`output/playwright/source-set-report-desktop-v3.png` and
`output/playwright/source-set-report-mobile-v3.png`. This closes the
source-output comparison view only; it is not a general runtime-comparison site
or a cross-workload score.

`compare-report` now provides the general two-run runtime comparison site.
Analysis is regenerated from checksum-verified measurement bundles; the page
retains every planned workload/scenario cell, including absent/changed workloads,
failed outcomes and insufficient-launch intervals. It shows both exact runtime
configurations, successful launch coverage, outcome counts, the independent
launch-bootstrap method, raw copied manifests, and a reproduction command; it
does not produce an aggregate successful-subset score. `verify-compare-report`
checks the report seal and both copied bundles, recomputes comparison analysis,
and rejects any mismatch. Reports are self-contained and use escaped embedded
JSON plus DOM text nodes. Live verification used `runs/regression-baseline` and
`runs/regression-candidate` as `wazero` vs `wazero`: all four planned
workload/scenario cells were common successful cells. The verifier passed, and
an independent SHA-256 pass checked all 72 sealed report files. The report was
opened in a browser, which rendered all four cells without console errors; the
page was then checked at desktop and 390px mobile widths, including after
loading with the browser offline. Screenshots are in
`output/playwright/runtime-comparison-desktop.png` and
`output/playwright/runtime-comparison-mobile.png`.

### Repeated source compilation wall-time experiments

Added `source-bench` and `source-bench-report`, using the distinct versioned
`source.build.tool_wall` metric. Configurations retain the source locks,
correctness-runtime pins, fixed repetition budget, warmup blocks, timeout and
randomization seed. Each variant gets a sacrificial source build and oracle
check before any measured block. Failed/unsupported correctness admission stops
measurement; exact artifact hashes gate each subsequent build. Trials retain
all source/build evidence, warmups and failure outcomes.

Variants execute sequentially in randomized blocks with fresh scratch and tool
processes. The measured value sums recipe-step process launch-through-wait wall
times, excluding staging, pin verification, independent validation, correctness
checks and inter-step gaps. It is explicitly not whole-pipeline elapsed time or
compiler CPU time. Statistics use complete builds, not steps, as replicates;
paired comparisons use only common successful blocks. Nulls represent unavailable
values, and intervals require at least three builds/pairs. Offline verification
checks the configuration, deterministic schedule, nested admission/build
evidence, exact output binding and timing arithmetic before analysis.

Tests cover deterministic seeded ordering, block/warmup coverage, correct
bootstrap units, failure retention, unavailable-vs-zero semantics, caller lock
immutability, resealed false timing rejection and wrong-oracle admission stopping
all measured builds. The real LLVM integration and all root race tests passed
under `WASMBENCH_SOURCE_LLVM_TEST=1`; vet passed.

`builds/source-bench-v1` contains two LLVM variants, each admitted on all six
runtime configurations, six measured blocks and one retained warmup block:
12 measured builds, two warmup builds and 12 sacrificial runtime oracle checks.
All builds matched their admitted artifacts. `reports/source-bench-v1.json`
passed offline verification and contains six paired blocks with bootstrap
intervals. An independent Node check recomputed each raw timing sum and checked
trial/outcome/interval coverage. This is shared-host functional evidence, not an
official compiler ranking.

Source compiler CPU/memory accounting, enforced resource and cache policies,
pilot-selected budgets, source-benchmark replay, source-aware website views and
broader compiler/ABI coverage remain open. The whole-product acceptance criteria
remain unchanged and are not yet satisfied.

### Source benchmark snapshot replay

Added `source-bench-replay`, preserving the complete locked compiler experiment
configuration and deterministic block schedule. It uses the saved admission
source snapshots, requires exact runner/tool/analyzer/runtime pins and matched
host identity, checks each rebuilt admission against the original artifact, and
reruns every runtime correctness oracle before measurement. Parent evidence is
linked by checksum-manifest SHA-256, configuration hash and admitted artifact
hashes. Offline verification checks reproduction metadata against the actual
new admissions. Replay collects new timing observations; it does not promise
identical timing or hermetic build dependencies.

The real LLVM integration test removes a source fixture created in its own
temporary directory before replay, proving independence from the original
source path without touching repository inputs. Tests also cover host mismatch
before output creation, schedule/artifact identity, existing-output preservation,
rejection of failed-admission baselines and resealed false reproduction metadata.
Root race tests with the LLVM integration enabled and vet passed.

`builds/source-bench-v2` and `builds/source-bench-replayed-v2` each completed
12 measured builds, two retained warmups and 12 sacrificial correctness checks
across six runtime configurations. Both generated verified reports. An independent
Node check confirmed the same configuration hash, all 14 schedule entries and
output hashes, and the exact parent checksum-manifest hash. The older
`builds/source-bench-v1` still verifies and can be reanalyzed offline; replay still
requires its original runner executable rather than silently substituting the
new CLI. Source benchmark CPU/memory collectors, resource enforcement and
website integration remain unfinished.

### Dedicated source compiler CPU accounting

Added `source-bench --profile cpu`, with a distinct measurement and analysis
version. Go process-state user/system accounting is captured after each tool
exits and the wall timer has stopped. Per-step observations retain collector,
version, unit, scope, accuracy, availability and user/system/combined values.
The metric registry defines their OS wait-accounting boundaries explicitly;
this is not guaranteed complete process-tree or cgroup coverage. The CPU pass
excludes controller, staging, validation and oracle work. Nanosecond serialization
does not assert nanosecond OS accounting precision.

Complete-build CPU totals drive CPU statistics and paired comparisons. Wall
durations remain raw diagnostics. Timing-only builds retain their prior
uninstrumented shape; missing/zero CPU observations are distinct. Replay
preserves the CPU profile, and offline verification checks per-step and
per-build CPU arithmetic, metadata, profile and measurement-version consistency.

Tests cover live CPU accounting, nonnegative/sum/overflow validation, missing
versus zero, timing-pass separation, CPU rather than wall aggregation, real
LLVM CPU observations and CPU-profile replay. Root race tests with LLVM
integration and vet passed. Linux/arm64 tests passed for the CPU collector and
profile-controlled source-build path; the Homebrew LLVM integration is explicitly
skipped in that container, not reported as Linux LLVM evidence.

`builds/source-cpu-v1` and `builds/source-cpu-replayed-v1` each passed 12 measured
builds, two warmups and 12 sacrificial correctness checks across all six runtime
configurations. Reports retain six paired CPU blocks. Independent Node checks
recomputed every user-plus-system sum, complete-build CPU total and median.
Older wall-time benchmark evidence still verifies and produces wall-time
statistics, without invented CPU observations. These are functional shared-host
results, not official compiler performance claims. Full-tree cgroup CPU,
compiler memory accounting, resource enforcement and source-aware website
views remain unfinished.

### Source tool cgroup isolation and memory receipts

Source benchmark tool steps now support Linux cgroup-v2-at-spawn isolation,
memory/swap/CPU quota/CPU-set/PID limits, descendant cleanup and explicit OOM
classification. Existing process-group settings survive cgroup setup. Each
step gets a fresh leaf; controller, validation and correctness processes stay
outside. No isolation fallback is allowed. Saved successful receipts validate
local controller readbacks, including normalized CPU-set intervals without
expanding potentially enormous ranges. Ancestor constraints and exclusive-core
ownership are not inferred from these local settings.

The dedicated memory profile records post-wait current memory and fresh-step
lifetime peak before cleanup. Its build metric is maximum step cgroup peak,
not sum, simultaneous whole-build peak, RSS or heap. Missing step observations
make the aggregate unavailable; real zeros survive. Reports use byte summary
fields, and replay preserves the requested policy and collection profile.

Verified: full root race suite with native LLVM integration enabled, and vet.
Linux/arm64 tests in a privileged ephemeral Docker private cgroup namespace
verified actual tool cgroup membership, memory observations, timeout descendant
cleanup, bounded OOM classification, leaf removal, receipt verification and
resource-preserving build replay. Source receipt/replay tests use fixture tools
and a fixture analyzer; they are lifecycle evidence, not Linux LLVM performance
or end-to-end runtime admission evidence. Native LLVM tests still cover existing
timing/CPU behavior. Unit tests cover tampered scope/units/values/policy, maximum
versus sum, missing versus zero, and byte rather than wall-time statistics.

Still pending for this seam: a real Linux compiler memory benchmark through
runtime admission and report/replay, richer retained failed-step evidence, and
source-aware website views. Full-tree compiler CPU and the broader unchecked
product requirements remain open. This is not full-product completion.

### Real Linux compiler memory benchmark and replay

Added an ephemeral Linux source integration image, guarded workflow script and
independent Node evidence checker under `recipes/source/`. Actual Debian LLVM
16.0.6 compilation/linking at O0 and O2 now passed the memory workflow on
Linux/arm64. Both variants independently validated and passed exact runtime
oracles on wazero/compiler, wazero/interpreter and V8 before measurements.

`builds/source-linux-memory-v1/{original,replayed}` each completed 12 measured
builds, two retained warmups and six runtime correctness trials. Both reports
passed nested bundle verification. The independent checker recomputed every
build's maximum step peak and byte median, checked six paired blocks, matched
schedule/configuration/output identities and verified the replay parent checksum
manifest hash. Original source paths were present for this live run; snapshot
independence remains covered by the separate replay regression test.

Image/tool preparation finished before measurement. Run networking was disabled;
only a private ephemeral Docker cgroup namespace was modified. This is real
compiler lifecycle evidence, not fixture-compiler evidence or official performance
qualification. The integration image resolves Debian packages during build;
locked executable hashes do not imply pinned transitive shared libraries.
Failed-step evidence retention, full-tree compiler CPU, source-aware website
views and other unchecked full-product requirements remain open.

### Failed source admission diagnostics

Failed source admissions now retain the partial build result in the enclosing
sealed benchmark, matching the existing measured-trial diagnostic retention.
Offline validation checks locked build identity, attempted step prefix, log
presence, CPU and memory metadata, and evidence-backed OOM classification.
Only the final attempted step may report OOM, cleanup failure or pre-spawn
isolation setup failure. Failed builds never receive aggregate performance
values, and failed admission still prevents all measured trials. Older admission
bundles without partial receipts remain readable without invented observations.

Root race tests with LLVM integration enabled and vet passed. Regression tests
exercise a real failing subprocess, archived CPU diagnostics and rejection of
resealed invalid metadata. The Linux/arm64 private-cgroup container test exercises
an actual bounded OOM during admission, retains its current/peak observations,
verifies the sealed failed benchmark and confirms there are no measured trials.
Standalone failed builds remain unsealed diagnostics; this addition covers their
enclosing source benchmark evidence. The full product remains incomplete.

### Explicit source tool timeout and cancellation outcomes

Failed tool steps now record an observed deadline-exceeded or canceled context
condition. Admissions and measured trials distinguish `timeout`, `canceled`,
`oom` and ordinary `build_failed`; OOM takes precedence when observations
coincide. This is not a causal claim based on signal text or an elapsed-time
heuristic. Offline verification requires the corresponding partial receipt,
rejects invalid/nonfinal context markers and rejects them on successful builds.
Analysis retains separate outcome counts and no numeric samples for failures.

Native race tests with LLVM integrations, vet, and Linux/arm64 container tests
passed. Real subprocess tests cover deadline expiration with a waiting child,
already-canceled contexts, retained receipts and rejection of missing context
evidence. Classification tests cover OOM precedence; analysis tests ensure all
four failure categories remain visible without fabricated samples. The Linux
suite also retains the cgroup cleanup and OOM admission checks. This addition
classifies tool failures; independent validation/oracle timeout categorization
and the other incomplete full-product requirements remain open.

### Offline fixed-baseline version history

Added `history --runs ... --runtime ... --out ...` for explicitly ordered,
checksum-verified runtime measurement bundles. The first run is the fixed
baseline, with independent-launch bootstrap comparisons for each later run.
Points preserve full manifests, absolute timing summaries, source checksum
manifest hashes, configuration changes, per-cell failure coverage and unavailable
comparisons. Incompatible hosts/protocols/profiles stay visible without invented
ratios; duplicate identities and invalid baselines are rejected. This does not
infer revision ancestry, interpolate missing values, bisect changes, correct for
multiple comparisons or confer official regression qualification.

Tests cover fixed rather than rolling baseline ratios, explicit point order,
changed hosts/artifacts, deterministic intervals, missing runtime configurations,
duplicate identities and correctness-only baselines. The live CLI report
`reports/history-v1.json` verified two recorded regression runs and retained
`runs/memory-live` as an incomparable profile rather than dropping it. Root
race tests with native LLVM integrations and vet passed. Interactive history
charts remain pending, as do other unchecked product requirements.

### Portable interactive history reports

`history-report` now builds a sealed offline directory directly from verified
run bundles. It recomputes fixed-baseline analysis, copies and re-verifies all
raw bundles, exports trial JSON and records original checksum-manifest hashes.
Existing outputs and outputs nested in an input bundle (including symlink aliases)
are rejected. Embedded JSON is HTML-escaped; labels and metadata use text nodes.

The self-contained browser view has workload/scenario selection, independent
ratio intervals with a 1x reference, visible incomparable points, per-run
absolute statistics/outcomes/configuration details, raw-data links and reproduction
guidance. It does not interpolate gaps or fabricate baseline self-comparison
uncertainty. No remote scripts, styles, fonts or assets are required.

`reports/history-html-v1` was generated from two recorded timing runs plus a
memory run deliberately retained as incomparable. Independent Node hashing
verified every copied/report file against the output checksum manifest.
Playwright checked desktop/mobile rendering, workload selection, expanded
evidence details, three retained rows and exactly one comparable plotted point;
mobile had no horizontal overflow and the browser had no console errors.
Screenshots are `output/playwright/history-{desktop,mobile}.png`.
Root race tests with LLVM integration and vet passed; unit tests cover nested
output protection and script-termination escaping. The broader publication
policy, automatic regression attribution and remaining full-product requirements
are still incomplete.

### Portable source-compiler reports

Added `source-bench-html` as an offline consumer of verified compiler benchmark
bundles. It copies/re-verifies nested raw evidence, preserves the source checksum
manifest hash and seals the report. A shared output-location guard rejects
directories inside source evidence, including aliases. HTML-sensitive embedded
JSON is escaped and metadata is inserted as text, not executable markup.

Timing, OS wait CPU and maximum-step cgroup memory select their own units and
summary fields. Views include confidence intervals, paired-block ratios,
correctness admission, source/tool/flag/oracle identities, outcomes, warmups,
per-build collector evidence, tool logs and replay instructions. Variant,
warmup and failure filters expose the complete schedule. Failed admission
shows unavailable summaries and no measured builds, while retaining partial
receipts and failure logs. Planned blocks are labeled as planned.

Live reports `reports/source-{timing,cpu,memory,failure}-html-v3` cover existing
LLVM timing/CPU/Linux-memory evidence plus an intentional invalid-Clang-option
admission failure in `builds/source-ui-failure-v1`. Browser checks found and fixed
a null trial-list error for failed admissions and mobile metric-name overflow.
Playwright then verified memory units, 14 retained builds, filtering to six
non-warmup builds for one variant, CPU units, failed-admission empty state and
mobile layout. Root race tests with LLVM integrations and vet passed; tests
also cover output protection and script-termination escaping. This delivers
source-aware offline views, not official publication qualification or the
remaining broader ABI, diagnostic and advanced experiment requirements.

### Source compiler Parquet tables

Source HTML reports now include versioned complete-build and tool-step Parquet
tables. Build rows retain configuration/measurement identity, profile, admission
versus trial stage, block, warmup, status, reason and separate nullable wall/CPU/
memory aggregates. Admission aggregates and block indices are null, while raw
step diagnostics remain available. Step rows retain tool/log identity, context
condition, nullable CPU/OOM fields and complete structured collector evidence
as JSON. Their rows are not independent build replications. Exports verify
source benchmarks and reject writes inside their input evidence.

Round-trip tests cover null versus real zero, admission/failure/warmup rows,
nullable OOM, preservation of unavailable-observation metadata, empty schemas
and overwrite rejection. Live reports for Linux LLVM memory, native LLVM CPU
and intentional admission failure generated both tables. DuckDB 1.5.6 read
them: memory contained two admission and 14 trial rows; excluding warmups gave
six measurements per variant with medians exactly matching the JSON report.
The failed admission retained a null aggregate and a separate available CPU
diagnostic for its failed tool. `analysis/compiler-builds.sql` provides a
configuration/profile-separated summary retaining failure counts.

These tables extend the offline data workflow; they do not complete the
remaining collector, ABI, machine-policy and advanced-experiment requirements.
# Within-launch warmup diagnostics

Timing summary analysis now uses `cluster-median-bootstrap-v3` and includes
`post-warmup-thirds-v1` per-trial diagnostics for steady/trajectory scenarios.
Three chronological windows cover every post-warmup observation; median spread
and median absolute deviation use explicit 10% descriptive thresholds, with a
15-observation minimum. The analysis does not claim statistical convergence,
infer tiers, or discard/cherry-pick samples. Drift overrides the between-launch
low-dispersion label in the report's existing stability column. Detailed reasons,
window medians, sample counts and relative diagnostics are in the analysis JSON.
Failed/short/invalid sequences and timer-resolution limits remain explicit.

Tests cover increasing/decreasing/reversing drift, flat and variable sequences,
missing/invalid evidence, and identical launch medians hiding within-launch drift.
This is a diagnostic screen, not adaptive warmup or a formal changepoint model.

Validation: `WASMBENCH_SOURCE_LLVM_TEST=1 go test -race ./...`, `go vet ./...`
and the CLI build passed. Regenerating `runs/trajectory-breakeven-v3` into
`reports/warmup-diagnostics-v1` produced diagnostics for all 36 trajectory trials
across six runtime configurations and two workloads, each retaining five warmup
and 20 measured samples. The dataset contains both quiet and drift-flagged
sequences; these shared-host observations are functional evidence, not official
runtime rankings or causal tier-transition evidence.

# Instance density: contracts, fixtures and wazero execution

The workload wire format now carries an optional density contract with a
simultaneously held instance count and explicit sharing policy. `shared_module`
means one engine and compiled module; `separate_engines` means independently
constructed engines and compiled modules, including their construction cost.
It does not claim that repeated compilation inside one engine bypasses caches.
The initial contract bounds groups to 1–128 fresh instances, one group per
sample, no warmup, and timing or memory passes (barriers only in memory).

The generated `density` suite has 16 contracts: 1/4/16/64 instances, two sharing
policies, and idle or 65,536-byte touched guest memory. Distinct generator
identities keep the four curves separate. The import-free MVP fixture writes
and reads every declared touched byte; a mutable call counter makes accidental
instance reuse fail the expected result. Idle refers to untouched linear
memory, not absence of an oracle invocation or all runtime activity.

The driver rejects density workloads on adapters without `can_density` and
rejects ordinary measured scenarios for density workloads. Wazero compiler and
interpreter now advertise the capability; other adapters remain unsupported.
Sacrificial admission verifies one complete fresh group, not just one instance.
Measured samples provision, initialize and invoke every member, verify every
result outside timing, hold the group at `density_ready`, and close all engines
before `density_released`. Allocation diagnostics cover provisioning; OS/cgroup
barrier windows additionally include verification and diagnostic transport.
No forced GC or actual allocator reclamation is implied by release.

`density.guest_memory.logical` sums accessible memory lengths across the group
with its own scope; it is neither RSS nor runtime overhead. Density-specific
marginal-cost analysis, additional runtime support, pooled instances and
snapshots remain pending.

Validation: `go test -race ./protocol ./corpus ./experiment` and scoped `go vet`
passed. Tests execute every generated fixture, inspect the full guest page,
detect second-call reuse, check wire identity and invalid contracts, and prove
the driver cannot silently substitute ordinary one-instance measurements.

Wazero tests additionally prove group cardinality, distinct simultaneously live
instances, engine-sharing policy, resource closure, ordered barriers and refusal
to report incorrect groups as ready, on both compiler and interpreter. Scoped
race tests and full `go vet ./...` passed. `runs/density-timing-v1` and
`runs/density-memory-v1` each contain 32 successful measured cells, 64 verified
samples, and 32 successful full-group admissions. An independent Node check
verified one group/no warmup despite CLI operations=9/warmup=3, all 192 memory
boundary events, and logical memory exactly count × 65,536 bytes. Both bundles
pass checksum verification. These concurrent shared-host functional runs are
not performance rankings; macOS process/cgroup memory remains explicitly
unsupported/unavailable, while Go allocation and logical-memory data are present.

# Scaling marginal-cost analysis

`scaling-launch-medians-v2` adds adjacent-point finite differences to every
scaling curve in report JSON. For each common independent block, each endpoint
reduces its eligible inner samples to a launch median; the difference is divided
by the increment in input size. The reported point is the median of these paired
differences, with the existing block-level bootstrap interval at three or more
pairs. This is not a difference of separately aggregated endpoint medians.
Negative values remain valid. Missing intermediate points are never bridged;
unmatched blocks, duplicate trial cells, duplicate sizes and numerical overflow
have explicit unavailable statuses. Collector domain, phase and denominator
remain those of the parent curve. Finite differences do not establish linear
scaling and snapshot deltas are not allocation volume.

Scoped race tests and vet passed. Regenerated reports at
`reports/density-memory-v1` and `reports/density-timing-v1` preserve sealed raw
inputs. An independent Node check found eight logical-memory curves and 24
finite differences of exactly 65,536 bytes per added instance, all correctly
without intervals because the source runs have one block. Unavailable macOS
RSS remains null; eight timing curves also retain insufficient-block labels.
Marginal values are currently in `data.json`; dedicated HTML visualization and
broader density support remain pending.

# Density and scaling marginal-cost visualization

The offline HTML report now renders adjacent-size marginal costs below each
full scaling curve. The signed linear axis includes zero; negative values are
not clipped. Intervals appear only when present in the analysis; missing points
produce no marker or connecting line. A table retains every interval, paired
block count and availability status. The selector now includes generator
identity, distinguishing density sharing and touch policies. This supersedes
the JSON-only limitation above; broader runtime/pooling/snapshot work remains.

`reports/density-memory-html-v2` was regenerated from the unchanged sealed input.
Playwright checked the real logical-memory curve (three 65,536-byte marginal
points, one paired block each, no confidence intervals), plus a temporary
in-browser synthetic signed/missing/interval case, restoring source data after
the check. Desktop and 390×844 mobile screenshots were inspected; the mobile
document remains 390px wide and the evidence table scrolls locally. Screenshots
are `output/playwright/density-marginal-desktop.png` and
`output/playwright/density-marginal-mobile.png`. No browser console errors were
reported. These visual checks used a local HTTP server, not direct file URLs.

# Wago density execution and cross-adapter conformance

Wago now supports both density policies using explicit fresh `Runtime` owners
and runtime-aware compile/instantiate APIs. Shared groups keep one runtime and
compiled module; separate-engine groups compile once in each fresh runtime.
All instances remain live through verification and the ready barrier. Release
waits on `Runtime.CloseContext` before closing modules; plain `Runtime.Close`
alone would not establish completed teardown. There is no forced collection or
claim of allocator reclamation. Exact API choices are in `density_policy` in
the locked runtime configuration.

Nested-module race tests cover all 16 workloads, ownership counts, distinct live
instances, completed runtime closure, logical-memory sums, phase order, and
wrong-oracle refusal. Built-adapter race conformance passes across Wago and both
wazero modes for every workload in timing and memory profiles. CI now selects
these conformance tests in the Wago job and the Go-platform matrix. Actionlint
and root vet passed; hosted CI has not been run for these local changes.

`runs/density-wago-timing-v1` and `runs/density-wago-memory-v1` each contain 16
successful measured cells (32 samples) and 16 successful full-group admissions.
Both bundles passed checksum verification; independent Node checks confirmed
the budgets, phase sequence and logical memory equal to count × 65,536 bytes.
These are functional shared-host checks, not performance-ranking evidence.
The sibling Wago checkout was read-only and its existing changes preserved.

# Wasmtime density execution

Cranelift and Winch now advertise density and its memory barriers. A group owns
one Store per instance, retaining all Stores through verification and the ready
snapshot. Shared mode owns one Engine/Module; separate mode constructs and
compiles once per fresh Engine. Rust ownership drops Stores before Modules and
Engines on both success and errors. The timer includes provisioning, input and
workload calls, excluding verification/release. Logical group memory uses its
dedicated scope and denominator. No allocator purge or reclamation is claimed.

Rust tests verify both backend policies, distinct live guest-memory addresses,
touched-page contents, reuse-sensitive oracles and invalid-contract rejection.
All eight adapter tests and four independent-analyzer tests pass. Built-adapter
race conformance passes for both modes across all 16 contracts and both profiles;
the Wasmtime CI job now selects it. Actionlint passes; hosted CI is unverified.

`runs/density-wasmtime-timing-v1` and `runs/density-wasmtime-memory-v1` each contain
32 successful measured cells (64 samples) and 32 full-group admissions. Both
bundles pass checksum verification. Independent Node checks verified sample
counts, group semantics, all memory phase boundaries, and logical memory exactly
count × 65,536 with an instance-group denominator. These are shared-host
functional checks, not official comparative performance evidence. V8 density,
pooled instances and restored snapshots remain unimplemented.

# V8 shared-module density

The Node adapter now supports simultaneous shared-module groups. Each sample
constructs one JS Module wrapper and distinct fresh Instances, holds every
instance through verification and the ready barrier, then drops the references
before the release barrier. The process V8 engine is reused, code caching remains
uncontrolled, and no engine disposal or forced GC is claimed. The locked
`density_policy` explicitly excludes engine construction from V8's timer.
Memory diagnostics include JS heap boundary snapshots (not allocation volume)
and group logical memory. Separate engines are not exposed by this embedding.

An explicit false `can_density_separate_engines` capability restricts the general
density capability. The driver preserves this mode as unsupported during both
admission and measurement. Direct adapter calls reject it before any phase
events. Existing descriptions without a mode restriction retain their original
density contract. This is partial V8 support, not proof of separately compiled
native code or isolated engines.

Cross-adapter race conformance passes for all six configurations, including V8's
unsupported-mode behavior. The test harness now queries descriptions directly
instead of assuming ResolveRuntimes populates them. Node float regression tests,
syntax validation and Actionlint passed. The Go-platform CI matrix now selects
V8 density conformance; hosted CI remains unverified.

`runs/density-v8-timing-v1` and `runs/density-v8-memory-v1` each preserve eight
successful shared-module cells and eight unsupported separate-engine cells.
Both pass checksum verification; independent Node checks confirm sample budgets,
memory boundaries, exact count × 65,536 logical bytes, and absent samples for
unsupported cells. These are functional shared-host runs, not comparative
performance evidence. Pooling, restored snapshots and remaining product
requirements are still open.

# Density boundary-change analysis and longer-cycle validation

`analysis/density_footprint.go` adds versioned within-cycle footprint analysis
for density and retained-engine density-cycle runs. It joins before/ready/released
snapshots only within one launch and identical metric/collector/scope/quality
identity. Signed provisioning, release and whole-cycle differences stay separate;
missing, ambiguous, unverified or failed evidence cannot become numeric deltas.
No allocation volume, reclamation guarantee, leak verdict or time rate is inferred.
JSON exports retain exact phase names; the memory-timeline view exposes the
corresponding window's comparison table alongside raw evidence and provenance.

Unit tests cover signed changes, genuine zero, increasing footprint during
release, collector/scope/quality isolation, missing boundaries, permission errors,
duplicate observations, unverified samples, invalid sequence, failed trials,
single-cycle evidence and independent-launch isolation. Full root Go tests and
vet passed; analysis/publish race tests passed. Browser checks of the Linux report
passed at desktop and 390×844 mobile sizes, with zero console errors/warnings.
The table scrolls locally on mobile; screenshots are
`output/playwright/density-footprint-{desktop,mobile}.png`. Testing used HTTP,
not direct file URLs.

`runs/density-cycle-sustained-v1` contains 32 successful measured cells across
Wasmtime on-demand/pooling and all 16 density workloads, with 100 verified cycles
per cell (3,200 total), plus sacrificial admissions. Bundle checksums passed;
an independent Node check confirmed sample budgets and correctness flags.
This macOS run has unavailable process-residency collectors, retained honestly
in `reports/density-cycle-sustained-v1`; it is a longer sequence functional check,
not duration-qualified sustained execution or leak qualification.

`reports/density-footprint-linux-v1` reanalyzes the unchanged sealed Linux
`runs/container-density-v1/density` bundle. Independent checks matched all 960
available per-cycle/domain differences to raw snapshots and verified all 1,440
unavailable comparisons remained null. This report has real Linux residency
evidence, but does not establish long-duration Linux reclamation behavior.
Snapshots, PMU/profiling and other product requirements remain open.

# Linux perf collector foundation

Added single-use, cgroup-targeted per-CPU perf event collection for all seven
initial hardware/software events. Raw integer counts and enabled/running times,
numeric event definitions, privilege scope and per-event availability are
preserved without scaling multiplexed values. Lifecycle/error/byte-order tests
pass under the race detector; Linux ARM64 and AMD64 cross-compilation passes.
The Linux ARM64 kernel smoke test in a read-only, capability-dropped container
reports permission_denied for all seven events, with no invented numeric values.
Full Go tests and vet pass. This does not qualify successful PMU collection.
Agent/CLI integration remains intentionally gated pending barrier and CPU
coverage integration. See `docs/PERF.md` for the contract and remaining work.

# Perf CPU coverage guard

The collector now checks exact coverage of the cgroup's online effective cpuset
before opening events, then checks effective/online CPU-set stability at Start
and Finish. Effective cpuset access is relative to the open cgroup descriptor.
Invalid/missing lists fail closed. Start failures close all event handles without
enabling counters; Finish failures preserve raw evidence but mark it
coverage_changed, retaining prior per-event statuses in the reason. This does
not detect topology changes that revert between boundary reads; controlled-host
policy still must prohibit concurrent reconfiguration.

Tests cover bounded cpulist parsing, overlapping/duplicate/reversed ranges,
empty/disjoint sets, partial/extra CPU selections and cleanup/invalidation at both
boundaries. Collector race tests, full Go tests/vet, Linux ARM64/AMD64 builds and
restricted Linux ARM64 kernel tests pass. The kernel smoke test now selects the
entire online effective cpuset rather than CPU 0. Positive PMU qualification and
agent barrier/CLI integration remain incomplete.

# Agent counters phase API

Added opt-in `Client.CallCounterPhases` with successful counters-preparation
tracking, strict lifecycle/sample barrier ordering, fresh per-sample perf windows,
and guaranteed active-window cleanup on errors/EOF. The Linux bridge discovers
the complete online effective cpuset from the already-owned cgroup descriptor;
unisolated/non-Linux clients retain unavailable evidence. Raw per-CPU records
remain separate from timing/memory observations. Final sample identity and
verification are required. Ordinary client calls are uninstrumented.

Handshake tests cover compile, instantiate, teardown, app-init, density and
density-cycle; failure tests cover unavailable opens, start/finish failures, EOF,
partial and denied events, malformed sequences, preparation-profile isolation
and unverified results. Agent/collector race tests, full root tests/vet and Linux
ARM64/AMD64 cross-compilation pass; Linux ARM64 container handshake tests pass.
These are protocol tests, not successful PMU counting. Adapter counters-only
support, sealed trial integration and publication remain unfinished; the CLI
counters gate remains in place.

# Wazero counters-only handshakes

Wazero compiler/interpreter now advertise compile and instantiate counter-phase
capabilities. The shared protocol contract explicitly requires core scalar exact
oracles, supported reset policies, barriers, one operation, zero warmup and
bounded sample counts. Invalid requests are rejected before barriers. Existing
memory lifecycle code is reused, but ReadMemStats/GoMemoryWindow calls and their
observations are skipped in counters mode. Verification and resource release
remain outside the measured counter window.

Built-adapter race conformance passed on both configurations, including lifecycle
initialization/input, incorrect oracles and invalid budgets. Tests assert no
memory observations and explicit unavailable counter records for unisolated
clients. The Go-platform CI job now selects this conformance test; hosted CI is
not verified. The runner remains gated until sealed trial/report integration and
positive hardware qualification are completed; these handshakes alone are not
a functioning end-user counters profile.

# Experimental counters runner and sealed evidence

The runner now accepts explicit counters passes with phase barriers, one operation
and no warmup, using advertised per-scenario capabilities and the shared core
scalar contract. Sacrificial admission uses an uninstrumented timing preparation
in a separate process. Measured calls use the agent counter API; their complete
sample-indexed raw counter phases and collector version are sealed in trial JSON
and retained by report JSON/raw downloads. Unsupported configurations are visible.
CLI progress reports counter-window availability separately from workload success.
Analysis version 4 excludes counters/profiling elapsed timers from latency
estimates; raw timers are still diagnostic evidence.

`runs/counters-wazero-v1` exercises two core workloads, compile/instantiate, both
wazero configurations and unsupported V8: 8 successful measured cells with 16
explicitly unavailable windows, 4 unsupported cells and 6 successful sacrificial
admissions. Checksums and independent JSON checks passed. This unisolated macOS
run does not establish positive hardware counting. The report is
`reports/counters-wazero-v1`. Full Go tests/vet and experiment/analysis/agent race
tests pass, including sealed evidence and null counter-pass latency assertions.
This is an experimental diagnostic path, not hardware-qualified publication.
Dedicated counter Parquet/UI, doctor probes and successful Linux PMU validation
remain open, alongside the other unfinished product requirements.

# Raw counter Parquet export

Reports now generate `counters.parquet` with nullable unsigned 64-bit count,
config and time columns; no float conversion, scaling or cross-CPU aggregation.
Each reading retains trial/window/event status, source indices, sample verification
and collector/privilege provenance. Windows without events and trials without
windows get explicit outcome rows, preserving failed, unsupported and admission
coverage. Ambiguous/missing sample verification remains null. Raw values on
invalidated readings remain diagnostic evidence rather than silently disappearing.

Publisher race tests cover UINT64_MAX, zero, null, multiplexed and coverage-invalid
readings, failed trial context, placeholder outcomes, empty typed tables and
overwrite refusal. Full Go tests and vet pass. `reports/counters-parquet-v1`
reexports the unchanged sealed `runs/counters-wazero-v1` bundle. DuckDB 1.4.1
independently reads the integer fields as UBIGINT and reports 16 unavailable
windows, 4 unsupported trial outcomes and 6 admission outcomes, with zero non-null
raw counts. `analysis/counters.sql` provides coverage and ordered raw-reading
queries without discarding unavailable cases or summing partial CPU coverage.
Counter UI, doctor probes and positive PMU qualification remain open.

# Doctor disabled-perf probe

`doctor` now returns structured `perf_cgroup_probe` evidence. Default calls do
not open perf descriptors; Linux reports not_requested and other platforms
report unsupported. `--perf-cgroup` explicitly selects an existing absolute
cgroup-v2 target and probes disabled opens across its online effective cpuset.
Per-event/CPU statuses distinguish open success, denied/unsupported events and
partial support; cleanup failures remain visible. No events are enabled and no
counts, cgroups, task movement or host-permission changes occur. Kernel paranoid
policy is context only, never a permission verdict or hardware qualification.

Native macOS CLI output verified unsupported/null values. The Linux ARM64 CLI
and kernel tests in a read-only capability-dropped container recorded 84 denied
event/CPU opens (seven events on 12 CPUs), all count/time fields null. Race tests
verify no enable/disable calls, closure, partial/unavailable outcomes and cleanup
errors. Full Go tests/vet and Linux ARM64/AMD64 builds pass. Positive PMU counting
and counter visualization remain unqualified/unfinished respectively.

# Offline counter evidence view

Reports now provide a trial-selectable counter table with explicit admission,
unsupported and failed outcomes, per-sample/CPU event readings, verification,
definitions and provenance. Every row remains accessible in 500-row windows.
Raw JSON and Parquet retain unsigned integers; a separately versioned display
dataset uses decimal strings for count/config/enabled/running fields to avoid
JavaScript precision loss. Both exports share the same row enumeration.

Publisher race tests cover UINT64_MAX, zero, null and invalidated statuses, plus
raw-evidence immutability. The regenerated `reports/counters-html-v1` uses the
unchanged sealed `runs/counters-wazero-v1` bundle. Browser checks verify mobile
390-pixel containment, synthetic 501-row pagination, exact maximum integers,
permission-denied visibility and zero console errors. Screenshots are retained
under `output/playwright/counters-{desktop,mobile}.png`. Positive hardware
counting, broader counter adapters and profiling remain unfinished.

# Wago counter phase integration

Wago now advertises compile/instantiate counter support for the shared core
scalar exact-result contract. Validation occurs before scenario dispatch, so
unsupported contracts, warmup, batches and missing barriers fail before entering
a phase. Existing memory barriers are reused, but Go allocator reads/observations
are disabled for counters. Verification and release follow the measured-end
barrier. Timing and memory preparation behavior is unchanged.

The rebuilt Wago adapter passed real-subprocess counter conformance alongside
both wazero configurations, including wrong-oracle and invalid-request cases.
Wago compile-memory barrier regression tests also pass under the race-enabled
driver. The live `runs/counters-wago-v1` bundle has four successful measured
trials and eight explicitly unavailable counter windows on macOS, with no claim
of positive PMU qualification. Its checksum seal verifies and its offline report
is in `reports/counters-wago-v1`. Root Go tests and vet pass. The pinned Wago CI
job now enables counter conformance; hosted execution is not yet verified.

# Wasmtime counter phase integration

Wasmtime Cranelift, Winch and pooling configurations now advertise the same
core scalar compile/instantiate counter contract. A Rust validator rejects
unsupported workload contracts and invalid budgets before dispatch or barriers.
Its tests cover both scenarios/reset policies, missing/negative/out-of-range
budgets and unsupported ABI/oracle/vector/command/density contracts. Compile
verification and instance initialization/input/verification remain outside the
counter window; instantiate guest-memory snapshots occur only in memory passes.

All 11 Rust adapter tests and four analyzer tests pass, and the release adapter
rebuilt. Race-enabled subprocess conformance and compile/instantiate memory
regressions pass for all three configurations. The live
`runs/counters-wasmtime-v1` records 12 successful measured trials and 24 explicit
unavailable counter windows on macOS. This verifies protocol integration, not
positive perf counting. Root Go tests/vet and workflow lint pass. CI enables the
three-configuration conformance test; hosted execution remains unverified.

# V8 counter phase integration

V8 now advertises the core scalar compile/instantiate counter handshake.
Counter validation precedes all scenario dispatch. MemoryUsage and logical-memory
snapshots are disabled in this profile; initialization, verification and reference
release follow the measured-end barrier. The configuration explicitly records
that background compiler/adapter work shares the cgroup window, API return does
not prove final-tier completion, and cache/GC remain uncontrolled.

Expanded subprocess counter conformance passes for all seven configurations,
including negative warmup, zero and oversized sample counts. V8 memory compile
and instantiate regressions pass with the race-enabled driver; Node syntax and
float tests pass. `runs/counters-v8-v1` contains four successful measured trials
with eight explicit unavailable windows on macOS, not qualified hardware counts.
The cross-platform Go CI counter step now includes V8.

# First-invocation counter boundaries

The shared protocol/agent now recognize an explicit first-call three-barrier
sequence, with collection ending at call return and release afterward. Wazero
compiler/interpreter implement it using fresh initialized instances, retained
engine/module, no workload pre-call and no memory snapshots. Result copying,
verification and release occur after collection. Failed calls still signal the
end boundary; initialization failures never open a counter window.

Real subprocess tests use the lifecycle fixture whose mutable global traps on a
second invocation. This verifies fresh instances and absence of hidden pre-calls,
as well as required initialization/input, wrong-oracle rejection and invalid
requests. First-call and existing compile/instantiate conformance, agent lifecycle
tests and protocol tests pass under the race-enabled driver. CI enables the new
test for both wazero configurations. Steady-batch counters remain unfinished.

# Wago first-invocation counters

Wago now implements the first-call counter sequence with the compiled module
retained and a fresh initialized instance per sample. Module/instance setup,
input installation and export resolution precede collection. Result copying,
oracle checks and instance release follow call return and the end barrier.
The measured export is called exactly once, including when it traps; failed
initialization emits no counter events. No allocator snapshots or forced GC
are introduced. The configuration and capability describe this scope explicitly.

The rebuilt adapter passes first-call subprocess conformance, including the
single-use lifecycle fixture, wrong oracle, call/init traps and invalid budgets.
Existing compile/instantiate counter and memory-phase regressions also pass
under the race-enabled driver. The live `runs/counters-wago-first-v1` contains
one successful measured lifecycle trial and three unavailable counter windows
on macOS, not positive PMU qualification. Pinned Wago CI enables the new test.

# First-call counters across all current configurations

Wasmtime Cranelift/Winch/pooling and V8 now implement the same first-call counter
sequence. Wasmtime prepares fresh stores/instances and buffers before collection,
retaining engine/module. V8 resolves exports/arguments before collection and
normalizes results afterward; instance/function references are dropped after
verification with no forced GC. Both end collection on call return even when
the invocation traps. Tier/cache and release policies remain explicit rather
than claiming guest-only counts or final-tier/reclamation completion.

All seven configurations pass the shared first-call subprocess conformance
suite. The four newly enabled configurations also pass compile/instantiate
counter and memory-phase regressions under the race-enabled driver. All Rust
adapter/analyzer tests, Node tests, root Go tests/vet and workflow lint pass.
`runs/counters-first-breadth-v1` exercises the four configurations on the
single-use lifecycle fixture: four successful measured trials and 12 explicitly
unavailable macOS counter windows. Positive hardware qualification remains open.
CI enables first-call conformance for each runtime family; hosted results have
not been observed.

# Steady-batch counter protocol and wazero integration

Counter budgets now distinguish single-operation lifecycle windows from steady
batches. Steady supports bounded explicit warmup and operation counts, with
strict ordered boundaries and exact sample/warmup/operation identity checks.
Wazero compiler/interpreter retain a fresh initialized instance across requested
warmup and measured batches, with no hidden pre-call. Every returned result is
verified outside collection; an incorrect middle result or warmup aborts the
request. Stateless contracts only; per-sample reset is explicitly unsupported.
Embedding Call allocations remain in the window; result containers are prepared
before it. Counter timers remain excluded from latency summaries.

Adversarial subprocess fixtures prove five-call limits, sixth-call traps, middle
result checking, warmup checking, request reset and no per-batch instance reset.
Protocol/agent and publisher race tests pass, as do full Go tests/vet and workflow
lint. CI enables wazero steady conformance. Counter Parquet v2 and the report add
nullable sample warmup/operation context, never inferred for ambiguous or missing
samples; SQL keeps these distinctions and does not normalize raw batch counts.

The sealed `runs/counters-steady-v1` verifies and produces
`reports/counters-steady-v1`: four successful measured wazero trials, eight warmup
and twelve measured counter windows, all with five operations and explicit macOS
unavailability. Two V8 steady trials remain unsupported. Positive hardware
counting and other steady-counter adapters remain unfinished.

# Wago steady counters and result ownership correction

Wago implements stateless steady counter batches with explicit retained warmup,
fresh request-level module/instance state, no hidden pre-call and verification
after collection. Adversarial tests pass for call budgets, instance retention,
middle-result failure, warmup failure and repeated requests. Counter collection
still includes barrier transport and embedding work, not guest-only attribution.

Inspection of Wago's Invoke contract exposed an existing timing correctness bug:
its result slice is instance-owned and overwritten by the next call. The old
batch loop retained aliases. The new timing-middle-wrong regression reproduced
an incorrectly successful five-operation sample before the fix. Both counter
and ordinary execution paths now copy each result into preallocated storage
before another invocation. Capture stays inside the measurement and is declared
in configuration; oracle checking stays outside. Old sealed results are not
rewritten and should not be used as proof of per-invocation verification.

The rebuilt adapter passes steady conformance alongside both wazero variants,
Wago first-call/compile/instantiate counter conformance and scalar/float trajectory
regressions under the race-enabled driver. `runs/counters-wago-steady-v1` records
two successful measured trials and ten unavailable macOS counter windows,
including four marked warmup windows. Wago CI enables the new conformance test;
positive PMU qualification remains unfinished.

# Steady counters across all current configurations

Wasmtime Cranelift/Winch/pooling and V8 now implement stateless steady-batch
counter collection. Wasmtime allocates distinct result buffers before each
window, retains one store across all explicit warmup/measured batches and
verifies every result afterward. V8 prepares arguments and a result container
before the window, stores raw return values, and normalizes/verifies afterward.
Both end collection on returned traps. V8 engine/cache/GC and background compiler
behavior remain explicit, with no final-tier or guest-only attribution claim.

All seven configurations pass the shared adversarial steady suite, including
incorrect intermediate timing results, warmup failures, exact call budgets,
instance retention and repeated requests. Compile/instantiate/first-call counter
regressions pass for the four updated configurations. All Rust/Node tests and
root Go tests/vet/workflow lint pass. CI enables steady conformance in each
runtime family; hosted execution remains unverified.

`runs/counters-steady-breadth-v1` has eight successful measured trials, 16 retained
warmup and 24 measured windows with five operations each. All macOS windows are
explicitly unavailable, not zero or positively qualified PMU counts. Steady
support now spans the current adapter set; profiling, hardware qualification and
other ABI/oracle counter contracts remain unfinished.

# Initial Go CPU profiling path

Wazero-interpreter now supports an explicit sampled Go-process CPU profiling
pass for stateless core scalar steady workloads. Collection surrounds the whole
run request, not guest-only calls, and retains gzip pprof bytes with module,
collector/toolchain, scope/window and digest metadata. Native-code profiling is
unadvertised and refused. Admission uses a separate unprofiled process. Failed
workloads can retain diagnostic artifacts; profiling timers remain excluded
from latency summaries. Output is bounded to 16 MiB with no partial overflow
artifact; a failed Start does not stop another profiler's session.

The runner and loader validate transport metadata/digest/bounded gzip integrity;
protobuf interpretation remains with pprof tooling. Reports provide exact-byte
content-addressed profile downloads and explicit unavailable/unsupported/failed
outcomes. Raw sealed trials remain the source of evidence. The documented local
pprof viewer provides interactive stack/flamegraph analysis; an embedded offline
flamegraph and native/perf/JIT attribution remain unfinished.

Profiler lifecycle and format tests, adapter conformance, resealed-evidence
validation and publisher tests pass under race instrumentation. Full Go tests,
vet and workflow lint pass. `runs/go-cpu-profile-v1` verifies and generates
`reports/go-cpu-profile-v1`, with two collected interpreter profiles and four
unsupported native/V8 trials. Independent `go tool pprof -top` parses the sum
artifact into interpreter/runtime stacks (dominated by runtime scheduling),
while every profiling latency summary/interval is null. CI enables interpreter
profiling conformance; hosted execution remains unverified.

# Offline sampled CPU trees and flamegraphs

Reports now decode retained pprof with pinned google/pprof
`v0.0.0-20251114195745-4902fdda35c8` and produce per-trial caller/callee trees.
The versioned analysis preserves inline frames, recursion, unresolved locations
and all label populations. Inclusive/self weights stay exact uint64 decimal
strings, with overflow and unsupported unit/type checks. Bounded depth, nodes,
expanded frames and label sizes produce explicit unavailable analysis rather
than partial output. Failed profiles remain diagnostic. Raw bytes are unchanged.

The offline flamegraph supports keyboard/click zoom and reset, declares its
2,000-frame render cap, and provides a paged exact-value table for every analyzed
frame. Raw profile/trial links and decoder identity accompany each graph. Tests
cover integer precision beyond 2^53, conservation, inline/recursive ordering,
unknown/empty stacks, negative weights, overflow, ambiguous units and budgets.

`reports/go-cpu-stacks-v2` reanalyzes the unchanged sealed interpreter run.
The sum tree's 15,750,000,000 ns exactly matches independent pprof output.
Browser checks verify zoom/reset, mobile containment, escaped labels, UINT64_MAX
display, 2,002-node pagination and render-cap disclosure; console errors are zero.
Root tests/vet and analysis/publisher race tests pass. Native/perf collection and
guest/JIT symbol attribution remain unfinished.

## V8 native CPU profile collection

The V8 adapter now advertises stateless scalar steady profiling through a local
Node inspector session (no listening port), retaining native `.cpuprofile` JSON.
Its scope is sampled isolate stacks across the complete run request, including
setup/warmup/verification, not all-process CPU or guest-only/tier attribution.
The helper is hashed in runtime locks and included in Docker packaging.

Protocol validation checks native envelope integrity, digest, size and timestamp
ordering. Reports export byte-identical content-addressed downloads, including
incorrect-result diagnostic profiles. Pprof analysis explicitly refuses V8 time
deltas; unsupported trees show unavailable analysis, not zero sample counts.
V8-specific offline stack visualization remains a separate unfinished subsystem.

Node tests verify restarts, raw JSON/checksums, retained workload-failure evidence
and pre-execution contract rejection. Built-adapter conformance covers correct
results, wrong oracles, barrier refusal and absence of memory instrumentation.
Go protocol/publisher tests cover malformed envelopes, scopes and native exports.
Root tests/vet, targeted race tests, Node tests and workflow lint passed locally.

`runs/v8-cpu-profile-v1` contains two successful collected profiles; bundle
checksums pass. Identity has 39 samples, sum 800, with two Wasm frame records in
sum; this is observation evidence, not symbol-completeness qualification.
`reports/v8-cpu-profile-v2` exports exact native bytes and keeps latency summaries
null. Browser verification checks native downloads, unsupported-analysis wording
and mobile containment. No dedicated-host or hosted-CI qualification is claimed.

## Offline V8 isolate sample trees

`v8-isolate-sample-tree-v1` now derives flamegraphs directly from native profile
sample IDs. Inclusive/self weights are exact recorded sample counts, never time
deltas, inferred CPU nanoseconds, hit counts or invocation counts. Node identity,
recursive occurrences, idle/GC frames and unresolved names remain distinct.
Unknown references, duplicate edges/IDs, multiple parents, cycles/disconnected
trees, invalid coordinates and explicit size/depth/label limits yield no partial
analysis. Missing samples and explicit empty samples have different outcomes.

`reports/v8-cpu-stacks-v2` derives 39 and 800 samples from the unchanged sealed
V8 run, matching independent native JSON counts exactly. Go's original CPU-time
fields remain unchanged and V8 uses separately unit-labeled weight fields.
The shared chart/table shows the active unit, supports zoom/reset and keeps raw
profile/trial links. Browser checks on the same report UI verified V8 zoom/reset,
16 sum frames, sample-count headers, a 390-pixel mobile viewport without page
overflow and zero console errors. The desktop screenshot was visually inspected.
Go-report regression checks preserve sampled CPU nanoseconds and rendered frames.

Tests cover conservation, recursive identities, failed diagnostic preservation,
unmodified raw bytes, malicious symbol text, invalid graphs, missing/empty data,
sample/depth/label budgets and missing source coordinates. Full tests/vet and
analysis/publisher race checks pass. Native/perf stack collection and complete
guest/tier attribution remain unfinished; this does not qualify official results.

## Pilot-selected fixed confirmation budgets

`pilot-plan` derives a versioned budget from a sealed timing pilot. It preserves
every declared cell and process-level bootstrap summary. A clearly labeled
square-root CI-width planning heuristic selects the maximum common budget;
missing/failed/unsupported cells, drift, invalid samples or cap overflow prevent
a ready plan. The heuristic is not a precision guarantee, paired-effect power
analysis or official-publication certification.

`pilot-run` strictly parses and recomputes the plan from sealed evidence, checks
the basic host/environment identity, and runs fresh processes with the fixed
budget. It embeds the entire plan and source digests in the confirmation lock,
indexes the new bundle and rejects launch overrides or confirmation-as-pilot
reuse. Lock validation and offline loading reject mismatched embedded budgets.
Runner/runtime/artifact pins and new-only output protections remain enforced.

Tests cover deterministic selection, common maximum, immutable pilot data,
missing/failed/unsupported cells, duplicate blocks, zero medians, invalid samples,
insufficient launches, cap overflow, edited source/decision data, trailing JSON,
new-only plans, failed admission, confirmation recycling and resealed budget
changes. Full tests/vet and analysis/experiment/CLI race checks pass locally.

Functional evidence: `runs/pilot-budget-v2` has twelve successful V8 compile
pilot trials; a deliberately loose 50% planning target chooses six launches per
cell. `runs/pilot-confirmation-v2` has twelve new successful measured trials,
the embedded source decision, a local index entry and verified checksums.
`reports/pilot-confirmation-v2` remains explicitly exploratory. The loose target
exercises the workflow, not a publishable precision policy. Dedicated-host
controls, resource-policy certification and official publication remain open.

## Fail-closed requested resource readback

`requested-cgroup-leaf-readback-v1` now compares explicitly requested controls
with kernel readback before spawn and before cleanup. Effective CPU sets must
match the requested set semantically; ancestor narrowing, fallback expansion,
unreadable fields and numeric drift fail closed. CPU lists now use the bounded
parser rather than syntax-only validation. Preflight failures clean up only the
new leaf and never launch unisolated. Endpoint failures retain diagnostic data
but cannot remain successful trials/tool steps.

Isolation records retain independent initial/final verification with boundary
and scope. Adapter and source-tool end boundaries remain distinct. Unrequested
controls, ancestor budgets, exclusive placement, frequency and continuous
enforcement are not certified. Numeric readback must match exactly, including
page-aligned memory limits; no kernel rounding is silently hidden.

Full Go tests/vet and agent/experiment/source-build race tests pass. New Linux
filesystem-reader and mismatch tests pass in the cached ARM64 package image with
all capabilities dropped, no network, a read-only root and no host cgroup writes.
AMD64 Linux compilation is checked separately. Delegated kernel enforcement
tests now assert both verification records but require an explicitly provisioned
environment; this turn does not establish their real-kernel qualification.

## Explicit NUMA-node allocation policy

`--mems` now selects allowed NUMA nodes in runtime plans/runs/checks and source
build benchmarks. The bounded node-list request is locked alongside CPU and
memory budgets, requires delegated cgroups and is written only into the fresh
worker leaf before process creation. Requested/effective node sets must match
at both boundaries under `requested-cgroup-leaf-readback-v2`; omission inherits
parent allowances. No physical-residency, first-touch, interleaving, exclusivity
or dedicated-machine guarantee is inferred from an allowed-node mask.

New successful NUMA-requested trials and source receipts require complete,
consistent initial/final verification. Offline loading independently checks the
raw values and rejects missing endpoints, forged success labels, wrong stages
and policy mismatches. Old receipts without NUMA requests are not upgraded.
The CLI preserves node policy in copied locks and rejects `--mems` overrides.

Unit/CLI/storage tests cover list validation, normalized equivalence, narrowing,
expansion, absent readback, forged values, receipt boundaries and lock identity.
Full tests/vet and agent/experiment/source-build/CLI race checks pass. Linux ARM64
filesystem-reader tests pass in a no-network, capability-dropped container;
Linux AMD64 tests cross-compile. The opt-in real-cgroup test checks the child's
`Mems_allowed_list` at spawn, but was skipped without delegated infrastructure.
Actual NUMA enforcement/placement and official-host qualification remain open.

### Observed host baselines for runtime and source benchmarks

The `host-policy` command captures versioned, new-only JSON baseline files.
Runtime locks and source benchmark configurations embed them. Both workflows
refuse a mismatching start observation before output-directory creation, retain
an end observation before sealing, and preserve raw execution outcomes when an
end mismatch prohibits performance analysis. Offline validation re-derives
checks; missing or forged endpoints are invalid evidence. Timing, CPU, memory,
scaling and aggregate derived views exclude affected measurements. Replay keeps
the pinned baseline. Matching uncontrolled or unavailable facts is not machine
qualification; no continuous monitoring or dedicated-host guarantee is claimed.

Verified locally: full Go tests/vet and focused race tests; LLVM source integration
with baseline, replay, environment refusal, resealed end-mismatch diagnostics,
and missing-endpoint rejection. CLI evidence is in
`builds/source-host-baseline-v1` and `builds/source-host-baseline-replay-v1`, each
with three measured builds and one retained warmup. A changed `GOGC` was refused
without creating `builds/source-host-refused-v1`. End mismatch tests use synthetic
boundary evidence, not a claim of live machine-policy drift. Source-build
standalone receipts and dedicated-host qualification remain separate work.

### Publication evidence gate

`publication-check` emits `publication-evidence-audit-v1` with eight separate
requirements, writes new-only JSON, and returns nonzero for unmet/unsupported
requirements. `publish` uses the same gate before report-directory creation.
An `official` manifest label no longer bypasses absent evidence. Pilot decisions
are fully recomputed against sealed source data; confirmation configuration,
host, launch budget and separate later run identity are checked. A relocated
pilot can be supplied without changing its pinned checksum identity.

Tests cover changed pilot cells/digests, runtime/workload/resource/host/budget
drift, unknown plan fields, reused run identity, absent/duplicate/incorrect
sacrificial checks, unsupported-cell retention, raw-evidence immutability,
relocated pilot lookup and new-only audit output. Full Go tests/vet and focused
analysis/CLI race tests passed. `reports/publication-audit-v2.json` records the
live audit of `runs/pilot-confirmation-v2`: pilot, artifact admission, sacrificial
correctness, timing scope and complete stable measurements passed; host baseline
and resource boundaries were unmet. Dedicated-machine qualification remains
unsupported and blocks official publication. The refused publish created no
`reports/official-refused-v1` directory. No official result is claimed.

### Empty isolated CPU-partition readiness

`doctor --cpu-partition PATH --measurement-cpus LIST` now exposes a bounded,
read-only Linux cgroup v2 probe. It checks valid isolated-root state, exact
effective/exclusive CPU masks, empty subtree population, online CPU stability,
online SMT sibling coverage and every controller thread's allowed CPU mask at
two observation boundaries. Thread-set changes invalidate readiness. Raw facts
retain missing/permission/read-budget failures, and readiness is independently
recomputed rather than accepted from a saved label. The live path verifies the
cgroup v2 filesystem. No cgroup, affinity, privilege or host setting is changed.

Full Go tests/vet and agent/CLI race tests pass. Linux ARM64 tests passed in the
cached no-network, read-only, capability-dropped container; Linux AMD64 tests
cross-compiled. Injected tests cover valid readiness, invalid roots, narrowed or
widened CPU masks, population, hotplug, online SMT leakage, controller/collector
overlap, thread churn, missing and oversized facts, and ordinary-filesystem
rejection. macOS CLI reports unsupported. The opt-in live-isolated-partition test
skips without an explicitly supplied partition and CPU list. No positive real
partition qualification was observed. Locked-run integration, continuous
monitoring and dedicated-machine qualification remain unfinished.

### Locked CPU-partition boundary requirements

Runtime and source benchmark locks now support
`require_isolated_cpu_partition`, exposed as
`--require-isolated-cpu-partition`. The requested parent and complete CPU mask
are bound to the resource policy. Runs refuse unready or unsupported starting
observations before output creation, then retain an ending observation after
worker cleanup. An ending failure seals diagnostic evidence and excludes all
measurements without rewriting raw correctness outcomes. Offline loading
recomputes readiness and rejects missing endpoints, changed policy and forged
success labels; unavailable end-filesystem receipts remain failure evidence.
Copied locks/reproduction/replay preserve the requirement without overrides.
The publication audit is now v2 and separately exposes this ninth requirement.

Tests cover locked policy preservation, override refusal, valid synthetic
boundaries, ending failures, missing/forged observations, failed start admission,
source eligibility and raw-evidence preservation. The LLVM integration test
passes, including partition refusal before output creation. Full Go tests/vet
and agent/experiment/analysis/source/CLI race checks pass. CLI planning retained
the flag in `.wasmbench/partition-policy-v1.lock`; runtime and source execution
correctly refused it on macOS without creating `runs/partition-refused-v1` or
`builds/partition-refused-v1`. The optional-policy V8 core run
`runs/partition-optional-portability-v1` completed two successful measurements
and verified checksums. A successful live isolated-partition run, continuous
monitoring and dedicated-machine qualification are still unverified/unfinished.

### Initial WASI Preview 1 reactor profile

The `reactors` corpus and `wasi-reactor` ABI now run on wazero compiler and
interpreter backends. The versioned no-I/O host profile has explicit once-only
`_initialize`, no ambient files/argv/environment/network, empty stdin, zero
randomness, synthetic clocks and rejected stream output. Compile, instantiate,
app-init, first-call, steady, teardown and controller-owned cold-process timing
have explicit boundaries. Memory passes record Go-managed API-window activity
and post-verification guest logical memory. Fresh and stateless contracts are
distinct; steady stateless batches have no hidden workload pre-call and retain
all warmup results. Other adapters remain unsupported rather than being relabeled
as core or command execution. Phase barriers, counters and profiling are not yet
qualified for this profile.

The WABT 1.0.41 fixture checks Wasm start/initialization/input ordering and real
Preview 1 argument/environment/random imports. Tests cover both backends and
profiles, all six adapter-owned lifecycle scenarios, missing/wrong initializer
signatures, incorrect middle results, false stateless declarations and forbidden
output from workload/oracle functions. Controller integration additionally
verifies sealed full lifecycle/cold-process bundles. Full Go tests/vet and
adapter/protocol/corpus/experiment race tests pass. Linux ARM64 reactor tests
passed in the restricted read-only container; Linux AMD64 tests cross-compiled.

`runs/wasi-reactors-timing-v1` retains 28 successful measured wazero trials and
14 unsupported V8 cells. `runs/wasi-reactors-memory-v1` retains 24 successes,
12 unsupported cells and 440 sample-level memory observations (including
sacrificial evidence); its report is `reports/wasi-reactors-memory-v1`.
Both bundles passed checksum verification. These are functional exploratory
checks, not sufficient replication for comparative performance claims. Broader
reactor host profiles and other runtime implementations remain unfinished.

### Wasmtime reactor lifecycle coverage

Wasmtime Cranelift, Winch and pooling now implement the explicit no-I/O Preview 1
reactor contract. Scalar/input/memory verification is shared with core workloads
through a generic Store state. WASI stream capture and synthetic clocks reuse the
command host implementation. Initialization occurs exactly once before input;
stateless steady batches have no hidden pre-call, and every result is verified.
Output attempted by workload or oracle functions fails even if guest code ignores
the write error. Teardown owns and drops the fresh Store/module/linker/engine.
Memory passes expose logical guest memory, not invented native allocator counts.

Local Cargo tests passed (13 adapter and 4 analyzer tests), including the new
48-case backend/reset/profile/scenario lifecycle matrix and negative initializer,
oracle, state-reuse and output tests. The subprocess conformance test passed for
all five supported configurations; CI now invokes it explicitly. Full Go tests
and vet passed. Hosted CI and Linux-native Wasmtime execution were not observed
in this increment.

`runs/wasmtime-reactors-timing-v1` contains 70 successful measured cells and 14
explicitly unsupported V8 cells across seven scenarios, including cold-process.
`runs/wasmtime-reactors-memory-v1` contains 36 successful measured Wasmtime cells.
These one-launch runs are functional checks, not comparative performance evidence.
The product goal remains incomplete, including component/Emscripten profiles,
snapshots, dedicated-host qualification and advanced native-code attribution.

### Exact tool preservation and restoration

New CLI locks now default to archiving their exact runner, adapter files and
independent analyzer. The flag is immutable under existing locks; old plans keep
their previous behavior. Bundle copies are read-only, non-executable and bound
to both the seal and locked hashes. `restore-tools` writes a new directory with
exact executable copies, workload/command fixtures, a relocated lock and source
checksum provenance, without executing code or overwriting installed tools.

A live replay exposed that Homebrew Node's executable is a loader for a separate
`@rpath/libnode.147.dylib`. Runtime resolution now pins Mach-O third-party library
closures: loader-relative libraries relocate with the executable, while absolute
libraries remain independently hash-verified host prerequisites. Their archived
copies retain evidence but do not rewrite native load commands. macOS shared-cache
libraries remain OS prerequisites; ELF/PE dependency discovery is unfinished.
This is exact-file preservation, not a hermetic image or universal relocation.

Unit tests cover overwritten original tools, preserved JS import layout,
tampering even after resealing, exclusive destinations, immutable source bundles,
unresolved native libraries and changed absolute library hashes. Live adapter
archive/replay conformance passed for wazero, Wasmtime and V8; CI now includes
explicit archive replay tests. Full Go tests/vet, targeted race tests and
actionlint passed locally. Hosted CI was not observed.

`runs/tool-archive-v1` and `runs/tool-archive-replay-v1` retain the initial
loader failure as evidence. After native-library pinning,
`runs/tool-archive-v2` and `runs/tool-archive-replay-v2` each contain six successful
measured cells plus six successful sacrificial checks, with verified bundle
checksums. The replay used its archived runner and relocated adapters. Each
bundle is about 179 MiB on this host; copies are not deduplicated. These are
one-launch functional checks, not performance comparisons. The previous normal
runner was retained at `bin/wasmbench-before-tool-archive` before rebuilding.

Pilot-confirmation relocation and source-build tool archives remain unfinished;
legacy bundles cannot recover bytes that were already overwritten. See
`docs/TOOL-ARCHIVES.md` for the operational and platform limits.

### Linux ELF startup dependency pinning

Runtime resolution now records `linux-glibc-startup-closure-v1`: a native glibc
loader probe with bounded output/deadline, exact interpreter and library hashes,
and loader control-file hashes or explicit absence. Runs re-resolve startup
libraries before output creation and reject changed name/hash sets. The lock
retains actual resolved paths, including restored `$ORIGIN` libraries. Offline
loading and restoration do not execute native probes. Loader overrides, audit
tags, nonempty system preloads and unsupported interpreters fail closed.

Linux ARM64 native tests passed in restricted, offline, read-only containers.
The Node/wazero archive-and-replay conformance passed with current read-only
adapter mounts. A separate compiler-equipped container passed the `$ORIGIN`
fixture after its original application/library directory was moved away; this
caught and fixed legitimate loader paths containing `bin/../lib`. Normalization
is allowed only when it preserves the library hash. Parser, malformed ELF,
metadata-binding, loader-override and wrong-library-hash tests pass. Linux AMD64
tests were cross-compiled, not executed on a native AMD64 host. Full Go tests/vet,
targeted race tests and actionlint passed; hosted CI remains unobserved.

`runs/elf-archive-linux-v1/source` and `runs/elf-archive-linux-v1/replayed` each
contain four successful measured cells and four successful sacrificial checks.
The replay used its archived Linux runner and restored adapters in a second
container. Both seals verify offline on macOS with the updated reader; the
generated report is `reports/elf-archive-linux-v1`. The run is a one-launch
functional check, not performance evidence. The container cache supplied the
independent analyzer and base OS, with their exact available identities recorded.

This policy covers runtime startup dependencies, not late `dlopen`, kernel vDSO,
continuous filesystem changes, arbitrary ELF loaders, or analyzer/controller
native-library closures. The standard glibc ARM64/AMD64 interpreter paths are
implemented; musl and PE discovery remain unfinished. Native probe work is
outside measurement but may warm library filesystem pages; it does not imply
filesystem-cold execution or dedicated-host qualification.

### Sealed native-output browser report

`export-code` and `disassemble-code` now include a static `index.html` over the
trial-to-image manifest. It distinguishes available mixed native images from
missing, failed and sacrificial trials, and links exact raw binaries and any
linear LLVM listing/synthetic ELF. The page explicitly withholds guest-function
instruction size and function/tier attribution. `verify-code --dir` checks the
outer seal, copied source run, trial mapping, exact image bytes, generated page,
and synthetic ELF `.text` identity when present. It does not rerun LLVM or
authenticate a producer; sealed disassembly text is still tool-produced
diagnostic evidence.

The live `runs/native-image-v1` export is
`reports/native-code-disassembly-v3`; its source bundle and derived report
verified offline. The opt-in installed-LLVM integration test, full Go tests,
vet, UI tests, and browser check passed. Browser links for both raw image and
linear listing returned HTTP 200. Per-function native-code mapping remains
unavailable from Wago's current public export, and broader generated-output
coverage remains open.

### Paired timing, memory and code evidence

Ordinary reports now accept `--code-run` alongside `--memory-run`. The code
bundle must match timing on observed host, resource/protocol policy, exact
runtime files/configuration and full workload contract. The report embeds a
separately sealed `code/` export and links each measured runtime/workload
result to its exact code trial. `verify-report` verifies that nested export and
recomputes the match from all copied raw bundles; a code bundle hidden from the
dataset or a re-sealed false match is rejected. A code image is never called
guest-only instruction size. Explicit adapter `native.guest_code` unsupported
diagnostics now remain unsupported instead of appearing merely not recorded.

The one-launch `runs/five-configurations-code-board-v1` passed all 25 compile
cells and its seal verified. Paired with the earlier timing and memory runs,
`reports/five-configurations-lifecycle-graph-v17` has 25 matched code records:
five Wago mixed images and 20 explicit unsupported exports. The nested report,
outer report and browser click-through to both available and unsupported
records verified. This is functional exploratory evidence, not a matched
multi-launch performance comparison or official host qualification.

The large lifecycle graph now includes the matched separate memory pass's
whole-trial peak RSS on each timing-stage hover, or an explicit unavailable
value. The peak is not described as a phase-only maximum. The regenerated
report passed offline verification; full Go tests, vet and UI tests passed.

### Sampled occupied CPU-partition evidence

Locked isolated-partition runtime trials now sample the occupied cgroup after
spawn, every 250 ms and before cleanup. Each sample seals the requested
effective/exclusive CPU masks, online CPUs, parent and worker population,
root/worker process membership and other child-cgroup population. Offline
loading recomputes every sample verdict, checks the worker path against the
trial's cgroup and the requested CPUs against the lock, and rejects partial
monitoring or forged success labels. A failed sample seals a diagnostic bundle
and prohibits derived performance results without changing the raw correctness
outcome. The publication audit is `publication-evidence-audit-v3` and exposes
the sampled-worker requirement separately from run-boundary readiness.

Injected read-only tests cover success, CPU-mask drift, process/child intruders,
missing files, tampered verdicts, partial trial coverage and offline loading of
a prohibited diagnostic bundle. Full Go tests and vet, focused agent/experiment/
analysis race tests, and Linux/arm64 focused tests passed. The Linux full
experiment suite could not execute the bundled Darwin-only independent analyzer;
its partition-focused tests passed. This remains sampled evidence: intervals between
observations, IRQs, controller-thread drift, other host workloads and producer
authenticity are not qualified. No positive live isolated-partition run or
dedicated-machine qualification is claimed.

### Distinct source-plus-runtime stack comparison

`stack-compare` and `stack-report` now implement the end-to-end stack identity:
both a source-build lock and a runtime configuration must change while the
source task, input hashes, correctness contract and analyzer policy stay fixed.
Each runtime run must bind to its own exact built Wasm and provenance; matched
host, resource/measurement protocol and runner are required. Runtime API phase
ratios use the independent-launch estimator, retain failed/unsupported cells,
and do not turn one-off compiler step durations into latency samples.

The portable report copies all four sealed inputs, links build and runtime
evidence, and shows phase effects, intervals and coverage. `verify-stack-report`
recomputes the analysis and rendered page from those copied bundles. Unit tests
reject changed tasks, output bindings, host/runner drift, and single-component
comparisons; a live generated report and resealed-tamper test pass. The report
at `reports/stack-wago-vs-wasmtime-v3` compares LLVM O0/Wago with LLVM
O2/Wasmtime on the same xorshift task, with three independent launches per phase.
Browser navigation confirmed the page and copied build link. These are
exploratory measurements on an uncontrolled host, not causal comparisons or
observed source-to-result end-to-end latency. Multi-workload stack analysis,
repeated source-build statistics and official host qualification remain open.

### Broader Wago catalog correctness admission

`import-wago --ids all` verified all catalog artifact hashes and emitted 132
versioned workload contracts into `.wasmbench/wago-all-catalog-v1.json`:
72 executable core contracts, 41 exact command contracts and 19 explicitly
unsupported command/custom-host contracts. The 72 core contracts were imported
separately as `.wasmbench/wago-core-catalog-v1.json` and exercised through
correctness-only adapter processes, not performance measurements.

`runs/wago-core-full-check-v1` sealed 72/72 successful Wasmtime Cranelift checks.
`runs/wago-core-five-config-check-v1` sealed 360 checks: Wago, wazero,
Wasmtime Cranelift and V8 each passed 72/72; Wasmtime Winch passed 67/72 and
retained five explicit SIMD compilation errors (`json-as-simd` serialize and
deserialize, `blake-as-simd` hash, and `utf-as-simd` convert and validate).
Both bundles passed checksum verification. They used local development-host
correctness passes with tool archives disabled, so they are neither official
performance evidence nor complete tool-byte replay bundles. The command ABI
subset, unsupported custom hosts and cross-runtime feature granularity remain
to be qualified separately.

### Wasmtime native function ranges

The pinned Wasmtime 46.0.1 adapter now exports `Module::text()` and
`Module::functions()` in a version-2 native image contract for core modules,
including Cranelift and Winch. The contract preserves defined Wasm indices
(imports are not renumbered), optional names, text-relative offsets and lengths,
backend tier and fixed generation zero. Range bytes can include padding and
embedded constants: the new `native.function_range_bytes` metric does not
redefine the instruction-only `native.guest_code` metric. Remaining text is not
automatically classified as stubs or metadata. Components remain unsupported
for native range export.

The protocol rejects out-of-bounds/overflowing/overlapping ranges, duplicate
indices, inconsistent backend tiers, nonzero generations and unexpected nested
module indices. Legacy version-1 mixed images remain supported without attribution.
The sealed offline export retains the mapping and renders a function table;
verification recomputes it from the source trial rather than trusting a reseal.
Original relocations and code lifetime events remain open.

`runs/wasmtime-function-code-v1` exercised five pinned Wago core workloads with
both backends: all ten code launches succeeded, after sacrificial correctness
checks. `reports/wasmtime-function-code-v1` exported all ten available text images
with their function mappings and passed `verify-code`. This is local diagnostic
evidence, not timing or official publication data. Rust extraction tests exercise
both backends and a nonzero defined function index after an import; protocol
and export tests cover invalid ranges and resealed mapping forgery.

### Per-function native assembly drilldown

`disassemble-code` now produces a version-2 report containing LLVM listings for
each engine-reported function range and backend generation, in addition to the
complete mixed-image listing. Range decoding uses the full byte-verified
synthetic ELF text section with start/stop addresses, preserving image-relative
PC/branch addresses instead of rebasing a slice. The offline HTML embeds escaped
assembly in expandable function rows and links to exact downloadable listings.
Every listing records the range, generated relative path, tool arguments and log.
Listings may decode embedded constants and padding; neither instruction-only
sizes nor compiler counters are inferred.

`reports/wasmtime-function-disassembly-v2` derives ten native images and twenty
function listings from the sealed `wasmtime-function-code-v1` run. It passed
`verify-code`. LLVM integration tests exercise ARM64 and AMD64 range boundaries
and reject resealed identity, arguments, coverage and embedded-listing forgeries.
The verifier validates source mappings, byte-identical text objects and embedded
listing/file equality, but does not independently reproduce LLVM's decoding.
Generated-code comparisons are implemented below; semantic assembly normalization
and compiler annotations are not inferred.

### Independent Wasm-body to native-range expansion

Native reports now include `native-function-expansion-v1`: each engine-reported
function range is joined by exact artifact digest and full Wasm function index
to the pinned independent analyzer's `core-structure-v3` body evidence. Body
bytes include encoded locals and operators, excluding the body-size prefix.
The report preserves exact body/range byte values alongside their ratio, linked
input evidence, fixed backend-generation coverage and unclassified image bytes.
The latter are not relabeled as trampolines, metadata or mapping capacity.
Missing/legacy input evidence has null ratios; imported or unknown function
indices and malformed body coverage fail rather than produce misleading values.

`reports/wasmtime-function-expansion-v1` reuses the sealed five-workload,
two-backend native run and contains twenty body/range joins with expandable
assembly. `verify-code` recomputes all expansion and coverage fields from the
copied source evidence. Tests exercise import offsets, unknown indices, missing
evidence, malformed/duplicate body records and a resealed false-ratio report.
These are retained compile snapshots, not lifetime tier coverage or
instruction-only size measurements.

### Sealed generated-code comparisons

`compare-code` now packages two verified native exports (including two runtime
configurations in one export) into a portable offline comparison. It preserves
every selected compile workload/block cell, requiring matched host, resource and
protocol identities and matching exact artifact/executable workload contracts.
Changed tasks remain incomparable. Missing trials, unsupported function
attribution and partial function coverage remain visible, without zero scores.
Functions join by full Wasm index, never name or array position; backend and
generation are retained separately on each side.

`native-function-comparison-v1` records exact range hashes, collision-independent
byte equality, range-size deltas and the first differing function-relative byte
offset. Expandable side-by-side listings retain original image-relative addresses.
The report embeds configurations, evidence links, coverage, a workload filter and
a shell-quoted offline reproduction command. `verify-code-comparison` verifies
both copied reports and recomputes the complete dataset and HTML byte-for-byte.
It never executes adapters, native code or LLVM.

`reports/cranelift-vs-winch-code-v2` compares all five workloads and ten matched
functions in the sealed Wasmtime range/assembly/expansion export. Unit tests
exercise reordered functions, moved but byte-identical ranges, changed input
contracts, missing functions/blocks, legacy attribution, malformed ranges,
host/protocol mismatches, duplicate cells and escaped diagnostic text. The live
comparison test rejects resealed equality/status, delta, listing and page forgeries.
These are diagnostic snapshot differences, not instruction-only sizes, semantic
equivalence claims or causal explanations for timing changes.

### Explicit eager guest-state checkpoint lifecycle

The new `checkpoints` suite exercises four separate operation boundaries:
`checkpoint-create`, `checkpoint-restore`, `checkpoint-first-write`, and
`checkpoint-execute`. Wazero compiler and interpreter advertise
`can_guest_checkpoint`, while generic `can_snapshot` remains false. The exact
generated core module contains fixed min=max linear memory, a mutable exported
i32 and three void/checksum functions, with no imports, tables, segments,
WasmGC objects or active-stack snapshot requirements. Module byte equality,
not a claimed export list, constrains this state model.

Creation allocates and copies every memory byte plus the scalar. Restore copies
into a separately instantiated fresh target, excluding target instantiation.
First write changes the final byte and global through one guest call; it is an
ordinary eager-copy write, not COW page materialization. Execution is one
full-memory checksum call. Every sample has fresh source/target/payload, with
engine/module held across the batch. Source mutation verifies independence;
complete restored state and exact checksum are verified outside timers. Memory
profiles add narrow Go allocator-window metrics, exact payload bytes and three
ordered boundary handshakes. Release does not imply reclamation, and GC is
never forced.

The controller rejects wrong budgets, mixed state contracts, unsupported
profiles and absent capabilities. It rechecks phase-specific scalar/checksum
and memory-digest evidence rather than trusting `verified` alone. Offline
loading revalidates the canonical artifact, sample sequence, state records,
profile, payload metric and boundary order. Tests cover both backends, all four
sizes/phases, two fresh samples, copy isolation in both directions, arbitrary
module changes, malformed contracts and forged saved evidence. Reports expose
checkpoint stage labels/colors, payload/state semantics, scaling curves and raw
trials, retaining unsupported rows.

`runs/guest-checkpoints-timing-v1` and `runs/guest-checkpoints-memory-v1` each
contain 96 successful measured launches (192 samples) and 48 V8 unsupported
cells, plus sacrificial checks. Three launches cover 1/4/16/64 pages, both
wazero backends and four phases. `reports/guest-checkpoints-v5` joins the exact
timing/memory identities and passes offline report verification. These are
local functional/exploratory measurements, not official performance claims;
the timing run overlapped a local test pass and has no dedicated-host isolation.
The memory run followed after that test pass completed.

Whole-instance/runtime-native snapshots, COW behavior, restored-instance
density, sustained reclamation qualification, lifetime code/tier events,
dedicated-host publication and other unchecked proposal requirements remain
open. This checkpoint experiment does not close those larger requirements.

## Retained-instance sustained sessions and duration qualification

The sustained scenario now runs the real integer-sum-256 workload on one
retained wazero compiler/interpreter instance. Its locked fixed sample count,
operations per batch, warmup and minimum cumulative non-warmup API time are
validated before execution and again from saved samples. Monotonic session
windows separate the API timer, allocator diagnostic brackets and verification
gaps. Every invocation result is checked outside the measured region. The first
invocation is not consumed by an untimed correctness check in the measured
process. Sacrificial validation remains a separate process.

Memory passes report scoped Go allocation-counter deltas, GC cycles and heap
endpoints. Allocation rate uses each observed diagnostic window, never heap
endpoint subtraction. Final engine/module/instance close has a separate logical
release record and allocator window; no forced GC or physical-reclamation claim.
Timing passes contain no allocator snapshots. Unsupported runtimes stay visible.
Session clocks and logical-release fields are nullable in Parquet v3. Short
fixed-budget sessions retain raw diagnostics as `duration_budget_not_met` and
are excluded from successful headline estimates. Paired passes require identical
duration, sample, operation and warmup budgets as well as existing exact runtime,
workload, host and resource identity checks.

`runs/sustained-session-timing-v1` and `runs/sustained-session-memory-v1` each
contain six successful measured sessions, three V8 unsupported cells and three
sacrificial checks. Each successful session has 3,000 measured batches of 1,000
calls plus ten warmup batches: 18,060 samples per pass. All sessions exceed the
locked one-second cumulative API target. Memory sessions have allocation-rate
observations at every sample and observed GC activity. These Darwin/arm64 runs
are functional/exploratory evidence, not dedicated-host performance publication.
Builds/tests were kept outside these measurement runs.

`reports/sustained-sessions-v2` passes current offline replay and links timing
and memory raw trials separately. Browser checks reached the final ten samples,
allocation-rate and logical-close evidence, with no horizontal overflow at
390 pixels. The older v1 report is preserved: its only replay difference was
the pinned renderer fingerprint after subsequent dashboard edits, not changed
measurements. `runs/sustained-short-budget-v1` proves the negative path live:
two correct samples remain unqualified against a one-second target; V8 remains
unsupported. Its report and bundle pass offline verification.

README documents runnable paired commands and boundaries. Protocol, adapter,
offline validation, analysis, Parquet and renderer tests cover clocks, warmup,
oracle results, operation counts, missing/duplicate allocator domains, short
budgets, release placement and sample-window reachability. Full reclamation/leak
qualification, post-release retained-live-memory observation, other runtime
implementations, broader stateful workloads, whole-instance/COW snapshots,
restored density, lifetime/tier events and remaining proposal requirements are
still open. This work does not mark the full product complete.

## Explicit post-Go-collection diagnostic after sustained release

`--sustained-post-collection` now locks an opt-in memory-only policy: one
explicit `runtime.GC` after logical engine/module/instance close and dropping
runtime references. It does not change the measured invocation timer, close
timer or default no-forced-collection policy. The controller requires the
separate advertised capability and rejects timing profiles or lock overrides.
Sacrificial admission does not collect. The adapter retains sample evidence and
its own process; this is a post-Go-collection heap observation, not runtime-only
retained-live memory, allocator purge, reclaimed RSS or leak qualification.

The final release record has an optional `post_collection` with monotonic
start/end offsets, explicit policy and seven allocator/heap/GC observations.
Offline sequence validation checks request/policy agreement, ordered clocks,
complete and unique metric domains, finite nonnegative values, integer counts
and at least one observed forced GC. Saved-bundle validation also bounds the
collection end by the observed trial lifetime. Analysis retains this record
under `retained-instance-session-v2`. Parquet v4 exports nullable request,
collection clock and policy columns; observation export retains scoped metrics
with no guest-operation count or warmup denominator. Regression comparison
rejects different collection policies. Timing/memory report joins intentionally
permit the memory-only collection option while preserving other matched budgets.

`runs/sustained-post-collection-memory-v1` contains six qualified sessions
(both wazero backends, three launches), three V8 unsupported cells and three
sacrificial checks. Each success has 500 measured batches of 1,000 calls plus
ten warmup batches and exceeds the locked one-second API-time target. All six
collection brackets report one forced GC. The matching timing run has six
qualified sessions, three unsupported cells and no collection records. Builds
and tests were outside both measurement runs. These local Darwin/arm64
measurements validate functionality, not official performance or reclamation.

`reports/sustained-post-collection-v1` verifies against copied raw evidence and
current analysis. The report labels collection policy per session, displays
post-collection heap/forced-cycle observations separately and links complete
clock/domain evidence. Renderer tests prevent a collected session from claiming
that no forced GC was requested. Protocol, adapter, offline validation and
Parquet tests exercise forged/missing domains, fractional counters, unsupported
profiles, process-lifetime bounds and nullable exports. The CLI rejects an
actual timing-plan request with the collection option. README documents usage
and the observation's limits. Physical-reclamation, sustained leak qualification
and the remaining full-product requirements remain open.

## V8 retained-instance sustained execution and separate heap domain

The Node/V8 adapter now advertises sustained execution for the same bounded
stateless core/exact-scalar contract. It creates a fresh module wrapper and
initialized instance per request, retains them across all declared warmup and
measured batches, resolves arguments/export and result buffers before timing,
and verifies every result after both the API timer and heap snapshots. No
untimed workload invocation consumes invocation one. Normal tiering remains
enabled; engine, code-cache and background-compilation behavior are uncontrolled.
Tier identities and final optimized-code completion are not observed or inferred.

Memory passes expose `process.memoryUsage().heapUsed` start/end snapshots in
`adapter_process_v8_heap`, with explicit operation and reference-release
brackets. These are not allocation counters, Go heap, RSS, guest-only memory or
retained-live bytes. Analysis version `retained-instance-session-v3` has a
distinct nullable V8 heap field, withholding duplicates/mismatched domains and
never filling Go metrics from V8 snapshots. The report selector includes that
domain and preserves unavailable metrics as gaps.

Release is `policy: js_references_dropped`, `closed: false`: module, instance and
export references are dropped, but Node's process engine/internal caches are
not closed. Protocol validation rejects contradictory/unknown policies and Go
collection for this release policy. Legacy release records default to
`runtime_closed` without redefining their meaning. Parquet v5 preserves the
nullable explicit release policy and false close observation. The controller
keeps forced post-Go-collection unsupported on V8. Reports display the release
policy explicitly instead of claiming engine closure.

Real protocol integration tests run the Node adapter through the controller
client for timing, memory, invalid budgets and unsupported collection. A
five-call state fixture proves no hidden pre-call; a wrong middle result is
rejected. Tests also cover JS release contradictions, unavailable/duplicate
V8 heap domains, nullable Parquet policy and misplaced saved release snapshots.

`runs/sustained-v8-timing-v1` and `runs/sustained-v8-memory-v1` each contain three
qualified measured sessions and one sacrificial check. Each session retains
3,000 measured batches of 10,000 calls plus ten warmup batches. All exceed the
locked one-second cumulative measured-API target. Timing has no heap snapshots;
all 9,030 memory samples have V8 heap endpoints and no Go metrics. All releases
remain explicitly reference-only. Builds/tests were kept outside both measured
passes. These Darwin/arm64 local functional measurements are not official
performance or tier/lifetime/reclamation claims.

`reports/sustained-v8-v2` is the current-renderer paired report; v1 preserves an
earlier renderer produced before the final build completed. Full Go tests/vet,
focused race tests, V8 and renderer tests cover this change. The original
scope remains open: runtime-native/whole-instance/COW snapshots, restored
density, physical reclamation/leak qualification, observed tier/code lifetime,
additional platform/ABI coverage, dedicated-host publication and remaining
proposal requirements are not completed by this adapter extension.

## Fresh and eager-restored guest-state density

The `guest-density` suite adds 32 contracts: two fixed-memory sizes (1 and 4
pages), fresh initialization versus eager guest-state restoration, unchanged
state versus one post-provisioning first write, and 1/4/16/64 simultaneously
held instances. Both wazero compiler and interpreter advertise
`can_guest_density`; whole-instance `can_snapshot` remains false. Unsupported
runtimes are explicit outcomes, not zero measurements or omitted coverage.

The exact checkpoint fixture has only fixed linear memory and one mutable i32.
One engine/module remains held for the request. Restored groups use one eager
independent memory/scalar payload created outside the timer. The source is
mutated after copying, checked for isolation, closed, and leaves scope before
the first measured group. Every sample creates a fresh simultaneous group.
The timer includes instantiation and fresh initialization or host restore-copy,
plus the declared optional guest write. No checksum or state verification runs
inside that timer. All memory bytes, scalar state and the full-memory checksum
of every instance are checked afterward. The shared payload is checked again
after target writes. Group release closes all instances, without forced GC.

Both provisioning policies touch all memory. `unchanged` means no additional
post-provisioning write, NOT physically untouched pages or COW materialization.
This experiment is shared-module eager guest-state density, not a whole-instance
snapshot, runtime-native snapshot, COW page-fault measurement or pooling policy.
Compile, source setup and checkpoint creation are excluded; their separately
measured checkpoint scenarios remain available. Logical close and reference
drop do not prove physical memory reclamation or leak freedom.

Memory snapshots bracket group provisioning, observe ready before verification,
then observe release while the engine/module and optional shared payload remain
held. Verification evidence and adapter buffers remain live. Go allocation
volume, heap endpoints, GC activity and exact logical group bytes have explicit
domains. Offline bundle loading rechecks exact canonical module bytes, every
instance, copy-isolation flags, payload size, measured/sacrificial budgets,
profile, barrier order and complete allocator/logical-memory evidence. Invalid
measured budgets are rejected rather than coerced.

Curve generator identities include policy, memory size and write state; instance
count is the only varied dimension. Full and marginal curves cannot merge those
different contracts. Density boundary-change analysis v2 also accepts the new
phase names. Nullable sample Parquet v6 stores complete guest-density evidence
JSON, preserving every per-instance digest, scalar and checksum and explicit
zero/false fresh-policy fields. Raw trial JSON remains the primary evidence.

Tests cover all contracts and both backends, timing/memory profile separation,
barrier order, arbitrary module rejection, last-instance corruption, incorrect
payload isolation, invalid allocator domains, missing metrics, offline artifact
tampering, independent policy curves, signed boundary changes and nullable
Parquet evidence. Full Go tests, vet and dashboard tests passed before live
measurement; builds/tests are kept outside those passes. Local measurements are
functional exploratory evidence, not dedicated-host publication qualification.

The first local memory bundle (`runs/guest-density-memory-v1`) is preserved as
diagnostic evidence, not the accepted memory pass: offline validation caught
controller boundary snapshots being classified as adapter allocator evidence,
and the cgroup phase mapper defaulting the new stage names to compile. The
validator now matches attached snapshots against ordered phase records while
validating the eight adapter metrics separately. The mapper recognizes guest
density and checkpoint stages, including unavailable peak/CPU outcomes at the
correct returned/ready boundary. Regression tests cover attached-snapshot
tampering and every checkpoint/guest-density phase mapping. No old sealed
bundle is edited or relabeled to hide its original collector behavior.

Verified live evidence is `runs/guest-density-timing-v1` and
`runs/guest-density-memory-v2`. Each has 192 successful measured trials and 96
explicit V8 unsupported trials across three launch blocks, with 384 measured
group samples and 8,160 per-instance state records. Timing has zero phase
barriers; memory has 1,152 ordered barriers. Sacrificial checks remain separate.
Both passes finish before subsequent builds/tests. The corrected memory run
passes offline validation with controller snapshots in their correct domains.

`reports/guest-density-v2` is the final paired report: 24 timing curves (including
eight V8 unavailable curves), 864 distinct memory curves, 16 available logical
group-memory curves, 16 available allocation-volume curves and links to all
288 measured memory-trial outcomes. Each available policy curve has four count
points and three marginal intervals, without merging policy/state/memory size.
The main workload view has 64 allocation, 64 Go-heap-end and 64 lifetime-peak-RSS
cells. The paired scaling view now includes only exact matched memory contracts;
profile labels stay explicit and memory clicks open raw-memory evidence rather
than substituting timing trials. Regression tests cover unmatched-contract
exclusion and failed-launch raw links. The earlier paired v1 is preserved before
this report-consumer extension.

`reports/guest-density-memory-v1` is the standalone detailed memory report with
6,144 independent-domain timelines and 1,920 boundary-change records. Darwin
procfs/cgroup snapshots remain explicitly unavailable, not invented zeros or
available Linux residency evidence. Both reports pass checksum and derived
dataset verification. Full Go tests/vet, focused protocol/experiment/analysis/
publish race tests and all 15 dashboard rendering tests pass. The paired report
is served locally at port 8110; the standalone memory report at 8109.

The full proposal remains open: runtime-native/whole-instance/COW snapshots,
physical reclamation/leak qualification, observed tiers and code lifetime,
remaining ABI/platform/runtime breadth and dedicated-host publication criteria
are not completed by this guest-state experiment.
# Large runtime latency and peak-RSS comparison

The headline graph now gives the combined compile/instantiate/execution bar
more horizontal space and labels its shared time scale separately from the
shared RSS scale. Missing measurements are explicitly labelled rather than
represented as a zero-valued scale. Existing stage selectors, compact hover
details and exact-stage evidence clicks remain available.

Verified report: `reports/five-configurations-lifecycle-graph-v22`, generated
from the saved five-configuration timing and RSS runs. Local preview:
`http://127.0.0.1:8112/`. All 18 renderer contract tests, `go test ./publish`,
the application build and `verify-report` passed; the preview returned HTTP 200.
RSS remains a whole-process maximum from a separate stage-focused launch,
not an isolated phase peak. These are exploratory local measurements.

## Verified whole-report reproduction and paired memory evidence

`reproduce-report --dir REPORT --out NEW_DIRECTORY` verifies the report and
preflights every input lock before creating output or starting measurements.
Primary, memory and code passes run sequentially with their original locked
profiles and exact runner hashes. The current runner is used only when its hash
matches; otherwise the sealed, deliberately non-executable runner archive is
copied into the new output's `runners/` directory, hash checked and executed.
Adapter/analyzer paths are not relocated or identities relaxed. Failures preserve
completed/partial evidence, stop later passes and do not generate a partial
comparison. Existing outputs, nested outputs and tampered reports are refused.
The final report is regenerated and verified using the current analysis/renderer.
The CLI help, README and report reproduction instructions expose this workflow.

Live fixture evidence: `reports/report-replay-source-v1` contains independently
measured wazero timing, memory and code passes for two core contracts. Both
`runs/report-replay-result-v1/report` (matching current runner at execution time)
and `runs/report-replay-archived-runner-v2/report` (different controller hash,
exact archived runner copies) verify. The source runner remains non-executable
and unchanged. Code API calls succeed but native export remains explicitly
unsupported for wazero; these are functional local checks, not performance or
native-code-export claims. The first archived-runner attempt v1 is preserved as
diagnostic failure evidence; its execute-permission issue prompted staging copies
instead of changing immutable archive permissions.

Full Go tests, focused vet, publish race tests and all 18 renderer contract tests
pass. Tests cover all-pass preflight, separate profiles/order, cancellation,
tampering, missing tools, output protection, preserved evidence on failure and
hash-checked runner staging. Measurement runs occurred after builds/tests ended.

The current five-configuration graph is
`reports/five-configurations-lifecycle-graph-v23` at port 8113. The refreshed
paired density report is `reports/guest-density-v4` at port 8114. Its timing and
paired memory timeline namespaces remain separate, even with colliding trial
IDs; memory curves have profile/runtime/metric filters and correct raw-memory
links. Dedicated `memory-samples.parquet` and `memory-observations.parquet`
downloads preserve the full raw memory pass, including unmatched/sacrificial
evidence. Verification regenerates both exports from the copied memory bundle
and compares digests, rejecting even resealed substitutions. Both reports pass
verification and the preview/download endpoints return HTTP 200.

This does not complete the full proposal: runtime-native/whole-instance/COW
snapshots, physical reclamation/leak qualification, observed tiers/code lifetime,
remaining ABI/platform/runtime breadth and dedicated-host publication remain
open. Reproduction does not establish identical performance or official gates.

## Stage-local latency/RSS Pareto comparisons

Reports now export `latency-rss-pareto-v1` points for every locked runtime,
workload and stage, retaining unavailable pairs. Only eligible timing-profile
stage medians and matched `process.peak_rss` memory-pass medians are joined;
allocator-specific values cannot substitute for RSS. Zero remains measured,
exact ties stay nondominated, and domination requires no larger coordinate with
at least one strictly smaller coordinate. Frontier comparisons are grouped by
workload/stage, not across corpus tasks or lifecycle windows.

The collapsible trade-off view below the main graph selects one stage, follows
the workload/runtime controls, recomputes its descriptive frontier among selected
configurations and never edits saved evidence. Keyboard-operable points and table
entries open timing details, source-qualified memory trials and both manifests.
Per-axis bootstrap intervals and independent timing/memory launch counts remain
separate. Missing pairs stay in the coverage table. The chart's interactive group
role preserves child evidence-button accessibility. Tests exercise these DOM
contracts deterministically; browser visual QA was not requested/performed.

RSS is the whole-process maximum including startup/setup, not an isolated phase
peak. Separate passes do not yield jointly observed time/memory samples or a
joint 95% confidence region. The median frontier is not statistical significance,
an aggregate score, end-to-end latency or an official performance conclusion.
Verification recomputes point values and classifications from copied raw evidence;
even a resealed derived-data frontier substitution is rejected.

`reports/five-configurations-lifecycle-graph-v25` contains 100 available points
across five configurations and five workloads; all single-launch timing intervals
remain unavailable. Its preview is at port 8116. `reports/guest-density-v6` has
96 points: 64 available with both per-axis intervals and 32 explicitly unavailable
V8 pairs; preview port 8117. Both reports pass verification, and both preview
endpoints return HTTP 200. Earlier generated versions remain unchanged.

Full Go tests, focused vet, publish race tests, the build and all 20 renderer
contract tests pass. Saved runs were reused; no new benchmark claims were made.
The overall proposal remains incomplete, including the open requirements above.

## Preserving original lifecycle-report analysis across updates

Lifecycle report generation now archives the exact builder executable, separately
from each input pass's runner. The archive is non-executable `builder/wasmbench`;
`builder.json` links its SHA-256, OS/architecture, dataset-file hash, analysis
version and renderer hash. `sealed-report-builder-v1` in the dataset records this
contract. The complete report seal covers receipt and archive. Normal verification
checks their consistency without executing them, alongside raw-data recomputation.

`verify-report --recorded-builder` and `reproduce-report --recorded-builder`
explicitly execute the recorded builder after whole-report seal, receipt and
platform checks. A temporary exclusive executable copy is hash checked and
removed afterward; the source archive is neither chmodded nor changed. Execution
warnings and report/README instructions distinguish checksum consistency from
publisher trust: these options must only be used with archives trusted to run.
Default verification never falls back to executing archive code. Historical
analysis/renderer differences retain a strict failure plus an opt-in recovery hint.

Verified live update boundary: `reports/report-builder-before-update-v1` was
generated and verified before the new reproduction instructions changed the
renderer. The final current build rejects it in default mode; recorded-builder
verification passes using its original renderer. Recorded-builder reproduction
created `runs/report-builder-original-analysis-v1` with independently rerun
timing, memory and code bundles, two measured trials per pass, and a verified
report. Source/output builder hash, renderer hash and analysis version match;
dataset-file hashes differ because these are new measurements. The original
source seal remains `818637f91c7db7df117f7e9707e16cf3edb872708412a9fcd395c1f38f0180d7`.
The source builder remains mode 0444. Builds/tests finished before rerunning
measurements. This establishes functional reproduction, not identical performance.

Full Go tests, focused vet, publish race tests and all 20 renderer contract tests
pass. New tests cover archival identity, temporary executable permissions and
cleanup, immutable source state, malformed/resealed receipt substitutions,
binary tampering, symlinks, missing/unrecorded archives, wrong platforms,
cancellation and explicit refusal of pre-archive reports.

Current previews: `reports/five-configurations-lifecycle-graph-v26` at port 8118
and `reports/guest-density-v7` at port 8119, preserving the existing static format.
This archive contract currently covers lifecycle reports only. Other report
families still require equivalent builder archival, and pre-feature archives
cannot be retroactively supplied with their original builders. All remaining
proposal requirements remain open; the overall goal is not complete.

## Builder archival and exact regeneration for six more report families

Runtime comparison, history, aggregate, source-build benchmark, source-set and
stack reports now archive their exact builder under
`sealed-report-family-builder-v2`. Receipts bind the builder/platform, dataset
and page hashes, renderer source hash, known family and regeneration contract.
The original lifecycle v1 format remains supported. Family dispatch is a fixed
allowlist, not a command or path read from arbitrary receipt data.

`verify-report --dir REPORT` detects these archived families, regenerates the
report from copied sealed inputs in its own temporary directory and compares the
complete file set and digests. Only the top-level builder/receipt are excluded
because they may differ across analyzer builds; nested input archives, HTML,
JSON, Parquet and raw evidence remain checked. Compilers, runtime adapters and
benchmarks are never executed by default verification. Existing comparison,
source-set and stack verifiers route new archived reports through this stronger
check. Current-renderer mismatch stays explicit. Recorded-builder verification
is opt-in trusted code execution, with the same staging/platform protections.

Tests cover portable comparison/history/aggregate regeneration, exact staged
builder hashes, and rejection of unrelated HTML even after updating the receipt
and resealing. Unsafe history locators and unknown family identifiers are
rejected. `aggregate-set --out FILE` now honors its requested file destination,
uses exclusive creation and refuses output inside sealed evidence; stdout
behavior without `--out` remains unchanged. A live aggregate workflow exposed
the previous silently ignored `--out`, and the corrected command/file protections
are tested.

Live artifacts `reports/family-builder-{comparison,history,aggregate,source,
source-set,stack}-v1` all pass default and recorded-builder verification.
They reuse already saved measurements; no benchmarks or source builds were
rerun. The source-set artifact is explicitly a same-input control, not evidence
of a toolchain difference. History and stack previews at ports 8120 and 8121
return HTTP 200; their builder archives remain mode 0444. Existing lifecycle
report v26 still verifies with the expanded CLI. Full Go tests, focused vet,
publish race tests, the build and all 20 renderer tests pass.

Remaining scope includes measurement replay for non-lifecycle families,
equivalent archival for native-code export/disassembly/comparison, and recovery
of original builders for pre-feature archives. These limitations do not narrow
the original product goal; all previously open proposal requirements remain open.

### Native report builder archival and offline regeneration

Native exports, disassembly pages and code comparisons now use the sealed
family-builder receipt/archive contract. New native payloads mark the archive
version; their established `native-code.json` filename is selected by a fixed
family allowlist, not a user-supplied receipt path. Comparison payloads retain
`data.json`. The receipt binds the exact builder, payload, HTML, renderer source
and platform. Dedicated native verifiers check receipts without recursively
dispatching regeneration; older unarchived reports keep their prior verifier.

`verify-report` now regenerates native images, raw-derived function ranges,
expansion analysis, comparisons and rendered pages. The complete regenerated
file set/hashes must match, excluding only the top-level builder/receipt. Nested
archives stay included. LLVM-produced objects/listings/logs remain sealed
diagnostics: object text bytes and listing mappings are checked, but no LLVM
command, adapter or native image is executed by default verification. This is
not independent instruction redisassembly, semantic verification or pinning of
LLVM dynamic dependencies. The pages expose offline verification instructions
and the explicit archived-code trust warning.

Tests cover native export/comparison regeneration, nonexecutable archives,
hash-checked staged builders, unsupported measurement-replay refusal, removed
or relabelled receipts, extra resealed files and forged resealed HTML. Installed
LLVM integration exercises ARM64 and AMD64 object/function mappings, and proves
offline disassembly regeneration succeeds when recorded LLVM paths do not exist.

Live artifacts `reports/native-builder-{export,disassembly,comparison}-v2`
pass both default and recorded-builder verification. They use saved code-pass
measurements, not new benchmarks; disassembly generation ran local LLVM offline.
The comparison selects Wasmtime Cranelift versus Winch from the same sealed
multi-runtime code bundle. Preserved v1 artifacts establish historical builder
behavior across an actual template change: current verification refuses their
old renderer; their exact original archived builders still verify all three.
The comparison preview at `http://127.0.0.1:8130` returns HTTP 200. Existing
lifecycle graph v27 verifies with the updated CLI. Full Go tests, focused vet,
publish race checks, LLVM integration and 21 UI interaction tests pass.

The preceding graph turn added an accessible expanded/compact layout toggle,
preserving stage/workload selections and data; graph v27 at port 8122 is retained.

Remaining work still includes non-lifecycle measurement replay, native/whole-
instance/COW snapshot breadth, tier/background-code lifetime observations,
physical reclamation qualification, additional ABI/platform/runtime coverage
and dedicated-host official-publication acceptance. Pre-feature archives still
require recovery of their original builders. The full product goal stays open.

### Measurement replay for runtime-based report families

`reproduce-report` now dispatches lifecycle, comparison, history, aggregate,
native export, native disassembly and native comparison reports. The common
orchestrator verifies each source report and preflights every input's lock,
adapter/analyzer files and exact runner before creating output or starting any
measurement. It uses the current runner only for an exact hash match, otherwise
staging the input's sealed nonexecutable runner into the new output. Passes run
sequentially. Failure/cancellation preserves evidence and stops later passes;
the final report is not generated from incomplete measurements.

Comparison sides stay separate, history preserves ordered source inputs, and
aggregate replay preserves the exact saved workload set/category weights. Native
replay collects new code-pass evidence, creates new exports, and reruns LLVM only
where the original side collected disassembly. It does not copy old code images
or listings into new measurement evidence. All required LLVM paths/hashes are
preflighted before measurements; the recorded tool identities are passed directly
to the disassembler, which checks those exact hashes before/after execution.
LLVM diagnostics run after measurement passes, never concurrently with them.
No transitive LLVM dependency pinning or instruction-semantics claim is added.

Recorded-builder replay supports these families when the archived executable
implements their replay workflow. Older builders retain their original explicit
unsupported-family behavior; a newer caller cannot add capabilities to an old
archive. Source-build, source-set and stack replay still refuse rather than
substitute old source compiler measurements for new ones.

Tests cover five non-LLVM family plans, all-pass-before-first-measurement order,
unchanged source seals, output-family regeneration, exclusive outputs, early
cancellation, late pass failures and partial-evidence retention. Installed LLVM
integration covers disassembly replay and a mixed comparison (one disassembled
side, one raw export), preserving independent side policies. These tests use
injected measurement callbacks; they are state/contract tests, not performance
evidence. Actual live replay is separately recorded below.

Live `runs/family-replay-comparison-v1` and `runs/family-replay-history-v1`
each contain two sequential independent runtime passes and regenerated reports.
`runs/family-replay-history-recorded-v1` establishes real archived-builder replay.
`runs/family-replay-native-disassembly-v1` contains a new code pass with ten
successful measured Wasmtime/Winch compile trials plus new LLVM listings.
`runs/family-replay-native-comparison-v1` replays baseline/candidate sequentially
through its archived builder (ten successful measured trials in each), creates
new per-side exports/listings and regenerates the comparison. The source is
`reports/family-replay-native-comparison-source-v1`. These are one-launch local
workflow checks, not statistical performance or official publication evidence.
No builds/tests were run concurrently with the live measurement passes.

All resulting reports verify; the native comparison passes default and archived
verification, with HTTP 200 preview at `http://127.0.0.1:8131`. Full Go tests,
focused vet, publish race tests, build, installed LLVM integration and all 21 UI
interaction tests pass. Remaining full-product work includes source-family
measurement replay and all previously open advanced/platform/publication gates;
the original product goal remains active.

### Full-width runtime graph by default

The lifecycle report now opens in its expanded layout: one runtime row, a
segmented selected-stage latency bar, and separate stage-focused peak RSS bars.
The compact toggle remains available and keeps the current selections. Stage
selectors, compact time/RSS hover details, and clicks into exact evidence remain
unchanged. RSS retains its independent shared byte scale and is never summed;
the latency sum is illustrative, not an observed end-to-end measurement.

Graph v28 regenerates the saved timing and memory inputs from graph v27 without
new benchmark measurements. Its report verification and HTTP preview at
`http://127.0.0.1:8133/` pass. All 22 deterministic UI tests and the publish Go
tests pass. Browser layout testing was not performed.

### Source-family measurement replay and cross-build Parquet reproduction

Source, source-set and stack report replay now run fresh compiler measurements.
Successful source builds seal the exact nonexecutable runner and original total
tool timeout. Source benchmark replay preserves its admission/warmup/trial
schedule. Source-set/stack replay rebuild all artifacts before either runtime
pass; newly built bytes must exactly reproduce the original artifact and source
contract. Runtime input copies contain those rebuilt bytes and retain their
original locks and seals. Old build evidence cannot substitute for a rebuild.
Every pass preflights original snapshots, runner/tool/analyzer pins and host
environment before any compiler measurement or output creation. Legacy builds
without recorded timeout evidence explicitly refuse exact report replay.

Tests cover two-workload sets, including identical Wasm bytes under distinct task
bindings and shared artifact paths; relative bundle locators; host/environment
drift; changed archived runners; late preflight failures; ordered build/runtime
passes; unchanged source seals; and refusal to publish copied old build results.
Real LLVM API integration passed. The additional opt-in archived-CLI integration
builds a real CLI before measuring, creates real source/runtime inputs, and uses
a different controller executable to stage the original runners and exercise
`source-rebuild`, `source-bench-replay` and `reproduce` as actual subprocesses for
all three source families. It verifies exact staged hashes, fresh build markers,
original timeouts, generated reports and original seals. Its local run index is
disposable; it does not add test artifacts to the user's run index.

This subprocess regression initially failed source report verification despite
identical Parquet rows: parquet-go's default creator footer depends on Go build
metadata, which differs between CLI and test binaries. All sample, observation,
counter, throughput and source-table writers now use the explicit
`parquet-export-v1` creator. Row schemas and measurement semantics do not change.
Cross-build exact byte comparison and per-export metadata/determinism regressions
cover the fix. Historical export bytes remain preserved; default current-builder
verification may refuse them, while trusted original builders remain the exact
historical path. No verifier comparison was relaxed.

Earlier live source-family evidence includes `runs/source-report-replay-v2`,
`runs/source-set-report-replay-v1` and `runs/stack-report-replay-v1`, collected
sequentially without concurrent builds/tests. These are small local workflow
checks, not official performance results. Source preflight now normalizes
relative bundle locators before checking snapshots. Correctness admission also
normalizes only empty host-file maps (`nil` versus `{}`); nonempty host pins still
compare exactly and identity failures name differing fields.

The full product goal remains active. Native/whole-instance/COW snapshots,
physical reclamation qualification, observed tier/background-code lifetime,
broader ABI/platform/runtime coverage and dedicated-host official publication
gates remain unproven or incomplete. Source tools' transitive native dependencies
are not pinned or hermetic.

Validation after the writer fix: `WASMBENCH_SOURCE_LLVM_TEST=1 go test -p 1
./...`, focused vet, sourcebuild/publish race tests, CLI build and all 22 UI
interaction tests pass. Graph v29 and source report v3 were regenerated into new
directories without modifying their previous versions or collecting new graph
measurements. Both verify with the current CLI; old graph v28 and source report
v2 verify through their recorded builders. The graph v29 preview at
`http://127.0.0.1:8134/` returns HTTP 200; earlier previews remain alive.

Live `runs/source-report-replay-v3` reruns the source benchmark through source
report v3's archived builder and the original input's archived source runner.
Its new report passes both current and recorded-builder verification. This is
a one-block correctness/replay check, not a statistical performance claim.
No builds or tests ran during this live measurement pass. Three test-only run
index rows from the first CLI fixture were removed after a database backup at
`.wasmbench/source-replay-test-index-backup.sqlite`; subsequent CLI fixtures use
their own temporary index and leave the repository-local index untouched.

### Verified controlled V8 compiler modes

The runtime registry and build command now accept `v8-liftoff-only` and
`v8-optimizing-only` as distinct configurations. Both use exact ordered flag
sequences enabling testing intrinsics, disabling lazy compilation and disabling
tier-up. Baseline-only uses `--liftoff-only` (no optimizing fallback); the
optimizing configuration uses `--no-liftoff`. The normal `v8` launch flags remain
unchanged. Backend identities describe baseline-only/optimizing-only rather than
assert a particular optimizing IR pipeline.

The pinned `compiler-mode.mjs` helper refuses extra/missing/reordered flags,
duplicate/unknown adapter options and `NODE_OPTIONS` overrides in controlled
modes. Describe/prepare verify one separate fixed, import/start-free calibration
module using the installed V8's `%IsLiftoffFunction` and `%IsTurboFanFunction`.
Both code classifications are checked before the calibration export's first
call; its own return value is then verified. Missing native inspection syntax or
contradictory classifications fail. No benchmark workload export is invoked by
the probe. Description records the calibration version/module hash, observed
booleans, intrinsic/V8 identity and scope in its effective configuration.
`can_control_compiler_mode` is true only after successful verification. Node,
adapter, helper and native dependencies use the existing content-pin/archive
contract. Flags, backend, probe and policy therefore affect configuration identity.

This satisfies installed-build controlled-mode calibration, not general workload
tier observation. `can_observe_tiers` remains false; no background-compilation,
per-function tier-coverage, code-retirement or fully-materialized-module event is
invented. V8's default tiering/cache/GC behavior is not reclassified by these
separate experimental configurations. Calibration code/cache work is outside
workload timers but remains part of each adapter process and its memory footprint.

Tests exercise real V8 code classification, missing/contradictory inspection,
flag/environment refusal, helper pinning, default-vs-controlled descriptions,
and an adversarial five-call trajectory that fails on hidden workload calls.
Full Go tests with `WASMBENCH_V8_COMPILER_MODE_TEST=1`, experiment race tests,
focused vet, CLI/build workflow and 29 Node helper/UI tests pass. Source compiler
integration remains opt-in and was verified in the preceding section; this turn
does not newly qualify every adapter/ABI under the controlled modes.

Live `runs/v8-compiler-modes-{timing,memory}-v1` each have 18 successful measured
compile/instantiate/first-call trials: two core workloads and three configurations.
`reports/v8-compiler-modes-v1` joins those separate passes. Archived-builder
replay in `runs/v8-compiler-modes-report-replay-v1` collects both fresh passes
sequentially and its report passes current and recorded-builder verification.
The paired report preview at `http://127.0.0.1:8135/` returns HTTP 200; graph v29
also still verifies. No builds/tests ran concurrently with these live passes.
These one-launch/one-sample local checks are not performance rankings, confidence
qualification or official publication results. The full product goal remains
active with workload tier/background-code observation and the other advanced,
platform and dedicated-host publication requirements still incomplete.

## Workload-level V8 exported-entry tier boundary diagnostics

Added a dedicated `v8-tier-observed` adapter under the existing batch protocol,
tool pinning, archive and replay contracts. It preserves default compiler/lazy/
tier-up flags while enabling installed-build testing inspection. It refuses
extra flags and `NODE_OPTIONS`. A separate fixed calibration module establishes
that inspection works; scheduling-dependent calibration state is not part of
configuration identity. Runtime description advertises tier observation only
for this diagnostic configuration, not normal V8 or the controlled-mode probes.

One retained workload module/instance/export is called individually from its
first requested invocation, with explicit warmup only. Sacrificial admission
is a separate process with one uninstrumented first call. An integration check
caught and fixed the missing advertised admission scenario. Current coverage is
import-free stateless i32-argument/single-i32-result core workloads without
explicit initialization, host/memory fixtures or advanced state contracts.

Versioned sample evidence includes exact artifact/export identity, invocation,
before/after code state and monotonic query brackets, operation clocks, collector
and pinned V8 version. Queries are non-atomic. Inconsistent classifications remain
unavailable with reasons. Controller and stored-bundle validation bind scope,
version, oracle, operation count, sample order, warmup and nonoverlapping clocks
to the locked contract. Successful traces must have every requested sample and
reading; failed/unsupported results remain visible. Resealing forged collector
versions, omitted readings or changed warmup does not bypass semantic validation.
Nullable `tier_window_json` exports preserve the full record in Parquet v7.

Static reports show independent before/after code-state points by invocation
without connecting lines that would infer transition events. All invocation
windows, exact clock brackets, unavailable reasons, warmup, collector scope and
raw-trial links remain accessible in paged tables. Instrumented latency remains
diagnostic only. UI regression checks cover 1,001 samples, later pages, trial
switch reset, unavailable states, failed recorded traces and evidence immutability.

Verified full Go tests with both V8 opt-in modes, 30 Node helper/UI tests, focused
vet, focused tier race tests, CLI build and `build --runtimes v8-tier-observed`.
Real adapter checks use the adversarial five-call fixture to detect hidden calls;
the controller test covers generated core workloads and sealed Load round trips.
No tests/builds ran concurrently with the subsequent live diagnostic collection.

`runs/v8-tier-boundaries-v1` retains two successful 1,002-invocation traces
(1,000 measured diagnostics plus two explicit warmup calls per workload), and
two successful uninstrumented sacrificial admissions. Both recorded an
uncompiled-to-Liftoff boundary difference; the sum workload also recorded a
Liftoff-to-optimizing boundary difference. This is code-state evidence, not exact
transition timing, executed-tier attribution, or a headline performance claim.
`reports/v8-tier-boundaries-v1` verifies; archived-runner replay in
`runs/v8-tier-boundaries-report-replay-v1` collected fresh successful traces and
its report passes current and recorded-builder verification. The local preview
at `http://127.0.0.1:8136/` serves HTTP 200. Updated existing timing/memory graph
data is preserved in a new verified renderer/export revision,
`reports/five-configurations-lifecycle-graph-v30`, at port 8137; no old report
was overwritten and no new timing/memory performance measurements were needed.
Playwright browser QA verified opening evidence, changing the independent trial
and reaching invocations 1,001–1,002, with no console errors. The rendered tier
section screenshot is retained at `output/playwright/v8-tier-boundaries.png`;
both report previews return HTTP 200. The browser is closed; servers are retained.

The full goal remains active. This slice does not establish background compiler
activity, internal-callee tier coverage, exact tier transitions, code creation/
retirement/lifetime or fully materialized compilation. Native/whole-instance/COW
snapshots, physical reclamation/leak qualification, wider runtime/platform/ABI
coverage and dedicated-host official publication qualification remain open.

## Retained failed tier trajectories and explicit invocation outcomes

Tier evidence v2 adds `returned`, `oracle_mismatch` and `guest_trap` invocation
outcomes plus failure reasons. The adapter catches unexpected guest exceptions
within the call bracket, takes its after-code snapshot, marks the failed call
unverified and stops immediately. It returns the buffered completed prefix and
failed call in the error response. Oracle mismatches likewise retain returned
i32 bits without claiming verification. Unexpected exceptions in ordinary exact
workloads are not successful expected-trap oracle observations.

The controller now retains sample payloads on adapter errors as raw-only evidence
by default. For tier diagnostics only, a shared validator promotes a failure
prefix to typed diagnostic Samples after checking locked profile, narrow scalar
contract, budget, ordering, warmup, module/export, collector/version, clock
brackets and oracle consistency. A failed invocation must be terminal and its
reason/outcome must agree with the failed trial. It cannot masquerade as a
verified return or successful launch. Invalid payloads stay in AdapterSamples
and do not enter tier plots or typed sample exports. Stored-bundle Load applies
the same validator. Legacy v1 records remain supported without synthesizing
invocation outcomes. Typed scalar arguments and mismatch results must fit i32.

Reports add per-invocation verification, outcome, reason and returned bits to
the exact table and hover text, with red outlines for unverified call snapshots.
Code-state colors remain separate from execution outcome. Empty omitted result
arrays on the Go wire are displayed as None for guest traps, not measured zero.
The existing nullable Parquet v7 tier JSON column round-trips v2 failure fields;
no extra column or reinterpretation of old data was needed. Failed prefixes never
produce latency/throughput estimates, including their earlier verified calls.
Abrupt process death/timeouts can still prevent buffer delivery; missing evidence
is not reconstructed or claimed complete.

Full Go tests with both V8 opt-ins, 30 Node helper/UI tests, focused vet and tier
race tests passed. Tests cover real first-call oracle failures, a five-success/
sixth-trap trajectory, sacrificial admission plus controller/sealed-load failure
round trips, prefix budget/order and malformed terminal outcomes, verification
and reason binding, old-record compatibility, Parquet retention and analysis
exclusion. The new analysis test initially used a wrong field name, was corrected
and then passed with the full suite. Tests/builds completed before live collection.

Live `runs/v8-tier-failure-v1` uses
`output/tier-failure-fixture-suite.json`: an explicitly adversarial, deliberately
false stateless contract, NOT performance data. Admission succeeds once; the
measured process records five verified calls and the sixth-call unreachable trap,
including two explicit warmups. CLI run exits nonzero while retaining the sealed
bundle. `reports/v8-tier-failure-v2` verifies and keeps all six diagnostics with
null latency/throughput estimates. Archived-runner replay in
`runs/v8-tier-failure-report-replay-v2` reproduces the failure prefix; its report
passes current and recorded-builder verification. Earlier report/replay revision
v1 is preserved; its original renderer remains verifiable with the archived builder.

Fresh normal-core collection `runs/v8-tier-boundaries-v2` and its verified report
retain two successful 1,002-call traces using explicit v2 outcomes. The main graph
was regenerated from existing timing/memory evidence into a new verified
`reports/five-configurations-lifecycle-graph-v31`, without new timing/memory
measurements or overwriting older reports. Local previews are ports 8138 (failure
diagnostics) and 8139 (main graph). Playwright browser QA verified the failed
outcome, red-outline snapshot tooltips, no-returned-result label and six rows,
with zero console errors. Screenshots are local QA artifacts under
`output/playwright/`; browser closed and report servers retained.

This is additional failure/provenance hardening, not completion of the full goal.
Background compilation, code generation/retirement/lifetime, fully materialized
compilation, native/whole-instance/COW snapshots, physical reclamation/leak
qualification, broader runtime/platform/ABI coverage and dedicated-host official
publication qualification remain open.

### Native V8 category events: collected, sealed and replayed

Added a separately pinned `v8-tier-traced` diagnostic configuration. It uses
the installed Node inspector's `NodeTracing` API for `v8.wasm`, calibrated on
an independent constant-result module before advertising support. Compiler,
lazy-compilation and tier-up settings remain production defaults; the existing
native-inspection flag is still required. Admission first calls remain outside
tracing and tier inspection. This is not a timing configuration.

Native Chrome trace JSON retains event order, duplicate metadata, unknown native
fields, PID/TID, phases, microsecond timestamps, optional durations and arguments.
Exact decimal nanosecond collection bounds and a trajectory epoch bridge are
validated against the locked module, V8/Node versions, workload contract and
delivered tier-window bounds. Explicit collected/incomplete/unavailable outcomes
distinguish delivery from compilation completeness. Truncation retains only a
prefix within a 4 MiB bound; total calls including warmup are capped at 10,000
for traced trajectories, without restricting ordinary tier observations.
Inspector construction/start/stop failures and missing/malformed notifications
do not repeat or hide workload execution. Unexpected guest failures retain the
valid invocation prefix and native evidence when the adapter can deliver it.

Reports expose a 500-event window selector and exact native event table, with
content-addressed `.trace.json` downloads plus raw trial links. Download bytes
are validated against raw evidence during report verification. Native intervals
are not summed, assigned automatically to workload functions, interpreted as
code installation/retirement, or admitted to headline latency/throughput. Native
event-table Parquet export and a combined thread/trajectory timeline are not yet
implemented; native JSON remains the authoritative artifact.

Verified installed Node v26.4.0 / V8 14.6.202.34-node.21 on Darwin/arm64:

- Full Go suite and `go vet ./...` passed; focused real V8 controller tests
  passed with `WASMBENCH_V8_TIER_TEST=1`. Node collector and UI tests include
  malformed/missing completion, bounded prefix, workload failure, zero durations,
  unavailable evidence, later event windows and forged trace rejection.
- Fresh `runs/v8-engine-events-v1` retains two successful 1,002-call trajectories.
  Identity delivered 14 native events; sum delivered 16, including
  `wasm.ExecuteCompilationUnits` and `wasm.TopTierCompilation` intervals on
  native TID 7427, distinct from metadata-identified main TID 259. These are
  compiler-named process events, not exact module/function or code-lifetime
  attribution. Scheduling-dependent counts/thread IDs are not calibration keys.
- `reports/v8-engine-events-v1` passes current and archived-builder verification.
  Archived-runner replay in `runs/v8-engine-events-report-replay-v1` succeeds;
  its report passes current verification.
- Deliberate adverse fixture `runs/v8-engine-events-failure-v1` exits nonzero
  as expected, while retaining five verified calls and the unverified sixth
  trap plus 14 native events. Its report passes verification. This fixture is
  diagnostic failure evidence, never performance data.
- Playwright browser QA verified native rows, workload selector, top-tier event,
  failure prefix and download links, with zero console errors. Screenshot
  `output/playwright/v8-native-engine-events.png` was visually inspected;
  browser closed. Retained previews: port 8140 normal and 8141 adverse fixture.

The full objective remains active. This provides actual category-scoped engine
activity, but does not establish fully materialized compilation, module/function
event attribution or code creation/retirement/lifetime. Native/whole-instance/COW
snapshots, physical reclamation/leak qualification, broader runtime/platform/ABI
coverage and dedicated-host official publication qualification also remain open.

### Typed native-engine event export and raw-bound verification

Added `engine-events.parquet`, version `native-engine-events-parquet-v1`, to
every newly built experiment report. It contains a single `trial_outcome` row
for each profiling trial and separate ordered `native_event` rows. Traced
outcomes retain collection status/reason and versioned scope, identity, collector,
window, quality, category, trace digest and exact collection clock fields.
Missing delivery, unsupported runtime capabilities and untraced admission remain
explicit coverage rows. Collected empty traces have event count zero; unavailable
or absent traces have null counts. Trial event counts appear only on outcome
rows, not repeated on every event row.

Native fields use nullable typed integers: native microsecond timestamps and
durations, native PID/TID, and event indices. Collection bounds/trajectory epoch
are unsigned 64-bit nanoseconds, preserving values above JavaScript's exact
number range. Zero durations are not missing durations. Native arguments and
the original per-event JSON retain unknown fields, absent/null arguments and
duplicate metadata, rather than reconstructing a lossy native event object.
Requested experiment module identity remains separate from event attribution.
The export contains no guest-function attribution, summed compiler work or
headline performance eligibility claims.

Report datasets declare the export version. Verification writes the canonical
Parquet stream from validated copied raw evidence into a digest and compares it
with the sealed download. Resealing changed derived data does not bypass this
check. Both the native-event section and its typed download remain accessible
for profiling reports with no delivered traces, with an explicit missing-data
message. Existing reports are preserved and retain trusted archived-builder
verification; their schema is not rewritten.

Verified:

- Full Go suite, `go vet ./...`, 30 Node collector/UI tests, focused race tests
  and real V8 tier/trace tests pass. Parquet round trips test full uint64 clock
  precision, zero/null, duplicate native metadata, empty/unavailable/missing/
  unsupported/admission outcomes, unknown fields and empty typed files.
- Real controller/report integration rejects a resealed altered Parquet file
  while retaining validation of its native trace and workload contract.
- New `reports/v8-engine-events-v2` and adverse-fixture
  `reports/v8-engine-events-failure-v2` derive from retained v1 raw runs, without
  overwriting their earlier reports or recollecting headline performance data.
  Current verification passes; normal v2 archived-builder verification passes.
  Earlier normal v1 still verifies with its original archived builder.
- Archived-runner replay in `runs/v8-engine-events-report-replay-v2` succeeds;
  the replay report passes current and recorded-builder verification. This
  replay collects new diagnostic events, not identical scheduling/timing values.
- Retained preview at port 8142 serves the new normal report. Live HTTP checks
  confirm versioned dataset, download anchor, a successful 37,905-byte Parquet
  response and native Parquet header/footer. UI tests cover missing-data
  download accessibility; no new browser layout claim is made for this turn.

The full objective remains active. A combined elapsed-time view of calls, tier
snapshots and compiler-thread intervals is still open, as are exact module/
function attribution, code lifetime/materialization, whole-instance/COW
snapshots, physical reclamation/leak qualification, broader runtime/platform/ABI
coverage and dedicated-host official publication qualification.

### Shared elapsed-time diagnostic timeline

The native-event viewer now plots delivered call windows, non-atomic before/
after code-inspection brackets and native process events on one elapsed-time
axis. It uses exact BigInt clock-origin subtraction before approximate pixel
conversion; native event timestamps retain their microsecond source resolution.
Native setup events before the trajectory epoch can have negative relative
coordinates. A missing bridge with delivered calls refuses alignment. Call
clocks that are not exactly representable in this viewer also refuse plotting,
leaving raw evidence accessible.

Calls retain invocation/sample identity, diagnostic latency, explicit warmup and
failed outcome. Before/after brackets have their own lanes and state colors;
they are not connected into inferred transitions. Native phase X intervals use
native wall duration only; other phases remain unpaired point markers rather
than fabricated B/E intervals. Unknown fields, native thread CPU clocks/durations
and complete raw event objects remain in click details/downloads. No nested or
overlapping durations are added together or converted into headline metrics.

Each native PID/TID has separate visual overlap tracks. This prevents intervals
from hiding one another without claiming native nesting or worker ownership.
Consistent `thread_name` metadata supplies a displayed name; absent/conflicting
metadata remains explicitly unreported/conflicting. Requested module identity
does not become native event attribution. Metadata records remain table-only.
The table now includes original zero-based event indices so plot details can be
matched to exact delivered rows.

Independent 500-call and 500-event window selectors reach all delivered records
and reset on trial changes. The axis fits selected windows, with visible counts
and a display-approximation caveat. Subpixel intervals have a labeled minimum
marker width, not an increased duration. Hover/focus summaries are compact;
click or Enter/Space reveals exact source brackets and diagnostic scope. SVG
height follows lane count, with a bounded scroll viewport and readable split
thread labels rather than the generic 180-pixel chart height.

Verified:

- Full Go suite and vet passed; final renderer-focused Go tests and real V8
  controller/report integration passed after the last style change. All 34
  current Node collector/UI tests pass. Timeline tests cover origins above
  2^53, negative relative events, overlap tracks, zero duration, unpaired phases,
  metadata conflicts, missing/unsafe clock bridges, failed outlines, keyboard
  details, final-call windows, trial reset and evidence immutability.
- New reports `reports/v8-engine-events-v3` and
  `reports/v8-engine-events-failure-v3` regenerate the retained raw v1 runs;
  current verification passes. The normal report passes recorded-builder
  verification. Earlier reports remain untouched.
- Archived-runner replay `runs/v8-engine-events-report-replay-v3` succeeds; its
  report passes current and recorded-builder verification. Replay ran before
  browser QA, not concurrently with builds or browser work.
- Playwright browser QA selected the sum workload's native top-tier interval,
  hovered/clicked its marker and verified exact relative bracket
  `[441042, 741042] ns`, native PID/TID 75149/7427 and unreported thread role.
  It reached calls 1001–1002, each with optimizing entry snapshots. This is
  native process activity plus separate code snapshots, not proof of executed
  tier or module/function-specific compilation.
- Adverse-fixture QA activated the sixth call with click and Enter, retaining
  `guest_trap`, unverified outcome, exact bracket `[96917, 108084] ns`, no
  returned bits and three failed-outline markers. This remains a deliberately
  failing correctness fixture, never headline performance evidence.
- Both browser views had no page overflow or console errors. Screenshots
  `output/playwright/v8-shared-timeline.png` and
  `output/playwright/v8-shared-timeline-failure.png` were visually inspected;
  browser closed. Retained previews are port 8143 normal and 8144 adverse.

The full objective remains active. This closes the shared observable-category
timeline gap, not exact module/function attribution, executed-tier proof,
fully materialized compilation or code creation/retirement/lifetime. Native/
whole-instance/COW snapshots, physical reclamation/leak qualification, broader
runtime/platform/ABI coverage and dedicated-host official publication
qualification remain open.

### Wasmtime fully materialized compile diagnostic

- Added `compile-materialized` for pinned Wasmtime 46.0.1 Cranelift and Winch.
  One code-pass compile per independent launch, no warmup or phase barriers;
  only stateless exact-result core workloads without compound fixtures.
- Version 3 native images retain the synchronous API completion contract,
  independent parser identity, imported/defined function counts and exact
  diagnostic compile timer. Every defined function must have a unique bounded
  native range, including unexported functions; imports are excluded.
- Collection checks counts against the independent analyzer before accepting
  a trial. Offline bundle loading repeats the gate and checks locked runtime,
  backend, capability, profile, sample sequence and timer consistency.
- Native export, expansion and comparison accept version 3. The browser shows
  the completion proof and raw input evidence, explicitly excluding lifetime,
  creation/retirement, instruction-only size and reclamation claims.
- Verification: `go test ./...`, `go vet ./...`, all 28 renderer tests, three
  Rust native-code tests, and the real opt-in controller round-trip test passed.
  The integration test rejects a shifted, internally coherent index mapping
  against the input analyzer, as well as missing completion proof.
- Fresh `runs/wasmtime-materialized-v1` records four successful native exports
  (two core workloads across both backends) and two unsupported V8 cells.
  `reports/wasmtime-materialized-v1` verifies with both current and archived
  builders. Browser QA opened its function detail and confirmed the proof,
  independent input link, diagnostic-only timer, no page overflow and no
  console errors. Preview retained at port 8145; browser closed.

The full objective remains active. This adds a verified materialized-compile
scenario, not native code lifetime or physical reclamation. Native/whole-instance/
COW snapshots, leak qualification, broader runtime/platform/ABI coverage and
dedicated-host official publication qualification remain open.

### Rust native allocator diagnostic builds

- `wasmtime-allocator` and `wasmtime-winch-allocator` resolve to a separate
  Cargo `native-allocator` feature build under `target/allocator`. Ordinary
  timing binaries have no global allocator hook. The diagnostic configurations
  advertise memory-only capability and reject timing passes without samples.
- The allocation-free synchronized accounting wrapper delegates to Rust
  `System`. It records successful layout requests across all threads, logical
  release sizes, outstanding requested bytes at API entry/return and maximum
  outstanding bytes at serialized accounting updates. Successful realloc is a
  full new-size request plus old-size logical release, including in-place
  realloc. Failed requests do not change counters. Overflow/inconsistency or
  values outside exact JSON-number range fail closed, not as zero.
- Scope explicitly excludes mmap/foreign allocators, allocator usable sizes,
  transient internal realloc copies, guest-only state, physical residency and
  reclamation. The hook observes actual routed calls, not source-level
  allocation expressions that a compiler may eliminate. No forced purge.
- Single-operation memory-pass compile, instantiate, first-call and steady
  requests support stateless exact-result core workloads without compound
  fixtures, warmup or barriers. Setup, verification and resource release are
  outside the API counter window. Broader allocator windows and explicit
  post-release/reclamation qualification remain open.
- Controller and offline bundle validation check six namespaced observation
  identities, exact integer values, completeness/uniqueness, profile/scope,
  request accounting balance and high-water bounds. Ordinary runtimes cannot
  claim those fields. Allocator operation evidence must be sample-qualified.
- The report has a dedicated trial/window table and raw links, preserving zeros
  and missing evidence separately. It never treats the instrumented timers as
  headline latency or joins different binaries/configurations to timing data.
- Verification passed: full Go suite and vet; all 30 renderer tests; three Rust
  accounting tests (including 8,000 requests across eight workers); real
  `WASMBENCH_RUST_ALLOCATOR_TEST=1` integration covering 16 successful cells and
  rejection of timing passes; actionlint. Linux CI build/test/integration steps
  are wired but no remote CI run is claimed.
- Fresh `runs/wasmtime-rust-allocator-v1` has 48 measured samples across both
  backends, two core workloads and all four operations. Final
  `reports/wasmtime-rust-allocator-v3` verifies with current and archived builders.
  Browser QA caught and fixed the serialized runtime-field mismatch and mobile
  select overflow. Final QA confirms 16 trial options, three sample rows,
  switching to trial 15 with its exact raw link, no overflow at 390px and no
  console errors. Raw trial 15 keeps first-call allocation (64 bytes/one
  request) followed by zero-request steady samples; no hidden prewarm.
  Screenshot
  `output/playwright/wasmtime-rust-allocator-mobile-v3.png` visually inspected;
  browser closed. Final preview is port 8148. Earlier v1/v2 report iterations
  are preserved, not presented as the verified final UI.

The full-product objective remains active: allocator instrumentation is a
declared diagnostic domain, not whole-host allocation or physical reclamation.
Native/whole-instance/COW snapshots, code lifetime, leak qualification, wider
runtime/platform/ABI coverage and dedicated-host official publication remain
unfinished.

### Rust allocator post-verification release windows

- Dedicated allocator builds now advertise `can_rust_allocator_release_windows`
  and a versioned, locked `rust-logical-release-v1` policy. Compile drops the
  measured Module while retaining its engine. Instantiate and first-call drop
  measured Store/Instance state and prepared import handles while retaining
  module/engine. Steady retains its Store until the final sample.
- Seven `host.rust.release.*` observations describe a separate window after
  correctness verification: requested bytes/count, logical freed bytes,
  outstanding entry/end, observed high-water and diagnostic drop elapsed time.
  Its entry baseline need not equal the API-return baseline. Observation
  encoding and buffered output are outside the release window; retained process
  allocation counts therefore are not interpreted as leaked guest state.
- Earlier steady samples carry explicit `not_applicable` observations without
  values. Controller/offline validation requires complete release evidence on
  final samples, rejects wrong domains and accounting, and validates the exact
  locked sample sequence. Legacy allocator bundles remain readable without
  synthesizing release measurements.
- The report exposes separate API and release tables, a declared drop policy,
  retained markers and exact raw links. These are logical allocator observations,
  not allocator purges, physical reclamation, or whole-process memory attribution.
- Verification: four Rust accounting tests, 31 renderer tests, full Go suite/vet,
  and the real adapter integration pass. The integration reads report Parquet
  and checks 280 available release values against raw samples, 56 typed-null
  retained observations, and their phase/domain/provenance. It also rejects
  release fields in API windows and altered sample order.
- `runs/wasmtime-rust-release-v1` preserves 16 measured cells / 48 samples across
  two backends, two workloads and four scenarios. The sealed report verifies at
  `reports/wasmtime-rust-release-v1`; preview is port 8149. Browser QA selected
  trial 15, checked its exact raw link and two retained rows followed by the
  final measured drop, and found no overflow at 390px or console errors.
  `output/playwright/wasmtime-rust-release-mobile-v1.png` was visually inspected;
  the browser was closed. This extends allocator retention evidence but does
  not complete physical-reclamation or leak qualification.

### Allocator diagnostics for floating and initialized core workloads

- The dedicated Wasmtime/Winch allocator build advertises the existing
  `can_verify_float_bits_v1` contract and uses the same typed argument/result,
  tolerance, NaN, infinity and signed-zero verification as the ordinary adapter.
  It accepts seven generated floating fixtures, including mixed argument types
  and multi-value results, without converting their result bits to JSON numbers.
- Compile, instantiate and first-call now accept fresh-instance-per-sample
  scalar core contracts. Explicit initialization and input injection remain
  outside their measured API windows; memory-output oracles run before the
  separate release window. Wasm start remains inside instantiation. Fresh-state
  steady execution is still explicitly unsupported, not silently reused.
- A shared workload gate enforces scalar core/reset constraints in controller
  preflight and offline allocator evidence validation. This prevents unsupported
  fresh steady requests from being presented as adapter errors. Wasmtime float
  mismatches now use the controller's incorrect-result classification marker.
- Real integration covers 56 floating cells and six initialized-workload cells
  across both backends; two fresh-steady cells are unsupported without samples.
  Direct wrong-oracle requests are rejected at every supported API boundary.
  Controller sacrificial checks mark wrong float values/memory contents as
  `incorrect_result`, and all corresponding measured cells are preflight-failed.
  Existing core integration still verifies sample-qualified release Parquet.
- Verification passed: full Go suite/vet, 31 renderer tests, actionlint, all
  22 instrumented Rust tests and 18 ordinary Rust tests. Both release binaries
  were rebuilt. Ordinary Wasmtime/Winch float integration also passed. CI now
  selects both allocator integration tests; remote CI execution is not claimed.
- Fresh sealed runs/reports `wasmtime-rust-floats-v1` and
  `wasmtime-rust-app-init-v1` retain 168 and 18 measured samples respectively.
  Both reports verify with current and recorded builders. Exact negative-zero
  result bits remain string `9223372036854775808`; neither dataset contains
  headline timing summaries. Previews are ports 8150 and 8151.

The full-product goal remains active. This adds workload breadth to a declared
allocator domain, not physical-reclamation/leak qualification, native snapshots,
code lifetime, complete ABI/platform coverage or official host qualification.

### Allocator API/release boundaries with process residency

- New allocator descriptions advertise `can_rust_allocator_phase_boundaries`,
  four supported barrier scenarios and locked `rust-allocator-boundaries-v1`.
  The sequence is before API, API returned, after verification/before drop, and
  after the release decision. Steady's last boundary deliberately does not say
  released: non-final samples retain their Store, and only the final drops it.
  Existing ordinary adapter three-boundary protocols are unchanged.
- All barrier calls are outside both Rust counter/timer windows. A real Rust
  adapter test allocates transport buffers in callbacks and checks the global
  tracker is inactive there, across both backends, four scenarios and phased/
  unphased requests. Unphased requests emit no unsolicited handshakes.
- Controller collection attaches four procfs snapshot outcomes at each boundary
  and preserves their scope/quality independently from allocator observations.
  Cgroup API peak/CPU accounting remains bracketed by only the first two
  boundaries; release snapshots neither rearm that peak nor claim release CPU.
- Offline validation requires the locked boundary protocol, complete ordered
  records, explicit process snapshot availability/provenance, and exact binding
  between each record's observations and the sample's attached evidence. Tests
  reject relabeled verification/release boundaries and detached sample evidence.
  Successful legacy/unphased allocator bundles require no added boundaries.
- The allocator report adds an independently labeled RSS/PSS/private/virtual
  boundary table, retained/final-drop states, explicit unavailable outcomes and
  a raw link. Snapshots are not allocation volume, exact phase peaks, attributed
  guest memory, or proof of physical reclamation. Barrier transport can affect
  process footprint. Mobile QA narrowed the sample column and added a named,
  keyboard-focusable horizontal scroll region; missing release evidence is not
  rendered as Store dropped.
- The real controller integration passes for core, all seven float fixtures and
  initialized core workloads, with and without barriers. Existing wrong-oracle,
  reset-policy and timing-profile rejection tests still pass. Verification also
  passed full Go tests/vet, 23 instrumented Rust tests, all 32 renderer tests,
  actionlint and an opt-in sealed Linux boundary-evidence gate. Remote CI has
  not been run; its existing allocator test selector includes the new test.
- A read-only, network-disabled, cap-drop-all Linux/arm64 container collected
  `runs/linux-rust-allocator-boundaries-v1/core`: 16 successful measured cells,
  48 samples, 192 boundaries and 768 available RSS/PSS/private/virtual snapshots.
  All 48 cgroup phase peaks are explicitly unavailable: no privileged cgroup
  setup or isolation claim was made. Docker's shared VM is exploratory, not a
  dedicated official measurement host. Builds finished before collection.
  The Linux instrumented binary was copied to `.wasmbench/allocator-linux/`;
  the ordinary Linux adapter was rebuilt back at its normal recipe path.
- Final `reports/linux-rust-allocator-boundaries-v2` verifies with both current
  and recorded builders. Browser QA selected trial 15 and checked 12 boundary
  rows, two retained states followed by final Store drop, exact raw link, no
  overflow at 390px, keyboard-focusable scrolling and zero console errors.
  `output/playwright/linux-rust-allocator-boundaries-mobile-v2.png` was visually
  inspected; browser closed. Final preview is port 8153; v1/report port 8152 and
  all earlier artifacts remain preserved.

The full-product goal remains active. This establishes separately qualified
allocator and process-boundary evidence, not reclamation/leak qualification,
native/whole-instance/COW snapshots, code lifetime, complete ABI/platform
coverage or dedicated-host official publication.

### Engine-native continuation snapshot measurement module

- Added a deterministic recursive Wasm artifact generator and a shared wazero
  measurement module using the pinned engine's native experimental capture and
  restore methods. It measures creation, restore-to-guest-resumption, first
  post-restore write and subsequent full-memory execution with distinct clocks.
- The restore clock ends at a guest-resumption marker because the native restore
  method never returns normally. It includes that marker transition and is not
  relabeled as embedding API-return latency. Creation excludes snapshotter
  lookup; restore excludes construction of its supplied return-value slice.
- Oracle checks restored recursive parameters, every memory byte and the scalar
  global. Memory/global changes deliberately survive stack restoration. The
  module records two full-memory digests, discards captures at invocation return
  and clears all results on correctness, cancellation or release errors.
- Real compiler qualification covers depths 0, 1, 8, 32 and 128 with three fresh
  instances each. The pinned interpreter fails the guest-resumption oracle at
  all five depths; its interface availability is not advertised as correctness.
  Negative tests cover unrelated memory corruption, cross-instance captures,
  invalid depths, cancellation and measurement after close.
- This is implementation qualification, not a released experiment family.
  Existing `can_snapshot: false` and eager guest-checkpoint definitions remain
  unchanged. See `docs/NATIVE_CONTINUATIONS.md` for precise clock contracts and
  the remaining admission/protocol/memory/export/report integration gates.
- Verification passed: `go test ./...`, `go vet ./...`, and fresh focused native
  continuation/generator tests after the final test cleanup. Measurements here
  are correctness-test diagnostics, not collected performance-run evidence.

The full-product goal remains active. Native continuation integration, whole-
instance/COW snapshots, restored density, physical reclamation/leak
qualification, code lifetime, broader ABI/platform coverage and dedicated-host
publication remain unfinished. No performance comparison is claimed from tests.

### Native continuation experiment integrated into the product workflow

- Added the versioned `continuations` suite and typed workload/result contracts.
  Five deterministic recursive-depth artifacts pass pinned independent analyzer
  admission. Controller preflight uses two sacrificial resumption samples;
  measured batches require one operation, no warmup and an explicit stage.
  Compiler-only capability/build/backend/protocol gating leaves the pinned
  interpreter unsupported. Generic `can_snapshot` stays false.
- Four scenarios keep capture, restore-to-guest-marker, first write and
  subsequent checksum execution distinct. Selected-stage barrier and Go memory
  collection callbacks live outside clocks, including inside resumed host code.
  Captures are discarded at active invocation return; verified fresh instances
  close before the release barrier. Compiled module/engine remain held through
  the batch. Native continuation release policy is explicitly distinguished.
- Offline validation binds canonical bytes, qualified identity, sequence,
  full-memory/global/stack proof, seven allocator/heap/GC observations and
  ordered boundaries. Each boundary must preserve four explicit procfs outcomes
  with correct provenance; attached sample observations must exactly match the
  raw phase records. Foreign/forged state proofs and detached snapshots fail.
- Sample Parquet v8 adds nullable continuation proof JSON, with round-trip and
  unsupported-null tests. Memory stage analysis accepts only the declared Go
  operation window. Resumption scaling uses its own measurement scope instead
  of embedding API-return latency. Report stage labels, explanatory notes and
  native chart title preserve that distinction; all four stages start selected.
- Fresh sequential CLI passes at `runs/native-continuation-v1/{timing,memory}`
  retain 60 successful measured compiler trials / 120 samples and 60 unsupported
  interpreter trials per pass. Memory has 360 measured boundary records. Seven
  Go-domain observations are separate from OS/cgroup outcomes. Darwin procfs
  residency is unsupported, not zero; stage-focused whole-process peak RSS is
  available separately. No builds/tests/browser QA overlapped either pass.
- Full Go tests/vet, 33 renderer tests, adapter stage/profile/barrier tests,
  forged offline evidence tests and optional paired sealed-run coverage gate
  passed. Final `reports/native-continuation-v3` verifies with both current and
  recorded builders and is served at port 8156; v1/port 8154 and v2/port 8155
  remain preserved.
  Browser QA confirmed default four stages, exact resumption click/raw links,
  compact hover time/RSS, interpreter gaps, zero console messages and no page
  overflow at 390px. Mobile screenshots are in `output/playwright/` and were
  visually inspected; the browser was closed. Visual QA caught unavailable RSS
  cells inheriting a default full-width colored fill: the renderer now omits
  those fills, with a regression assertion and fresh v3 browser confirmation
  of zero missing-cell fills, zero console messages and no 390px overflow.

The full-product goal remains active. This completes the scoped native stack
continuation workflow on the exploratory Darwin/arm64 host, not whole-instance
or COW snapshots, restored whole-instance density, physical reclamation/leak
qualification, code lifetime, broader ABI/platform coverage or dedicated-host
publication. Linux/native-AMD64 continuation qualification and native snapshot
storage-size observation remain open; the public interface exposes no size.

### Linux continuation qualification, replay and derived reports

- The unprivileged Linux recipe now finishes the full derived-artifact workflow:
  sequential timing/memory passes and locked reproductions, sealed validation,
  native coverage/residency gates, then distinct original/replay reports verified
  with current and archived builders. CI uploads the scoped directory including
  both reports. Remote CI was not executed here.
- Corrected Linux/arm64 evidence at `runs/linux-native-continuation-v3` retains
  matching host identity across original and replay passes inside one container.
  Each pair has 60 successful compiler trials, 60 unsupported interpreter trials
  and 120 samples per profile. Each memory pass has 360 boundaries and 1440
  available procfs RSS/PSS/private/virtual snapshots. All 120 cgroup phase peaks
  are unavailable. Native stack restoration is not whole-instance restoration.
- The reports under that bundle's `reports/{original,reproduced}` were generated
  after all measurement finished and verified inside a restricted Linux
  container. Archived-builder execution exposed the default non-executable
  `/tmp` mount; the recipe now permits execution there with `nosuid,nodev`, while
  retaining network-disabled, cap-drop-all and read-only root restrictions.
  The same command passed on retest. No existing evidence was overwritten.
- Coverage/residency Node gates and opt-in Go evidence tests passed for both
  pairs. Full Go tests/vet, shellcheck, shell syntax, Node syntax and actionlint
  passed. The Linux recipe and exact measured coverage/remaining limits are now
  documented in `docs/NATIVE_CONTINUATIONS.md`.
- Runtime chart v32 adds the same compact time/RSS tooltip to RSS buttons as to
  stage timing segments. All 33 renderer tests and publish tests passed; browser
  checks confirmed five runtime rows, exact RSS raw links, no 390px page overflow
  and zero console errors. The sealed paired report remains on port 8158.

The full-product goal remains active: whole-instance/COW snapshots, restored
whole-instance density, physical reclamation/leak qualification, code lifetime,
broader ABI/platform coverage and dedicated-host publication remain unproven.
Linux/arm64 qualification is verified; native AMD64 and remote CI are not.

### Native executable-text publication and retirement collector qualification

- Added opt-in Wasmtime `native-code-lifetime` feature and a Linux collector
  behind the public executable-memory publication hooks. Ordinary engines do
  not install it and no released capability/scenario is advertised yet. The
  ledger records unique publication identities, relative clocks, ranges, active
  published capacity and cumulative published capacity, not compiler emission,
  instruction-only sizes or physical reclamation.
- The custom publisher maintains W^X and the pinned engine's cache/pipeline
  coherence sequence. Initial Linux qualification exposed BTI on the actual
  host; the configuration method now pins matching codegen and page protection
  and both Cranelift/Winch execute correctly. Cache coherence uses an explicitly
  unsupported internal engine crate pinned to 46.0.1; upgrades require renewed
  safety/engine qualification, not assumed compatibility.
- Five real Linux/arm64 tests pass for both backends: Store-owned code remains
  callable after Module handles drop; actual unpublication occurs after Store
  drop; simultaneous modules and repeated publications retain exact distinct
  identities/totals; invalid callbacks preserve evidence; invalid and empty
  modules invent no events. Procfs verifies private RX executable protection.
  An empty-image test caught Wasmtime's zero-length retirement-only callback;
  handling it as a no-op now passes without fabricating a zero-sized range.
- Added `recipes/wasmtime-linux.sh test-code-lifetime` and Ubuntu CI selection.
  Local native AMD64 and remote CI are not verified. Tests are qualification,
  not measured performance runs. See `docs/NATIVE_CODE_LIFETIME.md` for exact
  scope, commands and the unfinished integration gates.

The full-product goal remains active. Locked diagnostic scenarios, module/range
attribution, wire validation/export and report lifecycle views are still needed
for this collector. Whole-instance/COW snapshots, restored density, physical
reclamation/leak qualification, broader ABI/platform coverage and official
publication remain separate incomplete requirements.

### Native executable-text lifecycle product integration and replay

- Added separate `wasmtime-code-lifetime` and `wasmtime-winch-code-lifetime`
  builds and a bounded `code-lifetime` code-profile contract. Ordinary timing
  engines remain uninstrumented. A fresh import-free core module is compiled,
  instantiated and oracle-verified, then verified again after all Module
  handles drop before Store and engine release. No timing samples are produced.
- Version-1 typed evidence binds the module and native-image digests, backend,
  architecture, host page size, publication range and complete function coverage.
  Decimal-string addresses preserve exact values above JavaScript's integer
  limit. Controller and offline validation reject inconsistent ownership,
  invented retention, foreign scenarios, wrong oracles, timing samples and
  missing independent input admission. Added explicit regression cases for
  absent/mismatched host page sizes and swapped backends.
- Added sealed `code-lifetimes.parquet` export with trial outcomes, nullable
  publication/checkpoint fields and exact addresses. Report verification
  recomputes the export from raw evidence, rejecting derived-file changes even
  after checksum resealing. The report shows active versus cumulative published
  capacity, publication events, ownership checkpoints and exact raw trial links.
  Diagnostic elapsed columns use unit-aware formatting and expose exact ns on
  hover, rather than incorrectly labeling formatted milliseconds as ns.
- `runs/linux-code-lifetime-v1/{original,reproduced}` each retain twelve successful
  measured trials, 24 callback events and 60 ownership checkpoints. Linux/arm64
  collection and replay ran sequentially in one restricted container. This is
  exploratory shared-VM evidence, not dedicated-host performance qualification.
  The Linux controller/sealed-report round trip passed; six real-engine collector
  tests cover both Cranelift and Winch. Native AMD64 and remote CI remain unrun.
- Full Go tests and vet passed; both sealed bundles and their forged-evidence
  rejection cases passed. All 34 UI tests, shellcheck, actionlint and Rust format
  checks passed. The final original report at `reports/linux-code-lifetime-v3`
  verifies with current and archived builders and is served on port 8161.
  Browser checks confirm selectable final-trial raw links, real retirement
  zeroes, three keyboard-focusable scroll regions, exact clock hover values,
  no 390px page overflow and no console errors. Earlier reports are preserved.
- The documented native Linux CLI builds, collects, replays, validates and
  renders this scenario. The pinned Docker build recipe emits a separate Linux
  binary; it does not itself perform collection or replace ordinary adapters.
  Ubuntu CI now includes a separate diagnostic build and controller/report test.

The full-product goal remains active. Publication capacity is not total compiler
emission or physical reclamation. Multiple-generation/host-bridge attribution,
whole-instance/COW snapshots, restored whole-instance density, reclamation/leak
qualification, broader ABI/platform coverage and official dedicated-host
publication remain unfinished and must not be inferred from these scoped gates.

### Packaged native code lifetime workflow and cross-runtime regression

- The product image now includes the separate Linux Wasmtime/Winch diagnostic
  binary alongside the independent analyzer. Added
  `recipes/test-linux-code-lifetime.sh`: resolves an immutable local image ID,
  rejects emulation and existing evidence paths, then collects, validates,
  replays and generates two independently verified reports in one restricted
  container. No host binary overrides are required. Optional read-only overrides
  remain available for adapter development. Builds finish before collection.
- The independent coverage gate requires all twelve core/backends/launch cells,
  zero timing samples, two callback events and five ownership checkpoints per
  trial, final zero active capacity and identical observed original/replay host
  identity. It supplements, never replaces, sealed protocol/source validation.
  Twelve portable gate tests reject incomplete or forged coverage; four shell
  recipe tests prove preservation of existing directories/dangling aliases and
  rejection of traversal/emulation before container launch/evidence creation.
- Fresh image `wasmbench:code-lifetime-package-v2`, immutable local ID
  `sha256:e5cf186c7634a494b1e901e9dba34ca64c5b57f5fe147f5a2b539dd019c8ee6e`,
  passed Linux/arm64 collection and replay without binary overrides. Evidence at
  `runs/linux-code-lifetime-packaged-v2/{original,reproduced}` contains twelve
  successful measured trials, 24 publication events and 60 ownership checkpoints
  per run. Both reports under `reports/{original,reproduced}` verified with
  current and archived builders. Current-host verification and forged-evidence
  Go tests also passed for both bundles. Original report preview: port 8162.
- Broader packaged regression caught an existing missing V8 `compiler-mode.mjs`
  helper. The image now includes all ordinary/tier/tracing runtime helpers;
  smoke validation checks every ordinary helper's pinned hash. Corrected the
  old gate's analyzer lock expectation to `artifact-structure-v1`, retaining
  explicit core report validation as `core-structure-v3`. Failed smoke evidence
  and both earlier image tags remain preserved.
- The corrected image passed `recipes/test-container.sh` at
  `runs/code-lifetime-image-smoke-v2`: core correctness/replay/reports, 147
  measured float trials, 120 successful density cells, 24 explicit unsupported
  cells, 240 live-group RSS snapshots and 30 paired marginal intervals. No
  builds/tests/browser activity overlapped those measurement passes.
- Full Go tests/vet and both images' internal Go suites passed; all 63 V8/UI/
  recipe JavaScript tests, actionlint, shell syntax and shellcheck passed.
  Mobile browser QA confirms final-trial raw links, real retirement zeroes,
  three scroll regions, no 390px page overflow and no console errors. CI now
  runs the packaged code workflow and uploads its evidence; remote CI and native
  AMD64 were not executed here. The codebase-design skill kept the coverage
  gate independent of collection and testable through one exported interface.

The full-product goal remains active. These shared-VM functional diagnostics do
not establish physical reclamation, total compiler emission, dedicated-host
publication, whole-instance/COW snapshots, restored whole-instance density or
the remaining ABI/platform breadth.

### Process-level COW snapshot functional qualification

- Added an opt-in, standalone Linux `qualify-process-snapshot` binary for pinned
  Wasmtime 46.0.1 Cranelift and Winch. It owns a single-threaded embedding,
  disables parallel compilation, and checks `/proc/self/task` before each fork.
  A separate guard probe proves multithreaded rejection without forking that
  process. Ordinary adapters and generic `can_snapshot` remain unchanged.
- A quiescent import-free fixture retains hidden global/table state, grown
  memory/table sizes and live passive segments in a forked template. The source
  mutates and drops passive segments, then releases its Store, Module and Engine
  before restoration. Each backend's two independent restored children verify
  original state and passive segments, mutate independently and exit before
  proof publication. Exact proof values are 64 before mutation, 125 after,
  127 for the passive-segment probe and three memory pages/table elements.
- The pinned Linux build recipe writes a separate target directory; ordinary
  builds do not enable the feature. The restricted qualification recipe retains
  exact Wasm, independent validated core structure, thread guard, backend proofs,
  immutable image identity and checksummed receipt. The receipt is consistency
  evidence, not a signature, sealed run bundle or performance authorization.
- Hardened the recipe to reject mutable/malformed image identities, non-Linux
  images and emulation before creating evidence. Eight shell-recipe tests cover
  these cases, existing-directory and dangling-alias preservation, and traversal.
  Seventeen typed evidence tests reject changed scope, timing claims, threads,
  input admission, backend coverage, source ownership and restoration proofs.
- Real sequential Linux/arm64 qualification at
  `runs/linux-process-snapshot-qualification-v1` and `-v2` passed both backends.
  All six evidence files and receipts are byte-identical across fresh runs.
  The immutable image is
  `sha256:e5cf186c7634a494b1e901e9dba34ca64c5b57f5fe147f5a2b539dd019c8ee6e`;
  the qualifier binary is
  `b4a8a9cbe15a05e8394dc92dc0f0b35aeaa2ea995326971274b60d78899d1169`.
  These are shared-VM functional observations, not dedicated-host performance.
- Full Go tests and vet passed. All 88 V8/UI/recipe JavaScript tests passed, along with shellcheck, actionlint
  and Rust formatting. The ordinary Darwin/arm64 adapter's 18 Rust tests passed
  during qualifier development. CI now separately builds/runs the qualifier;
  remote CI and native AMD64 have not been executed locally. The scope, safety
  constraints, commands and remaining gates are in `docs/PROCESS_SNAPSHOTS.md`.

The full-product goal remains active. This proves a narrowly scoped Linux
process-cloning candidate, not an engine snapshot API or general whole-instance
checkpoint. Timed capture/restore/first-write/execution, locked tool archival and
replay, child-inclusive memory accounting, simultaneous restored density,
failure/cleanup qualification, exports/report integration and host-resource/ABI
breadth remain incomplete. Physical COW costs and reclamation cannot be inferred.

### Process snapshot failure cleanup and exact qualifier retention

- Replaced the restored-child blocking wait with a shared, bounded proof/exit
  receiver and owned-child guard. Complete bytes, incorrect state or nonzero
  exit cannot become successful evidence. Wait polling retries EINTR; ECHILD
  clears ownership so cleanup cannot signal a reused PID. Error paths terminate
  and reap only their owned children. Polling deadlines do not claim guarantees
  against uninterruptible kernel stalls.
- Every fork descendant arms Linux parent-death SIGKILL and rechecks its parent
  identity. A dedicated single-threaded subreaper probe verifies termination of
  an adopted grandchild after its direct parent exits, and reaps it. The subreaper
  is confined to the standalone cleanup test, not ordinary runtime operation.
- Added native fault injection using the real proof receiver: one positive
  control plus missing/partial/wrong proof, complete proof followed by nonzero
  exit, complete proof followed by a stall, and a stall without proof. All six
  failures must be rejected and all seven direct children reaped. The parent-death
  case also must prove SIGKILL and reaping before cleanup evidence is emitted.
- Versioned the stronger cleanup contract as
  `linux-process-snapshot-cleanup-v2` and the qualification receipt as version 3.
  New bundles archive and execute the exact qualifier binary, bind its digest to
  the mounted build, and require cleanup evidence before issuing a receipt.
  Older development runs remain preserved and are not upgraded by inference.
  The Docker image/dynamic dependency closure is not archived by this recipe;
  this is not yet a sealed benchmark replay workflow.
- Added ten cleanup-validator tests, four receipt CLI tests, and a canonical
  Node-to-Docker architecture test. Fixed the actual x64/amd64 naming mismatch
  that would otherwise reject an AMD64 receipt. Synthetic CLI fixtures test
  consistency only; real-engine qualifications provide the functional proof.
- Fresh Linux/arm64 restricted-container runs at
  `runs/linux-process-snapshot-cleanup-v3` and `-v4` passed all native fault tests,
  parent-death cleanup, the multithreaded guard, and both backends' guest-state
  restoration. All eight evidence files, including archived qualifier and receipt,
  are byte-identical across these runs. Offline receipt verification passed.
  Final qualifier SHA-256 is
  `c920c0e18d1af4396ed01efc1227fc0534fd384b44c5aa8bb70d25e814177772`.
- All 103 V8/UI/recipe JavaScript tests, full Go tests/vet, shellcheck, actionlint
  and Rust format checks passed. All 18 ordinary Rust adapter tests and six
  independent analyzer tests passed on Darwin/arm64. The Linux qualifier builds passed. CI now runs
  the native failure probe, but remote CI/native AMD64 have not been executed.

The full-product goal remains active. This closes the scoped supervisor failure
gate, not arbitrary guest/runtime fault qualification. Timed process-snapshot
scenarios, exact dependency-closure archival and locked replay, child-inclusive
memory accounting, simultaneous restored density, exports/report integration,
host-resource/ABI breadth and official publication remain required work.

### Archived process snapshot functional replay

- Added `recipes/replay-linux-process-snapshots.sh`. It uses the retained
  qualifier executable and exact local image ID, never a current build or image
  tag. A trusted current verifier checks typed proofs and the retained receipt
  before Docker contact; immutable image identity, Linux/native architecture and
  a new output path are required. The restricted container rechecks evidence and
  the copied binary before executing, regenerates the fixture and every proof,
  and requires an identical receipt before issuing replay provenance.
- Added explicit `--verify-receipt` to the qualification verifier. It compares
  every fixed member digest and typed proof against the retained version-3 receipt,
  rejects file symlinks/non-regular members, and rejects unknown invocation modes.
  Replay rejects source directory aliases. Checksums establish consistency, not
  authenticity; untrusted or resealed third-party tools must not be replayed.
- Nine replay shell tests cover existing output/dangling-alias preservation,
  source/output traversal, invalid source without executable invocation, source
  aliases, image substitution, non-Linux images and emulation before creating
  output or starting a container. Seven receipt-mode tests cover unchanged,
  altered and missing evidence. A separate entry-alias regression caught and
  fixed a macOS realpath mismatch that could silently skip verifier execution
  when copied under `/var` rather than its canonical `/private/var` path.
- Real Linux/arm64 replays at `runs/linux-process-snapshot-replay-v1` and `-v2`
  used `runs/linux-process-snapshot-cleanup-v3`. Both passed backend restoration,
  thread guard, supervisor failures and parent-death cleanup from the archived
  executable. All nine retained qualification/provenance files are byte-identical
  between the two replays. Source/replayed receipt digest is
  `11b28591db42e14f7f6750859cf18a7f733fca5f3cd1afd2d0a3b287d206057f`.
  No builds, tests or browser work overlapped either functional replay.
- All 120 V8/UI/recipe JavaScript tests, full Go tests/vet, shellcheck, actionlint
  and Rust format checks passed. CI now collects and uploads process qualification
  plus archived-tool replay. Remote CI/native AMD64 remain unexecuted locally.

The full-product goal remains active. This is exact functional qualification
replay using a locally retained image, not portable dependency-closure archival,
sealed benchmark replay, performance data or official publication. Timed
process-snapshot scenarios, child-inclusive memory accounting, simultaneous
restored density, typed protocol/exports/report integration and remaining
host-resource/ABI/platform coverage still require implementation and verification.

### Process snapshot product contract and export

- Added the `process-snapshots` generator with the exact 343-byte qualified
  module and pinned artifact digest. A real wazero fixture test exercises the
  seeded memory/global/table result, growth, complete three-page memory hashes,
  first byte write, passive segments and dropped-segment traps. This validates
  guest behavior, not process cloning in wazero.
- Added the versioned `linux-process-cow-clone-v1` contract and distinct capture,
  restore, first-write and restored-execution scenarios. Capture/restore are
  source-clock process-control round trips; first-write/execution use child-local
  clocks. A first write to offset 65535 preserves result 64; the qualification
  executable's broader restored mutation to 125 is not interchangeable evidence.
- Centralized workload, request, configuration, lineage, clock, state and batch
  validation in the protocol interface, following the codebase-design skill.
  The live runner and offline bundle loader share these checks. Birth ticks are
  exact decimal strings; PID reuse requires a different birth identity. Samples
  require fresh captured/restored identities, a stable source adapter, released
  source guest resources, single-threaded pre-fork evidence and reaped children.
  Live execution checks the source PID against the launched adapter. Raw OS
  lineage attestation remains unimplemented.
- Added sacrificial restoration checks and explicit capability/platform gating.
  No shipped adapter advertises this capability. Timing-only admission rejects
  parent-only memory, warmup, barriers, batching and unrelated diagnostic payloads.
  Synthetic future-adapter tests cover all four stages and forged contract,
  configuration, artifact, state, lineage, sequence and clock evidence. They are
  not native timed-adapter qualification.
- Extended sample Parquet to `sample-evidence-parquet-v9` with nullable
  `process_snapshot_result_json`. Round-trip testing retains complete typed proof,
  birth strings above JavaScript's exact-integer limit, zero elapsed values and
  unsupported null values. No absent proof is inferred from existing qualifiers.
- Actual CLI run `runs/process-snapshot-contract-coverage-v1` terminated with
  three unsupported sacrificial checks and 12 unsupported runtime/stage cells,
  zero samples and no launched measurement adapters. Sealed-bundle verification
  passed. Both derived report versions verified; the latest export report at
  `reports/process-snapshot-contract-coverage-v2` also verified using its exact
  archived report builder. Earlier report artifacts were preserved.
- Full `go test ./...` and `go vet ./...` passed after the export change; all 34
  UI tests passed in this continuation. The earlier contract implementation also
  passed all 120 V8/UI/recipe tests. No Rust implementation changed here.

The full-product goal remains active. Native timed process-clone execution,
child-inclusive memory accounting and OS lineage evidence, simultaneous restored
density, portable dependency closure, locked benchmark replay and interactive
snapshot report views still require implementation. Broader ABI/platform coverage,
machine controls, pilot budgets and official publication are not complete.

### Native process snapshot timing worker

- Added `adapters/wasmtime/src/process_snapshot_timing.rs`, included privately
  by the existing Linux qualifier. Its `--timed-stage` mode performs real
  Cranelift/Winch process-COW capture, restore, single-byte first write and
  post-restore typed execution. Fresh templates/two independent restores,
  source resource teardown, single-threaded forks and bounded owned-child
  cleanup use the already qualified supervisor. It remains explicitly a
  development worker, not a registered product adapter.
- Capture/restore use source-clock ready-ack round trips. Restored processes
  report exact PID/birth identities and child-local operation timings. Typed
  function/memory handles are prepared before child readiness; full state/hash
  checks and proof encoding are outside timed operations. Failed reads, proof
  mismatches and unsuccessful child exits cannot yield successful samples.
- Added a restricted native-image measurement recipe archiving the executable,
  immutable image identity, functional/guard/cleanup proofs, eight output records
  and checksums. Added an opt-in real-evidence Go test using the shared protocol
  interface, following the codebase-design skill; missing files, wrong identities
  or any invalid sample sequence fail rather than becoming synthetic successes.
- Actual run `runs/linux-process-snapshot-timing-worker-v2` terminated on native
  Linux/arm64 with eight records/16 samples (two backends, four stages, two fresh
  captures each). All passed the typed Go proof/sequence verifier. Checksums and
  guard/cleanup/functional validators passed. Independent structure validation
  used the already retained analysis of the identical canonical fixture.
  Worker digest: `25d75dd1b2e00d98b9cb901cd91a1c0ba11559a17b61b0e9134d71726fbbcf72`.
  The earlier `-v1` run passed semantic checks but exposed possible overlap of
  child proof hashing with parent acknowledgment receipt. A post-timer release
  handshake fixes this; `-v1` is preserved, not silently treated as final timing
  evidence. The fresh `-v2` run used the rebuilt handshake implementation.
- Pinned Linux release build, full Go tests/vet, shellcheck and both Rust format
  checks passed. No build, test or browser work overlapped native measurements.

The full-product goal remains active. Product protocol/registry adapter wiring,
sealed experiment execution/replay, child-inclusive collectors and OS lineage
attestation, simultaneous restored density and report integration are still
required. These shared-VM development samples are not official performance data;
native AMD64 and broader failure/ABI/platform qualification remain unverified.

### Native process snapshot product adapter and sealed replay

- Added dedicated `--adapter=BACKEND` protocol mode and registered
  `wasmtime-process-snapshot` / `wasmtime-winch-process-snapshot` configurations,
  including normal CLI builds. Ordinary Wasmtime configurations remain unchanged.
  The codebase-design skill guided a shared collection implementation: standalone
  worker and protocol adapter use the identical timing/fork/cleanup path.
- Preparation checks the canonical artifact and scoped workload; failed
  preparation clears prior state. Only timing, declared stages, one operation,
  bounded samples, zero warmup and no barriers are admitted. Batched execution
  returns complete proofs through the ordinary protocol, and live verification
  checks source PID against the actual launched adapter.
- Added restricted native-container workflow for original collection, exact-tool
  replay and report verification. Added coverage verifier and ten schema/coverage
  regression tests. The first run exposed its mistaken `runtimes` field lookup;
  fixed to the actual `runtime_configurations` wire field. Its successful original
  bundle is preserved at `runs/linux-process-snapshot-adapter-v1`, without claiming
  that aborted recipe completed replay.
- Fresh `runs/linux-process-snapshot-adapter-v2` passed the complete workflow on
  Linux/arm64. Original and replay each have 18 successful trials: two sacrificial
  restoration checks and 16 timing trials (two launches, four stages, two backends),
  36 verified samples per bundle. Typed loader validation, coverage and locked
  configuration equality passed. The sample-v9 report passed current and exact
  archived-builder verification. Adapter digest is
  `e68f967029dde1d7e44e13005d40180a073650d156c4ce2f65fefb614de98150`.
- Pinned native Rust release build, Linux Go cross-build, full Go tests/vet,
  all 130 V8/UI/recipe Node tests (including ten new coverage tests), shellcheck
  and Rust format checks passed. No tests/builds
  overlapped original or replay measurements. Remote CI/native AMD64 have not
  been executed for these changes.

The full-product goal remains active. Child-inclusive collection and raw OS
lineage attestation, simultaneous restored density, snapshot-specific report
views, portable dependency closure, broader guest/host-resource failure and
ABI/platform coverage, machine controls and publication budgets remain required.
Shared-VM integration samples do not establish dedicated-host performance or
physical COW/reclamation claims.

### Snapshot live-process collector foundation

- Added `collectors/snapshot_process.go`: a native Linux reader for declared
  PID/birth/parent identities, with stat/status/smaps/stat bracketing and retained
  raw evidence. The codebase-design skill guided one shared raw validator for
  live collection and offline decoding, keeping parsing and lineage checks at
  the same interface. It never discovers or signals arbitrary processes.
- Birth strings remain exact above JavaScript's integer limit. Changed/reused
  PIDs, reparenting, dead states, truncated/duplicate fields, malformed units,
  negative values and byte/sum overflow are rejected. Status RSS/virtual size
  remain distinct from smaps PSS/private clean+dirty. Missing/denied smaps stays
  reason-qualified and null rather than zero; no peak or tree sum is inferred.
- Added 25 forged-evidence rejection cases, five injected read sequences and
  real Linux owned-process tests. Native helper parent-death cleanup pins the
  spawning Go thread until reaping, because Linux binds that signal to a thread.
  Helpers verify a three-process tree, reject incorrect live identities/parents,
  and release/reap descendants. These helpers are not Wasmtime COW clones.
- Native Linux/arm64 functional evidence at
  `runs/linux-snapshot-process-collector-v1` retains three raw readings, exact test
  executable/image, log and checksums. Live validation and later offline raw
  validation passed; checksums passed. Test executable digest is
  `f8c91526f14c8888cb830a3f5a220637e4b25ee34c500ba5d70f80e1b4cd28fe`.
  The additional private-sum-overflow fixture passed locally after this archive.
- Full Go tests/vet and shellcheck passed. Existing benchmark artifacts and
  services were preserved; no benchmark collection overlapped the checks.

The full-product goal remains active. This qualifies the collector, not live
Wasmtime snapshot lineage or child-inclusive memory. Those require a separate
diagnostic barrier flow: timing responses currently arrive after children are
reaped. Bind raw OS readings to held-live snapshot processes, retain/export the
evidence and implement the memory view before enabling memory admission. Density,
remaining ABI/platform/failure scope and official publication gates remain open.

### Held-live Wasmtime snapshot diagnostic barriers

- Extended the same private timing worker with an opt-in inspection observer.
  Ordinary timing has no observer, no live-process messages and no diagnostic
  pauses. Inspection pauses after source release, restore readiness, first write
  and execution, for two sequential restores per sample. Parent-death/owned-child
  cleanup remains shared rather than replaced by a second fork implementation.
- Added explicit memory-profile `inspect` mode and diagnostic capability.
  Normal memory `run` is refused. Snapshot messages and final diagnostic payload
  are separate from ordinary samples; the diagnostic profile/latency-ineligible
  label is validated. The codebase-design skill guided reuse of the worker and
  a single proof/event sequence validator across live and retained evidence.
- Added agent observer/release flow, seven-event sample binding and rejection
  tests for scope, eligibility, versions, missing/duplicate/reordered events,
  sample indices and all process identities. Five agent precondition cases
  reject before sending a request. Existing normal calls reject unsolicited
  snapshot barriers or diagnostic payloads.
- Real native inspection uses the independent procfs reader while all declared
  processes are held live. It verifies source PID/controller parent, template
  and restored parent links, exact births and one live thread. Completed guest
  state proofs bind the observed processes to both restorations.
- Linux/arm64 `runs/linux-snapshot-live-inspection-v2` passed both backends with
  two samples each: 28 barriers, 80 raw live process readings. Retained-evidence
  validation and all checksums passed. Exact adapter/test binaries are retained;
  adapter digest is `62770d91da0e66adf6bce02691168c66acb3ebc6fd3a3b87059415a4280392a5`.
  Earlier `-v1` failed artifact preparation due to a test path bug before
  inspection; it is preserved, not claimed as successful.
- Fresh ordinary timing workflow `runs/linux-process-snapshot-adapter-v3`
  passed all stages, sealed bundle validation, exact-tool replay, coverage and
  current/archived report-builder checks after the observer addition. No builds,
  tests or browser work overlapped that collection/replay.
- Native Rust build, Go cross-build/tests/vet, Rust format checks and shellcheck
  passed. CI now runs live inspection and retained validation; remote CI/native
  AMD64 have not been executed locally.

The full-product goal remains active. This closes live diagnostic process binding
for the scoped fixture, not general snapshot memory publication. CLI memory
admission, sealed raw-reading storage, metric exports and UI/report validation
remain required. Individual boundary readings do not imply tree peaks, isolated
COW fault costs, simultaneous restored density or physical reclamation. Broader
failure/ABI/platform coverage and official publication gates remain open.

### Sealed process-snapshot memory collection and replay (2026-09-30)

- Added a dedicated memory-profile runner path for the qualified native Linux
  snapshot adapters. Explicit live barriers are required. Ordinary phase events,
  the parent-process sampler and parent process high-water RSS are not used.
  Sacrificial correctness admission remains a separate timing restoration pass.
- `SnapshotMemoryEvidence` retains controller identity, seven ordered boundaries
  per sample, individual source/template/restored procfs readings and the
  completed state proofs. A shared validator re-derives process births, parent
  chains, thread counts, availability and collector ordering online and during
  sealed loading. Diagnostic timers remain nested and latency-ineligible.
  The codebase-design skill guided that shared collection/validation interface.
- Whole-tree cgroup lifetime readings, when isolation supplies them, remain
  separately scoped cgroup accounting; parent-only RSS, duplicate metrics,
  invented available values and foreign diagnostic payloads are rejected.
  Native restricted-container qualification had no delegated cgroup, so it
  does not qualify cgroup accounting or phase-only peaks.
- Parquet v10 adds nullable `snapshot_memory_evidence_json`. Exact birth strings,
  raw stat/status/smaps evidence, zero values and denied-smaps reasons round-trip.
  Memory rows do not gain top-level elapsed time, operation counts, sample
  indices or headline latency eligibility. Failed prefixes remain diagnostic.
- Added forgery/rejection tests, twelve coverage-verifier regression tests,
  opt-in real native runner tests for both backends/all stages/preflight, and a
  restricted-container sealed-run/replay/report recipe. CI now invokes the
  native product-path test; remote CI and native AMD64 are not yet verified.
- Native Linux/arm64 `runs/linux-snapshot-memory-adapter-v2` passed original and
  exact-tool replay. Each run has eighteen successful trials (two sacrificial,
  sixteen memory), 224 boundaries and 640 raw process readings. Both backends
  and all four stages passed typed sealed validation, coverage and current plus
  archived report-builder validation. `-v1` also passed before Parquet v10.
- Fresh timing regression `runs/linux-process-snapshot-adapter-v4` passed
  original/replay, all four stages, typed coverage and current/archived report
  verification without diagnostic barriers. Builds, tests and browser work did
  not overlap native collection/replay. Full Go tests/vet, 142 JavaScript tests,
  actionlint, shellcheck and retained native evidence validation passed.

This is exploratory shared-VM qualification of the fixed import-free fixture,
not official performance or general snapshots. Snapshot-specific interactive
report views, simultaneous restored-instance density, dedicated-host cgroup
qualification, broader failure/ABI/platform coverage, physical reclamation
evidence and publication gates remain open. The full-product goal stays active.

### Interactive snapshot process footprints and matched report proof (2026-09-30)

- Added versioned report derivation for validated snapshot process readings.
  Source, template and restored-child rows retain exact decimal byte counts,
  process birth ticks, controller-local reading brackets, availability reasons
  and raw record/reading coordinates. Derived numbers are revalidated from raw
  procfs evidence, not trusted adapter estimates or sums of process RSS.
- Standalone memory reports and strictly matched attached memory passes expose
  trial/sample/role/metric controls, per-process bars, all footprint metrics,
  exact identity details and raw-trial links. Every sample stays reachable;
  failed/unsupported outcomes remain visible but failed prefixes never produce
  successful numbers. Role filtering preserves one common per-sample scale.
  Missing/denied PSS/private memory does not produce a zero or full-width bar.
- Added explicit process capture/restoration/first-write/execution overview
  labels and distinct tones/patterns. These stages start selected together;
  process snapshots are no longer mislabeled as compile/instantiate stages.
  Boundary footprints are not injected into the peak-RSS track.
- Initial pairing of separately collected bundles was refused because their
  Docker hostnames differed. The gate was preserved. The recipe now optionally
  collects both profiles sequentially in one native container, before report
  construction. `runs/linux-snapshot-memory-paired-view-v1` passed both backends,
  all four stages, exact-tool memory replay, typed/coverage checks and current
  plus archived standalone/paired report verification. The attached memory view
  retains sixteen completed trials and 640 distinct process readings.
- Added derived-data tests for exact integers, zeros, denied smaps, invalid
  lineage, failed prefixes, sacrificial exclusion, matched filtering and paired
  raw-source links; retained native standalone/paired report tests passed.
  UI tests cover later samples/restorations, independent roles, shared scales,
  unavailable bars, stale-detail clearing, raw links and exact birth/bracket
  strings. Full Go tests/vet, all 144 JavaScript tests and shellcheck passed.
- Playwright desktop/mobile checks confirmed filters, click/keyboard details,
  matched raw-memory links and no console errors. A mobile numeric/unit wrap
  was found and fixed. Screenshots are under `output/playwright/`; native
  collection/replay ran without concurrent builds, tests or browser work.
- Final viewport checks also caught an intrinsic-width trial selector and
  transparent footprint fills. The selector is constrained to the panel and
  source/template/restored fills now have distinct colors. Open mobile evidence
  stays within the 390-pixel viewport. Latest native-rendered sealed report is
  `runs/linux-snapshot-memory-report-view-v1/report`, verified by both current
  and archived native builders from the same immutable matched measurements.

The fixed-fixture interactive report gate is now closed. Simultaneous restored
density, dedicated-host cgroup qualification, broader failure/ABI/platform
coverage, physical reclamation evidence and official publication remain open.
The full-product goal stays active; this is not a whole-product completion claim.

### Native simultaneous restored-group qualification (2026-09-30)

- Implemented a standalone, memory-profile process-COW density worker with
  bounded 1–32 concurrently live children and template/idle/touched/executed
  barriers. Child PID/birth membership is unchanged across group inspections.
  Distinct three-Wasm-page-end markers and full-memory hashes prove independent
  child state after all siblings execute, while the template retains its state.
- Added typed diagnostic proof and raw-evidence validators. Source/template/
  child ancestry, native backend, fixture digest, single-thread constraints,
  collector ordering, exact clock ownership and completed proofs are checked.
  Missing timers cannot become zero. Raw prefixes validate independently but
  cannot satisfy completed evidence. The codebase-design skill guided this
  shared validation interface for live collection and offline replay.
- Added raw forgery tests, native groups at 1/2/4/8/32 for both backends, all four
  invalid-continuation cleanup cases, and owned-source-death cleanup with eight
  live children. Tests signal only the subprocess owned by exec.Cmd, never
  reported PIDs; absence checks distinguish PID reuse and include zombies.
- Native ARM64 `runs/linux-snapshot-density-v3` passed ten completed groups and
  ten cleanup cases, archived-native and macOS retained validation, and SHA-256
  integrity checks. The 32-child group retains 104 separate raw process readings.
  The restricted recipe archives exact test/worker binaries and image identity;
  CI is configured to run it with an init reaper. Remote CI/AMD64 not verified.
- Existing sealed snapshot paths passed fresh paired timing/memory collection,
  exact-tool memory replay and current/archived standalone/paired report
  verification in `runs/linux-snapshot-density-regression-v1`. Collection
  overlapped no builds, tests or browser work. Full Go tests/vet, 145 JavaScript
  tests plus nine recipe safety tests, workflow/shell lint, native Rust rebuild
  and formatting passed. Final rebuilt-worker qualification and checksums passed
  again in `-v3`; earlier native evidence directories are preserved.

This is fixed-fixture qualification, not registered product density support or
headline performance. Product corpus/CLI admission, sealed trial storage,
typed exports and density/marginal reports remain open, alongside partial-fork
failure qualification, broader ABI/platform support, physical reclamation,
dedicated-host cgroup evidence and official publication. Full objective active.

### Sealed restored-density product path (2026-09-30)

- Added a distinct fixed-fixture density contract and versioned corpus counts
  1/2/4/8/32 without redefining sequential snapshot stages. Dedicated adapters
  advertise a bounded density inspection capability, not generic snapshots.
- Native memory batches inspect multiple fresh concurrent groups, each with
  four complete held-live barriers. The controller retains all per-process raw
  readings and completed proofs. Shared online/offline validation checks the
  exact locked count/backend/fixture, architecture, ancestry, group membership,
  sample order and collector clock ordering. Root-only RSS is bypassed.
- Sacrificial admission inspects two fresh groups in a separate memory process;
  admission and diagnostic timers are not headline performance. Failed prefixes
  cannot become successful estimates. Added protocol/sealed forgery tests and
  real native multi-group trial tests for both backends.
- Nullable Parquet v11 `snapshot_density_evidence_json` preserves groups, raw
  process births/brackets, zero clocks and denied smaps. It supplies no top-level
  elapsed/sample/operation numbers or latency eligibility. Added round-trip
  tests. The codebase-design skill guided the separate shared validation seam.
- Native Linux/ARM64 `runs/linux-snapshot-density-product-v2` passed collection,
  exact-tool replay, and current/archived report verification. Independent
  retained validation checks thirty successful trials, all five counts and both
  backends, forty measured groups and 1,448 raw readings per run. CI now runs
  the restricted product recipe and retained coverage validator.
- Full Go tests/vet, 154 JavaScript tests, workflow/shell lint and native Rust
  format/build passed. Final rebuilt-controller collection repeated the same
  full coverage and archived report checks. A retained Parquet reader verifies
  all thirty trial rows against their sealed JSON evidence. Reused template or
  child incarnations are rejected across supposedly fresh groups. Earlier
  evidence directories remain unchanged.
- The existing sequential snapshot timing/memory product path passed fresh
  paired collection, exact-tool memory replay, all-stage coverage and current
  plus archived standalone/paired report verification in
  `runs/linux-snapshot-density-product-regression-v1`. Native collection/replay
  overlapped no builds, tests or browser work.

Restored-density analysis now exports twelve curves (two backends, three held
stages, PSS/private sums) and block-paired adjacent-count marginal costs through
the ordinary report dataset. The dedicated viewer shows missing/zero values,
launch coverage and withheld intervals, with keyboard/click raw evidence.
`reports/linux-snapshot-density-view-v2` derives from the unchanged native v2
product run; current and archived report builders verify independently. Tests
cover complete native evidence, denied readings, forged cleanup, missing graph
gaps, negative marginal costs and raw links. This is non-atomic process-group
accounting, not summed RSS, exact physical peaks or headline latency.

The full objective remains active. Additional density metrics/diagnostic clock
views, partial-provisioning fault proof, broader ABI/platform
coverage, dedicated-host cgroup qualification and official publication remain
open. Existing generic scaling views do not substitute for this density view.

Density analysis v2 adds six validated diagnostic-clock curves (source
provisioning and child-local touch/execution means for both backends), bringing
the standalone viewer to eighteen curves. All five derived metrics now have
registry definitions and collector versions. Time and memory format separately;
point and paired-marginal uncertainty retain units/signs and missing coverage.
`reports/linux-snapshot-density-diagnostics-v2` uses the same immutable native
product evidence and verifies through current and archived report builders.
No new native collection or official performance claim is implied. Diagnostic
clock views are implemented; role-specific footprint exploration and partial
provisioning failure qualification remain open alongside broader product gates.

Role-specific footprint exploration is now implemented in ordinary standalone
density reports. `snapshot_density_views` exports all 1,448 per-process readings
from twenty measured launches in the retained native v2 run, with exact decimal
byte/timestamp strings, child identities and raw group/record/reading indices.
Successful rows require full validated trial proofs; failed prefixes remain raw
diagnostics only. `reports/linux-snapshot-density-processes-v1` verifies with both
current and archived builders. The viewer filters groups, stages and roles and
clears stale row details when selections change. Native retained analysis and
footprint tests now run in the packaged-container CI gate; local execution passes,
but no remote CI run is claimed. Partial-provisioning failure qualification,
broader ABI/platform and dedicated-host publication gates remain open.

Partial-provisioning cleanup now has a qualifier-only fault mode and eight native
ARM64 retained cases covering both backends at early/middle/last provisioning
positions, including 31 of 32 children. The held incomplete group's raw procfs
readings bind every incarnation; all declared processes must disappear after
normal template error cleanup, with no success proof or outer timeout. Partial
frames use a separate version rejected by normal product validators. The density
recipe and packaged CI gate include these cases and checksums. Retained tests
reject eight classes of false cleanup evidence. An existing procfs death-check
race (ESRCH rather than ENOENT) was reproduced in a portable regression and fixed
without suppressing permission/I/O errors; the failed first directory is retained.

`runs/linux-snapshot-density-partial-product-v1` passed all qualification cases,
thirty original/replayed product trials each, exact-tool replay and both report
builders with the rebuilt worker. No native collection overlapped builds/tests
or browser work. This qualifies injected ownership cleanup on a shared native
ARM64 VM, not official performance, actual OS resource failures or AMD64.
Broader ABI/platform, dedicated-host and failure qualification gates remain open;
the full objective remains active.

## Preview 2 filesystem reset qualification (2026-09-30)

The full-spec audit found that P2 command batches reused a writable staging
directory while declaring `fresh_instance_per_sample`. Fresh component instances
alone did not reset host filesystem state. A standalone pinned Rust
`wasm32-wasip2` guest now asserts pristine input, exclusively creates a sentinel,
and mutates its input on every invocation. Pristine single invocations passed on
both backends before the fix, while multi-sample sacrificial runs failed.
`runs/p2-filesystem-reset-red-v1` and `-red-v2` retain the original failed evidence
and old tools; their failed checks exclude all measured trials.

`p2commands.rs` now stages locked fixtures inside the sample loop and releases
each temporary directory after the sample's Store and instance. Fixture copying,
instantiation, verification and cleanup remain outside the first-call timer.
The diagnosing-bugs workflow used the actual guest/controller seam to separate
reused filesystem state from an invalid fixture or retained component state.

`TestComponentFilesystemResetBundle` qualifies Cranelift and Winch with independent
admission, single-invocation probes, two sacrificial checks, four measured trials,
exact stream hashes, immutable component bytes, sealed reload, and archived-tool
restoration/replay. `runs/p2-filesystem-reset-green-v2` and its `-replayed` sibling
each contain six successful trials and sixteen verified samples. The restored
adapter/analyzer executables and relocated lock are retained in `-restored-tools`.
This is API-level replay under the same test runner, not a claim that an archived
Go test executable supports the production CLI's reproduce command.

`reports/p2-filesystem-reset-green-v2` was generated offline and verified with
both current and archived report builders. Full Go tests and vet, twenty-four
Rust tests, rustfmt, shell syntax and workflow YAML parsing passed locally.
The Wasmtime CI job now builds the standalone guest using pinned Rust, runs the
original/replay regression against release adapters, and uploads run evidence
even on failure. No remote CI execution is claimed. Local debug-adapter runs on
Darwin/ARM64 prove functional reset behavior, not official latency qualification.
General component export calls, P2 memory/counter profiles and resource quotas,
broader platform qualification, and dedicated-host publication remain open.

## Preview 2 command lifecycle and memory passes (2026-09-30)

P2 commands now support compile, instantiate and first-call timing and memory
passes, still one operation per fresh-instance sample. Compile includes
`Component::new` validation; instantiate includes typed `Command::instantiate`
and nested core start functions, but not `wasi:cli/run`; first-call measures that
run function alone. Each compile/instantiate sample executes and verifies the
command outside its API timer. All paths retain the per-sample filesystem reset.
Memory passes optionally reuse the existing ordered before/returned/released
barriers; timing passes reject them. Engine/linker and shared compiled-component
retention, capture-buffer lifetime and staging cleanup are recorded explicitly in
the adapter's effective configuration. No forced allocator reclamation is added.

New component-specific lifecycle and barrier capabilities gate this expansion in
the controller; an old adapter can still participate in its original first-call
timing contract but cannot acquire invented memory/instantiation support. Eight
portable controller tests reject missing capabilities and unsupported contracts
before launch. Ten genuine adapter rejection cases cover timing barriers,
counters, steady mode, multi-operation requests and incorrect output on both
backends. Successful compilation alone cannot bypass the behavioral oracle.

`runs/p2-command-lifecycle-v2-{timing,memory}` and their archived-tool `-replayed`
siblings each have eight successful trials and sixteen verified samples: two
sacrificial checks and six measured backend/scenario cells. The twelve measured
memory samples in each memory bundle carry thirty-six ordered external boundary
events. Their unsupported Darwin procfs/cgroup readings remain unavailable;
process-lifetime high-water RSS is recorded separately, never relabelled as an
exact API-phase peak. The first `v1` bundles are retained: their samples passed,
but the first test incorrectly expected sacrificial checks per scenario and did
not account for explicit unsupported responses also returning client errors.

`reports/p2-command-lifecycle-v2` joins the separate timing/memory passes and
verifies with current and archived builders. Its browser preview at
`http://127.0.0.1:8172` shows all six timing segments and six RSS cells. Playwright
verified compact time/RSS hover, exact instantiate timing/memory raw links, stage
selection reducing each graph to four cells, and no horizontal document overflow
at 390px; the browser console had no errors or warnings. Full Go tests/vet,
twenty-four Rust tests, rustfmt, thirty-nine UI tests and workflow YAML parsing
pass locally; the original filesystem-reset regression also passes against the
expanded adapter. The CI qualification step now runs reset, lifecycle/replay and
negative-contract tests using release adapters and retains run evidence. Remote
CI and Linux P2 phase-collector qualification are not claimed. General component
export worlds, P2 counters/profiling/quotas, other platforms and dedicated-host
publication remain open; the full product objective is not complete.

## Native Linux P2 CLI and procfs qualification (2026-09-30)

The pinned Linux Wasmtime release binaries were rebuilt before collection.
`recipes/test-linux-p2.sh` now runs the production CLI in one native,
unprivileged, network-disabled, read-only container. Only explicit tools, fixture
and evidence paths are mounted. The fixture's Rust source and compiler receipt
hashes are included in workload provenance; the exact qualification script and
coverage verifier are copied before collection and checksummed afterward. Builds,
tests and browser work do not overlap collection. The shared Docker VM is not a
dedicated measurement host and no other user services are stopped.

Each timing/memory pass runs normally, through `wasmbench reproduce`, and through
explicit `restore-tools` followed by the restored production runner's locked
`run`. All six bundles in `runs/linux-p2-lifecycle-product-v3` passed seals and
coverage: forty-eight trials and ninety-six exact-output-verified samples across
Cranelift/Winch compile, instantiate and first-call. The three memory bundles
retain 108 ordered boundaries with available RSS/PSS/private/virtual procfs
observations attached to the exact samples. Whole-process `wait4` peak RSS stays
separate from boundary readings. No delegated cgroup or exact API-phase peak is
claimed. Archive replay verifies relocated adapter paths, exact file hashes,
unchanged effective configuration, command contract, runner hash and native host.

The first `linux-p2-lifecycle-product-v1` directory is preserved: all collection
succeeded, but a coverage assertion incorrectly required normal `reproduce` to
relocate already-installed, hash-matching adapters. The corrected `v2` additionally
passed explicit archived-tool replay. Final `v3` also archives the qualification
scripts and passes the strengthened replay-identity gate. No failed or older
evidence was overwritten.

The paired `v3/report` has six timing cells and six available peak-RSS cells.
Both the current analysis and archived Linux builder verified it inside the
native container; current cross-platform report verification and auxiliary file
checksums also pass locally. Preview: `http://127.0.0.1:8173`. Full Go tests/vet,
all 124 recipe tests (including twenty new P2 coverage/preservation cases), shell
syntax and CI workflow YAML parsing pass. Packaged-container CI builds the guest,
Linux adapter and controller, runs this native qualification and retains its
evidence; no remote CI run is claimed.

This closes native ARM64 Linux procfs/production-CLI qualification for the locked
P2 command lifecycle, not AMD64, cgroup isolation, official performance or full
Component Model coverage. General component worlds/exports, P2 resource quotas
and counters/profiling, broader platforms and dedicated-host publication remain
open. The full product objective remains active.

## AOT artifact correctness and archived CLI replay (2026-09-30)

The real-adapter AOT regression exposed two defects: Wago and Wasmtime AOT
production marked samples verified without checking the produced artifact's
behavior, and Wago AOT loading used the raw-Wasm-only `Load` API for native
bytes. Production now serializes inside the timer, then reloads that exact
artifact and checks its behavioral oracle outside the timer. Wago uses
`LoadTrustedArtifact` only for immutable bytes produced locally by the adapter;
Wasmtime's unsafe deserializer has the same local-byte trust boundary. No
arbitrary native-artifact ingestion was added. Load timing excludes behavior
verification and release. Effective configuration records the policy, separating
new results from older configuration identities. Old sealed bundles are
preserved; their production `verified` flag is not retroactive proof of artifact
round-trip correctness.

`TestBuiltAdapterAOTOracles` directly drives native batches, without a
controller sacrificial check that could hide a missing measured-operation
oracle. All thirty-six cases pass across Wago, Cranelift and Winch: production
and loading, timing and memory, and correct/wrong-result/wrong-memory oracles.
Wrong oracles must return an incorrect-result error with no verified samples;
unrelated API failures cannot pass that assertion. Correct batches retain exact
sample indices, operation counts and batch-average labels. CI enables this
regression in both pinned adapter jobs. Remote CI is not claimed.

Production CLI evidence in `runs/aot-correctness-product-v1-{timing,memory}`
and each `-archived` sibling passes seals and exact coverage. Each bundle has
forty-two successful trials and seventy-eight verified samples: six
single-sample sacrificial checks plus thirty-six two-sample measured trials.
Each of twelve workload/runtime/scenario cells has three independent launches.
Archived replay runs the restored production runner and relocated adapters,
with unchanged tool hashes, runtime descriptions and host identity. Memory
trials retain available whole-process peak RSS; timing samples carry no memory
instrumentation. Builds and tests finished before collection; passes ran
sequentially. This is local Darwin/ARM64 product qualification, not official
performance evidence or native Linux/AMD64 AOT qualification.

The paired `reports/aot-correctness-product-v1` verifies with both current and
archived report builders. Full Go tests/vet, twenty-four Rust tests and rustfmt
pass. README documents commands, timing/trust boundaries and allocator limits:
Wago allocation observations include untimed verification/release, process RSS
includes startup/setup, and serialized artifact bytes are not instruction size.
The broad ABI/platform, dedicated-host and publication requirements remain open;
the full product objective remains active.

## Measured-engine usability verification (2026-09-30)

The direct native `TestBuiltAdapterEngineInitOracles` regression reproduced
sixteen false-positive cases: wrong result and memory oracles in timing/memory
passes for wazero compiler/interpreter and Wasmtime Cranelift/Winch. Engine
construction returned successfully and samples were marked verified without
exercising the newly measured engine. This was not a controller preflight issue;
the regression calls adapter batches directly. Correct-oracle controls passed.

Both implementations now stop the construction timer, then compile, instantiate
and execute the locked workload using that exact newly created engine and check
its result/memory oracle. Host import registration, workload initialization and
input application, behavioral verification and resource release remain outside
the construction timer. Wazero uses a temporary adapter context bound to the
measured runtime rather than changing the prepared adapter's engine. Engine
release occurs on failed verification too. Wasmtime releases its checked module
before its engine. No forced reclamation was introduced.

The strengthened memory assertion additionally went red on both Wasmtime
backends: an unrelated untimed prepared instance's logical memory was attributed
to engine-init. That observation is now omitted for this scenario. Wazero's Go
allocation observations explicitly describe their full batch window, including
untimed usability verification and release, instead of implying constructor-only
allocations. Effective configuration records the initialization policy. The
direct regression now passes all twenty-four cases, requiring specific
incorrect-result errors and no returned verified samples for wrong oracles,
exact operation/sample shapes, explicit policy and correct memory domains. CI
enables it in the Go-adapter and Wasmtime jobs; remote CI is not claimed.

`runs/engine-init-correctness-v1-{timing,memory}` and their `-archived` siblings
all pass seals and exact coverage: thirty-two successful trials and fifty-six
verified samples per bundle, with eight measured workload/runtime cells at three
independent launches each. Archived replay executes restored runners/adapters
with relocated paths, unchanged file hashes, runtime descriptions and host
identity. Timing retains no memory observations; memory retains available
whole-process peak RSS and honestly labeled Go allocator windows, with no
unrelated guest-memory attribution. Collection ran sequentially after builds
and tests finished. This is Darwin/ARM64 functional/reproduction qualification,
not official performance, cgroup isolation or Linux/AMD64 qualification.

The paired `reports/engine-init-correctness-v1` verifies through current and
archived report builders. Full Go tests/vet, twenty-four Rust tests, rustfmt,
workflow YAML parsing and the thirty-six-case AOT regression pass. README now
documents usability, timing and memory boundaries. Previous sealed results were
not overwritten and do not acquire retrospective behavioral verification.
Broader ABI/platform qualification and dedicated-host publication still remain;
the full product objective remains active.

## Point-level setup/execution evidence inspection (2026-09-30)

The setup-versus-observed-invocation graph now exposes an inspector for every
finite measured point, including zero setup totals and N=0. Mouse activation or
Enter/Space opens the synthetic total, pointwise interval or explicit
insufficient-block status, complete/planned block coverage, selected setup phases,
exact observed sample prefix and included warmup count. Its table links every
contributing complete-block setup/trajectory trial, and the inspector includes
the exact runtime configuration and report reproduction link. N=0 explicitly
has no call-time contribution while retaining the complete-trajectory eligibility
requirement. This is inspection of existing synthetic analysis, not a new
end-to-end measurement, extrapolation or permanent-winner claim.

Playwright testing materially changed the implementation: the initial detail
panel was below all curve summaries, so it was moved immediately beneath the
plot and activation brings it into view. Native browser testing at 390px also
reproduced pointer interception by overlapping curve points. Runtime filtering
and per-point counts-table buttons now provide independent access without
jittering coordinates or inflating quantitative geometry. Finally, resize redraw
could clear an open inspector during navigation; resize now retains its selected
evidence, while workload/runtime changes still clear stale detail. Earlier
`reports/break-even-point-evidence-v1` through `v3` are preserved as iteration
evidence rather than overwritten.

Final `reports/break-even-point-evidence-v4` regenerates from the existing sealed
`runs/trajectory-breakeven-v3` bundle and verifies through current and archived
builders. No runtime measurement was recollected or relabelled. The preview is
`http://127.0.0.1:8177`; temporary earlier previews from this turn were stopped,
without affecting other user previews. Forty UI tests pass, including missing
point gaps, genuine zero, unique point access, keyboard activation, raw-link
escaping, exact configuration, absent/available intervals, warmup-prefix counts,
runtime filtering, resize retention and stale-detail clearing. Full Go tests/vet
pass. Browser checks confirm native click/keyboard and counts-table activation,
runtime/workload filter reset, resize retention, and zero document overflow at
390px. All nine selected raw links were fetched and checked against the exact
runtime/workload. The browser console reports no errors or warnings. The mobile
inspector screenshot is retained in
`output/playwright/break-even-point-mobile-v4.png`.

The broad goal is still not complete: platform/ABI qualification, explicit
empty-local-harness calibration beyond the existing guest identity fixture,
dedicated-host evidence and official publication remain open. This closes the
point-level graph-inspection gap, not those requirements.

## Empty local harness calibration qualification (2026-09-30)

The explicit `harness-calibration` scenario now runs in Wago, wazero
compiler/interpreter, Wasmtime Cranelift/Winch and V8. It qualifies the resident
stateless core scalar workload outside the timer, then measures only a local
loop writing iteration-plus-one into preallocated eight-byte slots. Each slot
is verified after timing. No Wasm/embedding call, warmup, barriers or memory
instrumentation occurs in the timed loop. The result is an iteration count,
not guest output. Requested samples and iterations are bounded; unsupported
contracts fail before controller launch or adapter sampling. Go uses a
non-inlined verifier, Rust a compiler black-box barrier, and V8 exactly
representable integer values in Float64Array slots. This is language-local
bookkeeping evidence, not a universal subtractable API overhead constant.

Sample validation rejects forged counts, result shape, verification, labels,
indices and observations. The versioned analysis classifies calibration as
`calibration_only`, excluding it from workload latency, useful-work throughput,
aggregates, scaling fits and setup/execution models. Publication audit v4 adds
an explicit workload-scope gate; tests cover both calibration-only and mixed
workload/calibration locks. The V8 helper is pinned and archived with its
adapter. README documents the runnable command, scope and interpretation.
CI now selects native calibration tests for all six configurations; this is
workflow wiring, not a claim that remote CI has run.

All Go tests and vet pass locally. The rebuilt production adapters pass all
54 native cases: individual and batched bookkeeping, wrong workload oracle,
memory profile, warmup, barriers, empty/oversized budgets and zero samples.
Workflow YAML parses. Sequential CLI collection produced sealed
`runs/harness-calibration-product-v1` and exact restored-tool replay produced
`runs/harness-calibration-product-v1-archived`. Each has 48 successful trials:
36 measured trials with 72 verified 1000-iteration batch samples, plus 12
sacrificial workload-oracle samples. Both bundle seals verify. Restoration
retains the relocated, hash-pinned V8 harness helper in its lock.

`reports/harness-calibration-product-v1` verifies with current and archived
builders. All 12 summary cells withhold workload latency while retaining raw
calibration timers; the publication command rejects the run on the explicit
workload-performance-scope requirement. The rejection audit is retained
separately as `reports/harness-calibration-product-v1-publication.json`, not
inserted into the sealed report. Earlier sealed reports were not overwritten.
These are local Darwin/ARM64 functional/replay results, not dedicated-host
performance or Linux/AMD64 qualification.

This closes collection, policy, archival and CLI qualification for the explicit
empty harness case. A dedicated diagnostic presentation remains to be added;
currently calibration samples are inspected through raw trial evidence rather
than scored or graphed as workload performance. Broader ABI/platform coverage,
dedicated-machine qualification and official publication remain open, and the
full product goal remains active.

## Calibration evidence presentation (2026-09-30)

The report now has an empty-local-harness evidence viewer, separate from
workload scoring. It selects one recorded process at a time, shows exact sample
indices/types, raw elapsed nanoseconds, iteration counts and returned results,
and computes diagnostic nanoseconds per iteration only for valid verified
timing calibration samples. Zero is retained; failed/unverified prefixes,
malformed counts/results, instrumentation and sacrificial workload checks do
not acquire normalized calibration values. Trials without samples keep their
outcome and reason. All samples remain reachable through 250-row windows;
changing trials resets the window. Every trial links its encoded raw JSON,
exact runtime configuration/policy and reproduction commands.

Browser verification exposed an empty workload comparison on calibration-only
reports and a four-pixel mobile selector overflow. Calibration-only reports now
hide that graph and open detailed evidence; mixed reports exclude calibration
from workload stage controls. The selector is constrained to the available
width. Both changes have regression coverage. A 501-sample DOM fixture checks
last-window access, failed prefixes, no samples, genuine zero, sacrificial
labels, raw-link escaping, exact settings and invalid denominators/results.
All 41 UI tests pass, and full Go tests/vet pass; publish tests were rerun after
the final style change.

`reports/harness-calibration-view-v3` is generated from the existing sealed
`runs/harness-calibration-product-v1`, without recollecting or modifying native
measurements. Current and archived report-builder verification pass. Earlier
view iterations v1/v2 remain preserved. The preview at
`http://127.0.0.1:8178` serves v3; only the temporary v1 preview from this work
was replaced, leaving other previews untouched. Native browser selection works,
the last trial's raw link was fetched and its runtime/workload/id checked, and
390px viewport testing reports zero document overflow and no console errors or
warnings. The inspected screenshot is
`output/playwright/harness-calibration-mobile-v3.png`; the table scrolls within
its own wrapper to retain all columns. The Playwright browser was closed.

The dedicated calibration presentation gap is closed. Broader ABI/platform
qualification, dedicated-machine evidence and official publication are still
not complete; the original full-product objective remains active.

## WASI/Emscripten command lifecycle memory barriers (2026-09-30)

Wazero compiler/interpreter and Wasmtime Cranelift/Winch now advertise and
implement command instantiation and first-call phase barriers, in addition to
their existing compile and teardown boundaries. The controller admits them
only through the exact scenario capability. Instantiation brackets the API
after preparing imports/Store/WASI and capture handles, including Wasm start
but excluding Emscripten constructors and argv marshaling. First-call brackets
the requested `_start`/`main` call including host activity and bounded capture;
exit/output checking occurs after the returned barrier. Final boundaries
follow verification and logical release of instance/Store and stdin handles,
with shared engine/module and fixture staging retained. Capture buffers stay
alive during evidence construction. Guest exit can already have closed guest
resources before the returned barrier; this is not physical reclamation.

Go allocation observations now bracket the matching command API rather than
the broad command lifecycle when these barriers are requested. OS/cgroup
snapshots retain their independent domains and transport-inclusive window
semantics. Phase names reuse the existing versioned instantiate/first-call
protocol, without redefining compile. Effective configurations declare the
boundaries and release limitations. Native testing caught command instantiation
being routed through the core-scalar phase handler; command routing now takes
precedence or explicitly bypasses that scalar-only handler.

The new 72-case native regression covers four configurations, WASI/Emscripten,
compile/instantiate/first-call, two samples, and correct/wrong-exit/wrong-output
oracles. All cases pass with exact ordered barriers, no successful sample for
bad oracles, one-operation normalization and precise Go allocation domains.
Both adapter CI jobs now run the corresponding native test; remote CI was not
run here. Full Go tests/vet, eighteen adapter Rust tests, rustfmt and workflow
YAML parsing pass locally. README documents the runnable command and boundaries.

Real application qualification imported GNU seq and coreutils sort from the
sibling Wago catalog into a new suite, without changing that checkout. The
sequential memory run `runs/command-lifecycle-phases-product-v1` and restored
archived-tool replay `runs/command-lifecycle-phases-product-v1-archived` each
retain 56 trials: eight sacrificial checks and 48 measured cells. GNU seq
passes on all four configurations; coreutils sort passes on wazero compiler,
interpreter and Wasmtime Cranelift, while Winch fails its sacrificial compile
with `Unimplemented Wasm load kind`. Its six measured cells remain
`preflight_failed`; both CLI runs correctly return nonzero and preserve the
evidence. The other 42 measured trials retain 84 verified samples and 252
ordered barriers. Both bundle seals verify, and
`reports/command-lifecycle-phases-product-v1` verifies with current and archived
builders. No failed cell was dropped, relabeled or given a timing estimate.

This is local Darwin/ARM64 memory diagnostic qualification, not Linux collector
or dedicated-host performance qualification. Darwin retains unavailable Linux
procfs/cgroup readings explicitly. The existing report consumes the recorded
phase events; no new fabricated phase peaks or headline timing are introduced.
Broader platform/ABI coverage and official dedicated-machine publication still
remain; the full product objective is active.

## Native Linux command lifecycle collector qualification (2026-09-30)

The new `recipes/test-linux-command-lifecycle.sh` recipe qualifies the command
instantiation/first-call barriers with actual Linux procfs readings, not merely
Darwin unavailable outcomes. It uses an existing digest-pinned native image,
read-only tool/fixture mounts, no network or credentials, dropped capabilities,
no-new-privileges and an unprivileged uid. It does not mutate host cgroups or
assert isolated CPUs/dedicated-host performance. Tools are built before all
sequential collection. Existing output and dangling aliases are preserved,
traversal and emulation are refused before evidence creation, and image IDs
must be full immutable hexadecimal SHA-256 digests.

The retained verifier checks exact platform, configurations/capabilities,
WASI/Emscripten workloads, budgets, successful oracles, unique cell/check
coverage, three ordered barriers per sample and exact sample attachment. Every
boundary requires available RSS/PSS/private/virtual procfs snapshots with byte
units, adapter-process scope and boundary-snapshot quality. Process-lifetime
wait4 peak RSS remains separate; Go allocation observations must use the
matching API-window denominator. Ordinary and archived replays must retain
host, runner/artifact/tool hashes, command contract and effective settings;
archived adapters must run from relocated restored paths. The gate never calls
these snapshots phase peaks or treats memory timers as headline latency.

Seventeen verifier and seven recipe safety tests pass. All 189 recipe/UI tests,
shell syntax and workflow YAML parsing pass. Packaged-container CI now builds
the Linux Go tools, runs this qualification after its pinned Wasmtime build,
and retains its evidence directory; remote CI has not been run here. README
documents native execution and the qualification limits.

The rebuilt native Linux/ARM64 Go/Rust tools completed
`runs/linux-command-lifecycle-product-v2`: original collection, ordinary
reproduction and relocated archived-tool replay each have 24 successful trials,
48 verified samples and 96 ordered boundaries. Across them, 72 trials retain
144 samples, 288 boundaries and 1,152 available procfs metric readings. All run
seals and exact collector/identity coverage gates pass. Current and archived
builders verify its report; all sixteen memory summary cells correctly
withhold headline timing. The wrapper's fixture/source/image/recipe/log
checksums independently verify from the host.

The first recipe iteration `linux-command-lifecycle-product-v1` remains intact:
all collection succeeded, but its new coverage gate incorrectly expected one
sacrificial sample rather than the actual two-sample command correctness
contract. The gate and regression fixture were corrected before fresh v2
collection; earlier evidence was not patched or resealed.

This closes native ARM64 procfs qualification of these command barriers.
AMD64 execution is wired but not locally proven. Cgroup phase-peak accounting,
dedicated-machine publication and broader ABI/platform qualification remain
separate outstanding requirements; the full original product goal is active.

### Ordered-vector instantiation memory boundaries (2026-09-30)

Vector instantiation no longer dispatches into scalar-only phase validation on
wazero, Wasmtime or V8. Compiler/interpreter and Cranelift/Winch configurations
advertise `can_vector_instantiate_phases` with an explicit effective lifecycle
policy. The controller admits this scenario only when that capability is present;
Wago continues to expose it as unavailable rather than fabricate support.

Each memory sample records before-instantiate, instance-returned and verified
instance-release barriers. Wasm start is included in instantiation; explicit
initialization, pointer resolution, input writes, ordered guest calls, oracle
checks and release are excluded. Go allocation counters and V8 heap snapshots
are captured around the API rather than the full vector lifecycle or barrier
transport. Wasmtime prepares its store before the first barrier. Shared compiled
modules/engines remain alive; V8 drops references without forced reclamation.

The shared vector driver closes instances on failed later oracles and failed
release handshakes, never qualifies those samples, and emits the instantiation
release stage only after the full sequence verifies. Protocol regressions and
the native adapter vector matrix cover a wrong second output and exact ordered
barriers across all five supported configurations. Existing CI native matrices
already execute this extended test. Full Go tests/vet, 18 Rust adapter tests,
13 V8 tests, Rust formatting, JS syntax and 41 report UI tests pass locally.

`runs/vector-instantiation-phases-product-v1` and its `-archived` replay use the
three imported BLAKE3 workloads, each with 35 ordered output oracles. Each sealed
bundle has 15 successful measured trials, 30 verified individual-operation
samples and 90 correctly ordered boundaries, with boundary observations attached
to the exact samples. Archived tools execute from restored paths. Both seals
verify. Current and archived report builders verify
`reports/vector-instantiation-phases-product-v1`, whose memory-only summaries
withhold headline timing. This is local Darwin/ARM64 diagnostic evidence:
procfs and cgroup values correctly remain unsupported/unavailable. Linux collector
qualification, vector first-call boundaries and Wago vector instantiation remain
unproven; dedicated-machine publication and the full original goal remain active.

### Wago vector instantiation and initialization-order qualification (2026-09-30)

Wago now implements and advertises vector instantiation memory barriers, closing
the previously recorded six-configuration coverage gap. It retains the compiled
module across fresh instances, captures seven Go allocator observations around
the instantiation API, and verifies every ordered output before instance release.
Start, explicit initialization, call sequence, release and allocator domains use
the same declared boundaries as the other adapters; no forced GC is introduced.
The sibling Wago checkout was used read-only through the existing source-digest
build recipe, without modifying its files.

`corpus/testdata/vector-initialization.wat` and its WABT 1.0.41 generated binary
require exactly one Wasm start, one explicit initializer and the ordered pair of
calls on each fresh instance. Native vector tests exercise correct and wrong
second outputs with this fixture for compile, instantiate and teardown on all
six configurations. They assert capability/policy presence, qualified sample
absence on failure, the individual-operation instantiation sample type, exact
boundaries and narrow Go/V8 diagnostic scopes. Existing native CI matrices pick
up the additional cases. The full native vector matrix, root Go tests/vet and
Wago-module tests/vet pass locally.

Original `runs/vector-instantiation-phases-product-v2` and its `-archived` replay
each have eighteen successful measured trials, 36 verified samples and 108
ordered boundaries across the three BLAKE3 workloads and all six configurations.
Every sample checks all 35 output oracles. Read-only audits confirm boundary
attachment, Wago allocator scopes, retained workload/tool/runner hashes and
effective settings, host identity, and relocated archived executable paths.
Both run seals verify. Current and archived report builders verify
`reports/vector-instantiation-phases-product-v2`. Earlier v1 evidence is preserved.

This proves the new path on local Darwin/ARM64, not Linux process collectors or
cgroup phase peaks. Vector first-call memory barriers, Linux qualification,
dedicated-host publication and broader platform/ABI evidence remain outstanding;
the original full-product goal is still active.

### Ordered-vector first-call memory window (2026-09-30)

All six initial configurations now advertise `can_vector_first-call_phases`
and the same explicit `vector_first_call_phases_policy`. The controller admits
phased vector first-call requests only with this capability. Fresh initialized
instances resolve pointers and write the first input before `before_first_call`;
`first_call_returned` follows the final ordered call before its output oracle.
`first_call_released` requires every output to verify and the instance to close.
Compiled modules remain alive across samples, with no forced reclamation.

The contiguous memory window necessarily contains earlier output checks and
later input writes. It is not the sum of API-only intervals. Go allocation
counters and V8 heap snapshots use `first-call/vector_sequence_call_window`
and the denominator
`ordered_calls_with_intercase_input_and_verification_excluding_last_oracle_release_and_barriers`.
The raw `sequence_call_sum` timer still times each individual call only. Metric
registry and README document this distinction; OS phase collectors may also
include diagnostic and handshake work. V8 reference release remains distinct
from physical reclamation.

Shared-protocol tests establish exact write/barrier/allocator/call/oracle/close
ordering, zero-valued diagnostics, failure at each of three handshakes, and wrong
first/final output cleanup. Native tests cover these failure prefixes on all six
configurations, single-case sequences and the explicit start/initialization
fixture. A wrong intermediate oracle retains only the before boundary; a wrong
last oracle retains the returned boundary but never a qualified sample or
verified-release boundary. Existing CI native matrices exercise the expanded
tests. Full root and Wago Go tests/vet, 18 Rust adapter tests, 54 V8/report tests,
Rust formatting and JS syntax pass locally.

`runs/vector-lifecycle-phases-product-v1` and its relocated `-archived` replay
both pass instantiation and first-call collection for three BLAKE3 vector
workloads on all six configurations. Each bundle has 36 successful measured
trials, 72 verified samples (36 first-call sequences) and 216 ordered boundaries;
every sample checks 35 outputs. Exact audits confirm sample-boundary attachment,
Go/V8 sequence scopes, workload/tool/runner hashes, effective settings, host
identity and restored executable paths. Both seals verify. Current and archived
builders verify `reports/vector-lifecycle-phases-product-v1`, with all 36
memory-only summary cells withholding headline timing. Earlier evidence remains
unchanged. This is Darwin/ARM64 diagnostic qualification, not Linux procfs/cgroup
qualification or dedicated-host publication; the full original goal stays active.

### Native Linux vector lifecycle collector qualification (2026-09-30)

`recipes/test-linux-vector-lifecycle.sh` runs both ordered-vector fixtures on
Wago, wazero compiler/interpreter, V8 and Wasmtime Cranelift/Winch using separately
built native tools. Its immutable image-ID/native-architecture checks reject
emulation before creating evidence. Existing directories and dangling aliases
are preserved. Collection runs without network or elevated capabilities, with
read-only tools/fixtures and no host cgroup mutations or build-time interference.

The semantic verifier requires both vector contracts and explicit initialization
ordering, all six configuration identities and capability/policy declarations,
one independent launch with two samples per cell, exact stage/sample attachment,
available Linux RSS/PSS/private/virtual boundary snapshots and separate wait4
process-lifetime peak RSS. Go/V8 diagnostics must preserve the narrow instantiate
window versus the broader ordered-call memory window. It preserves zero values
and rejects duplicate/missing cells, changed oracles, bogus scopes/quality,
incorrect sequence sample types, misattributed boundaries and substituted tools.
Ordinary and archived replay retain host, runner/artifact/tool hashes and effective
settings; archived executables must run from relocated paths.

Fresh native Linux/ARM64 controller, wazero, Wago and Rust tools completed
`runs/linux-vector-lifecycle-product-v1`. Original, ordinary replay and archived
replay each have 36 successful trials, 72 verified samples and 144 ordered
boundaries. Across them this is 216 samples (144 measured, 72 sacrificial),
432 boundaries and 1,728 available procfs readings. Both fixture sequences
verify. All seals, semantic gates and wrapper checksums pass. Current and archived
builders verify the report; all 24 memory-only cells correctly withhold headline
timing. The Wago source identity was recomputed against tracked inputs before
qualification; the sibling source checkout was not modified.

Packaged-container CI now includes the six-configuration recipe with Wago source
pinned to `9f01d145d54ac7ab458b6b2f6047db90a757410a`; README records its runnable
workflow and scope. Remote CI has not been run. Thirty-one new verifier, recipe
safety and packaging-dependency tests pass; all 220 recipe/UI tests pass, along
with shell syntax and workflow YAML parsing. Native AMD64 execution is wired but
not locally proven. Cgroup phase-peak accounting and dedicated-host publication
remain separate outstanding requirements.

This work also found that Docker omitted `adapters/v8/harness.mjs`, which the V8
adapter now imports at startup. Docker now copies it, with a regression guard for
every local adapter import. A fresh native image was built as
`wasmbench:vector-lifecycle-package-v1` (image ID
`sha256:e5e9eb19cb7ded680890db0702c20fdf4b98e264899ef631d08b3ebeac068ad1`),
including successful packaged Go tests and pinned Rust compilation. Its packaged
smoke evidence is tracked separately under `runs/vector-lifecycle-package-smoke-v1`.
The smoke workflow finished successfully using only packaged executables: core
collection/replay/report, 147 floating-point trials, 120 successful density cells
with 24 explicitly unsupported cells, 240 live-group RSS snapshots and 30 paired
marginal intervals. All six packaged bundle seals and the dedicated evidence
gate pass. No source-built executable was bind-mounted for this smoke run.
The original full-product goal remains active.

### Private-cgroup vector phase qualification (2026-09-30)

A read-only readiness check confirmed the ordinary Docker container exposes
cgroup v2 read-only. It cannot qualify phase peaks. The existing explicitly
ephemeral privileged harness instead ran in a private cgroup namespace, with no
host cgroup or credential bind mounts and no network. Native agent tests passed,
including historical peak versus same-FD reset, atomically spawned worker
placement, process-tree CPU, deadline descendant cleanup and OOM accounting.
The probe output is retained in `runs/linux-cgroup-vector-probe-v1/tests.log`.

The harness now has an opt-in vector phase path using the same two stateful
fixtures and six configurations as procfs qualification. Its actual native
Linux/ARM64 CLI run is sealed under `runs/linux-cgroup-vector-product-v1/run`:
36 successful trials, 72 verified samples (48 measured), 144 ordered boundaries,
48 available same-descriptor kernel-accounted phase peaks and 144 available
total/user/system process-tree CPU readings. Explicit 512 MiB memory, no swap,
100000/100000 CPU and 128-task limits read back exactly before and after each
trial. Worker leaves are unique and atomically selected at spawn.

The reusable vector verifier adds a cgroup mode that independently checks these
contracts and exact returned-boundary attachment, while preserving zero CPU
values. Ten additional tests reject wrong budgets, post-spawn movement, reused
leaves, endpoint drift, denied/global/process-scoped peaks, caller-thread CPU and
missing attachment. Existing procfs gates remain unchanged. The qualification
is about diagnostic barrier windows, which include transport; cgroup memory is
not RSS and vector sequence windows still include intercase setup/checks.
`docs/RESOURCE-VERIFICATION.md` documents required mounts, opt-in flags and scope.
Official publication, exclusive measurement CPUs and dedicated-machine policy
remain unqualified; the original full-product goal stays active.

### Reproducible private-cgroup vector qualification (2026-09-30)

`recipes/test-linux-vector-cgroup.sh` turns the earlier manual private-container
probe into a new-only reproducible workflow. It requires an explicit ephemeral
opt-in, resolves and pins an immutable image ID, rejects non-native architectures,
preserves existing evidence and dangling aliases, and mounts only selected tools,
fixtures, scripts and a new evidence directory. Controller and workers occupy
separate leaves of the disposable private namespace. No host cgroups, credentials
or Docker socket are mounted. Agent tests finish before collection begins.

The harness now supports ordinary and archived-tool replay in that same private
hierarchy. Each replay is seal-verified and independently coverage-checked for
unchanged workload/runner/adapter identities, correct archival relocation, exact
resource readbacks, 48 same-FD phase peaks and 144 process-tree CPU readings.
All three collections finish before report generation; current and archived
report builders and an 11-file wrapper checksum receipt must pass. Failed runs
retain evidence without printing qualification success.

Actual native Linux/ARM64 evidence is retained under
`runs/linux-cgroup-vector-replay-product-v1`: all three bundles passed, totaling
108 successful trials, 216 verified samples (144 measured), 432 boundaries,
144 available cgroup phase peaks and 432 available CPU readings. Both report
verification modes and the wrapper checksum receipt passed. Safety and replay
identity tests passed. The packaged-container CI workflow now invokes this
recipe and retains its evidence; remote CI has not been executed here.

Final validation: all 239 recipe/UI tests passed, `go test ./agent` passed,
both shell scripts passed syntax checks and the edited workflow YAML parsed.
The archived bundle's cgroup identity/coverage gate was also rerun offline and
passed with 36 trials, 72 samples, 144 boundaries, 48 peaks and 144 CPU readings.

This does not establish native AMD64 results, exclusive CPUs, continuous machine
policy, or dedicated-host publication. Dedicated-machine qualification remains
explicitly unsupported, not waived. The original full-product goal stays active.

### Occupied-partition controller affinity evidence (2026-09-30)

The full-spec machine-policy audit identified a separation gap: empty-partition
readiness observed controller/collector CPU masks at run boundaries, but occupied
samples only observed worker masks and cgroup membership. New
`sampled-occupied-partition-v2` samples also bracket a bounded controller thread
inventory and read each thread's allowed CPU mask. Every mask must be disjoint
from the complete measurement CPU set. Inventory drift, missing/denied fields,
invalid IDs, oversized status files and exhausted aggregate budgets fail closed.
No affinity or host settings are changed. Reads are bounded to 4096 threads,
64 KiB per status and 8 MiB total per affinity sample.

Historical v1 receipts remain readable under their original narrower scope,
without invented affinity evidence. Monitor/sample versions must agree. The
publication audit is now v5 and reports `sampled_controller_separation` as a
separate requirement, only satisfied by valid v2 worker/controller samples.
Positive v2 and legacy-negative audit tests distinguish these contracts while
the dedicated-machine gate remains unsupported.

Read-only native Linux/ARM64 agent tests passed in an unprivileged, networkless
container; output is retained in `runs/linux-partition-controller-probe-v1/tests.log`.
The actual `/proc/self/task` probe read controller masks and rejected overlap
with an actually allowed CPU. This proves live source collection and rejection,
not an isolated partition or dedicated-host qualification. Deterministic tests
cover controller/collector overlap, absent masks, inventory drift, tampered
receipts, legacy compatibility, mixed-version refusal and reader budgets.

The final native reader/budget test suite also passed under
`runs/linux-partition-controller-probe-v2/tests.log`. Agent/analysis race tests,
the agent/experiment/analysis/CLI/publish regression suites and targeted vet
checks passed. Live isolated-partition tests remain explicitly skipped without
an externally provisioned partition; no host isolation claim is made.

The original full-product goal remains active: IRQ/kernel interference, external
collector separation, controlled machine policy and dedicated-host qualification
still need evidence beyond this sampled-affinity seam.

### Windows executable and replay portability (2026-09-30)

The full-spec platform audit found that Windows cross-compilation already
succeeded, but built adapter/analyzer paths, restored runner/analyzer names and
recorded report-builder staging omitted `.exe`. Go's installed Windows lookup
implementation confirms extensionless names are resolved through PATHEXT rather
than used as native executable paths. `NativeExecutable` now centralizes native
filename selection for build, resolution, analyzer pinning, restoration and
recorded-builder staging. Native runtime executable hashes remain pinned.

Archived object paths remain extensionless data with unchanged hashes and
single-volume layout. Restored/staged executable names receive the native suffix;
binary bytes do not change. Multi-volume tool archives now allocate independent
volume trees, preserving script/helper relative imports when Node lives on C:
and the checkout on D:. This does not qualify native PE dependency closure.

`recipes/test-windows-lifecycle.ps1` adds a native-only, new-evidence workflow
for wazero compiler/interpreter and V8: core correctness, timing, ordinary replay,
archived-tool replay, a separate memory/barrier pass and both report builders.
Its semantic verifier requires both core workloads, complete compile/instantiate/
first-call coverage, verified outputs, matching host/tool/workload identities,
archived relocation and explicit absent Linux collector values. Twelve tests
reject missing cells, false correctness, invented zero footprints, wrong phase
attachment and altered replay identities. A Windows CI job builds the native
tools, runs path/admission tests and retains workflow evidence. Native Windows
execution remains pending; no remote CI was run or published here.

Local verification passed full Go tests, experiment/publish/CLI race tests,
`go vet ./...`, all 251 recipe/UI tests, PowerShell parsing and workflow YAML
parsing. CLI cross-builds produced PE32+ AMD64 and ARM64 programs; Windows
experiment tests cross-compiled. The PowerShell recipe refused this non-Windows
host before creating evidence. Windows cross-volume tests require native Windows
and remain skipped locally.

An actual current-code Darwin/ARM64 regression completed original, ordinary and
archived-tool replay under `runs/platform-paths-native-*-v1`; all 18 measured
cells per bundle succeeded and each seal passed. The report at
`reports/platform-paths-native-regression-v1` passed current and recorded builder
checks. This verifies shared archive behavior on the current native platform,
not Windows execution, containment, process footprint accounting or dedicated
machine publication. The original full-product goal remains active.

### Reader-independent tool provenance paths (2026-09-30)

The next full-spec raw-evidence audit found that the initial Windows filename
fix still used the reader's `filepath` dialect for archive reconstruction and
analyzer validation. A macOS/Linux reader could therefore reject valid recorded
Windows drive/UNC paths, and a Windows reader could reject POSIX ELF provenance.
The new bounded recorded-path parser treats producer paths as metadata, never
as local files to open. Archive-relative trees and host-library basenames are
derived with platform-independent rules. Single-volume standard archive layouts
are preserved; Windows multi-volume trees remain deterministic and keep helper
imports together.

Canonical drive-letter, UNC and POSIX paths are accepted. Relative/drive-relative
paths, device namespaces, traversal aliases, alternate data streams, reserved
Windows components, mixed producer dialects and case-fold collisions are refused.
Windows collision keys conservatively use Unicode simple folding, including
sigma aliases, without a quadratic pairwise scan. Unpinned absolute-looking
command arguments still fail rather than hiding behind a noncanonical spelling.
Analyzer metadata validation and Linux ELF interpreter/library receipts no
longer depend on the reader OS; actual file verification/execution remains
separate.

Tests cover both path dialects, drive and UNC trees, per-volume relative imports,
canonical/unsafe forms and ASCII/Unicode collisions. A sealed synthetic Windows
archive loaded successfully on this macOS host using only stored object bytes,
without opening recorded Windows sources. Its checksum remained unchanged.
Incompatible-host restoration failed before creating output. This is a portable
evidence-format test, not native Windows runtime evidence. Windows CI now includes
these path/analyzer/ELF contract tests; remote native execution remains pending.

Final validation passed full Go tests, experiment/publish race tests, targeted
vet, Windows CLI/test cross-compilation and workflow YAML parsing. The current
CLI re-verified the existing native Linux cgroup bundle
`runs/linux-cgroup-vector-replay-product-v1/run`, the Darwin bundle
`runs/platform-paths-native-regression-v1` and its current-builder report dataset.
No measured bundles were edited or executable archives launched by those reads.

The original full-product goal remains active. Native Windows execution and
dedicated-machine publication qualification remain unproven; portable provenance
validation is not machine certification, dependency completeness or authenticity.

### Read-only IRQ-affinity readiness

Added `doctor --irq-cpus` and a versioned raw-evidence receipt for Linux default,
requested and effective device-IRQ CPU masks at two observations. Online CPU and
IRQ inventories must remain stable. All masks must exclude the complete supplied
measurement CPU set. Missing effective masks are not replaced with requested
ones. File/inventory/aggregate limits fail closed; validation recomputes verdicts.
No host settings are written and non-Linux systems report unsupported.

Deterministic tests cover mask parsing, overlapping routes, unavailable and
malformed evidence, offline CPUs, changed inventories, forged verdicts, source
immutability and resource bounds. Full Go tests, agent race tests and targeted
vet passed. The native Linux ARM64 test suite passed in an unprivileged,
network-disabled, read-only container using image
`sha256:e5e9eb19cb7ded680890db0702c20fdf4b98e264899ef631d08b3ebeac068ad1`.
The live probe retained 142 facts and rejected CPU 0 because the observed default
IRQ mask included it. This is diagnostic evidence, not isolated-host qualification.

IRQ delivery, local interrupts, background work and unsampled intervals remain
unqualified. Official publication remains blocked; the full-product goal is not
complete, and native Windows execution remains pending.

### Locked runtime IRQ boundary evidence

Runtime `run`, `check`, and `plan` now support `--require-irq-affinity`, pinned in
the lock with explicit CPU/cgroup resources. A non-ready start refuses collection
before output creation. Both run boundaries are retained in the immutable sealed
manifest; an end failure seals diagnostics, returns an error, and excludes
performance estimates through the shared host-evidence eligibility path. Receipt
validation rejects missing boundaries, mismatched CPUs, altered verdicts,
reversed timestamps and inconsistent prohibition labels. Concurrent host-policy
failures retain their established label precedence without waiving IRQ failure.

Existing locks cannot be overridden, including with an explicit false flag.
Replay keeps the lock requirement and reprobes the destination host. Legacy
bundles remain readable. Publication audit v6 separately assesses device IRQ
boundary evidence; dedicated-machine qualification remains unsupported and cannot
be waived by this new requirement. Source compiler runs do not yet expose it.

Tests cover lock policy, CLI planning/replay preservation and override rejection,
real-run preflight refusal before output creation, raw boundary validation,
diagnostic-only end failures and separate publication gating. These fixtures do
not constitute successful isolated-host collection or interrupt-delivery evidence.

Final validation passed full Go tests, agent/experiment/analysis race tests,
targeted vet and Windows AMD64 CLI cross-compilation. A new negative CLI test
caught planning's post-initial-validation flag assignment; planning now validates
the final lock before writing it. The freshly built current CLI re-verified the
existing Linux cgroup replay bundle, Darwin path-regression bundle and its report
dataset without changing measured evidence.

### Source-compilation IRQ policy parity

`source-bench --require-irq-affinity` now pins the same requirement in the source
benchmark configuration digest, with explicit CPU/cgroup resources validated by
the shared agent policy. Source collection refuses a non-ready start before
output creation or build admission. The finish path retains the end receipt,
including failed-build diagnostics, and preserves host/partition failure-label
precedence. Source host-evidence validation delegates to the same runtime receipt
contract; all three compiler measurement profiles withhold estimates on failure.
Raw compiler trial statuses and observed values remain unchanged.

Source replay and report-family preflight retain the configuration requirement
and reprobe the destination host before running tools. Tests cover source receipt
validation, missing and altered boundaries, diagnostic-only end failures, config
hash/JSON roundtrip preservation, invalid resource policies and exclusion across
timing, CPU and memory summaries. No uninterrupted IRQ or dedicated-host control
is claimed. Official publication and native Windows execution remain unproven.

Validation passed full Go tests; agent, experiment, sourcebuild and analysis race
checks; targeted vet; and Windows AMD64 CLI cross-compilation. Native Linux ARM64
source receipt tests passed in an unprivileged, network-disabled, read-only
container. The opt-in native Darwin LLVM workflow passed sequentially, including
IRQ preflight refusal before output creation, admitted timing and CPU builds,
sealed verification, replay from stored source snapshots after removal of the
test-owned original, and host-mismatch diagnostic checks. This does not establish
successful IRQ-qualified collection on a configured Linux host.

### Matched host-control policy across report and regression tracks

The specification-wide gap check found that paired report identity compared
partition policy but omitted the newly locked IRQ requirement. Cross-run latency
and native-code comparisons compared resource budgets but omitted pinned host
baselines and partition/IRQ requirements. They could therefore treat differently
controlled experiments as matched despite equal observed host fingerprints.

Added one shared host-measurement-policy matcher used by paired passes, latency
regressions (including source-output and end-to-end-stack comparisons), and native
code comparison. It requires equal resource fields, pinned baselines, and both
locked requirements. Readiness/estimate eligibility remains independently
validated; equality never qualifies absent receipts or authenticates a machine.

Tests cover symmetric mismatch rejection, all budget fields, independently
allocated equal baseline maps, changed baseline values, legacy policy equality,
paired memory/code identity and both cross-run comparison tracks. Existing policy
and measurement records are not rewritten. Dedicated-host official qualification
and native Windows execution remain unproven; the full-product goal stays active.

Final validation passed full Go tests, experiment/analysis/publish race tests,
targeted vet and Windows AMD64 CLI cross-compilation. The current builder
re-verified the path-regression report. Historical big-graph and native-code
pages rejected current-builder verification because their renderer/analysis
receipts predate the current implementation; they were left untouched rather
than resealed. New current-builder artifacts were derived from their sealed raw
inputs, without collecting new measurements:

- `reports/runtime-policy-match-v1`: paired timing/memory report, self-verification
  and independent `verify-report` both passed.
- `reports/native-policy-baseline-v1` and `reports/native-policy-candidate-v1`:
  current native-image exports from the stored Cranelift/Winch raw bundles. No
  LLVM disassembly was run or substituted for the older listings.
- `reports/native-policy-comparison-v1`: current native function-byte comparison
  generated from those exports and independently verified.

These checks exercise matching legacy host-control policy on real sealed inputs;
they do not prove an IRQ-qualified collection or native Windows execution.

### Host-import-free typed component export calls

Added `component-u64-v1` for named Component Model exports with 0..16 checked u64
parameters and one exact u64 result, no imported host capabilities, and a fresh
Store/instance per sample. The protocol and controller gate require a specific
advertised capability, explicit work denominator and reset/oracle contract.
Compile, instantiate and first-call are timing-only one-operation scenarios;
signature/export lookup, output verification and release stay outside the timed
API. Generic component execution remains false rather than implying arbitrary
WIT/world support. Strings, records, resources, other scalar types, batching and
memory/profiling/counter collection remain unfinished.

The pinned wat 1.251.0 generator creates a new-only fixture whose core global
changes on every invocation. Repeated verified samples therefore test instance
reset, not merely deterministic arithmetic. The JSON workload pins the generated
artifact digest and the generated Wasm is ignored, not authored with shell writes.
Rust tests cover both backends and all three stages, u64-max roundtrip, wrong
types/arity, missing exports, traps, wrong oracle, denied unresolved imports and
unsupported requests. All 21 adapter and six analyzer tests passed with the
component-fixtures feature. Full Go tests, protocol/experiment race checks, vet,
Windows CLI cross-build and all 251 UI/recipe tests passed. CI now runs the same
fixture and sealed replay qualification; remote CI execution is not claimed.

The initial `runs/component-u64-native-v1` controller qualification overlapped
the tail of the full Go suite and is functional-only evidence. After that suite
terminated, the sequential `runs/component-u64-native-v2` qualification passed:
original and restored-tool bundles each retain two sacrificial checks and six
measured backend/stage cells, 20 exact-result-verified samples. Negative signature,
missing export, trap, wrong-oracle and memory-profile bundles retain no eligible
samples. These are Darwin ARM64 exploratory diagnostics, not official numbers.

The rebuilt release adapter and current production CLI collected
`runs/component-u64-cli-v1` and reproduced it into
`runs/component-u64-cli-replayed-v1`, sequentially after builds/tests ended.
Each has 12 successful measured trials across six cells (two launches per stage/backend) and two
sacrificial checks, totaling 38 verified samples. The ordinary report at
`reports/component-u64-cli-v1` self-verified and passed independent verify-report;
the reproduced run passed checksum verification. No sibling corpus was changed.
Dedicated-machine publication, native Windows execution and broader component
contracts remain unproven; the full-product goal stays active.

### Typed component memory lifecycle and matched report

Completed the separate `can_component_u64_memory_v1` path for compile,
instantiate and first-call on Cranelift and Winch. Before/returned/released
barriers stay outside API timers; signature checking and oracle verification
remain outside timing. Release drops the Store and compile-owned Component,
retaining engine/linker and any shared compiled Component, with no forced
allocator purge. Timing remains uninstrumented. Controller tests reject absent
memory/call/phase capabilities before launching an adapter. Rust tests cover all
sample boundaries, both backends, unphased memory, failure prefixes, wrong
oracles, and rejected diagnostic profiles.

Full Go tests, race checks, vet, all 23 adapter and six analyzer tests, Windows
CLI cross-build, and 251 existing UI/recipe tests passed before collection.
`runs/component-u64-memory-native-v1` retains sealed original/restored-tool
timing and memory qualification with exact oracle and sample attachment checks.
The sequential production CLI runs `runs/component-u64-memory-timing-cli-v1`
and `runs/component-u64-memory-cli-v1`, plus their `-replayed-v1` siblings, each
retain 38 verified samples across 12 measured trials and two sacrificial checks.
The paired `reports/component-u64-memory-cli-v1` verifies independently.

Native Linux ARM64, unprivileged/read-only/no-network collection used immutable
image `sha256:e5e9eb19cb7ded680890db0702c20fdf4b98e264899ef631d08b3ebeac068ad1`.
`runs/component-u64-linux-cli-v1` contains timing/memory original, ordinary
replay, and relocated archived-tool runs. All six seals and the paired report
verify. Each memory bundle has 108 ordered boundary events, with available
procfs RSS/PSS/private/virtual observations attached to exact samples; each
measured launch retains kernel-accounted whole-process peak RSS. The reusable
`recipes/verify-component-u64.mjs` validates this coverage and preserves Darwin
collector gaps rather than fabricating zeros. No cgroup phase peak or dedicated
host was used; these remain exploratory results. Broader WIT types, native
Windows execution and dedicated-machine qualification remain unfinished.
The new verifier's 12 positive/negative tests and the final combined 263
UI/recipe tests passed after all collection terminated. All four Darwin CLI
bundle seals also verified independently.

### Operator-signed dedicated-host qualification: verification and audit

Added `analysis/qualification.go` as the single signing/verification module for
`operator-dedicated-host-qualification-v1`. It binds an Ed25519 operator statement
to the exact run seal, lock, host digest, CPU allocation, cgroup parent and all
recorded trial windows. Publisher trust is supplied separately; an envelope's
own public key is never sufficient. Domain-separated canonical signatures,
strict JSON (including duplicate-key and nesting rejection), complete dedicated
host assertions, maintained machine policy descriptions and an operational log
are required. Signatures authenticate the operator's assertions, not physical
truth, continuous kernel observations, or remote attestation.

Publication audit v7 accepts independently verified operator evidence without
waiving host/IRQ/partition/resource/pilot/correctness/coverage/stability gates.
Unsigned defaults and manifest labels remain blocked. The ordinary `publish`
path remains deliberately closed until qualification and pilot evidence are
archived with its report and independently verified. This is not a completed
official-publication feature and no dedicated development host is certified.

Added CLI `qualification-draft`, `qualification-sign`,
`qualification-public-key`, and explicit `publication-check --qualification`
and `--qualification-key`. Drafts leave all operator claims and policy empty;
unchanged drafts cannot be signed. Private seeds stay outside archives, require
private Unix permissions, reject symlinks and opening-time inode substitution,
and are cleared best-effort after use. New draft/sign/audit outputs are rejected
inside sealed input bundles or symlink aliases. Unit/CLI tests use synthetic,
explicitly non-certified fixtures and prove wrong-key, tamper, identity/window,
incomplete-policy, duplicate-wire, output-preservation and no-gate-waiver cases.

Full Go tests passed on a quiet rerun; the first concurrent full-suite run had
one source-build test's analyzer process killed and that result was not counted
as passing. Analysis/CLI/experiment race tests, vet, Windows CLI cross-build,
and all 263 UI/recipe tests passed. Current CLI generated the intentionally
blank `.wasmbench/component-u64-linux-qualification-draft-v1.json` and blocked
`.wasmbench/component-u64-linux-publication-audit-v1.json` from real sealed
Linux input without changing it; subsequent bundle verification passed. These
are read-only diagnostics, not new measurements or qualification evidence.
Next required work is qualified publication archival/verification; real
dedicated-host, native Windows, and broader WIT validation remain unfinished.

## Qualified publication archive and independent reader verification

Implemented `publish --qualification --qualification-key` through sealed output:
all gates run before output creation; confirmation, pilot, public signed
statement, recomputable audit, dataset, renderer and builder are archived.
`verify-report --qualification-key` requires a separately supplied reader key
and recomputes gates from both archived bundles. Ordinary report verification
checks consistency under the recorded publisher key only. Recorded-builder
execution cannot be combined with operator trust verification. Existing output
and paths inside either immutable input remain rejected; private keys never
enter archives. Reproduction intentionally emits exploratory reports without
historical qualification or pilot sidecars.

Entirely synthetic acceptance fixtures prove positive archival and wrong-key,
tampered statement/audit/pilot/receipt, resealing, source preservation,
pre-output rejection and reproduction-without-transfer cases. Production CLI
published `reports/synthetic-qualified-publication-cli-v1` from the retained
synthetic fixture and independently verified it using the separately stored
test public key. These receipts contain fake kernel/analyzer evidence and are
not measurements or host certifications. Browser QA verified the publication
trust disclaimer and public evidence links with zero console errors;
`output/playwright/synthetic-qualified-publication-v1.png` records its rendering.

Full Go suite passed; subsequent publish/CLI tests passed after the reproduction
and trust-mode tests. Analysis/publish/CLI race tests, Go vet, Windows amd64 CLI
cross-build, and all 264 UI/recipe tests passed. No native Windows runtime
execution or dedicated-host certification is implied by these checks. README,
publication guide and CLI help now describe the implemented archive path.

The runtime graph preview was also regenerated and verified at
`reports/runtime-big-graph-preview-v2`: five configurations, selectable segmented
compile/instantiate/execution latency, matched stage-focused whole-process RSS,
compact hover and exact click evidence. Browser hover/RSS click checks passed;
`output/playwright/runtime-big-graph-preview-v2.png` records its rendering. No new
performance collection occurred. All work remains local and uncommitted.
Remaining full-product evidence includes real dedicated-host qualification,
native Windows execution, and broader component/WIT coverage.

## Required acceptance-test execution coverage (2026-09-30)

Re-read the full twelve-section specification and added `docs/ACCEPTANCE.md`
mapping its requirements to implementation seams and appropriate verification
routes. This is an evidence map, not a completion certificate. In particular,
bounded advertised component profiles are implemented while arbitrary WIT worlds
and later adapter/browser/device candidates remain explicit extensions. Default
unit-suite PASS, configured remote jobs and cross-builds cannot prove live
adapter/platform acceptance or dedicated-host publication.

The structured default Go suite passed thirteen packages but skipped dozens of
opt-in native/runtime tests. Added `recipes/verify-go-test-coverage.mjs` to require
named top-level Go test execution and PASS, package start/completion, no failed
events and no skipped descendants. It rejects missing/duplicate test execution,
truncation, malformed JSON, wrong packages and output-text-only PASS claims.
Seventeen tests cover the state machine, structured stream and actual CLI exit
status. Producers must also have their exit status checked via pipefail where
supported; this gate verifies coverage, not host truth or assertion adequacy.

Wasmtime CI now uses the gate for both typed-component timing/memory/replay and
Preview 2 filesystem-reset/lifecycle/negative-contract/capability qualification,
with explicitly pinned Node. Local real release-adapter execution passed both
typed tests and all four Preview 2 tests without skips. Deliberately omitting the
component fixture variables produced a Go package PASS but was rejected by the
gate, demonstrating the original verification gap and fail-closed behavior.
All 281 UI/recipe tests passed. No remote CI, native Windows execution or actual
dedicated host is claimed. Component docs no longer incorrectly say that typed
memory passes are unsupported; README links the acceptance map and distinguishes
implemented publication gates from host certification. Work remains local,
uncommitted and unpushed; the full goal remains unproven.

## Cross-runtime and source/native acceptance refresh (2026-09-30)

Re-read the complete specification. Ran nineteen opt-in test groups with
`-count=1 -json` through the required-execution coverage gate, with no skipped
required tests or descendants:

- Float, empty-harness calibration, application initialization, teardown,
  expected traps and trajectory across Wago, wazero compiler/interpreter,
  Wasmtime Cranelift/Winch and V8.
- Engine usability on wazero/Wasmtime compiler configurations; AOT oracles on
  Wago/Wasmtime; WASI/Emscripten command lifecycle and reactors on
  wazero/Wasmtime; tool archive replay on Wago/wazero/Wasmtime/V8.
- LLVM source integration and source benchmark, then six native disassembly,
  forged mapping, recorded-builder, source/stack family replay and archived CLI
  replay tests. These run real tools and check correctness/reproduction, not
  official statistical performance on this shared development machine.

Production CLI `check --suite core` retained
`runs/acceptance-core-six-runtime-v1`: twelve successful, verified sacrificial
trials for mechanisms/identity and algorithms/sum across six configurations.
Ordinary reproduction retained another twelve successful verified trials at
`runs/acceptance-core-six-runtime-replayed-v1`; both bundles are correctness-only
and checksum-verified. Performance reporting rejected the original before output
creation, so no `reports/acceptance-core-six-runtime-v1` directory exists.
Publication-check wrote a blocked audit to
`reports/acceptance-core-six-runtime-publication-audit-v1.json`. Subsequent source
bundle verification passed; correctness evidence cannot qualify as timing data.

Wago CI now pins Node and requires the six core conformance groups through the
coverage gate rather than trusting a package PASS. Workflow YAML parsed and all
seventeen gate unit/stream/CLI tests passed. No remote workflow was launched or
claimed passing; no changes were committed or pushed.

Using an isolated uv dependency environment with DuckDB 1.5.6, executed the
documented `analysis/launches.sql` on the current graph report's Parquet. All
100 runtime/workload/scenario/profile launch-count and median cells agreed with
the independently verified dataset. `analysis/compiler-builds.sql` returned
both retained source-variant groups from `reports/source-report-replay-source-v3`.
Current production CLI verified both that source report and
`reports/runtime-big-graph-preview-v2` again. No installed global Python package
or input report was modified.

Read-only local checks found macOS PowerShell, Wine and QEMU executables but no
running Windows VM. These cannot substitute for native Windows acceptance.
Requested an approved native Windows access/evidence path and a real dedicated
Linux host with an independently trusted operator key; no such host or operator
assertion is inferred from available tools. Full completion remains unproven.
