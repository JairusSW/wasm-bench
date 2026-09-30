#!/bin/sh
# Simultaneously held process-COW groups; qualification, not product timing.
set -eu
recipe_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
image_ref=${1:?pass an existing native Linux image}
bundle=${2:?pass a new evidence directory name}
mode=${3:-qualification}
case "$mode" in qualification|product) ;; *) exit 2;; esac
case "$bundle" in *[!a-zA-Z0-9_-]*|"") exit 2;; esac
evidence="$recipe_root/runs/$bundle"
if [ -e "$evidence" ] || [ -L "$evidence" ]; then
  printf 'Preserving existing evidence: %s\n' "$evidence" >&2
  exit 2
fi
test_binary="$recipe_root/.wasmbench/snapshot-density.test"
adapter="$recipe_root/.wasmbench/wasmtime-linux-target/process-snapshot/release/qualify-process-snapshot"
test -f "$test_binary"
test -f "$adapter"
set --
if [ "$mode" = product ]; then
  controller="$recipe_root/.wasmbench/wasmbench-linux-process-snapshot"
  test -f "$controller"
  set -- --mount "type=bind,src=$controller,dst=/opt/wasmbench/bin/wasmbench,readonly" \
    --mount "type=bind,src=$adapter,dst=/opt/wasmbench/adapters/wasmtime/target/process-snapshot/release/qualify-process-snapshot,readonly" \
    --tmpfs /opt/wasmbench/.wasmbench:rw,nosuid,nodev,mode=1777
fi
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
  --security-opt no-new-privileges --pids-limit 128 --user "$(id -u):$(id -g)" \
  --tmpfs /tmp:rw,exec,nosuid,nodev,mode=1777 \
  "$@" \
  --mount "type=bind,src=$evidence,dst=/evidence" \
  --mount "type=bind,src=$test_binary,dst=/density-test,readonly" \
  --mount "type=bind,src=$adapter,dst=/adapter,readonly" \
  --entrypoint sh "$image_id" -eu -c '
cp /density-test /evidence/density-test
cp /adapter /evidence/adapter
WASMBENCH_SNAPSHOT_DENSITY_ADAPTER=/evidence/adapter WASMBENCH_SNAPSHOT_DENSITY_EVIDENCE_DIR=/evidence /evidence/density-test -test.run "^(TestNativeSnapshotDensity|TestNativeSnapshotDensityCleanup|TestNativeSnapshotDensityTrial|TestNativeSnapshotDensityPartialCleanup)$" -test.v > /evidence/test.log
WASMBENCH_SNAPSHOT_RETAINED_DENSITY_EVIDENCE_DIR=/evidence /evidence/density-test -test.run "^(TestRetainedSnapshotDensity|TestRetainedSnapshotDensityPartialCleanup)$" -test.v > /evidence/retained.log
if [ "$1" = product ]; then
  cd /opt/wasmbench
  (
    ./bin/wasmbench run --suite process-snapshot-density --profile memory --phase-barriers \
      --runtimes wasmtime-process-snapshot,wasmtime-winch-process-snapshot \
      --scenarios process-snapshot-density --launches 2 --samples 2 --operations 1 --warmup 0 --out /evidence/original
    ./bin/wasmbench verify --run /evidence/original
    ./bin/wasmbench reproduce /evidence/original --out /evidence/reproduced
    ./bin/wasmbench verify --run /evidence/reproduced
    ./bin/wasmbench report --run /evidence/original --out /evidence/report
    ./bin/wasmbench verify-report --dir /evidence/report
    ./bin/wasmbench verify-report --dir /evidence/report --recorded-builder
  ) > /evidence/product.log
  WASMBENCH_SNAPSHOT_DENSITY_PRODUCT_EVIDENCE_DIR=/evidence /evidence/density-test -test.run "^TestRetainedSnapshotDensityProduct$" -test.v > /evidence/product-coverage.log
fi
cd /evidence
sha256sum density-test adapter container.json test.log retained.log \
  cranelift-1.json cranelift-2.json cranelift-4.json cranelift-8.json cranelift-32.json \
  winch-1.json winch-2.json winch-4.json winch-8.json winch-32.json \
  cleanup-cranelift-template_after_source_release.json cleanup-cranelift-idle.json \
  cleanup-cranelift-touched.json cleanup-cranelift-executed.json \
  cleanup-cranelift-source_killed_at_idle.json \
  cleanup-winch-template_after_source_release.json cleanup-winch-idle.json \
  cleanup-winch-touched.json cleanup-winch-executed.json \
  cleanup-winch-source_killed_at_idle.json \
  partial-cranelift-8-1.json partial-cranelift-8-4.json partial-cranelift-8-7.json partial-cranelift-32-31.json \
  partial-winch-8-1.json partial-winch-8-4.json partial-winch-8-7.json partial-winch-32-31.json > checksums.sha256
if [ "$1" = product ]; then sha256sum product.log product-coverage.log >> checksums.sha256; fi
sha256sum -c checksums.sha256 > integrity.log
' snapshot-density "$mode"
printf 'Verified native simultaneous snapshot density: %s\n' "$evidence"
