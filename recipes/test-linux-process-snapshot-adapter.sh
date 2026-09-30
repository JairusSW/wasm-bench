#!/bin/sh
# Build first. Sealed native timing/replay in one shared-VM container.
set -eu
recipe_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
image_ref=${1:?pass an existing wasmbench image}
bundle=${2:?pass a new evidence directory name}
case "$bundle" in *[!a-zA-Z0-9_-]*|"") exit 2;; esac
evidence="$recipe_root/runs/$bundle"
if [ -e "$evidence" ] || [ -L "$evidence" ]; then
  printf 'Preserving existing evidence: %s\n' "$evidence" >&2
  exit 2
fi
controller="$recipe_root/.wasmbench/wasmbench-linux-process-snapshot"
worker="$recipe_root/.wasmbench/wasmtime-linux-target/process-snapshot/release/qualify-process-snapshot"
test -f "$controller"
test -f "$worker"
image_id=$(docker image inspect --format '{{.Id}}' "$image_ref")
case "$image_id" in sha256:*) ;; *) exit 2;; esac
digest=${image_id#sha256:}
case "$digest" in *[!a-f0-9]*) exit 2;; esac
test "${#digest}" -eq 64
image_arch=$(docker image inspect --format '{{.Architecture}}' "$image_id")
test "$(docker image inspect --format '{{.Os}}' "$image_id")" = linux
daemon_arch=$(docker info --format '{{.Architecture}}')
case "$daemon_arch" in aarch64|arm64) daemon_arch=arm64;; x86_64|amd64) daemon_arch=amd64;; *) exit 2;; esac
test "$image_arch" = "$daemon_arch"
mkdir -p "$recipe_root/runs"
mkdir "$evidence"
docker image inspect --format '{"image_id":{{json .Id}},"architecture":{{json .Architecture}},"os":{{json .Os}}}' "$image_id" > "$evidence/container.json"
# shellcheck disable=SC2016
docker run --rm --init --network none --read-only --cap-drop ALL \
  --security-opt no-new-privileges --pids-limit 32 --user "$(id -u):$(id -g)" \
  --tmpfs /tmp:rw,exec,nosuid,nodev,mode=1777 \
  --tmpfs /opt/wasmbench/.wasmbench:rw,nosuid,nodev,mode=1777 \
  --mount "type=bind,src=$evidence,dst=/evidence" \
  --mount "type=bind,src=$controller,dst=/opt/wasmbench/bin/wasmbench,readonly" \
  --mount "type=bind,src=$worker,dst=/opt/wasmbench/adapters/wasmtime/target/process-snapshot/release/qualify-process-snapshot,readonly" \
  --mount "type=bind,src=$recipe_root/recipes/verify-linux-process-snapshot-run.mjs,dst=/verify.mjs,readonly" \
  --entrypoint sh "$image_id" -eu -c '
./bin/wasmbench run --suite process-snapshots \
  --runtimes wasmtime-process-snapshot,wasmtime-winch-process-snapshot \
  --scenarios process-snapshot-capture,process-snapshot-restore,process-snapshot-first-write,process-snapshot-execute \
  --launches 2 --samples 2 --operations 1 --warmup 0 --out /evidence/original
./bin/wasmbench verify --run /evidence/original
node /verify.mjs /evidence/original "$1"
./bin/wasmbench reproduce /evidence/original --out /evidence/reproduced
./bin/wasmbench verify --run /evidence/reproduced
node /verify.mjs /evidence/reproduced "$1" /evidence/original
./bin/wasmbench report --run /evidence/original --out /evidence/report
./bin/wasmbench verify-report --dir /evidence/report
./bin/wasmbench verify-report --dir /evidence/report --recorded-builder
' snapshot-sequence "$image_arch"
printf 'Verified exploratory native snapshot adapter/replay: %s\n' "$evidence"
