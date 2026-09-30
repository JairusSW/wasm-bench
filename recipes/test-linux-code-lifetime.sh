#!/bin/sh
# Builds finish first. Exploratory Linux diagnostics, no isolation or physical
# reclamation qualification. Preserve all original/replayed sealed artifacts.
set -eu
recipe_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
image_ref=${1:?pass a built wasmbench image}
bundle=${2:?pass a new evidence directory name}
case "$bundle" in *[!a-zA-Z0-9_-]*|"") printf 'Use a new evidence name containing only letters, digits, underscore or hyphen.\n' >&2; exit 2;; esac
evidence="$recipe_root/runs/$bundle"
if [ -e "$evidence" ] || [ -L "$evidence" ]; then
  printf 'Preserving existing evidence: %s; choose a new name.\n' "$evidence" >&2
  exit 2
fi
binary_dir=""
if [ "$#" -gt 2 ]; then
  binary_dir=$(CDPATH='' cd -- "$3" && pwd)
  for binary in wasmbench adapter-wasmtime-code-lifetime wasm-analyze; do
    test -f "$binary_dir/$binary"
  done
fi
image_id=$(docker image inspect --format '{{.Id}}' "$image_ref")
image_arch=$(docker image inspect --format '{{.Architecture}}' "$image_id")
image_os=$(docker image inspect --format '{{.Os}}' "$image_id")
test "$image_os" = linux
case "$image_arch" in arm64|amd64) ;; *) exit 2;; esac
daemon_arch=$(docker info --format '{{.Architecture}}')
case "$daemon_arch" in aarch64|arm64) daemon_arch=arm64;; x86_64|amd64) daemon_arch=amd64;; *) exit 2;; esac
if [ "$daemon_arch" != "$image_arch" ]; then
  printf 'Code lifetime qualification requires a native image, not emulation (%s vs %s).\n' "$image_arch" "$daemon_arch" >&2
  exit 2
fi
container() {
  set -- "$image_id" "$@"
  if [ -n "$binary_dir" ]; then
    set -- \
      --mount "type=bind,src=$binary_dir/wasmbench,dst=/opt/wasmbench/bin/wasmbench,readonly" \
      --mount "type=bind,src=$binary_dir/adapter-wasmtime-code-lifetime,dst=/opt/wasmbench/adapters/wasmtime/target/code-lifetime/release/adapter-wasmtime,readonly" \
      --mount "type=bind,src=$binary_dir/wasm-analyze,dst=/opt/wasmbench/adapters/wasmtime/target/release/wasm-analyze,readonly" \
      "$@"
  fi
  docker run --rm --network none --read-only --cap-drop ALL \
    --security-opt no-new-privileges --user "$(id -u):$(id -g)" \
    --tmpfs /tmp:rw,exec,nosuid,nodev,mode=1777 \
    --tmpfs /opt/wasmbench/.wasmbench:rw,nosuid,nodev,mode=1777 \
    --mount "type=bind,src=$evidence,dst=/evidence" \
    --mount "type=bind,src=$recipe_root/recipes/verify-linux-code-lifetime.mjs,dst=/verify.mjs,readonly" \
    --entrypoint /usr/bin/env "$@"
}
mkdir -p "$recipe_root/runs"
mkdir "$evidence"
docker image inspect --format '{"image_id":{{json .Id}},"architecture":{{json .Architecture}},"os":{{json .Os}}}' "$image_id" > "$evidence/container.json"
# One container keeps original/replay host identity stable. No builds, tests or
# report rendering run until all collection and replay measurement is complete.
# shellcheck disable=SC2016
container sh -eu -c '
./bin/wasmbench doctor > /evidence/doctor.json
./bin/wasmbench run --suite core \
  --runtimes wasmtime-code-lifetime,wasmtime-winch-code-lifetime \
  --scenarios code-lifetime --profile code \
  --launches 3 --samples 1 --operations 1 --warmup 0 --out /evidence/original
./bin/wasmbench verify --run /evidence/original
node /verify.mjs /evidence/original "$1"
./bin/wasmbench reproduce /evidence/original --out /evidence/reproduced
./bin/wasmbench verify --run /evidence/reproduced
node /verify.mjs /evidence/reproduced "$1" /evidence/original
./bin/wasmbench report --run /evidence/original --out /evidence/reports/original
./bin/wasmbench verify-report --dir /evidence/reports/original
./bin/wasmbench verify-report --dir /evidence/reports/original --recorded-builder
./bin/wasmbench report --run /evidence/reproduced --out /evidence/reports/reproduced
./bin/wasmbench verify-report --dir /evidence/reports/reproduced
./bin/wasmbench verify-report --dir /evidence/reports/reproduced --recorded-builder
' code-lifetime-sequence "$image_arch"
printf 'Verified exploratory Linux/%s code lifetime; image %s; evidence %s\n' "$image_arch" "$image_id" "$evidence"
