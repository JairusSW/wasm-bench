import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {spawnSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';

const verifier=fileURLToPath(new URL('./verify-linux-process-snapshot-run.mjs',import.meta.url));
const stages=['process-snapshot-capture','process-snapshot-restore','process-snapshot-first-write','process-snapshot-execute'];
function fixture(){
  const ids=['wasmtime-process-snapshot','wasmtime-winch-process-snapshot'];
  const manifest={host:{os:'linux',arch:'arm64'},lock:{
    options:{suite:'process-snapshots',profile:'timing',samples:2,operations:1,warmup:0,launches:1,scenarios:stages},
    runtime_configurations:ids.map(id=>({id})),workloads:[{id:'canonical'}]}};
  const samples=()=>[{verified:true,process_snapshot_result:{synthetic:true}},{verified:true,process_snapshot_result:{synthetic:true}}];
  const trials=ids.flatMap(id=>[...stages.map(scenario=>({runtime_configuration:id,scenario,block:0,status:'ok',profile:'timing',samples:samples()})),{runtime_configuration:id,scenario:stages[1],block:-1,status:'ok',profile:'timing',samples:samples()}]);
  return {manifest,trials};
}
function run(change){
  const root=fs.mkdtempSync(path.join(os.tmpdir(),'snapshot-coverage-'));
  try{
    const f=fixture();change?.(f);
    fs.mkdirSync(path.join(root,'trials'));
    fs.writeFileSync(path.join(root,'manifest.json'),JSON.stringify(f.manifest));
    f.trials.forEach((t,i)=>fs.writeFileSync(path.join(root,'trials',`${i}.json`),JSON.stringify(t)));
    return spawnSync(process.execPath,[verifier,root,'arm64'],{encoding:'utf8'});
  }finally{fs.rmSync(root,{recursive:true,force:true});}
}
test('coverage verifier uses the actual runtime_configurations wire field',()=>{
  assert.equal(run().status,0);
});
for(const [name,change] of Object.entries({
  'wrong architecture':f=>f.manifest.host.arch='amd64',
  'memory promoted to timing':f=>f.manifest.lock.options.profile='memory',
  'missing stage':f=>f.manifest.lock.options.scenarios=stages.slice(1),
  'missing trial':f=>f.trials.pop(),
  'unsupported cell':f=>f.trials[0].status='unsupported',
  'wrong configuration cell':f=>f.trials[0].runtime_configuration='unrelated',
  'partial sample sequence':f=>f.trials[0].samples.pop(),
  'unverified sample':f=>f.trials[0].samples[0].verified=false,
  'missing snapshot proof':f=>delete f.trials[0].samples[0].process_snapshot_result,
}))test(`coverage verifier rejects ${name}`,()=>assert.notEqual(run(change).status,0));

// These fixtures test coverage/schema only. Full native state proofs are checked
// by the Go loader and the opt-in real-worker evidence test, not this script.
