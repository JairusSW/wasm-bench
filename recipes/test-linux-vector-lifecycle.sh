#!/bin/sh
# Built native tools only; no measurement-time builds/network/host cgroup changes.
set -eu
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
for file in "$controller" "$wazero" "$wago" "$adapter" "$analyzer"; do test -f "$file"; done
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
# shellcheck disable=SC2016
docker run --rm --init --network none --read-only --cap-drop ALL \
  --security-opt no-new-privileges --pids-limit 128 --user "$(id -u):$(id -g)" \
  --tmpfs /tmp:rw,exec,nosuid,nodev,mode=1777 \
  --tmpfs /opt/wasmbench/.wasmbench:rw,nosuid,nodev,mode=1777 \
  --mount "type=bind,src=$controller,dst=/opt/wasmbench/bin/wasmbench,readonly" \
  --mount "type=bind,src=$wazero,dst=/opt/wasmbench/bin/adapter-wazero,readonly" \
  --mount "type=bind,src=$wago,dst=/opt/wasmbench/bin/adapter-wago,readonly" \
  --mount "type=bind,src=$adapter,dst=/opt/wasmbench/adapters/wasmtime/target/release/adapter-wasmtime,readonly" \
  --mount "type=bind,src=$analyzer,dst=/opt/wasmbench/adapters/wasmtime/target/release/wasm-analyze,readonly" \
  --mount "type=bind,src=$recipe_root/adapters/v8,dst=/opt/wasmbench/adapters/v8,readonly" \
  --mount "type=bind,src=$recipe_root/corpus/testdata,dst=/fixtures,readonly" \
  --mount "type=bind,src=$recipe_root/recipes/verify-linux-vector-lifecycle.mjs,dst=/verify.mjs,readonly" \
  --mount "type=bind,src=$recipe_root/recipes/test-linux-vector-lifecycle.sh,dst=/recipe.sh,readonly" \
  --mount "type=bind,src=$evidence,dst=/evidence" \
  --workdir /opt/wasmbench --entrypoint sh "$image_id" -eu -c '
vector_arch="$1"
cp /fixtures/vector-lifecycle.wasm /fixtures/vector-lifecycle.wat /fixtures/vector-initialization.wasm /fixtures/vector-initialization.wat /evidence/
cp /verify.mjs /evidence/verification.mjs
cp /recipe.sh /evidence/recipe.sh
node /evidence/verification.mjs suite /evidence
./bin/wasmbench doctor > /evidence/doctor.json
./bin/wasmbench run --suite /evidence/suite.json --runtimes wago,wazero,wazero-interpreter,v8,wasmtime,wasmtime-winch --scenarios instantiate,first-call --profile memory --phase-barriers --launches 1 --samples 2 --operations 1 --warmup 0 --out /evidence/original > /evidence/original.log
./bin/wasmbench reproduce /evidence/original --out /evidence/replayed > /evidence/replayed.log
./bin/wasmbench restore-tools --run /evidence/original --out /evidence/tools > /evidence/restoration.json
/evidence/tools/tools/runner/wasmbench run --lock /evidence/tools/suite.lock --out /evidence/archived > /evidence/archived.log
for name in original replayed archived; do
  ./bin/wasmbench verify --run "/evidence/$name"
  if [ "$name" = original ]; then node /evidence/verification.mjs /evidence/original "$vector_arch";
  else node /evidence/verification.mjs "/evidence/$name" "$vector_arch" /evidence/original "$name"; fi
done
./bin/wasmbench report --run /evidence/original --out /evidence/report
./bin/wasmbench verify-report --dir /evidence/report
./bin/wasmbench verify-report --dir /evidence/report --recorded-builder
cd /evidence
sha256sum container.json vector-lifecycle.wasm vector-lifecycle.wat vector-initialization.wasm vector-initialization.wat suite.json doctor.json recipe.sh verification.mjs original.log replayed.log restoration.json archived.log > checksums.sha256
sha256sum -c checksums.sha256 > integrity.log
' vector-lifecycle "$image_arch"
printf 'Qualified native Linux/%s vector lifecycle and procfs; evidence: %s\n' "$image_arch" "$evidence"
