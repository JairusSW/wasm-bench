import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import {pathToFileURL} from 'node:url';
const runtimes=['wazero','wazero-interpreter','v8'];
const workloads=['mechanisms/identity','algorithms/sum'];
const phases={compile:['before_compile','compiled','released'],instantiate:['before_instantiate','instantiated','instance_released'],'first-call':['before_first_call','first_call_returned','first_call_released']};
const read=p=>JSON.parse(fs.readFileSync(p));
export function verifyWindowsBundle(manifest,trials,memory=false,original,archived=false) {
  assert.equal(manifest.host.os,'windows');assert.ok(['amd64','arm64'].includes(manifest.host.arch));
  const lock=manifest.lock,o=lock.options;
  assert.equal(lock.archive_tools,true);assert.equal(o.correctness_only,false);assert.equal(o.profile,memory?'memory':'timing');assert.equal(!!o.phase_barriers,memory);
  assert.equal(o.launches,memory?1:2);assert.equal(o.samples,2);assert.equal(o.operations,memory?1:2);assert.equal(o.warmup,0);
  assert.deepEqual(o.scenarios,Object.keys(phases));assert.deepEqual(lock.runtime_configurations.map(r=>r.id),runtimes);assert.deepEqual(lock.workloads.map(w=>w.id),workloads);
  if(original){
    assert.deepEqual(manifest.host,original.host);assert.equal(lock.runner_sha256,original.lock.runner_sha256);
    for(const [i,w] of lock.workloads.entries())assert.equal(w.sha256,original.lock.workloads[i].sha256);
    for(const [i,r] of lock.runtime_configurations.entries()){
      const prior=original.lock.runtime_configurations[i];assert.deepEqual(Object.values(r.file_sha256).sort(),Object.values(prior.file_sha256).sort());assert.deepEqual(r.description,prior.description);
      if(archived)assert.notEqual(r.command[0],prior.command[0]);
    }
  }
  const expected=new Set(runtimes.flatMap(r=>workloads.flatMap(w=>Object.keys(phases).flatMap(s=>Array.from({length:o.launches},(_,b)=>r+'/'+w+'/'+s+'/'+b))))),checks=new Set();let samples=0,boundaries=0;
  for(const t of trials){
    assert.equal(t.status,'ok');assert.equal(t.profile,o.profile);assert.ok(runtimes.includes(t.runtime_configuration));assert.ok(workloads.includes(t.workload));
    for(const s of t.samples){assert.equal(s.verified,true);assert.equal(s.warmup,false);}
    if(t.block<0){assert.equal(t.scenario,'first-call');const key=t.runtime_configuration+'/'+t.workload;assert.ok(!checks.has(key));checks.add(key);assert.ok(t.samples.length>0);continue;}
    assert.ok(expected.delete(t.runtime_configuration+'/'+t.workload+'/'+t.scenario+'/'+t.block));assert.equal(t.samples.length,2);
    for(const [i,s] of t.samples.entries()){assert.equal(s.index,i);assert.equal(s.operations,o.operations);samples++;}
    if(!memory){assert.equal(t.phase_events?.length||0,0);continue;}
    assert.equal(t.phase_events.length,6);
    for(const [i,e] of t.phase_events.entries()){
      assert.equal(e.event.sample_index,Math.floor(i/3));assert.equal(e.event.stage,phases[t.scenario][i%3]);
      for(const metric of ['process.rss','process.pss','process.private','process.virtual']){
        const readings=e.observations.filter(x=>x.metric===metric);assert.equal(readings.length,1);const x=readings[0];assert.equal(x.status,'unsupported');assert.ok(x.value==null);assert.ok(x.reason);assert.equal(x.collector,'procfs');assert.equal(x.scope,'adapter_process');
        assert.ok(t.samples[Math.floor(i/3)].observations.some(y=>JSON.stringify(y)===JSON.stringify(x)));
      }
      boundaries++;
    }
    const peaks=t.observations.filter(x=>x.metric==='process.peak_rss');assert.equal(peaks.length,1);assert.equal(peaks[0].status,'unavailable');assert.ok(peaks[0].value==null);assert.ok(peaks[0].reason);
  }
  assert.equal(expected.size,0);assert.equal(checks.size,6);return {measured_samples:samples,boundaries};
}
export function verifyWindowsLifecycle(root){
  const original=read(path.join(root,'run','manifest.json')),coverage={};
  for(const name of ['run','replayed','archived','memory']){
    const dir=path.join(root,name),manifest=read(path.join(dir,'manifest.json')),trials=fs.readdirSync(path.join(dir,'trials')).filter(f=>f.endsWith('.json')).map(f=>read(path.join(dir,'trials',f)));
    coverage[name]=verifyWindowsBundle(manifest,trials,name==='memory',name!=='run'?original:undefined,name==='archived');
  }
  return coverage;
}
if(process.argv[1]&&import.meta.url===pathToFileURL(path.resolve(process.argv[1])).href)console.log(JSON.stringify(verifyWindowsLifecycle(process.argv[2])));
