# Bounded site export

```sh
wasmbench export-site --report reports/COMPLETE --out exports/NEW
```

The command uses the installed trusted report builder to verify the seal and
recompute its source dataset before projecting it. It never executes a builder
archived inside an input. The output must be new and outside the input report.
Older builder-incompatible reports fail verification; retain their legacy
exports or re-export them with an explicitly trusted matching producer.

`manifest.json` identifies the source report and seal independently. Its
`objects` inventory is bounded to 512 entries. `objects/SHA256` contains exact,
independently decoded JSON, at most 256 KiB each; consumers validate both full
hashes and decoded sizes. No data is synthesized from the website projection.
An oversized corpus, individual evidence row or catalog record fails explicitly
and leaves no complete export directory. Large evidence needs additional
chunking before this development format can handle it.

Records retain exact locked configurations, tracks, workloads, environment
facts, the producer metric registry, separate analysis versions, and result
summaries. The report records the actual exporting executable SHA-256, available
module/Go/VCS build fields, and the collecting runner's independent identity.
Referenced pass contexts retain full manifests, admission facts and locked options.
Timing launch medians and warmup arrays are separate evidence, excluded from
ordinary summary records. Trial/sample and observation objects retain pass, trial
and block identity. Trial envelopes link pass context, diagnostic details, adapter
samples and phase events through `references`; samples and observations retain
their legacy links. Scientific payloads are never interpreted as transport links.
Trial details preserve all remaining fields except `code_image`, whose binary
transport is pending. Individually oversized diagnostic objects fail explicitly.
The original report remains authoritative. Memory result profile
comes from its explicit stage metadata or its recorded report-level source;
timing-pass RSS is never multiplied by inner sample count.

Code-size measurement and raw-image availability are independent. This first
transport exports size descriptors, including engine-reported sizes without
images; it does not yet export native binaries or inspection resources. Exact
`size_bytes` values outside JavaScript's safe integer range are decimal strings
in descriptors and code-size summaries. No hash or download is offered for
unexported native bytes.

The current registry omits the `native.code_size` metric already used by code
records. Its export includes an explicit `unregistered` marker and retains the
size. The marker is not a replacement scientific definition. A real registry
addition needs report compatibility review.

A consumer may trust an authenticated producer's `source-recomputed` assertion,
but validating object hashes independently proves only transport integrity.
Operator publication qualification is separate. The website service intentionally
uses this wire boundary rather than importing harness execution/storage packages.

Tests compare verified-source timing fields with exported fields, cover a single
launch with multiple samples and absent intervals, separate and timing memory
passes, size-only code, unsafe integer precision, deterministic output, corrupt
input rejection and failed-output cleanup:

```sh
go test ./publish ./cmd/wasmbench
```

This is a development producer contract. Additional analytical evidence exports,
native inspection, package publication and the consuming frontend migration
remain separate follow-up work. Existing report formats and readers are unchanged.
