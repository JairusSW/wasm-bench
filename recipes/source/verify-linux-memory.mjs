import fs from 'node:fs';
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';

const root = process.argv[2];
assert(root, 'evidence directory required');
const read = path => JSON.parse(fs.readFileSync(`${root}/${path}`));
const original = read('original/benchmark.json');
const replay = read('replayed/benchmark.json');
assert.equal(original.config_sha256, replay.config_sha256);
assert.match(original.config_sha256, /^[a-f0-9]{64}$/);
assert.equal(replay.reproduction.parent_checksums_sha256,
  createHash('sha256').update(fs.readFileSync(`${root}/original/checksums.json`)).digest('hex'));
assert.deepEqual(original.admissions.map(a => a.artifact_sha256), replay.admissions.map(a => a.artifact_sha256));
assert.deepEqual(original.trials.map(t => [t.variant, t.block, t.warmup]),
  replay.trials.map(t => [t.variant, t.block, t.warmup]));
for (const name of ['original', 'replayed']) {
  const run = read(`${name}/benchmark.json`);
  const report = read(`${name}-report.json`);
  assert.equal(run.status, 'complete');
  assert.equal(run.config.profile, 'memory');
  assert.equal(run.trials.length, 14);
  assert.equal(run.admissions.length, 2);
  assert(run.admissions.every(a => a.status === 'ok'));
  assert.deepEqual(run.config.correctness_runtimes.map(r => r.id), ['wazero', 'wazero-interpreter', 'v8']);
  assert.equal(report.metric.name, 'source.build.max_step_cgroup_peak');
  assert.equal(report.comparisons[0].paired_blocks, 6);
  for (const trial of run.trials) {
    assert.equal(trial.status, 'ok');
    const build = read(`${name}/${trial.build_bundle}/build.json`);
    const peaks = build.steps.map(step => {
      assert.equal(step.resources.oom_kill, false);
      assert.equal(step.resources.isolation.mode, 'cgroup_v2_at_spawn');
      assert.equal(step.resources.isolation.effective['memory.max'], '536870912');
      const obs = step.resources.observations.find(o => o.metric === 'source.build.cgroup.peak');
      assert.equal(obs.status, 'available');
      assert.equal(obs.unit, 'bytes');
      assert(obs.value > 0);
      return obs.value;
    });
    assert.equal(trial.max_step_cgroup_peak_bytes, Math.max(...peaks));
  }
  for (let variant = 0; variant < 2; variant++) {
    const values = run.trials.filter(t => !t.warmup && t.variant === variant)
      .map(t => t.max_step_cgroup_peak_bytes).sort((a, b) => a-b);
    assert.equal(values.length, 6);
    assert.equal(report.summaries[variant].median_bytes_max_step_peak, (values[2]+values[3])/2);
    assert.equal(report.summaries[variant].median_ns_per_build, null);
  }
}
console.log('Verified original and replay: 12 measured builds + 2 warmups each; step maxima and byte medians match raw evidence.');
