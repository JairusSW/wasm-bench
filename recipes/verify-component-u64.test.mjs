import test from 'node:test';
import assert from 'node:assert/strict';
import {verifyComponentU64} from './verify-component-u64.mjs';

function fixture(profile='memory',os='linux') {
  const stages={compile:['before_compile','compiled','released'],instantiate:['before_instantiate','instantiated','instance_released'],'first-call':['before_first_call','first_call_returned','first_call_released']};
  const configurations=['wasmtime','wasmtime-winch'].map((id,i)=>({id,description:{backend:i?'winch':'cranelift',capabilities:{can_component_u64_calls_v1:true,can_component_u64_memory_v1:true}}}));
  const manifest={host:{os,arch:'arm64'},lock:{archive_tools:true,options:{profile,correctness_only:false,operations:1,warmup:0,phase_barriers:profile==='memory',scenarios:Object.keys(stages),launches:1,samples:1},runtime_configurations:configurations,workloads:[{id:'fixture',abi:'component',host_profile:'component-u64-v1',reset:'fresh_instance_per_sample',args:['7'],oracle:{kind:'exact_u64',expected:['8']}}]}};
  const trials=[];
  for(const {id} of configurations)for(const scenario of ['check',...Object.keys(stages)]) {
    const check=scenario==='check',s=check?'first-call':scenario;
    const sample={index:0,verified:true,warmup:false,operations:1,sample_type:'individual_operation',result:['8']};
    const trial={status:'ok',workload:'fixture',runtime_configuration:id,scenario:s,block:check?-1:0,samples:[sample],observations:[],phase_events:[]};
    if(profile==='memory'&&!check) {
      trial.observations.push({metric:'process.peak_rss',status:'available',value:4096,phase:s+'/process_lifetime',quality:'kernel_accounted_peak'});
      sample.observations=[];
      for(const stage of stages[s]) {
        const observations=['process.rss','process.pss','process.private','process.virtual'].map(metric=>({metric,phase:s+'/'+stage,unit:'bytes',scope:'adapter_process',profile:'memory',quality:'boundary_snapshot_only',...(os==='linux'?{status:'available',value:0,collector:'procfs'}:{status:'unsupported',reason:'procfs requires Linux'})}));
        trial.phase_events.push({event:{sample_index:0,stage},observations});sample.observations.push(...observations);
      }
    }
    trials.push(trial);
  }
  return {manifest,trials};
}
test('typed component qualification preserves real zero and explicit platform gaps',()=>{
  for(const os of ['linux','darwin'])for(const profile of ['timing','memory']) {
    const f=fixture(profile,os),result=verifyComponentU64(f.manifest,f.trials);
    assert.equal(result.samples,8);assert.equal(result.boundaries,profile==='memory'?18:0);
  }
});
for(const [name,mutate] of Object.entries({
  capability:f=>delete f.manifest.lock.runtime_configurations[0].description.capabilities.can_component_u64_memory_v1,
  oracle:f=>f.trials[1].samples[0].result=['9'],
  duplicate:f=>f.trials[2]=structuredClone(f.trials[1]),
  missing:f=>f.trials.pop(),
  stage:f=>f.trials[1].phase_events[1].event.stage='released',
  sample:f=>f.trials[1].phase_events[0].event.sample_index=1,
  attachment:f=>f.trials[1].samples[0].observations=[],
  rssScope:f=>f.trials[1].observations[0].phase='compile/compiled',
  procfsGap:f=>f.trials[1].phase_events[0].observations[0].status='unsupported',
  checkInstrumented:f=>f.trials[0].samples[0].observations=[{}],
}))test('typed component gate rejects '+name,()=>{
  const f=fixture();mutate(f);assert.throws(()=>verifyComponentU64(f.manifest,f.trials));
});
test('non-Linux collector gap cannot fabricate zero',()=>{
  const f=fixture('memory','darwin');f.trials[1].phase_events[0].observations[0].value=0;
  assert.throws(()=>verifyComponentU64(f.manifest,f.trials));
});
