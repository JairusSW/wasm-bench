#!/bin/sh
# Build the standalone qualifier first. No runtime capability or timing claim.
set -eu
recipe_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
image_ref=${1:?pass a built wasmbench image}
bundle=${2:?pass a new qualification directory name}
case "$bundle" in *[!a-zA-Z0-9_-]*|"") exit 2;; esac
evidence="$recipe_root/runs/$bundle"
if [ -e "$evidence" ] || [ -L "$evidence" ]; then
  printf 'Preserving existing evidence: %s\n' "$evidence" >&2
  exit 2
fi
qualifier="$recipe_root/.wasmbench/wasmtime-linux-target/process-snapshot/release/qualify-process-snapshot"
test -f "$qualifier"
image_id=$(docker image inspect --format '{{.Id}}' "$image_ref")
case "$image_id" in sha256:*) ;; *) printf 'Expected immutable image ID\n' >&2; exit 2;; esac
image_digest=${image_id#sha256:}
case "$image_digest" in *[!a-f0-9]*) printf 'Expected immutable image ID\n' >&2; exit 2;; esac
if [ "${#image_digest}" -ne 64 ]; then
  printf 'Expected immutable image ID\n' >&2
  exit 2
fi
image_arch=$(docker image inspect --format '{{.Architecture}}' "$image_id")
image_os=$(docker image inspect --format '{{.Os}}' "$image_id")
if [ "$image_os" != linux ]; then
  printf 'Qualification requires a Linux image\n' >&2
  exit 2
fi
daemon_arch=$(docker info --format '{{.Architecture}}')
case "$daemon_arch" in aarch64|arm64) daemon_arch=arm64;; x86_64|amd64) daemon_arch=amd64;; *) exit 2;; esac
if [ "$image_arch" != "$daemon_arch" ]; then
  printf 'Qualification requires native architecture, not emulation\n' >&2
  exit 2
fi
mkdir -p "$recipe_root/runs"
mkdir "$evidence"
docker image inspect --format '{"image_id":{{json .Id}},"architecture":{{json .Architecture}},"os":{{json .Os}}}' "$image_id" > "$evidence/container.json"
docker run --rm --init --network none --read-only --cap-drop ALL \
  --security-opt no-new-privileges --pids-limit 16 --user "$(id -u):$(id -g)" \
  --mount "type=bind,src=$evidence,dst=/evidence" \
  --mount "type=bind,src=$qualifier,dst=/qualifier,readonly" \
  --mount "type=bind,src=$recipe_root/recipes/verify-process-snapshot-qualification.mjs,dst=/verify.mjs,readonly" \
  --entrypoint sh "$image_id" -eu -c '
cp /qualifier /evidence/qualifier
/evidence/qualifier --write-fixture /evidence/fixture.wasm
./adapters/wasmtime/target/release/wasm-analyze /evidence/fixture.wasm default > /evidence/structure.json
/evidence/qualifier --test-thread-guard > /evidence/guard.json
/evidence/qualifier --test-failure-cleanup > /evidence/cleanup.json
/evidence/qualifier > /evidence/qualification.json
node /verify.mjs /evidence /qualifier > /evidence/receipt.json
'
printf 'Verified process-level COW functional qualification only: %s\n' "$evidence"
