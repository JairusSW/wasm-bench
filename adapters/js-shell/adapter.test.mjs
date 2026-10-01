import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import fs from 'node:fs';
import crypto from 'node:crypto';
import {performance} from 'node:perf_hooks';

const script=fs.readFileSync(new URL('./adapter.js',import.meta.url),'utf8');
const wasm=Buffer.from('0061736d0100000001060160017e017e030201000707010372756e00000a0601040020000b','hex');
const sha=bytes=>crypto.createHash('sha256').update(bytes).digest('hex');
const work=()=>({abi:'core',reset:'stateless',export:'run',args:['18446744073709551615'],oracle:{kind:'exact_u64',expected:['18446744073709551615']}});
const prepare=(bytes=wasm,w=work(),profile='timing')=>({method:'prepare',prepare:{artifact:'fixture',artifact_sha256:sha(bytes),profile,workload:w}});
const run=scenario=>({method:'run',run:{scenario,samples:2,operations:scenario==='steady'?3:1,warmup:scenario==='steady'?1:0}});
async function session(requests,bytes=wasm,overrides={}){
  const lines=requests.map((r,i)=>JSON.stringify({version:1,id:i+1,...r})),out=[];
  const context={arguments:['runtime=v8-shell','binary-sha256='+ 'a'.repeat(64)],readline:()=>lines.shift()||null,print:s=>out.push(JSON.parse(s)),readbuffer:()=>Uint8Array.from(bytes).buffer,performance,WebAssembly,...overrides};
  await vm.runInNewContext(script,context);
  return out;
}
for(const scenario of ['compile','instantiate','first-call','steady'])test('exact i64 '+scenario,async()=>{
  const out=await session([prepare(),run(scenario),{method:'close'}]);assert.equal(out[0].status,'ok');assert.equal(out[1].status,'ok');
  assert.equal(out[1].samples.length,scenario==='steady'?3:2);
  for(const s of out[1].samples){assert.equal(s.verified,true);assert.deepEqual(s.result,['18446744073709551615']);assert.ok(s.elapsed_ns>=0);}
});
test('SHA256 block and padding boundaries',async()=>{
  for(const n of [0,1,15,16,17,55,56,57,63,64,65,120,127,128,1000]){
    // A custom section gives the hash varying padding without changing execution.
    const data=Buffer.concat([wasm,Buffer.from([0,...leb(n+1),0]),Buffer.alloc(n)]);
    const out=await session([prepare(data),run('first-call')],data);assert.equal(out[0].status,'ok',String(n));assert.equal(out[1].status,'ok');
  }
});
function leb(n){const out=[];do{let b=n&127;n>>>=7;if(n)b|=128;out.push(b);}while(n);return out;}
test('wrong oracle cannot yield verified samples',async()=>{const w=work();w.oracle.expected=['7'];const out=await session([prepare(wasm,w),run('first-call')]);assert.equal(out[1].status,'error');assert.equal(out[1].samples,undefined);});
test('prepare digest mismatch clears state',async()=>{const bad=prepare();bad.prepare.artifact_sha256='0'.repeat(64);const out=await session([prepare(),bad,run('steady')]);assert.equal(out[1].status,'error');assert.match(out[2].reason,/prepare required/);});
test('failed argument validation clears state',async()=>{const w=work();w.args=['18446744073709551616'];const out=await session([prepare(),prepare(wasm,w),run('steady')]);assert.equal(out[1].status,'error');assert.match(out[2].reason,/prepare required/);});
test('extended contracts and profiles rejected',async()=>{
  for(const key of ['vectors','command','density','checkpoint','continuation','process_snapshot','guest_density','snapshot_density']){const w=work();w[key]={};assert.equal((await session([prepare(wasm,w)]))[0].status,'unsupported');}
  assert.equal((await session([prepare(wasm,work(),'profiling')]))[0].status,'unsupported');
});
test('invalid batches rejected before samples',async()=>{for(const [key,value] of [['samples',0],['operations',0],['warmup',-1],['samples',100001]]){const r=run('steady');r.run[key]=value;const out=await session([prepare(),r]);assert.equal(out[1].status,'error');}});
test('fresh reset cannot be batched as stateless',async()=>{const w=work();w.reset='fresh_instance_per_sample';const out=await session([prepare(wasm,w),run('steady')]);assert.equal(out[1].status,'unsupported');});
test('protocol and unknown scenario reject',async()=>{assert.equal((await session([{method:'describe',version:2}]))[0].status,'error');const out=await session([prepare(),run('teardown')]);assert.equal(out[1].status,'unsupported');});
test('no wall clock fallback',async()=>{await assert.rejects(session([],wasm,{performance:undefined,preciseTime:()=>0}),/monotonic clock unavailable/);});
test('describe advertises bounded support',async()=>{const [r]=await session([{method:'describe'}]);assert.deepEqual(r.description.abis,['core']);assert.deepEqual(r.description.scenarios,['compile','instantiate','first-call','steady']);assert.equal(r.description.capabilities.can_run_commands,undefined);assert.equal(r.description.capabilities.can_instantiate_separately,true);});
