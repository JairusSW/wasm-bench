# Native Windows runtime profile

The controller and wazero adapters build for Windows; runtime, analyzer and
restored/staged executables use `.exe` names. Cargo's independent analyzer is
resolved using its native output name. Archived objects retain their existing
extensionless data paths and hashes; restoration changes only executable names,
not bytes. Node and helper scripts may reside on different filesystem volumes:
each volume gets a separate archive tree while relative imports remain intact.
Native PE DLL dependencies are not independently resolved or archived; they
remain host prerequisites, not a claimed complete dependency closure.

Offline archive verification parses recorded drive-letter, UNC and POSIX
provenance independently of the reader's OS. It reads only stored bundle objects,
never the producer's executable paths. Device namespaces, drive-relative paths,
traversal aliases, alternate data streams and colliding case-insensitive Windows
archive names are rejected. Producer filenames must be canonical and portable;
this does not grant execution or imply native Windows measurements. Restoration
and recorded-builder execution still require the recorded OS and architecture.

Build before collecting in PowerShell 7 on a native Windows host:

```powershell
go build -trimpath -o bin/wasmbench.exe ./cmd/wasmbench
./bin/wasmbench.exe build --runtimes wazero,wazero-interpreter,v8
./recipes/test-windows-lifecycle.ps1 -Bundle windows-lifecycle
```

The recipe requires native process/OS architecture agreement and refuses
existing output. It collects sacrificial correctness, timing and a separate
barrier-instrumented memory pass for both built-in core workloads across wazero
compiler/interpreter and V8. Ordinary replay and archived-tool replay must
preserve workload, runner and adapter identities. Each bundle seal and the
semantic coverage gate must pass. Both current and recorded report builders
are verified after collection is terminal.

Windows does not acquire Linux collector semantics: procfs RSS/PSS/private/
virtual observations are explicitly unsupported with no values; wait4 process
peak RSS is unavailable. Go/V8 allocator observations remain in their declared
managed-heap domains. No zero-valued OS footprint or phase peak is invented.
Cgroup isolation, Linux perf, isolated CPU partitions and source-build process
groups remain unsupported. This profile does not establish native Windows
footprint accounting, Windows process-tree containment or official publication.

The `windows-lifecycle` CI job runs this workflow and retains evidence. Local
macOS tests prove filename/archival contracts, cross-compilation and non-Windows
refusal; they do not prove native Windows execution. Native qualification is
pending successful Windows job evidence. Linux remains the reference platform.
