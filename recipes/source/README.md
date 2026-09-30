# Source compiler integration checks

The `xorshift-llvm*.json` recipes use local Homebrew LLVM paths. `source-lock`
captures exact executable and input hashes. These are trusted local builds,
not hermetic builds: dynamically loaded libraries are not automatically pinned.

## Linux compiler memory workflow

`test-linux-memory.sh` runs real LLVM 16 O0/O2 builds, independently validates
their Wasm output, checks exact results with wazero/compiler, wazero/interpreter
and V8, then collects six randomized memory blocks plus one warmup block. It
replays from archived source snapshots and independently checks raw step maxima,
report medians, paired-block counts, output identity and parent evidence hashes.
It measures maximum tool-step cgroup peak, not heap or a whole-build peak.

Run from the repository root on a Linux/arm64 Docker host (including Docker
Desktop on Apple silicon). Build the standard container as
`wasmbench:admission-v1` first. Prepare all binaries/images before measurement:

```sh
docker build -f recipes/source/Dockerfile.linux-test \
  -t wasmbench:source-linux-v1 recipes/source
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath \
  -o .wasmbench/wasmbench-linux ./cmd/wasmbench
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath \
  -o .wasmbench/wazero-linux ./adapters/wazero
```

The image contains the independent analyzer from the standard build. The
following mounts current CLI and adapter code and keeps networking disabled.
The privileged container is solely an ephemeral integration test, never a safe
sandbox for untrusted recipes. Do not mount the host cgroup filesystem or run
the setup script on the host. The script checks for Docker and a private root
cgroup before changing controller settings.

```sh
source_test_root="$PWD"
mkdir -p builds
docker run --rm --privileged --cgroupns=private --network none \
  -e WASMBENCH_EPHEMERAL_CGROUP_TEST=1 \
  -v "$source_test_root/recipes/source:/recipes:ro" \
  -v "$source_test_root/builds:/evidence" \
  -v "$source_test_root/.wasmbench/wasmbench-linux:/opt/wasmbench/bin/wasmbench:ro" \
  -v "$source_test_root/.wasmbench/wazero-linux:/opt/wasmbench/bin/adapter-wazero:ro" \
  -v "$source_test_root/adapters/v8/adapter.mjs:/opt/wasmbench/adapters/v8/adapter.mjs:ro" \
  -v "$source_test_root/adapters/v8/profiling.mjs:/opt/wasmbench/adapters/v8/profiling.mjs:ro" \
  --entrypoint sh wasmbench:source-linux-v1 /recipes/test-linux-memory.sh
```

Output goes to `builds/source-linux-memory-v1`; an existing directory is rejected,
never overwritten. Archive it elsewhere before repeating this exact command.
The image is a test dependency, not a reproducibly pinned public compiler release:
Debian packages are resolved at image build time and executable hashes are then
locked. Shared-host results establish functionality, not official rankings.
