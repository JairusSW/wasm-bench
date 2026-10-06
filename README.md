<h1 align="center">Wasmbench</h1>

<p align="center">Reproducible WebAssembly experiments. Measurements you can inspect and replay.</p>

<p align="center">
  <a href="https://github.com/JairusSW/wasm-bench/actions/workflows/test.yml"><img src="https://github.com/JairusSW/wasm-bench/actions/workflows/test.yml/badge.svg" alt="Tests"></a>
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/go-%3E%3D1.26-00ADD8.svg" alt="Go >= 1.26"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache--2.0-blue.svg" alt="Apache-2.0"></a>
</p>

<p align="center">
  <a href="docs/REFERENCE.md">workflows</a> ·
  <a href="docs/ACCEPTANCE.md">support and acceptance</a> ·
  <a href="CHANGELOG.md">changelog</a> ·
  <a href="CONTRIBUTING.md">contributing</a>
</p>

Wasmbench compares fully specified runtime configurations, not just engine names.
Standalone adapters use each runtime’s embedding API, verify guest results, and
return raw samples. The controller locks inputs and collects evidence; the
website reads sealed reports.

> [!NOTE]
> Development build. Local Linux and macOS workflows have functional evidence.
> Native Windows acceptance and real dedicated-host publication qualification
> remain unverified. See the [acceptance map](docs/ACCEPTANCE.md).

## Why Wasmbench?

- Separate compile, instantiate, first-call, steady-execution and lifecycle measurements.
- Timing, memory, counters, code analysis and profiling are distinct passes.
- Raw samples, exact artifacts, configuration, uncertainty and replay instructions stay with the result.
- Unsupported, failed and unavailable results remain visible; missing measurements are never zero.
- A static, offline-capable dashboard with selectable stage bars, peak RSS, compact hover details and click-through evidence.

## Build

Requires Go 1.26+, Rust/Cargo (tested with 1.98.1), and Node.js for V8.
Build from a checkout; no release binaries or installer are published yet.

```sh
git clone https://github.com/JairusSW/wasm-bench.git
cd wasm-bench
make build
./bin/wasmbench doctor
```

The default build includes the independent Wasm analyzer and wazero adapter.
For Wasmtime or a local Wago checkout:

```sh
./bin/wasmbench build --runtimes wasmtime,wasmtime-winch
./bin/wasmbench build --runtimes wago --wago-source /path/to/wago
```

| Configuration | Embedding | Notes |
| --- | --- | --- |
| `wago` | Go | Built from an explicitly selected Wago checkout |
| `wazero`, `wazero-interpreter` | Go | Compiler and interpreter stay separate |
| `wasmtime`, `wasmtime-winch` | Rust | Cranelift and Winch stay separate |
| `v8` | Node.js | Production-default tiering; controlled modes are separate configurations |
| `v8-shell`, `spidermonkey`, `jsc`, `deno` | Direct JS engines | Core integer scalar timing and memory, with the bounded `identity-v1` callback |
| `wasmi`, `wavm`, `wasm3`, `wasmedge` | Native embeddings | Bounded core contracts; wasm3 has no separate instantiate stage |

Support depends on the scenario, ABI, profile, platform and advertised adapter capabilities.
WASI commands/reactors, bounded Emscripten hosts and Component Model profiles
have separate contracts. See [all workflows](docs/REFERENCE.md).
See [adapter setup and support limits](docs/ADAPTERS.md) for the additional engines.

Build the host-boundary workloads directly from the repository's Go source:

```sh
./bin/wasmbench corpus --suite calls --out calls.json
./bin/wasmbench check --suite calls.json --runtimes wasmtime,wazero,v8,jsc
```

The generated suite contains a plain exported call (host→Wasm) and a Wasm
export that calls the `wasmbench.identity(i32) -> i32` host import. Its source,
generator version, artifact digest and exact result oracle are retained in the
workload manifest.

## Run an experiment

```sh
./bin/wasmbench check --suite core
./bin/wasmbench run --suite core --out runs/example
./bin/wasmbench report --run runs/example --out reports/example
./bin/wasmbench verify-report --dir reports/example
./bin/wasmbench serve --dir reports/example
```

