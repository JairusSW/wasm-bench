#!/bin/sh
# Opt-in Docker Desktop/Linux validation. Privilege is confined to an ephemeral
# container's private cgroup namespace; never mount the host cgroup filesystem.
set -eu
test "${WASMBENCH_EPHEMERAL_CGROUP_TEST:-}" = 1
recipe_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
wago_root=$(CDPATH= cd -- "${1:?pass Wago checkout}" && pwd)
bundle=${2:?pass a new evidence bundle name}
suite_file=${WASMBENCH_PHASE_SUITE_FILE:-$recipe_root/.wasmbench/vector-suite.json}
case "$bundle" in *[!a-zA-Z0-9_-]*|"") exit 2;; esac
test ! -e "$recipe_root/runs/$bundle"
mkdir -p "$recipe_root/runs"
docker run --rm --privileged --cgroupns=private \
  -e WASMBENCH_EPHEMERAL_CGROUP_TEST=1 -e WASMBENCH_RUN_PHASE_SMOKE=1 \
  -e WASMBENCH_PHASE_BUNDLE="$bundle" -e WASMBENCH_PHASE_SUITE="${WASMBENCH_PHASE_SUITE:-/suite.json}" \
  -e WASMBENCH_PHASE_RUNTIMES="${WASMBENCH_PHASE_RUNTIMES:-wago,wazero,wazero-interpreter,wasmtime,wasmtime-winch,v8}" \
  -e WASMBENCH_PHASE_TIMEOUT="${WASMBENCH_PHASE_TIMEOUT:-30s}" \
  -e WASMBENCH_PHASE_SCENARIO="${WASMBENCH_PHASE_SCENARIO:-compile}" \
  --mount "type=bind,src=$recipe_root/.wasmbench/wasmbench-linux,dst=/opt/wasmbench/bin/wasmbench,readonly" \
  --mount "type=bind,src=$recipe_root/.wasmbench/wazero-linux,dst=/opt/wasmbench/bin/adapter-wazero,readonly" \
  --mount "type=bind,src=$recipe_root/.wasmbench/wago-linux,dst=/opt/wasmbench/bin/adapter-wago,readonly" \
  --mount "type=bind,src=$recipe_root/.wasmbench/wasmtime-linux-target/release/adapter-wasmtime,dst=/opt/wasmbench/adapters/wasmtime/target/release/adapter-wasmtime,readonly" \
  --mount "type=bind,src=$recipe_root/.wasmbench/wasmtime-linux-target/release/wasm-analyze,dst=/opt/wasmbench/adapters/wasmtime/target/release/wasm-analyze,readonly" \
  --mount "type=bind,src=$recipe_root/adapters/v8/adapter.mjs,dst=/opt/wasmbench/adapters/v8/adapter.mjs,readonly" \
  --mount "type=bind,src=$recipe_root/adapters/v8/profiling.mjs,dst=/opt/wasmbench/adapters/v8/profiling.mjs,readonly" \
  --mount "type=bind,src=$recipe_root/.wasmbench/agent-linux.test,dst=/agent.test,readonly" \
  --mount "type=bind,src=$recipe_root/agent/test-cgroup-container.sh,dst=/test-cgroup.sh,readonly" \
  --mount "type=bind,src=$suite_file,dst=/suite.json,readonly" \
  --mount "type=bind,src=$wago_root/corpus,dst=$wago_root/corpus,readonly" \
  --mount "type=bind,src=$recipe_root/runs,dst=/evidence" \
  --entrypoint sh wasmbench:dev /test-cgroup.sh
cd "$recipe_root"
WASMBENCH_LINUX_PHASE_BUNDLE="$recipe_root/runs/$bundle" \
  go test ./experiment -run '^TestLinuxPhaseEvidence$' -count=1 -v
