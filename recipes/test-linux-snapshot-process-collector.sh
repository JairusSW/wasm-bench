#!/bin/sh
# Collector functional qualification only; owned Go helpers, not Wasmtime clones.
set -eu
recipe_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
image_ref=${1:?pass an existing Linux image}
bundle=${2:?pass a new evidence directory name}
case "$bundle" in *[!a-zA-Z0-9_-]*|"") exit 2;; esac
evidence="$recipe_root/runs/$bundle"
if [ -e "$evidence" ] || [ -L "$evidence" ]; then
  printf 'Preserving existing evidence: %s\n' "$evidence" >&2
  exit 2
fi
collector_test="$recipe_root/.wasmbench/snapshot-process-collector.test"
test -f "$collector_test"
image_id=$(docker image inspect --format '{{.Id}}' "$image_ref")
case "$image_id" in sha256:*) ;; *) exit 2;; esac
digest=${image_id#sha256:}
case "$digest" in *[!a-f0-9]*) exit 2;; esac
test "${#digest}" -eq 64
test "$(docker image inspect --format '{{.Os}}' "$image_id")" = linux
image_arch=$(docker image inspect --format '{{.Architecture}}' "$image_id")
daemon_arch=$(docker info --format '{{.Architecture}}')
case "$daemon_arch" in aarch64|arm64) daemon_arch=arm64;; x86_64|amd64) daemon_arch=amd64;; *) exit 2;; esac
test "$image_arch" = "$daemon_arch"
mkdir -p "$recipe_root/runs"
mkdir "$evidence"
docker image inspect --format '{"image_id":{{json .Id}},"architecture":{{json .Architecture}},"os":{{json .Os}}}' "$image_id" > "$evidence/container.json"
# shellcheck disable=SC2016
docker run --rm --init --network none --read-only --cap-drop ALL \
  --security-opt no-new-privileges --pids-limit 64 --user "$(id -u):$(id -g)" \
  --mount "type=bind,src=$evidence,dst=/evidence" \
  --mount "type=bind,src=$collector_test,dst=/collector-test,readonly" \
  --entrypoint sh "$image_id" -eu -c '
cp /collector-test /evidence/collector-test
WASMBENCH_SNAPSHOT_PROC_EVIDENCE_DIR=/evidence /evidence/collector-test -test.run "Test(NativeSnapshotProcessLineage|SnapshotProcess)" -test.v > /evidence/test.log
cd /evidence
sha256sum collector-test container.json source.json template.json restored.json test.log > checksums.sha256
'
printf 'Verified native process collector using owned helpers: %s\n' "$evidence"
