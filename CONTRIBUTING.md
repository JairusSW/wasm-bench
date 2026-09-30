# Contributing

Keep changes focused, tested and explicit about measurement boundaries. Start
with [README](README.md), [workflow reference](docs/REFERENCE.md),
[acceptance](docs/ACCEPTANCE.md) and [versioning](VERSIONING.md).

## Set up and test

Use Go 1.26+, Rust/Cargo and Node.js. Run commands from the repository root.

```sh
make build
go test ./...
go vet ./...
node --test adapters/v8/*.test.mjs publish/report-ui.test.mjs recipes/*.test.mjs
cargo test --locked --manifest-path adapters/wasmtime/Cargo.toml --features component-fixtures
```

Opt-in native/adapter tests need their documented fixtures and tools. Use the
required-test gate in [acceptance](docs/ACCEPTANCE.md); skipped tests are not
native acceptance evidence. Linux collectors and Windows workflows require
their actual platform. Build before collecting; do not overlap benchmarks with
builds, uploads, tests or unrelated work on measurement CPUs.

## Measurement changes

- Define boundary, unit, scope, denominator, collector, quality and missing-data
  policy before adding a metric. Preserve versioned wire and evidence contracts.
- Verify useful guest behavior outside timing windows. Keep sacrificial checks
  separate and retain warmup, failures, traps, unsupported cases and raw evidence.
- Never substitute heap for RSS, sampled for exact peak, or batch averages for
  request percentiles. Do not infer compiler statistics from unproven heuristics.
- Replicate independent launches/blocks; report uncertainty and exact coverage.
  Performance claims need measured identities, resource policy and raw bundles.
- Preserve fixture bytes, hashes, licenses, generators and oracle provenance.
  Do not weaken admission or silently score only successful workloads.

## Send a change

Update the relevant tests, capability docs and Unreleased changelog. Explain the
change, affected boundaries, validation and limitations in the PR template.
Do not commit generated runs/reports/builds, native binaries, local knowledge
graphs, private keys or credentials. Put public result archives in a separately
reviewed publication destination, not in the source tree.

Contributions to project source are under Apache-2.0 unless explicitly stated
otherwise. Preserve the separately declared corpus/upstream licenses and add
appropriate attribution to NOTICE for incorporated third-party work.
