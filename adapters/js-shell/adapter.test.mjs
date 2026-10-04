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
async function session(requests,bytes=wasm,overrides={},argv){
  const lines=requests.map((r,i)=>JSON.stringify({version:1,id:i+1,...r})),out=[];
  const context={arguments:argv||['runtime=v8-shell','binary-sha256='+ 'a'.repeat(64)],readline:()=>lines.shift()||null,print:s=>out.push(JSON.parse(s)),readbuffer:()=>Uint8Array.from(bytes).buffer,performance,WebAssembly,...overrides};
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
test('JSC forced-tier warmups are excluded from measured samples',async()=>{
  const out=await session([prepare(),{method:'run',run:{scenario:'steady',samples:1,operations:1,warmup:0}}],wasm,{read:()=>Uint8Array.from(wasm).buffer},['runtime=jsc','tier-mode=omg-eager','binary-sha256='+'a'.repeat(64)]);
  assert.equal(out[0].status,'ok',JSON.stringify(out[0]));assert.equal(out[1].status,'ok',JSON.stringify(out[1]));assert.equal(out[1].samples.length,1);assert.equal(out[1].samples[0].index,0);assert.equal(out[1].samples[0].warmup,false);
});
test('JSC preserves requested warmups but omits its additional forced-tier calls',async()=>{
  const out=await session([prepare(),{method:'run',run:{scenario:'steady',samples:1,operations:1,warmup:1}}],wasm,{read:()=>Uint8Array.from(wasm).buffer},['runtime=jsc','tier-mode=omg-eager','binary-sha256='+'a'.repeat(64)]);
  assert.equal(out[1].status,'ok',JSON.stringify(out[1]));assert.deepEqual(out[1].samples.map(s=>[s.index,s.warmup]),[[0,true],[1,false]]);
});
test('JSC host callback steady timing preserves requested warmup count',async()=>{
  const bytes=Buffer.from('0061736d0100000001060160017f017f021601097761736d62656e6368086964656e74697479000003020100070d010962656e63686d61726b00010a08010600200010000b','hex');
  const w={...work(),export:'benchmark',host_profile:'identity-v1',args:['7'],oracle:{kind:'exact_u64',expected:['7']}};
  const out=await session([prepare(bytes,w),{method:'run',run:{scenario:'steady',samples:1,operations:1,warmup:3}}],bytes,{read:()=>Uint8Array.from(bytes).buffer},['runtime=jsc','tier-mode=omg-eager','binary-sha256='+'a'.repeat(64)]);
  assert.equal(out[0].status,'ok',JSON.stringify(out[0]));assert.equal(out[1].status,'ok',JSON.stringify(out[1]));assert.deepEqual(out[1].samples.map(s=>[s.index,s.warmup]),[[0,true],[1,true],[2,true],[3,false]]);
});
test('fresh reset cannot be batched as stateless',async()=>{const w=work();w.reset='fresh_instance_per_sample';const out=await session([prepare(wasm,w),run('steady')]);assert.equal(out[1].status,'unsupported');});
test('protocol and unknown scenario reject',async()=>{assert.equal((await session([{method:'describe',version:2}]))[0].status,'error');const out=await session([prepare(),run('teardown')]);assert.equal(out[1].status,'unsupported');});
test('no wall clock fallback',async()=>{await assert.rejects(session([],wasm,{performance:undefined,preciseTime:()=>0}),/monotonic clock unavailable/);});
test('describe advertises bounded support',async()=>{const [r]=await session([{method:'describe'}]);assert.deepEqual(r.description.abis,['core']);assert.deepEqual(r.description.scenarios,['compile','instantiate','first-call','steady']);assert.equal(r.description.capabilities.can_run_commands,undefined);assert.equal(r.description.capabilities.can_instantiate_separately,true);});

function abortFixture({trap=false,badSignature=false}={}) {
  const section=(id,data)=>[id,...leb(data.length),...data];
  const text=s=>[s.length,...Buffer.from(s)];
  const params=badSignature?[126,127,127,127]:[127,127,127,127];
  const body=[0,...(trap?[65,0,65,0,65,0,65,0,16,0]:[]),32,0,11];
  return Buffer.from([0,97,115,109,1,0,0,0,
    ...section(1,[2,96,4,...params,0,96,1,126,1,126]),
    ...section(2,[1,...text('env'),...text('abort'),0,0]),
    ...section(3,[1,1]),...section(7,[1,...text('run'),0,1]),
    ...section(10,[1,...leb(body.length),...body])]);
}
test('AssemblyScript abort import permits verified lifecycle execution',async()=>{
  const bytes=abortFixture(),w={...work(),host_profile:'assemblyscript-abort-v1'};
  for(const scenario of ['compile','instantiate','first-call','steady']) {
    const out=await session([prepare(bytes,w),run(scenario)],bytes);
    assert.equal(out[0].status,'ok');assert.equal(out[1].status,'ok');
    assert.ok(out[1].samples.every(s=>s.verified));
  }
});
test('AssemblyScript abort throws and cannot yield verified samples',async()=>{
  const bytes=abortFixture({trap:true}),w={...work(),host_profile:'assemblyscript-abort-v1'};
  const out=await session([prepare(bytes,w),run('first-call')],bytes);
  assert.equal(out[0].status,'ok');assert.equal(out[1].status,'error');
  assert.match(out[1].reason,/AssemblyScript abort/);assert.equal(out[1].samples,undefined);
});
test('AssemblyScript abort rejects wrong signature and undeclared profile',async()=>{
  const bad=abortFixture({badSignature:true}),w={...work(),host_profile:'assemblyscript-abort-v1'};
  assert.equal((await session([prepare(bad,w)],bad))[0].status,'unsupported');
  const bytes=abortFixture();assert.equal((await session([prepare(bytes,work())],bytes))[0].status,'unsupported');
});


test('fresh steady isolates mutable guest state before every sample',async()=>{
  const section=(id,data)=>[id,...leb(data.length),...data];
  const body=[0,35,0,65,1,106,36,0,35,0,11];
  const bytes=Buffer.from([0,97,115,109,1,0,0,0,
    ...section(1,[1,96,0,1,127]),...section(3,[1,0]),
    ...section(6,[1,127,1,65,0,11]),...section(7,[1,3,114,117,110,0,0]),
    ...section(10,[1,...leb(body.length),...body])]);
  const w={...work(),reset:'fresh_instance_per_sample',args:[],oracle:{kind:'exact_u64',expected:['1']}};
  const out=await session([prepare(bytes,w),{method:'run',run:{scenario:'steady',samples:3,operations:1,warmup:2}}],bytes);
  assert.equal(out[0].status,'ok');assert.equal(out[1].status,'ok',JSON.stringify(out[1]));
  assert.equal(out[1].samples.filter(s=>!s.warmup).length,3);
  assert.ok(out[1].samples.every(s=>s.verified && s.result[0]==='1'));
});

function vectorFixture() {
  const section=(id,data)=>[id,...leb(data.length),...data];
  const body=[0,32,2,32,1,58,0,0,11];
  return Buffer.from([0,97,115,109,1,0,0,0,
    ...section(1,[1,96,3,127,127,127,0]),...section(3,[1,0]),...section(5,[1,0,1]),
    ...section(7,[2,3,114,117,110,0,0,6,109,101,109,111,114,121,2,0]),
    ...section(10,[1,...leb(body.length),...body])]);
}
const vectorWork=()=>({abi:'core',reset:'fresh_instance_per_sample',export:'run',args:[],oracle:{kind:'exact_vectors',expected:[]},vector_byte_budget:8,vectors:{input_offset:0,output_offset:100,output_len:1,mod:251,cases:[{len:0,out:'00'},{len:3,out:'03'}]}});
test('vector lifecycle and execution sequences verify each case',async()=>{
  const bytes=vectorFixture();
  for(const scenario of ['compile','instantiate','first-call','steady']) {
    const out=await session([prepare(bytes,vectorWork()),{method:'run',run:{scenario,samples:3,operations:1,warmup:scenario==='steady'?2:0}}],bytes);
    assert.equal(out[0].status,'ok',JSON.stringify(out[0]));assert.equal(out[1].status,'ok',JSON.stringify(out[1]));
    assert.equal(out[1].samples.filter(s=>!s.warmup).length,3);assert.ok(out[1].samples.every(s=>s.verified));
  }
});
test('vector wrong output, memory bounds and byte budget cannot yield samples',async()=>{
  const bytes=vectorFixture();
  for(const mutate of [w=>w.vectors.cases[1].out='04',w=>w.vectors.output_offset=65536]) {
    const w=vectorWork();mutate(w);const out=await session([prepare(bytes,w),{method:'run',run:{scenario:'steady',samples:1,operations:1,warmup:0}}],bytes);
    assert.equal(out[1].status,'error');assert.equal(out[1].samples,undefined);
  }
  const w=vectorWork();w.vector_byte_budget=1;assert.equal((await session([prepare(bytes,w)],bytes))[0].status,'error');
});

test('describe declares measured vector lifecycle support',async()=>{const [r]=await session([{method:'describe'}]);assert.equal(r.description.capabilities.can_run_vectors,true);assert.equal(r.description.capabilities.can_vector_compile_phases,undefined);});
