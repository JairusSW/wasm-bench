#!/bin/sh
# Exploratory, unprivileged Linux qualification. Builds must finish before this
# script. No host cgroup mount, CPU-isolation, exact phase peak or official claim.
set -eu
recipe_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
image_ref=${1:?pass a built wasmbench image}
bundle=${2:?pass a new evidence directory name}
case "$bundle" in *[!a-zA-Z0-9_-]*|"") exit 2;; esac
binary_dir=""
if [ "$#" -gt 2 ]; then
  binary_dir=$(CDPATH='' cd -- "$3" && pwd)
  for binary in wasmbench adapter-wazero wasm-analyze; do
    test -f "$binary_dir/$binary"
  done
fi
image_id=$(docker image inspect --format '{{.Id}}' "$image_ref")
image_arch=$(docker image inspect --format '{{.Architecture}}' "$image_id")
case "$image_arch" in arm64|amd64) ;; *) exit 2;; esac
evidence="$recipe_root/runs/$bundle"
mkdir -p "$recipe_root/runs"
# mkdir fails on any existing file/directory, preserving earlier evidence.
mkdir "$evidence"
docker image inspect --format '{"image_id":{{json .Id}},"architecture":{{json .Architecture}},"os":{{json .Os}}}' "$image_id" > "$evidence/container.json"
container() {
  set -- "$image_id" "$@"
  if [ -n "$binary_dir" ]; then
    set -- \
      --mount "type=bind,src=$binary_dir/wasmbench,dst=/opt/wasmbench/bin/wasmbench,readonly" \
      --mount "type=bind,src=$binary_dir/adapter-wazero,dst=/opt/wasmbench/bin/adapter-wazero,readonly" \
      --mount "type=bind,src=$binary_dir/wasm-analyze,dst=/opt/wasmbench/adapters/wasmtime/target/release/wasm-analyze,readonly" \
      "$@"
  fi
  docker run --rm --network none --read-only --cap-drop ALL \
    --security-opt no-new-privileges --user "$(id -u):$(id -g)" \
    --tmpfs /tmp:rw,exec,nosuid,nodev,mode=1777 \
    --tmpfs /opt/wasmbench/.wasmbench:rw,nosuid,nodev,mode=1777 \
    --mount "type=bind,src=$evidence,dst=/evidence" \
    --mount "type=bind,src=$recipe_root/recipes/verify-linux-continuations.mjs,dst=/verify.mjs,readonly" \
    --entrypoint /usr/bin/env "$@"
}
container ./bin/wasmbench doctor > "$evidence/doctor.json"
# $1 is deliberately expanded by the inner container shell, not this shell.
# shellcheck disable=SC2016
container sh -eu -c '
./bin/wasmbench run --suite continuations --runtimes wazero,wazero-interpreter \
  --scenarios continuation-create,continuation-resume,continuation-first-write,continuation-execute \
  --profile timing --launches 3 --samples 2 --operations 1 --warmup 0 --out /evidence/timing
./bin/wasmbench run --suite continuations --runtimes wazero,wazero-interpreter \
  --scenarios continuation-create,continuation-resume,continuation-first-write,continuation-execute \
  --profile memory --phase-barriers --launches 3 --samples 2 --operations 1 --warmup 0 --out /evidence/memory
./bin/wasmbench verify --run /evidence/timing
./bin/wasmbench verify --run /evidence/memory
node /verify.mjs /evidence "$1"
./bin/wasmbench reproduce /evidence/timing --out /evidence/reproduced/timing
./bin/wasmbench reproduce /evidence/memory --out /evidence/reproduced/memory
./bin/wasmbench verify --run /evidence/reproduced/timing
./bin/wasmbench verify --run /evidence/reproduced/memory
node /verify.mjs /evidence/reproduced "$1"
# Derive reports only after both measurement passes and replays have finished.
# Keep each report distinct; never overwrite collected or earlier derived data.
./bin/wasmbench report --run /evidence/timing --memory-run /evidence/memory --out /evidence/reports/original
./bin/wasmbench verify-report --dir /evidence/reports/original
./bin/wasmbench verify-report --dir /evidence/reports/original --recorded-builder
./bin/wasmbench report --run /evidence/reproduced/timing --memory-run /evidence/reproduced/memory --out /evidence/reports/reproduced
./bin/wasmbench verify-report --dir /evidence/reports/reproduced
./bin/wasmbench verify-report --dir /evidence/reports/reproduced --recorded-builder
' continuation-sequence "$image_arch"
printf 'Verified exploratory Linux/%s continuations; image %s; evidence %s\n' "$image_arch" "$image_id" "$evidence"
