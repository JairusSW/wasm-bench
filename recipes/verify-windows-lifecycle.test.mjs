import test from 'node:test';
import assert from 'node:assert/strict';
import {verifyWindowsBundle} from './verify-windows-lifecycle.mjs';
function fixture(memory=false){
  const stages={compile:['before_compile','compiled','released'],instantiate:['before_instantiate','instantiated','instance_released'],'first-call':['before_first_call','first_call_returned','first_call_released']};
  const manifest={host:{os:'windows',arch:'amd64'},lock:{archive_tools:true,runner_sha256:'runner',options:{correctness_only:false,profile:memory?'memory':'timing',phase_barriers:memory,launches:memory?1:2,samples:2,operations:memory?1:2,warmup:0,scenarios:Object.keys(stages)},runtime_configurations:['wazero','wazero-interpreter','v8'].map(id=>({id,command:['C:/tools/'+id+'.exe'],file_sha256:{binary:id},description:{runtime:id}})),workloads:['mechanisms/identity','algorithms/sum'].map(id=>({id,sha256:id}))}};
  const trials=[];
  for(const r of manifest.lock.runtime_configurations)for(const w of manifest.lock.workloads){
    trials.push({runtime_configuration:r.id,workload:w.id,scenario:'first-call',block:-1,profile:manifest.lock.options.profile,status:'ok',samples:[{verified:true,warmup:false}]});
    for(const scenario of Object.keys(stages))for(let block=0;block<manifest.lock.options.launches;block++){
      const t={runtime_configuration:r.id,workload:w.id,scenario,block,profile:manifest.lock.options.profile,status:'ok',samples:[0,1].map(index=>({index,verified:true,warmup:false,operations:manifest.lock.options.operations,observations:[]})),phase_events:[],observations:[]};
      if(memory){
        t.observations.push({metric:'process.peak_rss',status:'unavailable',reason:'wait4 unavailable'});
        for(const s of t.samples)for(const stage of stages[scenario]){
          const observations=['process.rss','process.pss','process.private','process.virtual'].map(metric=>({metric,status:'unsupported',reason:'Linux only',collector:'procfs',scope:'adapter_process'}));
          t.phase_events.push({event:{sample_index:s.index,stage},observations});s.observations.push(...observations);
        }
      }
      trials.push(t);
    }
  }
  return {manifest,trials};
}
test('Windows timing and memory coverage retain explicit Linux collector gaps',()=>{
  const a=fixture(),b=fixture(true);
  assert.deepEqual(verifyWindowsBundle(a.manifest,a.trials),{measured_samples:72,boundaries:0});
  assert.deepEqual(verifyWindowsBundle(b.manifest,b.trials,true),{measured_samples:36,boundaries:108});
});
for(const [name,change] of Object.entries({
  platform:f=>f.manifest.host.os='linux',
  architecture:f=>f.manifest.host.arch='386',
  missingCell:f=>f.trials.pop(),
  duplicateCheck:f=>f.trials.push(f.trials[0]),
  oracle:f=>f.trials[1].samples[0].verified=false,
  fabricatedZero:f=>f.trials[1].phase_events[0].observations[0].value=0,
  wrongBoundary:f=>f.trials[1].phase_events[0].event.stage='compiled',
  missingSnapshot:f=>f.trials[1].phase_events[0].observations.pop(),
  rssGuess:f=>f.trials[1].observations[0].status='available',
  missingAttachment:f=>f.trials[1].samples[0].observations=[],
}))test('Windows gate rejects '+name,()=>{const f=fixture(true);change(f);assert.throws(()=>verifyWindowsBundle(f.manifest,f.trials,true));});
test('Windows replay preserves tool identity and requires archived relocation',()=>{
  const a=fixture(),b=fixture();assert.doesNotThrow(()=>verifyWindowsBundle(b.manifest,b.trials,false,a.manifest));
  assert.throws(()=>verifyWindowsBundle(b.manifest,b.trials,false,a.manifest,true));
  for(const r of b.manifest.lock.runtime_configurations)r.command[0]='D:/restored/'+r.id+'.exe';
  assert.doesNotThrow(()=>verifyWindowsBundle(b.manifest,b.trials,false,a.manifest,true));
  b.manifest.lock.runner_sha256='changed';assert.throws(()=>verifyWindowsBundle(b.manifest,b.trials,false,a.manifest,true));
});
