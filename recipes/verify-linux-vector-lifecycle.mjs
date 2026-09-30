import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import {pathToFileURL} from 'node:url';
const digest=s=>crypto.createHash('sha256').update(s).digest('hex');
const read=p=>JSON.parse(fs.readFileSync(p));
const stages={instantiate:['before_instantiate','instantiated','instance_released'],'first-call':['before_first_call','first_call_returned','first_call_released']};
const runtimes=['wago','wazero','wazero-interpreter','v8','wasmtime','wasmtime-winch'];
const sequenceDenominator='ordered_calls_with_intercase_input_and_verification_excluding_last_oracle_release_and_barriers';
const goMetrics=['host.alloc.bytes','host.alloc.count','host.heap.start','host.heap.end','host.gc.cycles','host.gc.forced_cycles','host.gc.pause_time'];
export function createVectorLifecycleSuite(root) {
  const workloads=['vector-lifecycle','vector-initialization'].map(file=>{
    const artifact=path.join(root,file+'.wasm');
    return {schema:1,id:'vectors/'+file,family:'mechanisms',artifact,sha256:digest(fs.readFileSync(artifact)),abi:'core',export:'benchmark',...(file==='vector-initialization'?{initialize:'initialize'}:{}),reset:'fresh_instance_per_sample',oracle:{kind:'exact_vectors'},vectors:{input_offset:32,output_offset:16,output_len:1,mod:3,cases:[{len:0,out:'ab'},{len:7,out:'ab'}]},vector_byte_budget:1024,work_unit:'vector_sequence',units_per_invocation:1,license:'Apache-2.0',source:file+'.wat sha256:'+digest(fs.readFileSync(path.join(root,file+'.wat')))};
  });
  fs.writeFileSync(path.join(root,'suite.json'),JSON.stringify(workloads,null,2),{flag:'wx'});
}
// CLI verifies bundle seals before this semantic coverage gate. No cgroup claim.
export function verifyVectorLifecycleCoverage(root,arch,originalRoot,restored=false) {
  assert.ok(['arm64','amd64'].includes(arch));
  const manifest=read(path.join(root,'manifest.json')),lock=manifest.lock,o=lock.options;
  assert.equal(manifest.host.os,'linux');assert.equal(manifest.host.arch,arch);
  assert.equal(o.correctness_only,false);assert.equal(o.profile,'memory');assert.equal(o.phase_barriers,true);
  assert.deepEqual(o.scenarios,Object.keys(stages));assert.equal(o.launches,1);assert.equal(o.samples,2);assert.equal(o.operations,1);assert.equal(o.warmup,0);assert.equal(lock.archive_tools,true);
  assert.deepEqual(lock.runtime_configurations.map(r=>r.id),runtimes);
  for(const r of lock.runtime_configurations) {
    assert.ok(r.description.effective_configuration.vector_instantiate_phases_policy);
    assert.ok(r.description.effective_configuration.vector_first_call_phases_policy.includes('intercase input writes and oracle checks'));
    for(const scenario of Object.keys(stages))assert.equal(r.description.capabilities['can_vector_'+scenario+'_phases'],true);
  }
  assert.deepEqual(lock.workloads.map(w=>w.id),['vectors/vector-lifecycle','vectors/vector-initialization']);
  for(const w of lock.workloads) {
    assert.equal(w.abi,'core');assert.equal(w.reset,'fresh_instance_per_sample');assert.equal(w.oracle.kind,'exact_vectors');
    assert.deepEqual(w.vectors,{input_offset:32,output_offset:16,output_len:1,mod:3,cases:[{len:0,out:'ab'},{len:7,out:'ab'}]});
    assert.equal(w.initialize||'',w.id.endsWith('initialization')?'initialize':'');
  }
  if(originalRoot) {
    const old=read(path.join(originalRoot,'manifest.json'));
    assert.deepEqual(manifest.host,old.host);assert.equal(lock.runner_sha256,old.lock.runner_sha256);
    for(const [i,w] of lock.workloads.entries()) {assert.equal(w.sha256,old.lock.workloads[i].sha256);assert.deepEqual(w.vectors,old.lock.workloads[i].vectors);assert.equal(w.initialize,old.lock.workloads[i].initialize);}
    for(const [i,r] of lock.runtime_configurations.entries()) {
      const prior=old.lock.runtime_configurations[i];
      if(restored)assert.notEqual(r.command[0],prior.command[0],'archived adapter must be relocated');
      assert.deepEqual(Object.values(r.file_sha256).sort(),Object.values(prior.file_sha256).sort());
      assert.deepEqual(r.description.effective_configuration,prior.description.effective_configuration);
    }
  }
  const expected=new Set(runtimes.flatMap(r=>lock.workloads.flatMap(w=>Object.keys(stages).map(s=>r+'/'+w.id+'/'+s)))),checks=new Set();
  const trials=fs.readdirSync(path.join(root,'trials')).filter(f=>f.endsWith('.json')).map(f=>read(path.join(root,'trials',f)));
  assert.equal(trials.length,36);let samples=0,boundaries=0;
  for(const t of trials) {
    assert.equal(t.status,'ok');assert.ok(runtimes.includes(t.runtime_configuration));assert.ok(lock.workloads.some(w=>w.id===t.workload));assert.equal(t.profile,'memory');
    assert.equal(t.samples.length,2);
    for(const [i,s] of t.samples.entries()) {
      assert.equal(s.index,i);assert.equal(s.verified,true);assert.equal(s.warmup,false);assert.equal(s.operations,1);assert.equal(s.sample_type,t.scenario==='first-call'?'sequence_call_sum':'individual_operation');samples++;
    }
    if(t.block<0) {assert.equal(t.scenario,'first-call');assert.equal(t.phase_events?.length||0,0);const key=t.runtime_configuration+'/'+t.workload;assert.ok(!checks.has(key));checks.add(key);continue;}
    assert.equal(t.block,0);assert.ok(expected.delete(t.runtime_configuration+'/'+t.workload+'/'+t.scenario));assert.equal(t.phase_events.length,6);
    const peak=t.observations.filter(x=>x.metric==='process.peak_rss');assert.equal(peak.length,1);assert.equal(peak[0].status,'available');assert.ok(Number.isSafeInteger(peak[0].value)&&peak[0].value>0);assert.equal(peak[0].unit,'bytes');assert.equal(peak[0].scope,'adapter_process');assert.equal(peak[0].phase,t.scenario+'/process_lifetime');assert.equal(peak[0].quality,'kernel_accounted_peak');assert.equal(peak[0].collector,'wait4_rusage');
    for(const [i,e] of t.phase_events.entries()) {
      const stage=stages[t.scenario][i%3];assert.equal(e.event.stage,stage);assert.equal(e.event.sample_index,Math.floor(i/3));
      for(const metric of ['process.rss','process.pss','process.private','process.virtual']) {
        const readings=e.observations.filter(x=>x.metric===metric);assert.equal(readings.length,1);const x=readings[0];
        assert.equal(x.status,'available');assert.ok(Number.isSafeInteger(x.value)&&x.value>=0);assert.equal(x.unit,'bytes');assert.equal(x.scope,'adapter_process');assert.equal(x.phase,t.scenario+'/'+stage);assert.equal(x.collector,'procfs');assert.equal(x.quality,'boundary_snapshot_only');assert.equal(x.profile,'memory');
        assert.ok(t.samples[Math.floor(i/3)].observations.some(y=>JSON.stringify(y)===JSON.stringify(x)),'snapshot must belong to exact sample');
      }
      boundaries++;
    }
    for(const s of t.samples) {
      const go=t.runtime_configuration==='wago'||t.runtime_configuration.startsWith('wazero'),js=t.runtime_configuration==='v8';
      if(go||js) {
        const scope=go?'adapter_process_go_heap':'adapter_process_v8_heap',obs=s.observations.filter(x=>x.scope===scope);
        assert.deepEqual(obs.map(x=>x.metric).sort(),(go?goMetrics:['host.js_heap.start','host.js_heap.end']).slice().sort());
        for(const x of obs){assert.equal(x.status,'available');assert.ok(Number.isSafeInteger(x.value)&&x.value>=0);assert.equal(x.phase,t.scenario==='first-call'?'first-call/vector_sequence_call_window':'instantiate/api_window');assert.equal(x.normalization_denominator,t.scenario==='first-call'?sequenceDenominator:go?'instantiation_including_start_excluding_initialization_verification_release':'process_snapshot');}
      }
      const logical=s.observations.filter(x=>x.metric==='guest.memory.logical');assert.equal(logical.length,1);assert.equal(logical[0].status,'available');assert.equal(logical[0].value,65536);assert.equal(logical[0].scope,'guest_linear_memory');assert.equal(logical[0].phase,t.scenario+'/vector_verified');
    }
  }
  assert.equal(checks.size,12);assert.equal(expected.size,0);assert.equal(samples,72);assert.equal(boundaries,144);
  return {arch,trials:36,samples,boundaries};
}
export function verifyCgroupVectorLifecycleCoverage(root,arch,originalRoot,restored=false) {
  const coverage=verifyVectorLifecycleCoverage(root,arch,originalRoot,restored),m=read(path.join(root,'manifest.json'));
  assert.deepEqual(m.lock.options.resources,{cgroup_parent:'/sys/fs/cgroup/wasmbench-workers',memory_max_bytes:536870912,disable_swap:true,cpu_quota_us_per_100000:100000,pids_max:128});
  const paths=new Set();let peaks=0,cpuReadings=0;
  for(const file of fs.readdirSync(path.join(root,'trials')).filter(f=>f.endsWith('.json'))) {
    const t=read(path.join(root,'trials',file)),isolation=t.isolation;
    assert.equal(isolation.mode,'cgroup_v2_at_spawn');assert.ok(isolation.path.startsWith(m.lock.options.resources.cgroup_parent+'/wasmbench-'));assert.ok(!paths.has(isolation.path));paths.add(isolation.path);
    for(const v of [isolation.verification,isolation.final_verification]) {
      assert.equal(v.version,'requested-cgroup-leaf-readback-v2');assert.equal(v.status,'verified');
      for(const [k,value] of Object.entries({'memory.max':'536870912','memory.swap.max':'0','memory.oom.group':'1','cpu.max':'100000 100000','pids.max':'128'}))assert.equal(v.effective[k],value);
    }
    assert.equal(isolation.verification.stage,'before_spawn');assert.equal(isolation.final_verification.stage,'response_end_before_cleanup');
    if(t.block<0)continue;
    for(const [i,s] of t.samples.entries()) {
      const phase=t.scenario+'/barrier_window';
      const obs=s.observations.filter(o=>o.metric==='cgroup.memory.phase_peak');assert.equal(obs.length,1);const peak=obs[0];
      assert.equal(peak.status,'available');assert.ok(Number.isSafeInteger(peak.value)&&peak.value>0);assert.equal(peak.unit,'bytes');assert.equal(peak.scope,'adapter_cgroup');assert.equal(peak.phase,phase);assert.equal(peak.collector,'cgroup_v2_same_fd');assert.equal(peak.quality,'kernel_accounted_peak');assert.equal(peak.normalization_denominator,'diagnostic_operation_including_barrier_transport');
      assert.ok(t.phase_events[i*3+1].observations.some(o=>JSON.stringify(o)===JSON.stringify(peak)));peaks++;
      for(const metric of ['time.cpu.total','time.cpu.user','time.cpu.system']) {
        const cpu=s.observations.filter(o=>o.metric===metric);assert.equal(cpu.length,1);const o=cpu[0];
        assert.equal(o.status,'available');assert.ok(Number.isSafeInteger(o.value)&&o.value>=0);assert.equal(o.unit,'ns');assert.equal(o.scope,'adapter_cgroup_process_tree');assert.equal(o.phase,phase);assert.equal(o.collector,'cgroup_v2_cpu.stat');assert.equal(o.quality,'kernel_accounted_delta');assert.equal(o.normalization_denominator,'diagnostic_operation_including_barrier_transport');
        assert.ok(t.phase_events[i*3+1].observations.some(x=>JSON.stringify(x)===JSON.stringify(o)));cpuReadings++;
      }
    }
  }
  assert.equal(peaks,48);assert.equal(cpuReadings,144);return {...coverage,peaks,cpu_readings:cpuReadings};
}
if(process.argv[1]&&import.meta.url===pathToFileURL(path.resolve(process.argv[1])).href) {
  if(process.argv[2]==='suite')createVectorLifecycleSuite(process.argv[3]);
  else if(process.argv[2]==='cgroup')console.log(JSON.stringify(verifyCgroupVectorLifecycleCoverage(process.argv[3],process.argv[4],process.argv[5],process.argv[6]==='archived')));
  else console.log(JSON.stringify(verifyVectorLifecycleCoverage(process.argv[2],process.argv[3],process.argv[4],process.argv[5]==='archived')));
}
