#!/bin/sh
# Development native timing qualification; not a registered benchmark adapter.
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
worker="$recipe_root/.wasmbench/wasmtime-linux-target/process-snapshot/release/qualify-process-snapshot"
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
docker run --rm --init --network none --read-only --cap-drop ALL \
  --security-opt no-new-privileges --pids-limit 16 --user "$(id -u):$(id -g)" \
  --mount "type=bind,src=$evidence,dst=/evidence" \
  --mount "type=bind,src=$worker,dst=/worker,readonly" \
  --entrypoint sh "$image_id" -eu -c '
cp /worker /evidence/worker
/evidence/worker --test-thread-guard > /evidence/guard.json
/evidence/worker --test-failure-cleanup > /evidence/cleanup.json
/evidence/worker > /evidence/qualification.json
for backend in cranelift winch; do
  for stage in capture restore first-write execute; do
    /evidence/worker --timed-stage "$backend" "process-snapshot-$stage" 2 > "/evidence/$backend-$stage.json"
  done
done
cd /evidence
sha256sum worker container.json guard.json cleanup.json qualification.json cranelift-capture.json cranelift-restore.json cranelift-first-write.json cranelift-execute.json winch-capture.json winch-restore.json winch-first-write.json winch-execute.json > checksums.sha256
'
printf 'Native development worker evidence (not official performance): %s\n' "$evidence"
