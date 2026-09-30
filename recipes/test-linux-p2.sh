#!/bin/sh
# Native CLI round trip with procfs. No privileged container, host cgroups,
# network, credentials, builds during collection or official latency claims.
set -eu
recipe_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
image_ref=${1:?pass an existing native Linux image}
bundle=${2:?pass a new evidence directory name}
case "$bundle" in *[!a-zA-Z0-9_-]*|"") exit 2;; esac
evidence="$recipe_root/runs/$bundle"
if [ -e "$evidence" ] || [ -L "$evidence" ]; then
  printf 'Preserving existing evidence: %s\n' "$evidence" >&2
  exit 2
fi
controller="$recipe_root/.wasmbench/wasmbench-linux-p2"
adapter="$recipe_root/.wasmbench/wasmtime-linux-target/release/adapter-wasmtime"
analyzer="$recipe_root/.wasmbench/wasmtime-linux-target/release/wasm-analyze"
fixture="$recipe_root/.wasmbench/p2-reset-fixture/p2-filesystem-reset.component.wasm"
compiler="$recipe_root/.wasmbench/p2-reset-fixture/compiler.txt"
for file in "$controller" "$adapter" "$analyzer" "$fixture" "$compiler"; do test -f "$file"; done
image_id=$(docker image inspect --format '{{.Id}}' "$image_ref")
case "$image_id" in sha256:*) ;; *) exit 2;; esac
test "$(docker image inspect --format '{{.Os}}' "$image_id")" = linux
image_arch=$(docker image inspect --format '{{.Architecture}}' "$image_id")
daemon_arch=$(docker info --format '{{.Architecture}}')
case "$daemon_arch" in aarch64|arm64) daemon_arch=arm64;; x86_64|amd64) daemon_arch=amd64;; *) exit 2;; esac
if [ "$image_arch" != "$daemon_arch" ]; then
  printf 'Native image required, not emulation (%s vs %s).\n' "$image_arch" "$daemon_arch" >&2
  exit 2
fi
mkdir -p "$recipe_root/runs"
mkdir "$evidence"
docker image inspect --format '{"image_id":{{json .Id}},"architecture":{{json .Architecture}},"os":{{json .Os}}}' "$image_id" > "$evidence/container.json"
# shellcheck disable=SC2016
docker run --rm --init --network none --read-only --cap-drop ALL \
  --security-opt no-new-privileges --pids-limit 128 --user "$(id -u):$(id -g)" \
  --tmpfs /tmp:rw,exec,nosuid,nodev,mode=1777 \
  --tmpfs /opt/wasmbench/.wasmbench:rw,nosuid,nodev,mode=1777 \
  --mount "type=bind,src=$controller,dst=/opt/wasmbench/bin/wasmbench,readonly" \
  --mount "type=bind,src=$adapter,dst=/opt/wasmbench/adapters/wasmtime/target/release/adapter-wasmtime,readonly" \
  --mount "type=bind,src=$analyzer,dst=/opt/wasmbench/adapters/wasmtime/target/release/wasm-analyze,readonly" \
  --mount "type=bind,src=$fixture,dst=/fixture.wasm,readonly" \
  --mount "type=bind,src=$compiler,dst=/compiler.txt,readonly" \
  --mount "type=bind,src=$recipe_root/corpus/testdata/p2-filesystem-reset.rs,dst=/fixture.rs,readonly" \
  --mount "type=bind,src=$recipe_root/recipes/verify-linux-p2.mjs,dst=/verify.mjs,readonly" \
  --mount "type=bind,src=$recipe_root/recipes/test-linux-p2.sh,dst=/recipe.sh,readonly" \
  --mount "type=bind,src=$evidence,dst=/evidence" \
  --workdir /opt/wasmbench --entrypoint sh "$image_id" -eu -c '
p2_arch="$1"
cp /fixture.wasm /evidence/fixture.component.wasm
cp /fixture.rs /evidence/fixture.rs
cp /compiler.txt /evidence/compiler.txt
cp /verify.mjs /evidence/verification.mjs
cp /recipe.sh /evidence/recipe.sh
node /evidence/verification.mjs suite /evidence
./bin/wasmbench doctor > /evidence/doctor.json
for profile in timing memory; do
  set --
  if [ "$profile" = memory ]; then set -- --phase-barriers; fi
  ./bin/wasmbench run --suite /evidence/suite.json --runtimes wasmtime,wasmtime-winch \
    --scenarios compile,instantiate,first-call --profile "$profile" "$@" \
    --launches 1 --samples 2 --operations 1 --warmup 0 --out "/evidence/$profile" \
    > "/evidence/$profile.log"
  ./bin/wasmbench reproduce "/evidence/$profile" --out "/evidence/$profile-replayed" \
    > "/evidence/$profile-replay.log"
  ./bin/wasmbench restore-tools --run "/evidence/$profile" --out "/evidence/$profile-tools" \
    > "/evidence/$profile-restoration.json"
  "/evidence/$profile-tools/tools/runner/wasmbench" run --lock "/evidence/$profile-tools/suite.lock" \
    --out "/evidence/$profile-archived" > "/evidence/$profile-archived.log"
done
# All original/replay collection is terminal before verification/rendering.
for profile in timing memory; do
  ./bin/wasmbench verify --run "/evidence/$profile"
  node /evidence/verification.mjs "/evidence/$profile" "$p2_arch" "$profile"
  ./bin/wasmbench verify --run "/evidence/$profile-replayed"
  node /evidence/verification.mjs "/evidence/$profile-replayed" "$p2_arch" "$profile" "/evidence/$profile"
  ./bin/wasmbench verify --run "/evidence/$profile-archived"
  node /evidence/verification.mjs "/evidence/$profile-archived" "$p2_arch" "$profile" "/evidence/$profile" archived
done
./bin/wasmbench report --run /evidence/timing --memory-run /evidence/memory --out /evidence/report
./bin/wasmbench verify-report --dir /evidence/report
./bin/wasmbench verify-report --dir /evidence/report --recorded-builder
cd /evidence
sha256sum container.json fixture.component.wasm fixture.rs compiler.txt suite.json doctor.json recipe.sh verification.mjs timing.log memory.log timing-replay.log memory-replay.log timing-restoration.json memory-restoration.json timing-archived.log memory-archived.log > checksums.sha256
sha256sum -c checksums.sha256 > integrity.log
' linux-p2 "$image_arch"
printf 'Qualified native Linux/%s P2 lifecycle and procfs; evidence: %s\n' "$image_arch" "$evidence"
