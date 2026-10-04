# Preserving and restoring exact tools

New CLI `plan`, `run` and `check` experiments default to `--archive-tools=false`.
The policy is locked. Existing locks retain their original policy; neither true
nor false may override it at execution. Use `--archive-tools=true` when creating
a plan or run to retain exact tool bytes for later restoration. Without an archive,
tools are still hash-pinned but must remain available separately for replay.
Library callers opt in with `Lock.ArchiveTools`.

Before workload admission or adapter execution, the runner retains its exact
executable, declared adapter files and independent analyzer in one immutable,
content-addressed cache. The default is the OS user cache directory under
`wasm-bench/tools/sha256`; set `WASMBENCH_TOOL_CACHE` to choose its location.
Each run's `tools/` paths and new report snapshots are regular read-only hard
links to those cached bytes, rather than per-report copies. Identical tools across
runtimes and runs occupy one file. Cache and experiment outputs must be on the
same filesystem; a failed hard link reports how to select a suitable cache.

Every retained file is still covered by the bundle seal and checked against its
locked SHA-256. Cache hits reject changed source bytes or corrupt cache blobs.
These links never point to mutable installations. Do not modify cached files or
archived `tools/` files in place: hard links share an inode. Normal adapter
rebuilds write separate installation files. Portable archives retain exact bytes
and can be restored without the original cache. Removing a cache directory does
not invalidate existing hard links. Other report evidence remains independently
copied (using copy-on-write on macOS where available).

## Reusing Wago adapter builds

Wago builds use the same cache root under `builds/wago/`. The build key includes
the selected revision, source contents, adapter and harness Go/assembly inputs,
dependency declarations and stable Go toolchain settings. Repeated collection
reuses the recorded binary only after checking its digest. Changed source,
adapter code or toolchain settings produce a new build. The installed executable
is replaced atomically; existing reports continue to reference their exact tools.

## Compact existing local archives

On a clone-capable filesystem, compact byte-identical large files beneath archived
`tools/` directories without deleting trials, artifacts, checksums or replay paths:

```sh
node recipes/compact-tool-archives.mjs runs reports           # read-only estimate
node recipes/compact-tool-archives.mjs --apply runs reports   # compact local copies
```

The script hashes candidates and verifies each replacement before an atomic rename.
It uses independent copy-on-write clones, not hardlinks, and fails if cloning is not
supported. Stop writers to these bundles first. Logical file counts and `du` may
remain unchanged; check actual free space with `df`. Repeated compaction estimates
can include already-shared bytes. No old run is automatically deleted.

## Restore without overwriting installations

Only restore bundles you trust. Checksums establish integrity, not authenticity
or safety. No code is executed by `restore-tools`, including `describe` or other
adapter discovery calls.

```sh
wasmbench restore-tools --run runs/ID --out .wasmbench/replay/ID
# Read restoration.json, then explicitly execute the preserved runner:
.wasmbench/replay/ID/tools/runner/wasmbench run \
  --lock .wasmbench/replay/ID/suite.lock --out runs/replayed-ID
```

The destination must be new and outside the sealed source bundle. Restoration
copies workloads, command fixtures and tools; retains helper-module relative
paths; makes only executable entry points executable; and writes a relocated
lock plus `restoration.json`. The record includes the source bundle/checksum and
lock hashes. It does not modify or overwrite original tool locations.

The recorded runner is required: another runner still fails the exact executable
hash check. The restored files have the same content hashes, but relocation
changes path-bound runtime configuration identity. This is not a claim of an
identical host or timing distribution. Pilot-confirmation locks cannot currently
be relocated because that would change their fixed confirmation identity.
Ordinary `reproduce` continues to use original locked paths.

## Native libraries and platform limits

New runtime resolution inspects Mach-O native dependency closures. Loader-relative
third-party libraries (for example Homebrew Node's `@rpath/libnode` runtime) are
pinned and copied alongside the executable using the required relative layout.
Absolute-path third-party libraries are separately pinned in `host_file_sha256`.
Their bytes are archived as evidence, but their load paths are not patched:
restoration and execution require those exact libraries at their original paths.
Replacing one fails verification, even if the executable itself is unchanged.

macOS shared-cache libraries under `/usr/lib` and `/System/Library` remain part of
the compatible OS requirement. Native PE dependency discovery is not yet
implemented; those executables retain the explicit declared-file contract. The
archive does not capture every possible dynamic load, environment variable,
external resource or system library. It is not a hermetic execution image.
Restoration requires the source OS and architecture; it does not emulate them.

### Linux ELF startup libraries

`linux-glibc-startup-closure-v1` qualifies the standard glibc loaders on native
Linux ARM64 (`/lib/ld-linux-aarch64.so.1`) and AMD64
(`/lib64/ld-linux-x86-64.so.2`). Other interpreters, including musl, are not yet
qualified. Static ELF executables record an empty startup-library set without
invoking a loader.

Runtime resolution invokes the trusted local ELF interpreter with `--list`,
under a deadline and bounded output. This is a native tool invocation during
planning, not an untrusted-binary inspection sandbox. Use only trusted runtime
builds. The adapter's application entry point and Wasm workload are not invoked
by this probe. The loader and every reported startup library are hash-pinned;
libraries are included in the archive with their relative filesystem layout.
This preserves executable-relative `$ORIGIN` layouts on restoration. The loader
itself remains an exact host prerequisite at the original `PT_INTERP` path.

The policy rejects nonempty `LD_*`/`GLIBC_TUNABLES` overrides, ELF audit tags and
system preloads. It pins `/etc/ld.so.cache` and `/etc/ld.so.preload`, including
explicit absence. A changed control file invalidates the lock even if the old
library files remain available. Copies of existing control files are evidence;
restoration never installs them into `/etc`.

`run` re-resolves the startup dependencies before creating the output directory
and before measurement. Every library name/hash must match the lock. Actual
resolved paths are then retained in `elf_startup_dependencies` for that run.
Thus a restored binary cannot silently switch to different system libraries
merely because all archived files still pass their checksums. Qualification is
a pre-run boundary check, not continuous monitoring of host filesystem changes.
The probe may warm native library filesystem pages; filesystem coldness remains
separate from process/guest coldness.

Offline bundle loading and `restore-tools` do not run this probe. They check
sealed bytes and metadata; restoration additionally verifies host prerequisites.
The restored runner performs the live resolution check only when explicitly run.
This covers startup dependencies, not every library loaded later through
[`dlopen` or other runtime loading](https://sourceware.org/glibc/manual/latest/html_node/Dynamic-Linker.html).
Kernel-provided vDSO mappings are also outside the archived-file domain.
Independent analyzer and controller native-library closures are not yet captured
by this runtime-specific policy.

Legacy bundles without tool archives cannot recover tools that have already been
replaced. Hashes and build recipes are evidence, not a substitute for missing exact
bytes. Source-toolchain build bundles use their separate replay workflow and do
not yet adopt runtime tool archives.

## Timing trials with peak RSS

Use `run --profile timing --timing-peak-rss` to collect one kernel-accounted
adapter process lifetime RSS peak from each timing trial's existing wait4 result.
The flag is locked and cannot override a saved plan. It supports ordinary
compilation, instantiation, first-call and steady scenarios. Three compilation
samples produce one trial peak, not three invented memory samples. The peak
includes startup, setup, verification and all samples; it is not phase-only RSS.

Reports identify these rows with `source_run` and `source_profile: timing`.
They can retain a separate single-sample steady memory pass for allocator and
heap metrics without repeating compilation or instantiation. Both source runs
remain explicit in `memory_source`, and report verification recomputes the join.
