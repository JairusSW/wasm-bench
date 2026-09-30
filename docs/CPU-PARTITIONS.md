# Read-only CPU-partition readiness

Probe an already configured Linux cgroup v2 partition before launching workers:

```sh
wasmbench doctor --cpu-partition /sys/fs/cgroup/measurement --measurement-cpus 2-3
```

The probe never creates a partition, moves tasks, changes affinity, or writes
kernel settings. The live Linux path must reside on a cgroup v2 filesystem.
macOS and other platforms return `unsupported`; omitted arguments return
`not_requested`. Doctor reports diagnostic statuses in JSON, not via its exit
code. Missing files, denied access and invalid state are not treated as success.

`empty-isolated-partition-readiness-v1` requires two observations showing:

- A valid `isolated` partition root, not `member`, `root`, or an invalid root.
- Effective and exclusive-effective CPU masks exactly matching the requested
  complete CPU set. A subset is intentionally insufficient.
- `cgroup.events` reporting an empty subtree (`populated 0`). Run the probe
  before workers start, not while they occupy the partition.
- All requested CPUs online, with no online-set change between observations.
- Every controller thread's allowed CPU mask disjoint from measurement CPUs.
  This includes in-process collector threads. The thread inventory must remain
  unchanged during the probe; otherwise retry after activity settles.
- Every online SMT sibling of a measurement CPU included in the partition.
  Offline siblings are allowed, but hotplug changes invalidate the observation.

Raw source facts are retained, with availability statuses. Each file is bounded
to 64 KiB, total reads to 8 MiB, and the controller inventory to 4096 threads.
`CheckCPUPartition` independently recomputes the result from those facts and does
not trust a stored success label.

A `ready_at_observed_boundaries` result is **not official machine qualification**.
Reads are not atomic. Activity or affinity changes between reads can escape
detection; IRQs, pinned kernel work, frequency policy, other processes launched
later, and external collectors are not qualified. It is not a continuous monitor
and does not unblock `publish` by itself.

## Locked run requirement

Runtime `run`, `check`, and `plan`, plus `source-bench`, accept:

```sh
wasmbench run --suite core --runtimes v8 \
  --cgroup-parent /sys/fs/cgroup/measurement --cpus 2-3 \
  --require-isolated-cpu-partition --out runs/partition-checked
```

The boolean requirement is pinned in the experiment lock/source benchmark
configuration. Its partition path and complete CPU set come from the resource
policy; existing locks cannot override the boolean, even to false. Reproduction
and source benchmark replay retain it. This option does not configure isolation:
an administrator must already have configured a suitable partition and controller
placement. Unsupported platforms refuse execution instead of weakening policy.

Before creating the output directory, the runner requires a ready observation.
After all trials and worker cleanup, it probes again. A failure then seals the
diagnostic bundle, returns an error, and prohibits measurement eligibility.
Offline loaders re-derive both observations, require their paths/CPU masks to
match the lock, and reject missing endpoints or forged success labels. A failure
to read the ending filesystem is retained as unavailable evidence, never success.
Raw correctness outcomes and samples remain intact; derived latency, memory,
scaling and aggregate views exclude affected trials. Source timing/CPU/memory
summaries apply the same rule. Start/end checks bracket the complete experiment,
including preparation and sacrificial admission. Runtime adapter trials also
sample the occupied partition immediately after spawn, every 250 ms, and before
worker cleanup. `sampled-occupied-partition-v2` records raw effective/exclusive
CPU masks, online CPUs, parent/worker population, root and worker process
membership, and other child-cgroup population. The verdict is recomputed on
offline load; a changed mask, extra process, populated sibling child, unreadable
fact, or sample overflow prohibits performance eligibility while preserving the
sealed raw trial. Every started adapter trial must have samples once this
evidence is used in a bundle. The worker must contain exactly the adapter PID;
adapter subprocesses in the same leaf are not qualified by this policy.
Source-build benchmarks do not yet record occupied-partition samples.

Version 2 also inventories every controller thread before and after each sample
and checks each allowed CPU mask for overlap with the measurement CPUs. This
includes in-process collector and monitor threads. A changed inventory,
missing/denied mask, overlap, malformed inventory or exhausted read budget fails
the observation. The inventory is bounded to 4096 threads; status reads are
bounded to 64 KiB each and 8 MiB total per affinity sample. These reads do not
change affinity. Stable inventories do not exclude short-lived threads between
reads; allowed masks are not actual CPU-residency traces. External collectors,
IRQs and kernel work remain outside this evidence.

Historical v1 samples retain their original masks/population/single-worker scope,
without invented controller observations. A monitor cannot mix v1 and v2 sample
contracts. The publication audit separately requires v2 controller separation,
in addition to sampled worker membership and the still-unimplemented dedicated
machine contract.

These are periodic observations, not continuous monitoring. Short-lived
interference between samples may escape detection. The audit reports sampled
worker evidence separately but still blocks official publication on missing
dedicated-machine qualification.

The publication audit reports this boundary requirement separately from host
baseline matching and leaf resource readbacks. Dedicated-machine qualification
still requires additional evidence and remains unsupported.

An opt-in Linux integration test uses the same read-only live probe. Configure
the partition externally, then set `WASMBENCH_CPU_PARTITION` and
`WASMBENCH_MEASUREMENT_CPUS` and run
`go test ./agent -run TestCPUPartitionLiveReadOnly -v`. Without both variables the
test skips; it never grants privileges or configures the host itself.

The interpretation follows the Linux kernel's
[cgroup v2 cpuset partition contract](https://www.kernel.org/doc/html/v6.11/admin-guide/cgroup-v2.html#cpuset).
Isolated partitions exclude other partitions from their exclusive CPU set and
disable scheduler load balancing; these properties alone do not establish a
quiet or dedicated measurement host.
