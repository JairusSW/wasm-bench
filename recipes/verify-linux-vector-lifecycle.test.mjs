import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {createVectorLifecycleSuite,verifyVectorLifecycleCoverage,verifyCgroupVectorLifecycleCoverage} from './verify-linux-vector-lifecycle.mjs';
const stages={instantiate:['before_instantiate','instantiated','instance_released'],'first-call':['before_first_call','first_call_returned','first_call_released']};
const goMetrics=['host.alloc.bytes','host.alloc.count','host.heap.start','host.heap.end','host.gc.cycles','host.gc.forced_cycles','host.gc.pause_time'];
function fixture(t) {
  const root=fs.mkdtempSync(path.join(os.tmpdir(),'wasmbench-vector-coverage-'));t.after(()=>fs.rmSync(root,{recursive:true,force:true}));fs.mkdirSync(path.join(root,'trials'));
  for(const name of ['vector-lifecycle','vector-initialization'])for(const ext of ['wasm','wat'])fs.copyFileSync(new URL('../corpus/testdata/'+name+'.'+ext,import.meta.url),path.join(root,name+'.'+ext));
  createVectorLifecycleSuite(root);const workloads=JSON.parse(fs.readFileSync(path.join(root,'suite.json')));
  const configurations=['wago','wazero','wazero-interpreter','v8','wasmtime','wasmtime-winch'].map(id=>({id,command:['/installed/'+id],file_sha256:{['/installed/'+id]:'same'},description:{capabilities:{can_vector_instantiate_phases:true,'can_vector_first-call_phases':true},effective_configuration:{vector_instantiate_phases_policy:'explicit',vector_first_call_phases_policy:'intercase input writes and oracle checks'}}}));
  const manifest={host:{os:'linux',arch:'arm64'},lock:{runner_sha256:'runner',archive_tools:true,options:{correctness_only:false,profile:'memory',phase_barriers:true,scenarios:Object.keys(stages),launches:1,samples:2,operations:1,warmup:0},workloads,runtime_configurations:configurations}};
  const trials=[],sample=(index,scenario)=>({index,verified:true,warmup:false,operations:1,sample_type:scenario==='first-call'?'sequence_call_sum':'individual_operation',observations:[]});
  for(const r of configurations)for(const w of workloads) {
    trials.push({runtime_configuration:r.id,workload:w.id,scenario:'first-call',block:-1,status:'ok',profile:'memory',samples:[sample(0,'first-call'),sample(1,'first-call')]});
    for(const scenario of Object.keys(stages)) {
      const samples=[sample(0,scenario),sample(1,scenario)],events=[];
      for(const s of samples) {
        const go=r.id==='wago'||r.id.startsWith('wazero'),js=r.id==='v8';
        if(go||js)s.observations.push(...(go?goMetrics:['host.js_heap.start','host.js_heap.end']).map(metric=>({metric,status:'available',value:0,scope:go?'adapter_process_go_heap':'adapter_process_v8_heap',phase:scenario==='first-call'?'first-call/vector_sequence_call_window':'instantiate/api_window',normalization_denominator:scenario==='first-call'?'ordered_calls_with_intercase_input_and_verification_excluding_last_oracle_release_and_barriers':go?'instantiation_including_start_excluding_initialization_verification_release':'process_snapshot'})));
        s.observations.push({metric:'guest.memory.logical',status:'available',value:65536,scope:'guest_linear_memory',phase:scenario+'/vector_verified'});
        for(const stage of stages[scenario]) {
          const observations=['process.rss','process.pss','process.private','process.virtual'].map(metric=>({metric,status:'available',value:metric==='process.private'?0:4096,unit:'bytes',scope:'adapter_process',phase:scenario+'/'+stage,collector:'procfs',quality:'boundary_snapshot_only',profile:'memory'}));s.observations.push(...observations);events.push({event:{sample_index:s.index,stage},observations});
        }
      }
      trials.push({runtime_configuration:r.id,workload:w.id,scenario,block:0,status:'ok',profile:'memory',samples,phase_events:events,observations:[{metric:'process.peak_rss',status:'available',value:8192,unit:'bytes',scope:'adapter_process',phase:scenario+'/process_lifetime',quality:'kernel_accounted_peak',collector:'wait4_rusage'}]});
    }
  }
  const save=()=>{fs.writeFileSync(path.join(root,'manifest.json'),JSON.stringify(manifest));trials.forEach((x,i)=>fs.writeFileSync(path.join(root,'trials',i+'.json'),JSON.stringify(x)));};return {root,manifest,trials,save};
}
test('complete vector coverage retains zero private residency and zero allocator counts',t=>{const f=fixture(t);f.save();assert.deepEqual(verifyVectorLifecycleCoverage(f.root,'arm64'),{arch:'arm64',trials:36,samples:72,boundaries:144});});
for(const [name,change] of Object.entries({
  platform:f=>f.manifest.host.os='darwin',
  capability:f=>delete f.manifest.lock.runtime_configurations[0].description.capabilities['can_vector_first-call_phases'],
  policy:f=>f.manifest.lock.runtime_configurations[0].description.effective_configuration.vector_first_call_phases_policy='API only',
  oracle:f=>f.manifest.lock.workloads[0].vectors.cases[1].out='ac',
  initializer:f=>delete f.manifest.lock.workloads[1].initialize,
  verification:f=>f.trials[1].samples[0].verified=false,
  sampleType:f=>f.trials[2].samples[0].sample_type='individual_operation',
  duplicate:f=>f.trials[2].scenario='instantiate',
  boundary:f=>f.trials[1].phase_events.pop(),
  order:f=>f.trials[1].phase_events[0].event.stage='instantiated',
  identity:f=>f.trials[1].phase_events[0].event.sample_index=1,
  unavailable:f=>f.trials[1].phase_events[0].observations[0].status='unavailable',
  scope:f=>f.trials[1].phase_events[0].observations[0].scope='whole_host',
  quality:f=>f.trials[1].phase_events[0].observations[0].quality='kernel_accounted_peak',
  attachment:f=>f.trials[1].samples[0].observations=[],
  peak:f=>f.trials[1].observations[0].phase='instantiate/api_window',
  failure:f=>f.trials[1].status='incorrect_result',
  allocator:f=>f.trials[2].samples[0].observations[0].normalization_denominator='API_only',
  duplicateMetric:f=>f.trials[2].samples[0].observations[0].metric='host.alloc.count',
  logical:f=>f.trials[1].samples[0].observations.find(o=>o.metric==='guest.memory.logical').value=0,
}))test('vector coverage rejects '+name,t=>{const f=fixture(t);change(f);f.save();assert.throws(()=>verifyVectorLifecycleCoverage(f.root,'arm64'));});
test('suite generation refuses to overwrite evidence',t=>{const f=fixture(t);assert.throws(()=>createVectorLifecycleSuite(f.root));});
test('archival requires relocated adapters and unchanged identities',t=>{const a=fixture(t),b=fixture(t);a.save();b.save();assert.throws(()=>verifyVectorLifecycleCoverage(b.root,'arm64',a.root,true));for(const r of b.manifest.lock.runtime_configurations){r.command=['/restored/'+r.id];r.file_sha256={['/restored/'+r.id]:'same'};}b.save();assert.doesNotThrow(()=>verifyVectorLifecycleCoverage(b.root,'arm64',a.root,true));b.manifest.lock.workloads[0].sha256='changed';b.save();assert.throws(()=>verifyVectorLifecycleCoverage(b.root,'arm64',a.root,true));});
function cgroupFixture(t) {
  const f=fixture(t),parent='/sys/fs/cgroup/wasmbench-workers';
  f.manifest.lock.options.resources={cgroup_parent:parent,memory_max_bytes:536870912,disable_swap:true,cpu_quota_us_per_100000:100000,pids_max:128};
  for(const [i,trial] of f.trials.entries()) {
    const effective={'memory.max':'536870912','memory.swap.max':'0','memory.oom.group':'1','cpu.max':'100000 100000','pids.max':'128'};
    trial.isolation={mode:'cgroup_v2_at_spawn',path:parent+'/wasmbench-'+i,verification:{version:'requested-cgroup-leaf-readback-v2',stage:'before_spawn',status:'verified',effective:{...effective}},final_verification:{version:'requested-cgroup-leaf-readback-v2',stage:'response_end_before_cleanup',status:'verified',effective:{...effective}}};
    if(trial.block<0)continue;
    for(const [index,s] of trial.samples.entries()) {
      const common={status:'available',phase:trial.scenario+'/barrier_window',normalization_denominator:'diagnostic_operation_including_barrier_transport'};
      const observations=[{...common,metric:'cgroup.memory.phase_peak',value:4096,unit:'bytes',scope:'adapter_cgroup',collector:'cgroup_v2_same_fd',quality:'kernel_accounted_peak'},...['time.cpu.total','time.cpu.user','time.cpu.system'].map(metric=>({...common,metric,value:0,unit:'ns',scope:'adapter_cgroup_process_tree',collector:'cgroup_v2_cpu.stat',quality:'kernel_accounted_delta'}))];
      s.observations.push(...observations);trial.phase_events[index*3+1].observations.push(...observations);
    }
  }
  return f;
}
test('cgroup qualification requires exact leaf budgets, phase peaks and process-tree CPU',t=>{const f=cgroupFixture(t);f.save();assert.deepEqual(verifyCgroupVectorLifecycleCoverage(f.root,'arm64'),{arch:'arm64',trials:36,samples:72,boundaries:144,peaks:48,cpu_readings:144});});
test('cgroup replay retains workload/tool identities and archival relocation',t=>{
  const a=cgroupFixture(t),b=cgroupFixture(t);a.save();b.save();
  assert.doesNotThrow(()=>verifyCgroupVectorLifecycleCoverage(b.root,'arm64',a.root));
  assert.throws(()=>verifyCgroupVectorLifecycleCoverage(b.root,'arm64',a.root,true));
  for(const r of b.manifest.lock.runtime_configurations){r.command=['/restored/'+r.id];r.file_sha256={['/restored/'+r.id]:'same'};}b.save();
  assert.doesNotThrow(()=>verifyCgroupVectorLifecycleCoverage(b.root,'arm64',a.root,true));
  b.manifest.lock.runner_sha256='different';b.save();assert.throws(()=>verifyCgroupVectorLifecycleCoverage(b.root,'arm64',a.root,true));
});
for(const [name,change] of Object.entries({
  budgets:f=>f.manifest.lock.options.resources.disable_swap=false,
  placement:f=>f.trials[1].isolation.mode='move_after_spawn',
  reusedLeaf:f=>f.trials[2].isolation.path=f.trials[1].isolation.path,
  endingReadback:f=>f.trials[1].isolation.final_verification.effective['memory.max']='max',
  deniedPeak:f=>f.trials[1].samples[0].observations.find(o=>o.metric==='cgroup.memory.phase_peak').status='permission_denied',
  globalPeak:f=>f.trials[1].samples[0].observations.find(o=>o.metric==='cgroup.memory.phase_peak').collector='cgroup_v2_global_peak',
  peakScope:f=>f.trials[1].samples[0].observations.find(o=>o.metric==='cgroup.memory.phase_peak').scope='adapter_process',
  cpuScope:f=>f.trials[1].samples[0].observations.find(o=>o.metric==='time.cpu.total').scope='caller_thread',
  attachment:f=>f.trials[1].phase_events[1].observations=f.trials[1].phase_events[1].observations.filter(o=>o.metric!=='cgroup.memory.phase_peak'),
}))test('cgroup qualification rejects '+name,t=>{const f=cgroupFixture(t);change(f);f.save();assert.throws(()=>verifyCgroupVectorLifecycleCoverage(f.root,'arm64'));});
