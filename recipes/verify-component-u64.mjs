// Structural qualification supplements, but never replaces, CLI seal verification.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import {pathToFileURL} from 'node:url';

const stages={compile:['before_compile','compiled','released'],instantiate:['before_instantiate','instantiated','instance_released'],'first-call':['before_first_call','first_call_returned','first_call_released']};
export function verifyComponentU64(manifest,trials) {
  const lock=manifest.lock,o=lock.options;
  assert.ok(['timing','memory'].includes(o.profile));
  assert.equal(o.correctness_only,false);assert.equal(o.operations,1);assert.equal(o.warmup,0);
  assert.equal(o.phase_barriers,o.profile==='memory');assert.equal(lock.archive_tools,true);
  assert.deepEqual(o.scenarios,Object.keys(stages));assert.ok(o.launches>0&&o.samples>0);
  assert.deepEqual(lock.runtime_configurations.map(r=>r.id),['wasmtime','wasmtime-winch']);
  for(const [i,r] of lock.runtime_configurations.entries()) {
    assert.equal(r.description.backend,i?'winch':'cranelift');
    assert.equal(r.description.capabilities.can_component_u64_calls_v1,true);
    assert.equal(r.description.capabilities.can_component_u64_memory_v1,true);
  }
  assert.equal(lock.workloads.length,1);const w=lock.workloads[0];
  assert.equal(w.abi,'component');assert.equal(w.host_profile,'component-u64-v1');
  assert.equal(w.reset,'fresh_instance_per_sample');assert.equal(w.oracle.kind,'exact_u64');
  assert.deepEqual(w.oracle.expected,['8']);assert.deepEqual(w.args,['7']);
  const cells=new Set(lock.runtime_configurations.flatMap(r=>o.scenarios.flatMap(s=>Array.from({length:o.launches},(_,b)=>`${r.id}/${s}/${b}`))));
  const checks=new Set(lock.runtime_configurations.map(r=>r.id));
  let samples=0,boundaries=0;
  assert.equal(trials.length,2+cells.size);
  for(const t of trials) {
    assert.equal(t.status,'ok');assert.equal(t.workload,w.id);
    const check=t.block<0;
    assert.equal(t.samples.length,check?1:o.samples);
    if(check) {assert.equal(t.scenario,'first-call');assert.ok(checks.delete(t.runtime_configuration));}
    else assert.ok(cells.delete(`${t.runtime_configuration}/${t.scenario}/${t.block}`),'duplicate/unknown measured cell');
    for(const [i,s] of t.samples.entries()) {
      assert.equal(s.index,i);assert.equal(s.verified,true);assert.equal(s.warmup,false);
      assert.equal(s.operations,1);assert.equal(s.sample_type,'individual_operation');assert.deepEqual(s.result,['8']);samples++;
    }
    const events=t.phase_events||[],memory=o.profile==='memory'&&!check;
    assert.equal(events.length,memory?3*o.samples:0);
    if(!memory) {assert.equal(t.observations?.length||0,0);for(const s of t.samples)assert.equal(s.observations?.length||0,0);continue;}
    const peak=t.observations.filter(x=>x.metric==='process.peak_rss');assert.equal(peak.length,1);
    assert.equal(peak[0].status,'available');assert.ok(peak[0].value>0);
    assert.equal(peak[0].phase,t.scenario+'/process_lifetime');assert.equal(peak[0].quality,'kernel_accounted_peak');
    for(const [i,e] of events.entries()) {
      const stage=stages[t.scenario][i%3];assert.equal(e.event.stage,stage);assert.equal(e.event.sample_index,Math.floor(i/3));
      assert.ok(e.observations.length>0);
      for(const metric of ['process.rss','process.pss','process.private','process.virtual']) {
        const obs=e.observations.filter(x=>x.metric===metric);assert.equal(obs.length,1);const x=obs[0];
        assert.equal(x.phase,`${t.scenario}/${stage}`);assert.equal(x.unit,'bytes');assert.equal(x.scope,'adapter_process');assert.equal(x.profile,'memory');assert.equal(x.quality,'boundary_snapshot_only');
        if(manifest.host.os==='linux') {assert.equal(x.status,'available');assert.ok(Number.isSafeInteger(x.value)&&x.value>=0);assert.equal(x.collector,'procfs');}
        else {assert.notEqual(x.status,'available');assert.equal(x.value,undefined);assert.ok(x.reason);}
        assert.ok(t.samples[Math.floor(i/3)].observations.some(y=>JSON.stringify(y)===JSON.stringify(x)),'exact sample attachment');
      }
      boundaries++;
    }
  }
  assert.equal(cells.size,0);assert.equal(checks.size,0);
  return {os:manifest.host.os,arch:manifest.host.arch,profile:o.profile,trials:trials.length,samples,boundaries};
}
if(process.argv[1]&&import.meta.url===pathToFileURL(path.resolve(process.argv[1])).href) {
  const root=process.argv[2],read=file=>JSON.parse(fs.readFileSync(file,'utf8'));
  console.log(JSON.stringify(verifyComponentU64(read(path.join(root,'manifest.json')),fs.readdirSync(path.join(root,'trials')).map(f=>read(path.join(root,'trials',f))))));
}
