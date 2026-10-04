# WAVM object extraction

`wavm-object.cpp` compiles a core module through the public WAVM C++ API and
writes `Runtime::getObjectCode()` to disk. It never instantiates or runs the
workload. This is a diagnostic helper; it is not yet wired into the collector.

Build against the exact SDK used by the timing adapter. WAVM's installed C++
headers also require the matching source release's `Inline/xxhash` directory:

```sh
c++ -std=c++17 -DWAVM_API= wavm-object.cpp \
  -I"$WAVM_SDK/include" -I"$WAVM_SOURCE/Include/WAVM/Inline/xxhash" \
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
as this release's C API engine. Linux ELF extraction still needs verification.
