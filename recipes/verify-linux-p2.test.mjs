import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import crypto from 'node:crypto';
import {verifyP2Coverage} from './verify-linux-p2.mjs';

const digest=s=>crypto.createHash('sha256').update(s).digest('hex');
const stages={compile:['before_compile','compiled','released'],instantiate:['before_instantiate','instantiated','instance_released'],'first-call':['before_first_call','first_call_returned','first_call_released']};
function fixture(t) {
  const root=fs.mkdtempSync(path.join(os.tmpdir(),'wasmbench-p2-coverage-'));
  t.after(()=>fs.rmSync(root,{recursive:true,force:true}));
  fs.mkdirSync(path.join(root,'trials'));
  const workload={id:'p2/filesystem-reset',abi:'component',reset:'fresh_instance_per_sample',oracle:{kind:'exact_command'},command:{stdout_sha256:digest('filesystem reset verified\n'),stderr_sha256:digest('')}};
  const manifest={host:{os:'linux',arch:'arm64'},lock:{archive_tools:true,options:{correctness_only:false,profile:'memory',scenarios:Object.keys(stages),launches:1,samples:2,operations:1,warmup:0,phase_barriers:true},workloads:[workload],runtime_configurations:['wasmtime','wasmtime-winch'].map((id,i)=>({id,description:{backend:i?'winch':'cranelift',capabilities:{can_component_command_lifecycle:true,can_component_command_phases:true},effective_configuration:{component_command_lifecycle_policy:'explicit boundaries'}}}))}};
  const sample=i=>({index:i,verified:true,warmup:false,operations:1,sample_type:'individual_operation',command_result:{exit_code:0,stdout_sha256:workload.command.stdout_sha256,stderr_sha256:workload.command.stderr_sha256},observations:[]});
  const trials=[];
  for(const r of manifest.lock.runtime_configurations) {
    trials.push({id:`check-${r.id}`,runtime_configuration:r.id,workload:workload.id,scenario:'first-call',status:'ok',block:-1,samples:[sample(0),sample(1)]});
    for(const scenario of Object.keys(stages)) {
      const samples=[sample(0),sample(1)],phase_events=[];
      for(let i=0;i<2;i++)for(const stage of stages[scenario]) {
        const observations=['process.rss','process.pss','process.private','process.virtual'].map(metric=>({metric,status:'available',value:metric==='process.private'?0:4096,unit:'bytes',scope:'adapter_process',phase:`${scenario}/${stage}`,collector:'procfs',quality:'boundary_snapshot_only',profile:'memory'}));
        samples[i].observations.push(...observations);
        phase_events.push({event:{sample_index:i,stage},observations});
      }
      trials.push({id:`trial-${r.id}-${scenario}`,runtime_configuration:r.id,workload:workload.id,scenario,status:'ok',block:0,samples,phase_events,observations:[{metric:'process.peak_rss',status:'available',value:8192,phase:scenario+'/process_lifetime',quality:'kernel_accounted_peak',collector:'wait4_rusage'}]});
    }
  }
  const save=()=>{fs.writeFileSync(path.join(root,'manifest.json'),JSON.stringify(manifest));for(const [i,r] of trials.entries())fs.writeFileSync(path.join(root,'trials',`${i}.json`),JSON.stringify(r));};
  return {root,manifest,trials,save};
}
test('complete native coverage keeps measured zero private memory',t=>{
  const f=fixture(t);f.save();assert.deepEqual(verifyP2Coverage(f.root,'arm64','memory'),{arch:'arm64',profile:'memory',trials:8,samples:16,boundaries:36});
});
test('archived replay refuses reuse of installed adapter paths',t=>{
  const original=fixture(t),replayed=fixture(t);
  for(const f of [original,replayed]) {
    f.manifest.lock.runner_sha256=digest('runner');f.manifest.lock.workloads[0].sha256=digest('wasm');
    for(const r of f.manifest.lock.runtime_configurations) {r.command=['/installed/adapter'];r.file_sha256={'/installed/adapter':digest('adapter')};}
    f.save();
  }
  assert.doesNotThrow(()=>verifyP2Coverage(replayed.root,'arm64','memory',original.root));
  assert.throws(()=>verifyP2Coverage(replayed.root,'arm64','memory',original.root,true),/relocated archived adapter/);
  for(const r of replayed.manifest.lock.runtime_configurations) {r.command=['/restored/adapter'];r.file_sha256={'/restored/adapter':digest('adapter')};}
  replayed.save();assert.doesNotThrow(()=>verifyP2Coverage(replayed.root,'arm64','memory',original.root,true));
  replayed.manifest.lock.runtime_configurations[0].file_sha256['/restored/adapter']=digest('different');
  replayed.save();assert.throws(()=>verifyP2Coverage(replayed.root,'arm64','memory',original.root,true));
});
for(const [name,change] of Object.entries({
  'wrong platform':f=>f.manifest.host.os='darwin',
  'missing capability':f=>delete f.manifest.lock.runtime_configurations[0].description.capabilities.can_component_command_phases,
  'incorrect command output':f=>f.trials[1].samples[0].command_result.stdout_sha256=digest('bad'),
  'unverified sample':f=>f.trials[1].samples[0].verified=false,
  'duplicate cell':f=>f.trials[2].scenario='compile',
  'missing phase':f=>f.trials[1].phase_events.pop(),
  'out of order':f=>f.trials[1].phase_events[0].event.stage='compiled',
  'wrong sample identity':f=>f.trials[1].phase_events[0].event.sample_index=1,
  'unavailable process snapshot':f=>f.trials[1].phase_events[0].observations[0].status='unavailable',
  'wrong collector scope':f=>f.trials[1].phase_events[0].observations[0].scope='whole_host',
  'false exact peak':f=>f.trials[1].phase_events[0].observations[0].quality='kernel_accounted_peak',
  'missing sample evidence':f=>f.trials[1].samples[0].observations=[],
  'peak attributed to phase':f=>f.trials[1].observations[0].phase='compile/api_window',
  'incorrect-result trial':f=>f.trials[1].status='incorrect_result',
}))test(`P2 coverage rejects ${name}`,t=>{
  const f=fixture(t);change(f);f.save();assert.throws(()=>verifyP2Coverage(f.root,'arm64','memory'));
});
