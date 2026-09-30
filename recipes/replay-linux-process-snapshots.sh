#!/bin/sh
# Functional replay only. Source bundle and exact local image must be trusted.
set -eu
recipe_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
source_name=${1:?pass a retained qualification bundle name}
bundle=${2:?pass a new replay directory name}
case "$source_name" in *[!a-zA-Z0-9_-]*|"") exit 2;; esac
case "$bundle" in *[!a-zA-Z0-9_-]*|"") exit 2;; esac
source_bundle="$recipe_root/runs/$source_name"
evidence="$recipe_root/runs/$bundle"
if [ -e "$evidence" ] || [ -L "$evidence" ]; then
  printf 'Preserving existing evidence: %s\n' "$evidence" >&2
  exit 2
fi
test -d "$source_bundle"
test ! -L "$source_bundle"
node "$recipe_root/recipes/verify-process-snapshot-qualification.mjs" \
  "$source_bundle" "$source_bundle/qualifier" --verify-receipt >/dev/null
image_id=$(node -e 'process.stdout.write(JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")).image_id)' "$source_bundle/receipt.json")
observed_id=$(docker image inspect --format '{{.Id}}' "$image_id")
test "$observed_id" = "$image_id"
image_arch=$(docker image inspect --format '{{.Architecture}}' "$image_id")
image_os=$(docker image inspect --format '{{.Os}}' "$image_id")
test "$image_os" = linux
daemon_arch=$(docker info --format '{{.Architecture}}')
case "$daemon_arch" in aarch64|arm64) daemon_arch=arm64;; x86_64|amd64) daemon_arch=amd64;; *) exit 2;; esac
if [ "$image_arch" != "$daemon_arch" ]; then
  printf 'Replay requires native architecture, not emulation\n' >&2
  exit 2
fi
mkdir "$evidence"
docker image inspect --format '{"image_id":{{json .Id}},"architecture":{{json .Architecture}},"os":{{json .Os}}}' "$image_id" > "$evidence/container.json"
docker run --rm --init --network none --read-only --cap-drop ALL \
  --security-opt no-new-privileges --pids-limit 16 --user "$(id -u):$(id -g)" \
  --mount "type=bind,src=$evidence,dst=/evidence" \
  --mount "type=bind,src=$source_bundle,dst=/source,readonly" \
  --mount "type=bind,src=$recipe_root/recipes/verify-process-snapshot-qualification.mjs,dst=/verify.mjs,readonly" \
  --entrypoint sh "$image_id" -eu -c '
node /verify.mjs /source /source/qualifier --verify-receipt > /dev/null
cp /source/qualifier /evidence/qualifier
node /verify.mjs /source /evidence/qualifier --verify-receipt > /dev/null
/evidence/qualifier --write-fixture /evidence/fixture.wasm
cmp /source/fixture.wasm /evidence/fixture.wasm
./adapters/wasmtime/target/release/wasm-analyze /evidence/fixture.wasm default > /evidence/structure.json
/evidence/qualifier --test-thread-guard > /evidence/guard.json
/evidence/qualifier --test-failure-cleanup > /evidence/cleanup.json
/evidence/qualifier > /evidence/qualification.json
node /verify.mjs /evidence /source/qualifier > /evidence/receipt.json
cmp /source/receipt.json /evidence/receipt.json
'
node -e '
const fs=require("fs"),crypto=require("crypto"),path=require("path");
const source=fs.readFileSync(path.join(process.argv[1],"receipt.json")),replay=fs.readFileSync(path.join(process.argv[2],"receipt.json"));
require("assert").deepStrictEqual(source,replay);
const sha=bytes=>crypto.createHash("sha256").update(bytes).digest("hex");
console.log(JSON.stringify({version:"linux-process-snapshot-replay-v1",qualification_only:true,source_receipt_sha256:sha(source),replayed_receipt_sha256:sha(replay),image_id:JSON.parse(source).image_id,exact_evidence_match:true}));
' "$source_bundle" "$evidence" > "$evidence/replay.json"
printf 'Verified archived process-snapshot functional replay only: %s\n' "$evidence"
