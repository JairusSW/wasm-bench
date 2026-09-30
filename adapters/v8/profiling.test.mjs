import test from 'node:test';
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {profileRun} from './profiling.mjs';

const prep={profile:'profiling',artifact_sha256:'a'.repeat(64),workload:{abi:'core',oracle:{kind:'exact_u64'},reset:'stateless',export:'run'}};
const request={scenario:'steady',samples:1,operations:1,warmup:0};
test('native profile bytes, identity, restart and failed workload evidence',async()=>{
  for(const fails of [false,true,false]){
    const result=await profileRun(prep,request,async()=>{
      const end=performance.now()+35;while(performance.now()<end) Math.sqrt(Math.random());
      if(fails)throw new Error('oracle mismatch');
      return {samples:[{verified:true}]};
    });
    if(fails){assert.equal(result.status,'error');assert.equal(result.reason,'oracle mismatch');}
    else assert.equal(result.samples[0].verified,true);
    const p=result.cpu_profile,bytes=Buffer.from(p.data,'base64'),native=JSON.parse(bytes);
    assert.equal(p.status,'collected');assert.equal(p.module_sha256,prep.artifact_sha256);
    assert.equal(p.format,'v8-cpuprofile-json');assert.equal(p.scope,'adapter_v8_isolate_sampled_stacks');
    assert.equal(p.sha256,createHash('sha256').update(bytes).digest('hex'));
    assert.ok(Array.isArray(native.nodes));assert.ok(native.endTime>=native.startTime);
  }
});
test('reject invalid requests before invoking workload',async()=>{
  for(const patch of [{samples:0},{operations:1000001},{warmup:-1},{phase_barriers:true},{scenario:'compile'}]){
    await assert.rejects(profileRun(prep,{...request,...patch},()=>assert.fail('executed invalid request')));
  }
  await assert.rejects(profileRun({...prep,profile:'timing'},request,()=>assert.fail('executed invalid profile')));
});
