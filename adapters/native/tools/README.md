# WAVM object extraction

`wavm-object.cpp` compiles a core module through the public WAVM C++ API and
writes `Runtime::getObjectCode()` to disk. It never instantiates or runs the
workload. The WAVM adapter also uses this API in its code-profile `inspect` request,
reporting executable-section size with the relocatable-object scope.

Build against the exact SDK used by the timing adapter. WAVM's installed C++
headers also require its installed `Inline/xxhash` include directory:

```sh
c++ -std=c++17 -DWAVM_API= wavm-object.cpp \
  -I"$WAVM_SDK/include" -I"$WAVM_SDK/include/WAVM/Inline/xxhash" \
  -L"$WAVM_SDK/lib" -Wl,-rpath,"$WAVM_SDK/lib" -lWAVM -o wavm-object
./wavm-object workload.wasm workload.o
```

The output is a **relocatable object**, not the linked executable image. Its
total file size includes debug information, relocations, symbols, and unwind
metadata and must not be reported as native code size. A collector must parse
its executable sections, record this scope, and retain relocation information
for any code viewer. Do not cast the C API's opaque module handles to internal
WAVM structures.

Verified on macOS arm64 with `nightly-2026-04-05`: the host-to-Wasm call fixture
produced a 1,696-byte Mach-O object with a 48-byte `__text` section, independently
checked with `file` and `otool -l`. The API uses the same default `FeatureSpec`
as this release's C API engine. Linux amd64 protocol verification also passed: a return-42 module produced
a 2,608-byte ELF object containing 46 executable bytes in `.ltext` (with an
empty `.text`), independently checked with `readelf -SW`.

`../tests/fixtures/wavm-call-arm64.o` retains that exact diagnostic object for
parser regression tests. It is not a workload and is never executed.

`../tests/fixtures/wavm-return42-amd64.o` retains that ELF object, testing
executable sections beyond the conventional `.text` name.

## WASI embedding checks

The private `wavm_wasi.h` helper uses WAVM's public WASI resolver, process,
instance, and invocation APIs. `CommandInstance` keeps compilation separate
from per-instance WASI setup and captures `proc_exit`. Its buffers retain output
after guest descriptor closure and enforce output limits. The fixture filesystem
permits reads and rejects mutation, parent traversal, and symlinks. The adapter protocol now supports WASI command timing, memory phase barriers,
and native-size inspection. Command verification checks exact exit status and
raw stdout/stderr digests, with a separate canonical digest for LLVM output.
`../tests/wavm-wasi-protocol.py ADAPTER` checks these paths and output-limit
rejection; full corpus compatibility must be established separately.

The native tests use the same SDK as the adapter:

```sh
c++ -std=c++17 -DWAVM_API= -I"$WAVM_SDK/include" \\
  -I"$WAVM_SDK/include/WAVM/Inline/xxhash" ../tests/wavm-wasi.cpp \\
  -L"$WAVM_SDK/lib" -Wl,-rpath,"$WAVM_SDK/lib" -lWAVM -o wasi-test
./wasi-test
```

Run the same build command with `wavm-readonly.cpp` or `wavm-stdio.cpp` to check
filesystem and buffer behavior. All three checks passed on macOS arm64 and
Linux amd64 with `nightly-2026-04-05`.
