#!/bin/sh
# Explicitly opted-in disposable private-cgroup Docker qualification only.
# Never bind host cgroups or credentials. Build all tools before collection.
set -eu
test "${WASMBENCH_EPHEMERAL_CGROUP_TEST:-}" = 1 || {
  printf 'Requires WASMBENCH_EPHEMERAL_CGROUP_TEST=1 for privileged private-container tests.\n' >&2
  exit 2
}
recipe_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
image_ref=${1:?pass an existing native Linux image}
bundle=${2:?pass a new evidence directory name}
case "$bundle" in *[!a-zA-Z0-9_-]*|"") exit 2;; esac
evidence="$recipe_root/runs/$bundle"
if [ -e "$evidence" ] || [ -L "$evidence" ]; then printf 'Preserving existing evidence: %s\n' "$evidence" >&2; exit 2; fi
controller="$recipe_root/.wasmbench/wasmbench-linux-vector"
wazero="$recipe_root/.wasmbench/adapter-wazero-linux-vector"
wago="$recipe_root/.wasmbench/adapter-wago-linux-vector"
adapter="$recipe_root/.wasmbench/wasmtime-linux-target/release/adapter-wasmtime"
analyzer="$recipe_root/.wasmbench/wasmtime-linux-target/release/wasm-analyze"
agent_test="$recipe_root/.wasmbench/agent-cgroup-vector.test"
for file in "$controller" "$wazero" "$wago" "$adapter" "$analyzer" "$agent_test"; do test -f "$file"; done
image_id=$(docker image inspect --format '{{.Id}}' "$image_ref")
case "$image_id" in sha256:*) ;; *) exit 2;; esac
image_digest=${image_id#sha256:}
test "${#image_digest}" = 64 || exit 2
case "$image_digest" in *[!0-9a-f]*) exit 2;; esac
test "$(docker image inspect --format '{{.Os}}' "$image_id")" = linux
image_arch=$(docker image inspect --format '{{.Architecture}}' "$image_id")
daemon_arch=$(docker info --format '{{.Architecture}}')
case "$daemon_arch" in aarch64|arm64) daemon_arch=arm64;; x86_64|amd64) daemon_arch=amd64;; *) exit 2;; esac
test "$image_arch" = "$daemon_arch" || { printf 'Native image required, not emulation.\n' >&2; exit 2; }
mkdir -p "$recipe_root/runs"
mkdir "$evidence"
docker image inspect --format '{"image_id":{{json .Id}},"architecture":{{json .Architecture}},"os":{{json .Os}}}' "$image_id" > "$evidence/container.json"
cp "$0" "$evidence/recipe.sh"
cp "$recipe_root/agent/test-cgroup-container.sh" "$evidence/setup.sh"
cp "$recipe_root/recipes/verify-linux-vector-lifecycle.mjs" "$evidence/verification.mjs"
docker run --rm --network none --privileged --cgroupns=private --read-only \
  --env WASMBENCH_EPHEMERAL_CGROUP_TEST=1 \
  --env WASMBENCH_RUN_VECTOR_PHASE_SMOKE=1 \
  --env WASMBENCH_REPLAY_VECTOR_PHASE_SMOKE=1 \
  --tmpfs /tmp:rw,exec,nosuid,nodev,mode=1777 \
  --tmpfs /opt/wasmbench/.wasmbench:rw,nosuid,nodev,mode=1777 \
  --mount "type=bind,src=$agent_test,dst=/agent.test,readonly" \
  --mount "type=bind,src=$evidence/setup.sh,dst=/setup.sh,readonly" \
  --mount "type=bind,src=$controller,dst=/opt/wasmbench/bin/wasmbench,readonly" \
  --mount "type=bind,src=$wazero,dst=/opt/wasmbench/bin/adapter-wazero,readonly" \
  --mount "type=bind,src=$wago,dst=/opt/wasmbench/bin/adapter-wago,readonly" \
  --mount "type=bind,src=$adapter,dst=/opt/wasmbench/adapters/wasmtime/target/release/adapter-wasmtime,readonly" \
  --mount "type=bind,src=$analyzer,dst=/opt/wasmbench/adapters/wasmtime/target/release/wasm-analyze,readonly" \
  --mount "type=bind,src=$recipe_root/adapters/v8,dst=/opt/wasmbench/adapters/v8,readonly" \
  --mount "type=bind,src=$recipe_root/corpus/testdata,dst=/fixtures,readonly" \
  --mount "type=bind,src=$evidence/verification.mjs,dst=/vector-verifier.mjs,readonly" \
  --mount "type=bind,src=$evidence,dst=/evidence" \
  --workdir /opt/wasmbench --entrypoint sh "$image_id" /setup.sh > "$evidence/collection.log" 2>&1
# Collection and both replay paths are terminal before this integrity pass.
docker run --rm --network none --read-only --cap-drop ALL --security-opt no-new-privileges \
  --mount "type=bind,src=$evidence,dst=/evidence" \
  --workdir /evidence --entrypoint sh "$image_id" -eu -c '
sha256sum container.json recipe.sh setup.sh verification.mjs suite.json vector-lifecycle.wasm vector-lifecycle.wat vector-initialization.wasm vector-initialization.wat restoration.json collection.log > checksums.sha256
sha256sum -c checksums.sha256 > integrity.log
'
printf 'Qualified private native Linux/%s cgroup vector collection/replay; evidence: %s\n' "$image_arch" "$evidence"
