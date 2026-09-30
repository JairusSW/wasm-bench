// Scope/coverage check after the Go loader's full typed-proof validation.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
const [root, arch, original] = process.argv.slice(2);
assert.ok(root && ['arm64','amd64'].includes(arch));
const read=(base,file)=>JSON.parse(fs.readFileSync(path.join(base,file),'utf8'));
const m=read(root,'manifest.json'), o=m.lock.options;
assert.equal(m.host.os,'linux');assert.equal(m.host.arch,arch);
assert.equal(o.suite,'process-snapshots');assert.equal(o.profile,'timing');
assert.equal(o.samples,2);assert.equal(o.operations,1);assert.equal(o.warmup,0);
const stages=['process-snapshot-capture','process-snapshot-restore','process-snapshot-first-write','process-snapshot-execute'];
assert.deepEqual([...o.scenarios].sort(),[...stages].sort());
assert.deepEqual(m.lock.runtime_configurations.map(r=>r.id).sort(),['wasmtime-process-snapshot','wasmtime-winch-process-snapshot'].sort());
assert.equal(m.lock.workloads.length,1);
const trials=fs.readdirSync(path.join(root,'trials')).map(f=>read(root,'trials/'+f));
assert.equal(trials.length,2+8*o.launches);
for(const t of trials){
  assert.equal(t.status,'ok',t.reason);assert.equal(t.samples.length,2);
  assert.equal(t.profile,'timing');assert.ok(stages.includes(t.scenario));
  for(const s of t.samples){assert.equal(s.verified,true);assert.ok(s.process_snapshot_result);}
}
for(const r of m.lock.runtime_configurations)for(const stage of stages){
  assert.equal(trials.filter(t=>t.block>=0&&t.runtime_configuration===r.id&&t.scenario===stage).length,o.launches);
}
if(original){
  const source=read(original,'manifest.json');
  assert.deepEqual(m.lock.runtime_configurations,source.lock.runtime_configurations);
  assert.deepEqual(m.lock.workloads,source.lock.workloads);
  assert.deepEqual(m.lock.options,source.lock.options);
}
console.log('Native snapshot timing coverage verified; exploratory only.');
