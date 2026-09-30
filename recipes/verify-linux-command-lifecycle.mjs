import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import {pathToFileURL} from 'node:url';
const digest=s=>crypto.createHash('sha256').update(s).digest('hex');
const read=p=>JSON.parse(fs.readFileSync(p));
const stages={instantiate:['before_instantiate','instantiated','instance_released'],'first-call':['before_first_call','first_call_returned','first_call_released']};
const runtimes=['wazero','wazero-interpreter','wasmtime','wasmtime-winch'];
export function createCommandLifecycleSuite(root) {
  const workloads=['wasi-command','emscripten'].map(abi=>{
    const file=abi==='emscripten'?'emscripten-command':'command',artifact=path.join(root,file+'.wasm');
    return {schema:1,id:'commands/'+abi,family:'applications',artifact,sha256:digest(fs.readFileSync(artifact)),abi,export:abi==='emscripten'?'main':'_start',host_profile:abi==='emscripten'?'emscripten-stdio-v1':'wasi-preview1-readonly-v1',reset:'fresh_instance_per_sample',oracle:{kind:'exact_command'},work_unit:'command',units_per_invocation:1,license:'Apache-2.0',source:file+'.wat sha256:'+digest(fs.readFileSync(path.join(root,file+'.wat'))),command:{argv:abi==='emscripten'?['fixture']:['test','arg'],stdin:Buffer.from(abi==='emscripten'?'':'abc').toString('base64'),exit_code:abi==='emscripten'?0:7,stdout_sha256:digest(abi==='emscripten'?'':'abc'),stderr_sha256:digest(abi==='emscripten'?'':'err'),output_limit_bytes:128}};
  });
  fs.writeFileSync(path.join(root,'suite.json'),JSON.stringify(workloads,null,2),{flag:'wx'});
}
// CLI checks seals first. This gate checks procfs semantics, not isolation.
export function verifyCommandLifecycleCoverage(root,arch,originalRoot,restored=false) {
  assert.ok(['arm64','amd64'].includes(arch));
  const manifest=read(path.join(root,'manifest.json')),lock=manifest.lock,o=lock.options;
  assert.equal(manifest.host.os,'linux');assert.equal(manifest.host.arch,arch);
  assert.equal(o.correctness_only,false);assert.equal(o.profile,'memory');assert.equal(o.phase_barriers,true);
  assert.deepEqual(o.scenarios,Object.keys(stages));assert.equal(o.launches,1);assert.equal(o.samples,2);assert.equal(o.operations,1);assert.equal(o.warmup,0);assert.equal(lock.archive_tools,true);
  assert.deepEqual(lock.runtime_configurations.map(r=>r.id),runtimes);
  for(const r of lock.runtime_configurations) {
    assert.ok(r.description.effective_configuration.command_lifecycle_phases_policy);
    for(const scenario of Object.keys(stages))assert.equal(r.description.capabilities['can_command_'+scenario+'_phases'],true);
  }
  assert.deepEqual(lock.workloads.map(w=>w.abi),['wasi-command','emscripten']);
  for(const w of lock.workloads) {assert.equal(w.reset,'fresh_instance_per_sample');assert.equal(w.oracle.kind,'exact_command');}
  if(originalRoot) {
    const old=read(path.join(originalRoot,'manifest.json'));
    assert.deepEqual(manifest.host,old.host);assert.equal(lock.runner_sha256,old.lock.runner_sha256);
    for(const [i,w] of lock.workloads.entries()) {assert.equal(w.sha256,old.lock.workloads[i].sha256);assert.deepEqual(w.command,old.lock.workloads[i].command);}
    for(const [i,r] of lock.runtime_configurations.entries()) {
      const prior=old.lock.runtime_configurations[i];
      if(restored)assert.notEqual(r.command[0],prior.command[0],'archived adapter must be relocated');
      assert.deepEqual(Object.values(r.file_sha256).sort(),Object.values(prior.file_sha256).sort());
      assert.deepEqual(r.description.effective_configuration,prior.description.effective_configuration);
    }
  }
  const expected=new Set(runtimes.flatMap(r=>lock.workloads.flatMap(w=>Object.keys(stages).map(s=>r+'/'+w.id+'/'+s)))),checks=new Set();
  const trials=fs.readdirSync(path.join(root,'trials')).filter(f=>f.endsWith('.json')).map(f=>read(path.join(root,'trials',f)));
  assert.equal(trials.length,24);let samples=0,boundaries=0;
  for(const t of trials) {
    assert.equal(t.status,'ok');assert.ok(runtimes.includes(t.runtime_configuration));
    const w=lock.workloads.find(w=>w.id===t.workload);assert.ok(w);assert.equal(t.profile,'memory');
    assert.equal(t.samples.length,2);
    for(const [i,s] of t.samples.entries()) {
      assert.equal(s.index,i);assert.equal(s.verified,true);assert.equal(s.warmup,false);assert.equal(s.operations,1);assert.equal(s.sample_type,'individual_operation');
      for(const key of ['exit_code','stdout_sha256','stderr_sha256'])assert.equal(s.command_result[key],w.command[key]);samples++;
    }
    if(t.block<0) {assert.equal(t.scenario,'first-call');assert.equal(t.phase_events?.length||0,0);const key=t.runtime_configuration+'/'+t.workload;assert.ok(!checks.has(key));checks.add(key);continue;}
    assert.equal(t.block,0);assert.ok(expected.delete(t.runtime_configuration+'/'+t.workload+'/'+t.scenario));assert.equal(t.phase_events.length,6);
    const peak=t.observations.filter(x=>x.metric==='process.peak_rss');assert.equal(peak.length,1);assert.equal(peak[0].status,'available');assert.ok(peak[0].value>0);assert.equal(peak[0].phase,t.scenario+'/process_lifetime');assert.equal(peak[0].quality,'kernel_accounted_peak');assert.equal(peak[0].collector,'wait4_rusage');
    for(const [i,e] of t.phase_events.entries()) {
      const stage=stages[t.scenario][i%3];assert.equal(e.event.stage,stage);assert.equal(e.event.sample_index,Math.floor(i/3));
      for(const metric of ['process.rss','process.pss','process.private','process.virtual']) {
        const readings=e.observations.filter(x=>x.metric===metric);assert.equal(readings.length,1);const x=readings[0];
        assert.equal(x.status,'available');assert.ok(Number.isSafeInteger(x.value)&&x.value>=0);assert.equal(x.unit,'bytes');assert.equal(x.scope,'adapter_process');assert.equal(x.phase,t.scenario+'/'+stage);assert.equal(x.collector,'procfs');assert.equal(x.quality,'boundary_snapshot_only');assert.equal(x.profile,'memory');
        assert.ok(t.samples[Math.floor(i/3)].observations.some(y=>JSON.stringify(y)===JSON.stringify(x)),'snapshot must belong to exact sample');
      }
      boundaries++;
    }
    if(t.runtime_configuration.startsWith('wazero'))for(const s of t.samples) {
      const observations=s.observations.filter(x=>x.scope==='adapter_process_go_heap');assert.equal(observations.length,7);
      for(const x of observations){assert.equal(x.phase,t.scenario+'/api_window');assert.equal(x.normalization_denominator,'operation_excluding_verification_release_and_barriers');}
    }
  }
  assert.equal(checks.size,8);assert.equal(expected.size,0);assert.equal(samples,48);assert.equal(boundaries,96);
  return {arch,trials:24,samples,boundaries};
}
if(process.argv[1]&&import.meta.url===pathToFileURL(path.resolve(process.argv[1])).href) {
  if(process.argv[2]==='suite')createCommandLifecycleSuite(process.argv[3]);
  else console.log(JSON.stringify(verifyCommandLifecycleCoverage(process.argv[2],process.argv[3],process.argv[4],process.argv[5]==='archived')));
}
