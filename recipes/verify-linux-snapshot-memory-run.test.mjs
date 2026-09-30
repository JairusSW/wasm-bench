import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {spawnSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';
const verifier=fileURLToPath(new URL('./verify-linux-snapshot-memory-run.mjs',import.meta.url));
const stages=['process-snapshot-capture','process-snapshot-restore','process-snapshot-first-write','process-snapshot-execute'];
function fixture(){
  const ids=['wasmtime-process-snapshot','wasmtime-winch-process-snapshot'];
  const manifest={host:{os:'linux',arch:'arm64'},lock:{options:{suite:'process-snapshots',profile:'memory',phase_barriers:true,samples:2,operations:1,warmup:0,launches:1,scenarios:stages},runtime_configurations:ids.map(id=>({id})),workloads:[{id:'canonical'}]}};
  const evidence=()=>({version:'linux-process-snapshot-memory-v1',controller_pid:100,diagnostics:{profile:'memory',latency_eligible:false,samples:[{},{}]},records:Array.from({length:14},(_,i)=>({readings:Array.from({length:i%7===0?2:3},()=>({stat_before:'raw',stat_after:'raw',status:'raw',process:{start_time_ticks:'9007199254740993'}}))}))});
  const trials=ids.flatMap(id=>[...stages.map(scenario=>({runtime_configuration:id,scenario,block:0,status:'ok',profile:'memory',samples:[],snapshot_memory:evidence()})),{runtime_configuration:id,scenario:stages[1],block:-1,status:'ok',profile:'memory',samples:[{},{}]}]);
  return {manifest,trials};
}
function run(change){
  const root=fs.mkdtempSync(path.join(os.tmpdir(),'snapshot-memory-coverage-'));
  try{
    const f=fixture();change?.(f);
    fs.mkdirSync(path.join(root,'trials'));
    fs.writeFileSync(path.join(root,'manifest.json'),JSON.stringify(f.manifest));
    f.trials.forEach((t,i)=>fs.writeFileSync(path.join(root,'trials',`${i}.json`),JSON.stringify(t)));
    return spawnSync(process.execPath,[verifier,root,'arm64'],{encoding:'utf8'});
  }finally{fs.rmSync(root,{recursive:true,force:true});}
}
test('memory coverage checks the distinct diagnostic payload',()=>assert.equal(run().status,0));
for(const [name,change] of Object.entries({
  'timing profile':f=>f.manifest.lock.options.profile='timing',
  'no barriers':f=>f.manifest.lock.options.phase_barriers=false,
  'missing stage':f=>f.manifest.lock.options.scenarios=stages.slice(1),
  'unsupported trial':f=>f.trials[0].status='unsupported',
  'missing trial':f=>f.trials.pop(),
  'wrong runtime':f=>f.trials[0].runtime_configuration='other',
  'promoted sample':f=>f.trials[0].samples=[{}],
  'eligible timer':f=>f.trials[0].snapshot_memory.diagnostics.latency_eligible=true,
  'missing child':f=>f.trials[0].snapshot_memory.records[1].readings.pop(),
  'parent rss':f=>f.trials[0].observations=[{scope:'adapter_process'}],
  'numeric birth':f=>f.trials[0].snapshot_memory.records[1].readings[2].process.start_time_ticks=1,
}))test(`memory coverage rejects ${name}`,()=>assert.notEqual(run(change).status,0));
// Synthetic fixtures validate coverage only. The typed Go loader validates raw
// procfs lineage, ordering, guest state and qualified native runtime identity.
