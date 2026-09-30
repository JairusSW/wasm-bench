import assert from 'node:assert/strict';
import { readFileSync, readdirSync, existsSync } from 'node:fs';
import { join } from 'node:path';
import { createHash } from 'node:crypto';

const root = process.argv[2];
assert(root, 'evidence directory required');
const json = path => JSON.parse(readFileSync(path, 'utf8'));
let original;
for (const name of ['check', 'run', 'reproduced']) {
  const dir = join(root, name);
  const manifest = json(join(dir, 'manifest.json'));
  assert.equal(manifest.lock.analyzer.validation_profile, 'default');
  assert.equal(manifest.lock.analyzer.analysis_version, 'artifact-structure-v1');
  assert.deepEqual(manifest.lock.runtime_configurations.map(r => r.id), ['wazero', 'wazero-interpreter', 'v8']);
  const reports = readdirSync(join(dir, 'validation'));
  assert.equal(reports.length, 2);
  for (const workload of manifest.lock.workloads) {
    const report = json(join(dir, 'validation', `${workload.sha256}.json`));
    assert.equal(report.sha256, workload.sha256);
    assert.equal(report.validated, true);
    assert.equal(report.encoding, 'core-module');
    assert.equal(report.analysis_version, 'core-structure-v3');
  }
  const trials = readdirSync(join(dir, 'trials')).map(file => json(join(dir, 'trials', file)));
  assert.equal(trials.length, name === 'check' ? 6 : 30);
  for (const trial of trials) {
    assert.equal(trial.status, 'ok', `${name}: ${trial.id}: ${trial.reason}`);
    if (trial.block >= 0) {
      assert.equal(trial.samples.length, 3);
      assert(trial.samples.every(sample => sample.verified));
    }
  }
  if (name === 'check') {
    assert.equal(manifest.kind, 'correctness_only');
    assert.equal(manifest.publication, 'prohibited');
  } else {
    assert.equal(manifest.trial_order.length, 24);
    assert.equal(manifest.publication, 'local_exploratory');
  }
  if (name === 'run') original = manifest;
  if (name === 'reproduced') {
    assert.deepEqual(manifest.lock.analyzer, original.lock.analyzer);
    assert.equal(manifest.lock.runner_sha256, original.lock.runner_sha256);
    assert.deepEqual(manifest.lock.runtime_configurations, original.lock.runtime_configurations);
    for (const report of reports) {
      assert.deepEqual(readFileSync(join(dir, 'validation', report)), readFileSync(join(root, 'run', 'validation', report)));
    }
  }
}
assert(existsSync(join(root, 'report', 'index.html')));
for (const name of ['floats', 'float-phases']) {
  const dir = join(root, name), manifest = json(join(dir, 'manifest.json'));
  assert.equal(manifest.lock.workloads.length, 7);
  assert(manifest.lock.workloads.every(w => w.oracle.kind === 'float_bits_v1'));
  const v8 = manifest.lock.runtime_configurations.find(r => r.id === 'v8');
  for(const name of ['floats.mjs','profiling.mjs','compiler-mode.mjs']){
    const helper = Object.entries(v8.file_sha256).find(([p]) => p.endsWith('/adapters/v8/'+name));
    assert(helper, 'V8 helper must be packaged and pinned: '+name);
    assert.equal(helper[1], createHash('sha256').update(readFileSync(helper[0])).digest('hex'));
  }
  const trials = readdirSync(join(dir, 'trials')).map(f => json(join(dir, 'trials', f)));
  const measured = trials.filter(t => t.block >= 0);
  assert.equal(trials.length, name === 'floats' ? 105 : 84);
  assert.equal(measured.length, name === 'floats' ? 84 : 63);
  for (const t of trials) assert.equal(t.status, 'ok', `${name}: ${t.id}: ${t.reason}`);
  for (const t of measured) {
    const trajectory = t.scenario === 'trajectory';
    assert.equal(t.samples.length, trajectory ? 5 : name === 'floats' ? 3 : 2);
    for (const [i, s] of t.samples.entries()) {
      assert.equal(s.index, i);
      assert.equal(s.verified, true);
      assert.equal(s.warmup, trajectory && i < 2);
      if (trajectory || t.scenario === 'teardown' || name === 'float-phases') assert.equal(s.operations, 1);
      if (name === 'float-phases') {
        const stages = t.scenario === 'compile' ? ['before_compile','compiled','released'] : t.scenario === 'instantiate' ? ['before_instantiate','instantiated','instance_released'] : ['before_teardown','torn_down'];
        for (const stage of stages) assert(s.observations.some(o => o.phase === `${t.scenario}/${stage}`), 'missing phase evidence');
      }
    }
  }
}
const densityDir=join(root,'density'),densityManifest=json(join(densityDir,'manifest.json'));
assert.equal(densityManifest.lock.workloads.length,16);
assert.equal(densityManifest.lock.options.launches,3);
const densityTrials=readdirSync(join(densityDir,'trials')).map(f=>json(join(densityDir,'trials',f)));
assert.equal(densityTrials.length,192);
let readyRSS=0,successful=0,unsupported=0;
for(const t of densityTrials){
  const w=densityManifest.lock.workloads.find(w=>w.id===t.workload);
  const unavailable=t.runtime_configuration==='v8'&&w.density.sharing==='separate_engines';
  assert.equal(t.status,unavailable?'unsupported':'ok',`${t.id}: ${t.reason}`);
  if(unavailable){assert.equal((t.samples||[]).length,0);if(t.block>=0)unsupported++;continue;}
  assert.equal(t.samples.length,t.block<0?1:2);
  for(const s of t.samples){
    assert.equal(s.verified,true);assert.equal(s.operations,1);assert.equal(s.warmup,false);
    const logical=s.observations.find(o=>o.metric==='density.guest_memory.logical');
    assert.equal(logical.value,w.density.instances*65536);
    assert.equal(logical.scope,'instance_group_linear_memory');
    assert.equal(logical.normalization_denominator,'instance_group');
  }
  if(t.block<0)continue;
  successful++;
  assert.equal(t.phase_events.length,6);
  t.phase_events.forEach((p,i)=>{
    assert.equal(p.event.sample_index,Math.floor(i/3));
    assert.equal(p.event.stage,['before_density','density_ready','density_released'][i%3]);
    const rss=p.observations.find(o=>o.metric==='process.rss');
    assert.equal(rss.status,'available');assert(rss.value>0);
    assert.equal(rss.quality,'boundary_snapshot_only');
    if(p.event.stage==='density_ready')readyRSS++;
  });
}
assert.equal(successful,120);assert.equal(unsupported,24);assert.equal(readyRSS,240);
const densityReport=json(join(root,'density-report','data.json'));
const logicalCurves=densityReport.scaling.filter(c=>c.measurement.metric==='density.guest_memory.logical');
assert.equal(logicalCurves.length,10);
for(const c of logicalCurves){
  assert.equal(c.points.length,4);assert.equal(c.marginal_costs.length,3);
  for(const m of c.marginal_costs){assert.equal(m.paired_blocks,3);assert.equal(m.status,'available');assert.equal(m.median_per_added_unit,65536);assert.equal(m.ci95_low,65536);assert.equal(m.ci95_high,65536);}
}
assert(existsSync(join(root,'density-report','index.html')));
console.log('Verified packaged core/replay/report, 147 float trials, and density: 120 successful cells, 24 unsupported, 240 live-group RSS snapshots and 30 paired marginal intervals.');
