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
`objects` inventory is bounded to 512 entries. Larger exports use an empty
`objects` array and `inventoryPages` descriptors. Each content-addressed inventory
page is `{schema:1,objects:[...]}` with at most 512 payload descriptors, and the
root contains at most 512 pages. A page descriptor records its full SHA-256,
decoded `bytes`, payload `objects` count and total payload `contentBytes`.
Consumers must verify all four before granting payload upload permissions; these
commitments let admission reserve full declared storage before page expansion.
Payloads may not also appear as inventories or be repeated across pages.

`objects/SHA256` contains exact, independently decoded JSON, at most 256 KiB
each, or admitted `binary` native-image objects at most 16 MiB. Native originals
are stored by their complete SHA-256, without base64 in website evidence.
Consumers validate both full hashes and decoded sizes. No data is
synthesized from the website projection. More than 262,144 payload objects, an
individual oversized catalog/result record or evidence value above 64 MiB fails
explicitly and leaves no complete export directory. Legacy small-manifest encoding
is unchanged.

Oversized JSON evidence becomes a `json-resource` descriptor with `schema:1`,
`encoding:"json-utf8"`, original `bytes` and `sha256`, and ordered `references` to
fragments. Each `json-fragment` contains `schema:1` and at most 120 KiB of UTF-8
`text`. Concatenate the text bytes before parsing: JSON escapes may cross fragment
boundaries, but UTF-8 code points never do. The descriptor and fragment files each
stay within the ordinary 256 KiB object ceiling. A logical resource is limited to
64 MiB and 1,024 fragments. Consumers verify fragment hashes, byte count and
original hash, and reject invalid/ambiguous reassembled JSON before publication.
Resource reads return one descriptor/fragment; assembly is an explicit evidence
consumer action. Small evidence objects retain their existing encoding.

Paged inventories require two-stage consumer admission. The website service
reserves page and payload bytes up front, uploads the page, then attaches it to
its specific import before granting leaf permissions. The coordinator implements
this protocol. A consumer must verify all page commitments and reject unresolved
or repeated payload references before publishing a complete corpus job. Producer
and consumer support are still development branches; production pins remain
unchanged.

Records retain exact locked configurations, tracks, workloads, environment
facts, the producer metric registry, separate analysis versions, and result
summaries. The export manifest records the actual exporting executable SHA-256, available
module/Go/VCS build fields. The report retains the collecting runner's independent
identity; an exporter upgrade does not modify that canonical report.
Referenced pass contexts retain full manifests, admission facts and locked options.
Timing launch medians and warmup arrays are separate evidence, excluded from
ordinary summary records. Trial/sample and observation objects retain pass, trial
and block identity. Trial envelopes link pass context, diagnostic details, adapter
samples and phase events through `references`; samples and observations retain
their legacy links. Scientific payloads are never interpreted as transport links.
Trial details exclude the `code_image` base64 payload; admitted originals have
separate binary and metadata resources. Oversized diagnostic values use the referenced JSON resource format.
The original report remains authoritative. Memory result profile
comes from its explicit stage metadata or its recorded report-level source;
timing-pass RSS is never multiplied by inner sample count.

Code-size measurement and raw-image availability are independent. The producer
now reuses the existing `nativeRecord` rules and `CodeImage.Validate` contract to
export admitted images from the separately verified code pass. It checks trial,
runtime, workload, module hash, full image hash and recorded image size. Withheld
records do not export content. Engine-reported size-only records retain their
unavailable-content descriptors.

Artifact descriptors stay below 10 KiB. They link small image metadata and
independently bounded function-array resources, retaining exact offsets, lengths,
Wasm indices, tiers and generation. Original function names and ranges are not
inferred or regenerated. Function inspection can be available while disassembly
remains explicitly unavailable; existing native-image interpretation accompanies
the artifact. No request-time disassembly is introduced.

The website service/coordinator admit `binary` objects with separate 16 MiB limits.
Original downloads and selected byte reads require published artifact membership;
metadata/function resources load independently. Empty originals retain their full
hash and available-content state. The service's observation policy prevents a
representation update from creating another measurement of the same sealed source.
Offline disassembly derivatives and broader native scale tests remain open; these
are development branches, not a deployed native-inspection service. Exact
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

This is a development producer contract. Standalone analytical files, offline
native derivatives, package publication and the consuming frontend migration
remain separate follow-up work. Existing report formats and readers are unchanged.

Each result now includes a bounded `measurementMethod` and its full
`measurementMethodId`. The descriptor retains the source profile, metric,
scenario, statistic, locked protocol/runner/host-policy fields and a normalized
per-scenario recipe. Suite names and other scenario selections are scheduling
metadata and do not split the recipe; the actual scenario sample override remains.
Unsafe integer seeds and durations use exact decimal strings in this recipe.
The descriptor is at most 64 KiB, with a 32 KiB recipe and at most 32 distinct
observation identities.

Memory identities come from the contributing source trials and the existing
memory observation eligibility function: definition version, unit, scope, phase,
collector/version, quality, actual profile and normalization denominator survive.
Trial-scoped RSS is not multiplied by timing sample counts. Pass IDs and exporter
binary identities are provenance and do not define a reusable method. Changed
collector versions or locked recipes produce different method digests. Timing
samples and native code records do not universally name their collector, so those
descriptors say `not_recorded`; the exporter does not infer a collector from the
OS or runtime name. A missing source pass yields an unavailable descriptor.

These selectors project existing verified evidence without changing source
statistics or scientific definitions. Website compatibility/cohort policy still
needs to decide which recorded methods can be compared; matching a digest is not
a substitute for operator qualification or independent source verification.

Report descriptors also retain `sourceSchema`, independent source analysis
versions, and `analysisSections` under version `source-fields-v1`. Each of at most
64 entries names an original derived report JSON field and references a
`report-analysis` envelope bound to the report and field. Throughput, scaling,
counter displays, CPU stacks, memory timelines and other derived JSON values are
copied from the verified source, with no new analysis. Future derived fields are
retained when their bounded source names fit the transport. Large section bodies
use the same JSON-resource/fragments contract as detailed trial evidence.

Core typed results, metrics and passes keep their existing transport; report
metadata contains references rather than complete analysis arrays. Consumers must
include the section roots in import verification, portable recovery and retention.
Legacy reports without these references remain readable. Standalone analytical
files and bulk archives are not represented by these JSON sections.

Native metadata also records `functionIndexVersion: producer-order-v1` and
`functionShards`: ordered shard digests with exact row counts. The original
`functions`/`references` remain for existing chunk readers. This permits a selected
function page to skip preceding shards without loading every function. Consumers
must validate counts against shard rows before publishing. At most 4,096 shards
and one million attributed functions fit this index contract. Function ordering,
indices, tier/generation and byte ranges remain producer-owned; no disassembly is
performed by this transport or by ordinary API reads.

Results with a complete source pass and recorded timestamp now include a bounded
`samplingGroup`. It identifies the exact source manifest and canonically ordered
trial-ID/trial-hash population for the runtime, workload, scenario and profile.
Trial count describes source trial records, not an inferred number of independent
launches. Separate passes with coincident block numbers have different identities;
report-analysis versions and exporter binaries do not define the group. Missing
source context yields no invented group. Memory uses its contributing trial set;
timing retains its source outcome population, including failed trials. Statistical
analysis and cross-report history/uncertainty policy remain consumer concerns.
