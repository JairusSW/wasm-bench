# Requested resource-policy readback

Linux cgroup requests are now verified before spawning an adapter or source-build
tool. `requested-cgroup-leaf-readback-v2` compares each explicit request with its
kernel readback: memory limit/OOM grouping, swap prohibition, CPU quota/period,
task limit, and both requested and effective CPU and NUMA-node sets.

`--mems 0` (or a comma/range node list) requests allowed NUMA nodes for `run`,
`plan`, `check` and `source-bench`. It requires an explicitly delegated cgroup.
The node request is part of the resource policy in the lock/receipt; existing
locks cannot be overridden with `--mems`. Omission inherits parent allowances.
Both `cpuset.mems` and `cpuset.mems.effective` must match before spawn and at the
final boundary. No parent settings or existing-task policies are modified.
The request constrains allowed allocation nodes, not exact physical residency,
interleaving, first-touch behavior, per-page placement or exclusive NUMA access.
For newly requested NUMA policies, successful stored trials and tool receipts
must contain consistent v2 initial/final evidence; offline loading rechecks it.
Historical v1 receipts without NUMA requests retain their original meaning.

CPU sets are compared as bounded sets, not strings: `1-3` and `3,1,2` match.
Duplicates, overlaps, reversed ranges and expansions above 4,096 CPUs are
rejected during policy validation. A narrowed **or expanded** effective set
does not match the request, even if writing `cpuset.cpus` succeeded. Linux can
grant different CPUs because of ancestors or hotplug; see the
[kernel cgroup documentation](https://docs.kernel.org/admin-guide/cgroup-v2.html#cpuset).

Requested numeric limits must read back exactly. In particular, use a page-aligned
memory limit; a kernel-rounded value is not silently accepted as the requested
budget. Missing/unreadable fields receive `unavailable`, not guessed defaults.
A mismatch or unavailable explicit readback prevents process launch. Unspecified
controls remain inherited; they are not certified by this check.

`isolation.verification` retains the `before_spawn` result. Adapters get another
check at `response_end_before_cleanup`; source tools at `tool_exit_before_cleanup`.
`isolation.final_verification` retains an independent copy of those settings.
An endpoint mismatch makes an otherwise successful trial/tool step fail, while
preserving its measurements and the failed readback. Existing timeout/OOM/error
outcomes remain failures. Checks take place outside adapter-local measurement
loops and tool wall timers; no continuous polling is added to timing passes.

These are boundary observations of explicitly requested **leaf** controls, not
proof of uninterrupted enforcement, exclusive CPUs, actual CPU residency, NUMA
placement, effective ancestor budgets, fixed frequency, absence of competing
work, or dedicated-machine qualification. A changed-and-restored setting between
boundaries can go undetected. `not_requested` means no explicit controls were
selected, not unlimited resources. Non-Linux unisolated runs retain their prior
uncontrolled status; requested Linux isolation still fails without fallback.

Unit tests cover normalized CPU sets, narrow/wide readback, missing/unreadable
fields, numeric drift and independent evidence copies. Linux filesystem-injected
tests exercise the reader and endpoint drift without pretending regular files
enforce limits. Delegated-cgroup lifecycle tests additionally assert both real
boundary records when that explicitly configured test environment is available.

## Private-container vector phase qualification

A reusable new-only recipe collects and verifies the original, ordinary replay
and archived-tool replay inside the same disposable private hierarchy. Build
the native tools documented in the README's Linux vector recipe, then build
the matching native agent test executable before starting:

```sh
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go test -c -o .wasmbench/agent-cgroup-vector.test ./agent
WASMBENCH_EPHEMERAL_CGROUP_TEST=1 sh recipes/test-linux-vector-cgroup.sh \
  wasmbench:ci linux-vector-cgroup
```

Use `GOARCH=amd64` instead for a native AMD64 daemon. The recipe checks the
immutable image ID and rejects mismatched image/daemon architectures before
creating evidence. Explicit opt-in is required because the qualification
container is privileged. It never mounts host cgroups, Docker sockets or
credentials. Existing evidence directories and dangling aliases are preserved.
The ordinary procfs recipe remains unprivileged; use this additional recipe
only in a trusted local Docker environment, not for untrusted submissions.

All collections finish before report rebuilding and integrity checks. Each
bundle seal and the semantic coverage gate must pass; replay additionally
matches workload, runner and adapter identities, with relocation required for
archived tools. The recipe verifies both current and recorded report builders
and retains a checksum receipt for setup, verifier, fixtures, image identity,
restoration receipt and collection log. A failure retains its evidence and
does not print a qualification success message.

The existing `agent/test-cgroup-container.sh` harness has an opt-in
`WASMBENCH_RUN_VECTOR_PHASE_SMOKE=1` path. It requires its explicit ephemeral-test
flag, `/.dockerenv`, and a private cgroup namespace (`/proc/self/cgroup` is
`0::/`). Do not run this setup on the host. It creates controller and worker
subgroups only in that disposable container, then runs agent tests before
collecting both ordered-vector fixtures across all six initial configurations.

Build a native Linux agent test executable before collecting:
`CGO_ENABLED=0 go test -c -o .wasmbench/agent-cgroup-vector.test ./agent`.
On a non-Linux build host, set `GOOS=linux` and the native Docker daemon's
`GOARCH`; emulated evidence is not native qualification. Use a pinned Linux
image containing the controller, wazero, Node/V8 helpers and independent analyzer.
The harness needs read-only mounts at `/agent.test`, `/setup.sh`, `/fixtures`
(the corpus testdata), `/vector-verifier.mjs` (the vector qualification verifier),
and the exact Wago/Wasmtime adapters at their normal `/opt/wasmbench` paths.
Mount a new evidence directory at `/evidence` and writable tmpfs at `/tmp` and
`/opt/wasmbench/.wasmbench`. Run with `--network none --read-only --privileged
--cgroupns=private`; never bind the host's `/sys/fs/cgroup` or credentials.

The privileged test is separate from the ordinary unprivileged procfs recipe.
Its semantic gate is callable offline, after verifying the bundle seal:

```sh
node recipes/verify-linux-vector-lifecycle.mjs cgroup runs/PRIVATE/run arm64
```

It requires memory.max 512 MiB, swap disabled, CPU quota 100000/100000 and
pids.max 128, verified exact before/after readbacks and unique atomically spawned
worker leaves. All measured samples need a same-descriptor kernel-accounted
`cgroup.memory.phase_peak` and total/user/system process-tree CPU deltas attached
to the exact returned boundary. Zero CPU deltas are valid; missing or denied
peaks are not qualification. Values describe barrier windows including transport,
not pure API intervals or process RSS. Private-container success does not qualify
exclusive CPUs, host frequency/NUMA policy, uninterrupted control, a dedicated
machine, or official publication.