Fresh runs default to one process launch and one operation per sample. Timing
collects three samples for compilation, instantiation and steady execution, and
one for first call and other scenarios. Non-timing profiles take one sample.
Explicit `--samples` or `--samples-by-scenario` flags override the sample defaults;
existing locks keep their recorded policy. Timing also captures whole-process
peak RSS from those same trials; `--timing-peak-rss=false` disables it.

Open the address printed by `serve` (default: `http://127.0.0.1:8080`).
Runs retain measurements and input artifacts, but do not copy runtime binaries by
default. Add `--archive-tools=true` to `plan` or `run` for a self-contained tool
archive; otherwise replay requires the exact tools separately. See
[archive storage and compaction](docs/TOOL-ARCHIVES.md).
Select a workload and runtime configurations, toggle stages at the top, hover
for time and memory, and click a bar to inspect its raw evidence.
Compile, instantiate and execution share a segmented latency bar; RSS has its
own scale. Stage medians are separately measured, not end-to-end latency.

To collect additional memory diagnostics alongside the timing-pass RSS, collect a separate matched pass:

```sh
./bin/wasmbench run --suite core --profile memory --phase-barriers --out runs/example-memory
./bin/wasmbench report --run runs/example --memory-run runs/example-memory --out reports/example-paired
./bin/wasmbench verify-report --dir reports/example-paired
./bin/wasmbench serve --dir reports/example-paired
```

Peak RSS is the whole-process maximum for a stage-focused run, not a phase-only
peak. Linux phase accounting, allocator counters and residency snapshots retain
their own domains and availability labels.

## Inspect and reproduce

```sh
./bin/wasmbench verify --run runs/example
./bin/wasmbench inspect --run runs/example --workload algorithms/sum
./bin/wasmbench reproduce runs/example --out runs/example-replayed
```

Inputs, binaries, protocol, configuration, host and resource policy form the
experiment identity. Replay requires the exact locked tools and a compatible
host. Outputs are new-only; existing evidence is not overwritten.

Reports export JSON and Parquet for offline analysis. Source-to-Wasm and
end-to-end compiler/runtime tracks, paired comparisons, history, break-even,
scaling, density and snapshot diagnostics use the same evidence model.

## Publication and safety

Local reports are exploratory. Correctness-only checks cannot become performance
data. Official publication requires a sealed pilot, passing machine/resource
controls and operator qualification verified against an independently trusted
public key. A signature authenticates assertions, not physical host truth.
See [publication](docs/PUBLICATION.md) and [measurement policies](docs/HOST-BASELINE.md).

This is a trusted local experiment runner, not an untrusted-submission sandbox.
Do not run unknown guest code or archived executables with secrets or elevated
privileges. Build before measuring; keep unrelated work off measurement CPUs.

## Develop

```sh
go test ./...
go vet ./...
node --test adapters/v8/*.test.mjs adapters/js-shell/*.test.mjs publish/report-ui.test.mjs recipes/*.test.mjs
```

The default Go suite skips opt-in adapter/platform tests. A package PASS is not
proof of native acceptance. Follow the [contributing guide](CONTRIBUTING.md) and
[required-test coverage workflow](docs/ACCEPTANCE.md).

## License

Project source is licensed under [Apache-2.0](LICENSE). See [NOTICE](NOTICE)
for attribution. Corpus fixtures and recipes with explicit license declarations
retain those licenses, including the [MIT corpus license](corpus/LICENSE).
Third-party runtime dependencies and imported workloads retain their upstream licenses.

`export-site` also emits an explicit `report.tar.gz` file resource containing the
exact sealed report files and `checksums.json`. The offline derivative uses
`sealed-files-tar-gzip-v1`: sorted files, fixed tar metadata and gzip level 1.
Original file bytes are rechecked during packing; links and escaping paths are
rejected. The source seal digest accompanies bounded 1 MiB content chunks.
Limits are 100,000 files, 16 MiB seal metadata, 8 GiB raw input and 1 GiB compressed
output. This preserves existing sealed builder bytes without executing them.
It does not replace a shared session tool bundle or imply hermetic replay.
