#!/bin/sh
# Exercises only packaged binaries; no source-built executable is bind-mounted.
set -eu
recipe_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
image_ref=${1:-wasmbench:dev}
bundle=${2:?pass a new evidence directory name}
case "$bundle" in *[!a-zA-Z0-9_-]*|"") exit 2;; esac
image_id=$(docker image inspect --format '{{.Id}}' "$image_ref")
evidence="$recipe_root/runs/$bundle"
mkdir -p "$recipe_root/runs"
mkdir "$evidence"
container() {
  docker run --rm --network none --read-only --cap-drop ALL \
    --security-opt no-new-privileges --user "$(id -u):$(id -g)" \
    --tmpfs /tmp:rw,nosuid,nodev,mode=1777 \
    --tmpfs /opt/wasmbench/.wasmbench:rw,nosuid,nodev,mode=1777 \
    --mount "type=bind,src=$evidence,dst=/evidence" \
    --mount "type=bind,src=$recipe_root/recipes/verify-container-evidence.mjs,dst=/verify.mjs,readonly" \
    --entrypoint /usr/bin/env "$image_id" "$@"
}
container ./bin/wasmbench doctor > "$evidence/doctor.json"
container ./bin/wasmbench check --suite core --runtimes wazero,wazero-interpreter,v8 --out /evidence/check
container ./bin/wasmbench run --suite core --runtimes wazero,wazero-interpreter,v8 \
  --scenarios compile,first-call --launches 2 --samples 3 --operations 1 --out /evidence/run
container ./bin/wasmbench reproduce /evidence/run --out /evidence/reproduced
container ./bin/wasmbench run --suite floats --runtimes wazero,wazero-interpreter,v8 \
  --scenarios compile,instantiate,trajectory,teardown --launches 1 --samples 3 --warmup 2 --operations 9 --out /evidence/floats
container ./bin/wasmbench run --suite floats --runtimes wazero,wazero-interpreter,v8 \
  --profile memory --phase-barriers --scenarios compile,instantiate,teardown \
  --launches 1 --samples 2 --operations 9 --out /evidence/float-phases
container ./bin/wasmbench run --suite density --runtimes wazero,wazero-interpreter,v8 \
  --profile memory --phase-barriers --scenarios density \
  --launches 3 --samples 2 --operations 9 --warmup 3 --out /evidence/density
for name in check run reproduced floats float-phases density; do
  container ./bin/wasmbench verify --run "/evidence/$name"
done
container ./bin/wasmbench report --run /evidence/run --out /evidence/report
container ./bin/wasmbench report --run /evidence/density --out /evidence/density-report
container node /verify.mjs /evidence
printf 'Verified packaged image %s; evidence: %s\n' "$image_id" "$evidence"
