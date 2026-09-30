#!/bin/sh
# Only for an ephemeral, privileged Docker container with --cgroupns=private.
# Never run this setup on the host. The mounted test binary is read-only.
set -eu
test "${WASMBENCH_EPHEMERAL_CGROUP_TEST:-}" = "1"
test -f /.dockerenv
test "$(cat /proc/self/cgroup)" = "0::/"
mkdir /sys/fs/cgroup/wasmbench-controller /sys/fs/cgroup/wasmbench-workers
printf '%s' "$$" > /sys/fs/cgroup/wasmbench-controller/cgroup.procs
printf '%s' '+memory +cpu +pids +cpuset' > /sys/fs/cgroup/cgroup.subtree_control
printf '%s' '+memory +cpu +pids +cpuset' > /sys/fs/cgroup/wasmbench-workers/cgroup.subtree_control
export WASMBENCH_TEST_CGROUP_PARENT=/sys/fs/cgroup/wasmbench-workers
/agent.test -test.v
if test "${WASMBENCH_RUN_VECTOR_PHASE_SMOKE:-}" = "1"; then
  # Only this ephemeral container's private cgroup hierarchy is configured.
  cp /fixtures/vector-lifecycle.wasm /fixtures/vector-lifecycle.wat /fixtures/vector-initialization.wasm /fixtures/vector-initialization.wat /evidence/
  node /vector-verifier.mjs suite /evidence
  /opt/wasmbench/bin/wasmbench run --suite /evidence/suite.json \
    --runtimes wago,wazero,wazero-interpreter,v8,wasmtime,wasmtime-winch \
    --profile memory --phase-barriers --scenarios instantiate,first-call \
    --launches 1 --samples 2 --operations 1 --warmup 0 \
    --cgroup-parent "$WASMBENCH_TEST_CGROUP_PARENT" \
    --memory-max 536870912 --no-swap --cpu-quota-us 100000 --pids-max 128 \
    --out /evidence/run
  /opt/wasmbench/bin/wasmbench verify --run /evidence/run
  vector_arch=$(uname -m)
  case "$vector_arch" in aarch64) vector_arch=arm64;; x86_64) vector_arch=amd64;; *) exit 2;; esac
  node /vector-verifier.mjs cgroup /evidence/run "$vector_arch"
  if test "${WASMBENCH_REPLAY_VECTOR_PHASE_SMOKE:-}" = "1"; then
    /opt/wasmbench/bin/wasmbench reproduce /evidence/run --out /evidence/replayed
    /opt/wasmbench/bin/wasmbench restore-tools --run /evidence/run --out /evidence/tools > /evidence/restoration.json
    /evidence/tools/tools/runner/wasmbench run --lock /evidence/tools/suite.lock --out /evidence/archived
    for vector_bundle in replayed archived; do
      /opt/wasmbench/bin/wasmbench verify --run "/evidence/$vector_bundle"
      node /vector-verifier.mjs cgroup "/evidence/$vector_bundle" "$vector_arch" /evidence/run "$vector_bundle"
    done
    /opt/wasmbench/bin/wasmbench report --run /evidence/run --out /evidence/report
    /opt/wasmbench/bin/wasmbench verify-report --dir /evidence/report
    /opt/wasmbench/bin/wasmbench verify-report --dir /evidence/report --recorded-builder
  fi
fi
if test "${WASMBENCH_RUN_SOURCE_TESTS:-}" = "1"; then
  /sourcebuild.test -test.v
fi
if test "${WASMBENCH_RUN_CGROUP_SMOKE:-}" = "1"; then
  /opt/wasmbench/bin/wasmbench run --suite core --runtimes wazero,v8 \
    --profile memory --scenarios compile,steady --launches 1 --samples 2 \
    --operations 2 --warmup 0 --cgroup-parent "$WASMBENCH_TEST_CGROUP_PARENT" \
    --memory-max 536870912 --no-swap --cpu-quota-us 100000 --pids-max 128 \
    --out /evidence/linux-cgroup-v1
  /opt/wasmbench/bin/wasmbench verify --run /evidence/linux-cgroup-v1
fi
if test "${WASMBENCH_RUN_PHASE_SMOKE:-}" = "1"; then
  phase_bundle="${WASMBENCH_PHASE_BUNDLE:-linux-cgroup-phase-v1}"
  phase_scenario="${WASMBENCH_PHASE_SCENARIO:-compile}"
  case "$phase_scenario" in compile|teardown|app-init|instantiate) ;; *) exit 2;; esac
  case "$phase_bundle" in *[!a-zA-Z0-9_-]*|"") exit 2;; esac
  /opt/wasmbench/bin/wasmbench run --suite "${WASMBENCH_PHASE_SUITE:-core}" --runtimes "${WASMBENCH_PHASE_RUNTIMES:-wazero,v8}" \
    --timeout "${WASMBENCH_PHASE_TIMEOUT:-30s}" \
    --profile memory --phase-barriers --scenarios "$phase_scenario" --launches 2 --samples 3 \
    --operations 5 --warmup 0 --cgroup-parent "$WASMBENCH_TEST_CGROUP_PARENT" \
    --memory-max 536870912 --no-swap --cpu-quota-us 100000 --pids-max 128 \
    --out "/evidence/$phase_bundle"
  /opt/wasmbench/bin/wasmbench verify --run "/evidence/$phase_bundle"
fi
