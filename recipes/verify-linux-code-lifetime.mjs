// Additional coverage gate; run only after `wasmbench verify` validates the seal,
// typed lifecycle evidence and independent native function attribution.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import {pathToFileURL} from 'node:url';

export function verifyCodeLifetimeCoverage(root, arch, originalRoot) {
  assert.ok(['arm64','amd64'].includes(arch), 'declare native Linux architecture');
  const read=p=>JSON.parse(fs.readFileSync(p,'utf8'));
  const manifest=read(path.join(root,'manifest.json')), options=manifest.lock.options;
  assert.equal(manifest.host.os,'linux');assert.equal(manifest.host.arch,arch);
  assert.ok(Number.isSafeInteger(manifest.host.page_size)&&manifest.host.page_size>0);
  if(originalRoot)assert.deepEqual(manifest.host,read(path.join(originalRoot,'manifest.json')).host,'replay must retain the same observed host identity and policy');
  assert.equal(options.correctness_only,false);assert.equal(options.profile,'code');
  assert.deepEqual(options.scenarios,['code-lifetime']);assert.equal(options.launches,3);
  assert.equal(options.samples,1);assert.equal(options.operations,1);assert.equal(options.warmup,0);
  assert.equal(options.phase_barriers,false);
  const runtimes=manifest.lock.runtime_configurations;
  assert.deepEqual(runtimes.map(r=>r.id),['wasmtime-code-lifetime','wasmtime-winch-code-lifetime']);
  for(const [i,r] of runtimes.entries()){
    const d=r.description;assert.equal(d.runtime,'wasmtime');assert.equal(d.runtime_version,'46.0.1');
    assert.equal(d.backend,i===0?'cranelift':'winch');assert.equal(d.capabilities.can_code_lifetime,true);
    assert.equal(d.capabilities.can_snapshot,false);
    assert.equal(d.effective_configuration.code_lifetime,'wasmtime-executable-publication-v1');
  }
  assert.deepEqual(manifest.lock.workloads.map(w=>w.id),['mechanisms/identity','algorithms/sum']);
  const expected=new Set();
  for(const r of runtimes)for(const w of manifest.lock.workloads)for(let block=0;block<3;block++)expected.add([r.id,w.id,block].join('/'));
  let trials=0,events=0,checkpoints=0;
  for(const name of fs.readdirSync(path.join(root,'trials'))){
    const t=read(path.join(root,'trials',name));if(t.block<0)continue;
    const key=[t.runtime_configuration,t.workload,t.block].join('/');
    assert.ok(expected.delete(key),'duplicate or unexpected diagnostic cell: '+key);
    assert.equal(t.status,'ok');assert.equal(t.profile,'code');assert.equal(t.scenario,'code-lifetime');
    assert.ok(!t.samples?.length&&!t.adapter_samples?.length&&!t.phase_events?.length,'diagnostic events must not masquerade as timing/phase samples');
    const l=t.code_lifetime;assert.ok(l&&t.code_image,'complete native evidence required');
    assert.equal(l.architecture,arch);assert.equal(l.page_size,manifest.host.page_size);
    assert.equal(l.events.length,2);assert.equal(l.checkpoints.length,5);
    assert.deepEqual(l.events.map(e=>e.kind),['published','unpublished']);
    assert.deepEqual(l.checkpoints.map(c=>c.stage),['compiled','instance_verified','module_handles_dropped','store_dropped','engine_dropped']);
    assert.equal(l.events[1].active_capacity,0);assert.equal(l.checkpoints[4].active_capacity,0);
    assert.ok(l.events[1].cumulative_published_capacity>0);
    trials++;events+=l.events.length;checkpoints+=l.checkpoints.length;
  }
  assert.equal(expected.size,0,'missing measured diagnostic cells');
  return {arch,trials,events,checkpoints};
}

if(process.argv[1]&&import.meta.url===pathToFileURL(path.resolve(process.argv[1])).href){
  console.log(JSON.stringify(verifyCodeLifetimeCoverage(process.argv[2],process.argv[3],process.argv[4])));
}
