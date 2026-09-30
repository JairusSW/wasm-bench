import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {verifyCommandLifecycleCoverage} from './verify-linux-command-lifecycle.mjs';
const stages={instantiate:['before_instantiate','instantiated','instance_released'],'first-call':['before_first_call','first_call_returned','first_call_released']};
function fixture(t) {
  const root=fs.mkdtempSync(path.join(os.tmpdir(),'wasmbench-command-coverage-'));t.after(()=>fs.rmSync(root,{recursive:true,force:true}));fs.mkdirSync(path.join(root,'trials'));
  const workloads=['wasi-command','emscripten'].map(abi=>({id:abi,abi,reset:'fresh_instance_per_sample',oracle:{kind:'exact_command'},sha256:'wasm',command:{exit_code:0,stdout_sha256:'out',stderr_sha256:'err'}}));
  const configurations=['wazero','wazero-interpreter','wasmtime','wasmtime-winch'].map(id=>({id,command:['/installed/'+id],file_sha256:{['/installed/'+id]:'same'},description:{capabilities:{can_command_instantiate_phases:true,'can_command_first-call_phases':true},effective_configuration:{command_lifecycle_phases_policy:'explicit'}}}));
  const manifest={host:{os:'linux',arch:'arm64'},lock:{runner_sha256:'runner',archive_tools:true,options:{correctness_only:false,profile:'memory',phase_barriers:true,scenarios:Object.keys(stages),launches:1,samples:2,operations:1,warmup:0},workloads,runtime_configurations:configurations}};
  const trials=[],sample=index=>({index,verified:true,warmup:false,operations:1,sample_type:'individual_operation',command_result:{exit_code:0,stdout_sha256:'out',stderr_sha256:'err'},observations:[]});
  for(const r of configurations)for(const w of workloads) {
    trials.push({runtime_configuration:r.id,workload:w.id,scenario:'first-call',block:-1,status:'ok',profile:'memory',samples:[sample(0),sample(1)]});
    for(const scenario of Object.keys(stages)) {
      const samples=[sample(0),sample(1)],events=[];
      for(const s of samples) {
        if(r.id.startsWith('wazero'))s.observations.push(...Array.from({length:7},()=>({scope:'adapter_process_go_heap',phase:scenario+'/api_window',normalization_denominator:'operation_excluding_verification_release_and_barriers'})));
        for(const stage of stages[scenario]) {
          const observations=['process.rss','process.pss','process.private','process.virtual'].map(metric=>({metric,status:'available',value:metric==='process.private'?0:4096,unit:'bytes',scope:'adapter_process',phase:scenario+'/'+stage,collector:'procfs',quality:'boundary_snapshot_only',profile:'memory'}));s.observations.push(...observations);events.push({event:{sample_index:s.index,stage},observations});
        }
      }
      trials.push({runtime_configuration:r.id,workload:w.id,scenario,block:0,status:'ok',profile:'memory',samples,phase_events:events,observations:[{metric:'process.peak_rss',status:'available',value:8192,phase:scenario+'/process_lifetime',quality:'kernel_accounted_peak',collector:'wait4_rusage'}]});
    }
  }
  const save=()=>{fs.writeFileSync(path.join(root,'manifest.json'),JSON.stringify(manifest));trials.forEach((x,i)=>fs.writeFileSync(path.join(root,'trials',i+'.json'),JSON.stringify(x)));};return {root,manifest,trials,save};
}
test('complete command coverage preserves zero private residency and both ABIs',t=>{const f=fixture(t);f.save();assert.deepEqual(verifyCommandLifecycleCoverage(f.root,'arm64'),{arch:'arm64',trials:24,samples:48,boundaries:96});});
for(const [name,change] of Object.entries({
  platform:f=>f.manifest.host.os='darwin',
  capability:f=>delete f.manifest.lock.runtime_configurations[0].description.capabilities.can_command_instantiate_phases,
  oracle:f=>f.trials[1].samples[0].command_result.stdout_sha256='bad',
  verification:f=>f.trials[1].samples[0].verified=false,
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
  allocator:f=>f.trials[1].samples[0].observations[0].normalization_denominator='command',
}))test('command coverage rejects '+name,t=>{const f=fixture(t);change(f);f.save();assert.throws(()=>verifyCommandLifecycleCoverage(f.root,'arm64'));});
test('archival requires relocated adapters and unchanged identities',t=>{const a=fixture(t),b=fixture(t);a.save();b.save();assert.throws(()=>verifyCommandLifecycleCoverage(b.root,'arm64',a.root,true));for(const r of b.manifest.lock.runtime_configurations){r.command=['/restored/'+r.id];r.file_sha256={['/restored/'+r.id]:'same'};}b.save();assert.doesNotThrow(()=>verifyCommandLifecycleCoverage(b.root,'arm64',a.root,true));b.manifest.lock.workloads[0].sha256='changed';b.save();assert.throws(()=>verifyCommandLifecycleCoverage(b.root,'arm64',a.root,true));});
