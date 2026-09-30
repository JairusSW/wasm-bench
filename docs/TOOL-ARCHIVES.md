# Preserving and restoring exact tools

New CLI `plan`, `run` and `check` experiments default to `--archive-tools=true`.
The policy is locked. Existing locks retain their original policy; neither true
nor false may override it at execution. Use `--archive-tools=false` when creating
a plan if disk cost is unacceptable. Library callers opt in with `Lock.ArchiveTools`.

Before workload admission or adapter execution, the runner copies its own exact
executable, every declared adapter file, and the independent analyzer into the
bundle's `tools/` directory. Each copy must match its locked SHA-256. These are
regular, non-executable, read-only copies, not symlinks to mutable installations.
They are covered by the bundle seal. Offline loading also checks their hashes
against the lock, so resealing a changed tool does not make it match the plan.
This increases disk use, especially for shared-library runtimes and multiple
configurations of the same runtime. Copies are not currently deduplicated.

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
