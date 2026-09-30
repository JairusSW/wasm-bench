# Read-only IRQ-affinity readiness

```sh
wasmbench doctor --irq-cpus 2-3
```

Supply the complete measurement CPU set, including any SMT siblings allocated
to the experiment. The `irq_affinity_probe` JSON records the requested CPUs,
version, scope, timestamp, derived verdict, and raw facts. It never writes CPU
or IRQ settings, changes services, or requests elevated privileges.

On Linux, two sequential observations read the online CPU list, the hexadecimal
default IRQ mask, the IRQ directory inventory, and each IRQ's requested and
effective CPU lists. Every observed mask must exclude the measurement CPUs;
the online CPU set and IRQ inventory must remain identical at both observations.
Missing effective-affinity evidence is unavailable, not substituted with the
requested mask. Sources retain availability statuses and reasons.

The kernel documents requested affinity and default routing in its
[IRQ-affinity contract](https://www.kernel.org/doc/html/latest/core-api/irq/irq-affinity.html).
Requested routes are not evidence of actual interrupt delivery.

Verdicts distinguish `not_requested`, `invalid_request`, `unsupported`,
`unavailable`, `invalid_evidence`, `not_ready`, and
`disjoint_at_observed_boundaries`. The last verdict describes only observed
device-IRQ masks, not a continuously controlled host. Non-Linux platforms return
an explicit unsupported result. `doctor` remains a diagnostic command, not a
publication gate, and its successful exit does not imply host readiness.

Reads are bounded to 4096 directory entries per inventory, 64 KiB per file,
and 8 MiB of aggregate file content. Oversized or incomplete evidence fails
closed. The receipt validator recomputes the verdict from facts rather than
trusting the stored label. Checksums and internally consistent receipts do not
authenticate the producing machine.

## Locked runtime-run requirement

```sh
wasmbench plan --runtimes wazero --cgroup-parent /sys/fs/cgroup/measurement \
  --cpus 2-3 --require-irq-affinity --out irq.lock
wasmbench run --lock irq.lock --out runs/irq-confirmation
```

The boolean requirement and explicit CPU resource budget are part of the lock.
Planning does not qualify the machine. A run probes before creating its output
directory or preparing adapters, and refuses unavailable, unsupported, overlapping
or invalid evidence. It probes again after all trials and retains both receipts
in the sealed manifest. An end failure returns an error after sealing diagnostics
and withholds performance estimates. Other host-policy failures may take priority
in the publication label; the raw IRQ end receipt still determines eligibility.
Offline verification recomputes both verdicts, checks the locked CPU identity and
boundary order, and refuses missing or inconsistent receipts.

Replay preserves this requirement and reprobes the destination host. Explicit
`--require-irq-affinity` flags cannot override an existing lock, even when false.
Legacy runs without the requirement remain readable but do not satisfy the new
publication audit's IRQ-boundary requirement.

## Source-compilation benchmarks

`source-bench` accepts the same `--require-irq-affinity`, `--cgroup-parent` and
`--cpus` options. The requirement is included in `benchmark.lock.json` and its
configuration digest; both boundary receipts are in `benchmark.json`. The start
check precedes benchmark output creation and sacrificial build admission. The
end check runs before sealing, including on paths that retain failed builds.
Failed end evidence is diagnostic-only for timing, CPU and memory profiles;
compiler exit statuses and raw readings remain intact. Replay and report-family
preflight retain the requirement and reprobe the destination host before builds.
Legacy source benchmarks without this field remain verifiable.

These are outer benchmark boundaries, not individual compiler-step barriers or
continuous IRQ observations. Compiler subprocesses use the declared cgroup CPU
budget; correctness-admission adapters remain separate from measured compiler
steps. This does not qualify background work or official source publication.

## Comparison identity

Paired report passes and cross-run latency/native-code comparisons must agree on
the locked IRQ requirement, isolated-partition requirement, complete resource
budget and pinned host baseline. Matching observed host fingerprints does not
permit joining a run with this requirement to one without it. Matching requested
controls also does not establish readiness: each run's receipts must separately
pass validation before measurements are eligible.

This check does not qualify interrupt delivery, local timers, NMIs, IPIs,
softirqs, kernel work, external collectors, background processes, or changes
between observations. The publication audit checks its locked boundary evidence
separately; it is not sufficient for official run qualification.
Dedicated-machine publication remains unsupported; see
[publication evidence](PUBLICATION.md) and [CPU partitions](CPU-PARTITIONS.md).
