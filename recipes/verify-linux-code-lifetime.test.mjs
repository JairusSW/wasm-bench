import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {verifyCodeLifetimeCoverage} from './verify-linux-code-lifetime.mjs';

function fixture(t){
  const root=fs.mkdtempSync(path.join(os.tmpdir(),'wasmbench-code-coverage-'));
  t.after(()=>fs.rmSync(root,{recursive:true,force:true}));
  const runtimes=['wasmtime-code-lifetime','wasmtime-winch-code-lifetime'].map((id,i)=>({id,description:{runtime:'wasmtime',runtime_version:'46.0.1',backend:i===0?'cranelift':'winch',capabilities:{can_code_lifetime:true,can_snapshot:false},effective_configuration:{code_lifetime:'wasmtime-executable-publication-v1'}}}));
  const manifest={host:{os:'linux',arch:'arm64',page_size:4096},lock:{options:{correctness_only:false,profile:'code',scenarios:['code-lifetime'],launches:3,samples:1,operations:1,warmup:0,phase_barriers:false},runtime_configurations:runtimes,workloads:[{id:'mechanisms/identity'},{id:'algorithms/sum'}]}};
  const trials=[];
  for(const r of runtimes)for(const w of manifest.lock.workloads)for(let block=0;block<3;block++)trials.push({runtime_configuration:r.id,workload:w.id,block,status:'ok',profile:'code',scenario:'code-lifetime',code_image:{},code_lifetime:{architecture:'arm64',page_size:4096,events:[{kind:'published',active_capacity:4096,cumulative_published_capacity:4096},{kind:'unpublished',active_capacity:0,cumulative_published_capacity:4096}],checkpoints:['compiled','instance_verified','module_handles_dropped','store_dropped','engine_dropped'].map(stage=>({stage,active_capacity:stage==='engine_dropped'?0:4096}))}});
  const save=()=>{fs.mkdirSync(path.join(root,'trials'),{recursive:true});fs.writeFileSync(path.join(root,'manifest.json'),JSON.stringify(manifest));trials.forEach((trial,i)=>fs.writeFileSync(path.join(root,'trials',i+'.json'),JSON.stringify(trial)));};
  save();return {root,manifest,trials,save};
}

test('complete diagnostic coverage and matched replay host pass',t=>{
  const a=fixture(t),b=fixture(t);
  assert.deepEqual(verifyCodeLifetimeCoverage(a.root,'arm64'),{arch:'arm64',trials:12,events:24,checkpoints:60});
  assert.deepEqual(verifyCodeLifetimeCoverage(b.root,'arm64',a.root),{arch:'arm64',trials:12,events:24,checkpoints:60});
  b.manifest.host.page_size=16384;b.save();
  assert.throws(()=>verifyCodeLifetimeCoverage(b.root,'arm64',a.root),/same observed host/);
});

for(const mode of ['missing','duplicate','unsupported','timing','profile','architecture','page_size','retention','checkpoint_order','wrong_runtime','snapshot_alias'])test('reject '+mode+' coverage',t=>{
  const f=fixture(t),trial=f.trials[0];
  switch(mode){
    case 'missing':fs.unlinkSync(path.join(f.root,'trials','0.json'));break;
    case 'duplicate':f.trials.push(structuredClone(trial));break;
    case 'unsupported':trial.status='unsupported';break;
    case 'timing':trial.samples=[{verified:true}];break;
    case 'profile':f.manifest.lock.options.profile='timing';break;
    case 'architecture':trial.code_lifetime.architecture='amd64';break;
    case 'page_size':trial.code_lifetime.page_size=16384;break;
    case 'retention':trial.code_lifetime.events[1].active_capacity=4096;break;
    case 'checkpoint_order':trial.code_lifetime.checkpoints.reverse();break;
    case 'wrong_runtime':f.manifest.lock.runtime_configurations[0].description.runtime_version='other';break;
    case 'snapshot_alias':f.manifest.lock.runtime_configurations[0].description.capabilities.can_snapshot=true;break;
  }
  if(mode!=='missing')f.save();
  assert.throws(()=>verifyCodeLifetimeCoverage(f.root,'arm64'));
});
