#!/bin/sh
# Requires the ephemeral source integration image and a PRIVATE cgroup namespace.
# /recipes is this directory, read-only; /evidence is a writable evidence mount.
# The current Linux CLI/adapters must be mounted over the image's older binaries.
set -eu
test "${WASMBENCH_EPHEMERAL_CGROUP_TEST:-}" = "1"
test -f /.dockerenv
test "$(cat /proc/self/cgroup)" = "0::/"
mkdir /sys/fs/cgroup/wasmbench-controller /sys/fs/cgroup/wasmbench-workers
printf '%s' "$$" > /sys/fs/cgroup/wasmbench-controller/cgroup.procs
printf '%s' '+memory +cpu +pids +cpuset' > /sys/fs/cgroup/cgroup.subtree_control
printf '%s' '+memory +cpu +pids +cpuset' > /sys/fs/cgroup/wasmbench-workers/cgroup.subtree_control
source_evidence=/evidence/source-linux-memory-v1
mkdir "$source_evidence"
node --input-type=module - "$source_evidence" <<'JS'
import fs from 'node:fs';
const out = process.argv[2];
for (const opt of [0, 2]) {
  const recipe = JSON.parse(fs.readFileSync('/recipes/xorshift-llvm.json'));
  recipe.id = `linux-llvm-xorshift-o${opt}`;
  recipe.inputs['xorshift.c'].path = '/recipes/xorshift.c';
  recipe.tools.clang.path = '/usr/bin/clang-16';
  recipe.tools.linker.path = '/usr/bin/wasm-ld-16';
  recipe.steps[0].args = recipe.steps[0].args.map(x => x === '-O2' ? `-O${opt}` : x);
  fs.writeFileSync(`${out}/o${opt}.json`, JSON.stringify(recipe, null, 2), {flag: 'wx'});
}
JS
runner=/opt/wasmbench/bin/wasmbench
for opt in 0 2; do
  "$runner" source-lock --recipe "$source_evidence/o$opt.json" --out "$source_evidence/o$opt.lock.json"
done
tool_cpus=$(cat /sys/fs/cgroup/wasmbench-workers/cpuset.cpus.effective)
"$runner" source-bench --locks "$source_evidence/o0.lock.json,$source_evidence/o2.lock.json" \
  --runtimes wazero,wazero-interpreter,v8 --profile memory --blocks 6 --warmup 1 --seed 42 \
  --cgroup-parent /sys/fs/cgroup/wasmbench-workers --memory-max 536870912 --no-swap \
  --cpu-quota-us 100000 --cpus "$tool_cpus" --pids-max 128 --out "$source_evidence/original"
"$runner" source-bench-report --bundle "$source_evidence/original" --out "$source_evidence/original-report.json"
"$runner" source-bench-replay --bundle "$source_evidence/original" --out "$source_evidence/replayed"
"$runner" source-bench-report --bundle "$source_evidence/replayed" --out "$source_evidence/replayed-report.json"
node /recipes/verify-linux-memory.mjs "$source_evidence"
