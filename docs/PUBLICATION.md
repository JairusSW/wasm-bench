# Publication evidence audit

Exploratory reports are available with `report`. Official publication has a
separate gate: editing a manifest's publication label cannot qualify a run.

```sh
wasmbench publication-check --run runs/confirmation --out publication-audit.json
# If the original pilot was moved, supply its new location without changing it:
wasmbench publication-check --run runs/confirmation --pilot-run archived/pilot
# Publishing additionally requires separately trusted operator evidence (below).
```

`publication-check` loads checksum-verified evidence and writes a new-only audit
file (or JSON to stdout), then exits nonzero if any requirement is unmet or
unsupported. It does not modify run bundles, host settings, or publication labels.
`publish` applies the same audit before creating a report directory.

The versioned audit separately reports:

- Timing measurement scope, excluding correctness-only and instrumented passes.
- A pinned host baseline matching both recorded boundaries.
- Locked device-IRQ affinity receipts at both run boundaries, recomputed from
  default, requested and effective masks disjoint from measurement CPUs. This
  establishes observed routing masks, not delivery or continuous control.
- A locked isolated CPU-partition requirement with independently consistent
  ready observations before preparation and after worker cleanup. This is a
  boundary check, not continuous isolation.
- Valid occupied-partition samples for each successful measured runtime trial,
  checking the requested CPU masks and single worker between those boundaries.
  Intervals between samples remain unqualified.
- Sampled controller/collector CPU separation. Version 2 occupied samples
  bracket bounded controller-thread inventories and per-thread allowed CPU
  masks; every mask must be disjoint from measurement CPUs. Legacy v1 samples
  remain readable but cannot satisfy this separate requirement.
- The complete pilot decision, recomputed from the sealed pilot. Confirmation
  must preserve the pilot's experiment contract except for the fixed chosen
  launch count and embedded decision. Host observations must match. A separate,
  later run identity is required, but timestamps do not prove independence.
- Pinned independent artifact admission for every declared artifact.
- One successful sacrificial correctness trial per runtime/workload, retaining
  verified outputs separately from measured launches.
- Complete successful, verified launches and estimable intervals for every
  declared cell, with detected within-launch instability rejected. Unsupported
  and failed cells cannot be dropped to qualify an incomplete matrix.
- Explicit CPU, NUMA-node and memory budgets, with independently recomputed
  spawn/end resource readbacks for successful trials.
- Dedicated-machine qualification.

The v7 audit supports **externally trusted operator qualification**. An Ed25519
signature authenticates the operator's assertions; it does not independently
prove their physical truth. The publisher must obtain the public key through a
separate trusted channel and deliberately supply it. A key included in an
envelope is never trusted automatically. No manifest label, self-signed key,
host baseline, or cgroup boundary observation replaces this trust decision.

The operator statement binds the exact run ID, run seal, locked experiment,
host fingerprint, measurement CPUs and cgroup parent. Its validity interval must
cover manifest creation and all recorded trial windows, and issuance must follow
collection. It asserts exclusive measurement CPU reservation, controller/collector
separation, exclusion of unrelated work/builds/uploads, and maintained machine
policy throughout that interval. Frequency, SMT, NUMA, huge-page and OS policy
descriptions and a reviewed operational log are mandatory. Operators must
establish these conditions independently; the runner does not alter the machine
or turn prose into kernel proof. All existing kernel, resource, pilot,
correctness, complete-matrix and stability gates remain mandatory.

```sh
# Produces an intentionally incomplete draft; cannot qualify or sign unchanged.
wasmbench qualification-draft --run runs/confirmation --out operator-draft.json
# Review/fill the statement and save a new operator-reviewed.json. Do not claim
# dedication for a development machine, shared container, or unqualified host.
# The operator provisions a private 32-byte hex Ed25519 seed outside the repo.
wasmbench qualification-public-key --seed-file /secure/operator.seed \
  --out operator-public.json
wasmbench qualification-sign --run runs/confirmation \
  --statement operator-reviewed.json --seed-file /secure/operator.seed \
  --out operator-signed.json
# Publisher obtains operator-public.json independently through a trusted channel.
wasmbench publication-check --run runs/confirmation --pilot-run runs/pilot \
  --qualification operator-signed.json --qualification-key operator-public.json \
  --out publication-audit.json
wasmbench publish --run runs/confirmation --pilot-run runs/pilot \
  --qualification operator-signed.json --qualification-key operator-public.json \
  --out public-report
# Readers obtain this key independently; do not trust a key from the report.
wasmbench verify-report --dir public-report \
  --qualification-key /trusted/operator-public.json
```

The private seed is read only for signing/export, requires private permissions
on Unix, and is never copied to the statement, audit or run bundle. Protect it
with an OS ACL on Windows. Keep keys outside untrusted workloads and publishing
credentials out of the measurement worker. Do not put secrets in the operational
log: the signed statement is intended to become public evidence. A trusted key
may be replaced or rejected by the publisher; this v1 format does not implement
an online revocation service, remote attestation or tamper-proof host telemetry.

Signing and draft/audit outputs are new-only and must remain outside input
bundles, including symlink aliases. Strict JSON parsing rejects unknown fields,
duplicate keys, excess nesting, oversized documents and trailing values.
Qualification is for one exact historical collection, not a reusable lease for
future runs. Reproduction needs a new statement for the new seal and windows.

The qualified publication archive path is implemented. `publish` checks all gates
before creating output, then archives the sealed confirmation (`raw/`), sealed
pilot (`raw-pilot/`), public signed statement (`qualification.json`), recomputable
audit (`publication-audit.json`), dataset, renderer and recorded analysis builder.
The report seal covers these files; neither input bundle is modified. Existing
output directories and output nested inside either input are rejected.

Ordinary `verify-report` checks integrity and consistency under the recorded
publisher key, not independent reader trust. Supplying `--qualification-key`
additionally authenticates against the reader's separately trusted key and
recomputes every publication gate from archived evidence. A wrong key, altered
statement, pilot, audit or receipt is rejected even after resealing the archive.
This trust-verification mode cannot be combined with `--recorded-builder`, which
executes archived code and requires a different explicit trust decision.

`reproduce-report` produces new exploratory measurements; it does not transfer
historical qualification to a new run seal or collection window. Qualify the new
collection independently before publishing it. Acceptance fixtures are entirely
synthetic and exercise archive validation, not machine certification. No real
dedicated host has been qualified by the local tests. Exploratory reports, raw
evidence export and coverage analysis remain available.

`doctor --irq-cpus` offers a separate [read-only IRQ diagnostic](IRQ-AFFINITY.md).
It observes device-IRQ route masks, not delivery or uninterrupted machine control.
Runtime runs can lock `--require-irq-affinity` to collect and enforce both boundary
receipts. Neither doctor nor the locked receipt can waive dedicated-machine
qualification or alone make an exploratory run eligible for official publication.

Checksums establish local file integrity, not producer authenticity. The pilot
budget is a planning heuristic, not a precision guarantee or proof of statistical
power. See [pilot planning](PILOTS.md) and [host baselines](HOST-BASELINE.md).
