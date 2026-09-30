import {test} from 'node:test';
import assert from 'node:assert/strict';
import {EventEmitter} from 'node:events';
import {createHash} from 'node:crypto';
import {traceRun,traceProbe} from './tracing.mjs';

const moduleHash='a'.repeat(64);
const event={pid:1,tid:2,ts:10,ph:'X',cat:'v8.wasm',name:'wasm.SyncCompile',dur:3,args:{id:0},native_extra:'preserved'};
function fixture({startError,stopError,complete=true,chunks=[[event,event]],malformed=false}={}) {
  const calls=[];let disconnected=false;
  class Session extends EventEmitter {
    connect(){calls.push('connect');}
    async post(method,params){
      calls.push(method);
      if(method==='NodeTracing.start'){
        assert.deepEqual(params,{traceConfig:{includedCategories:['v8.wasm']}});
        if(startError)throw new Error('start unavailable');
      }else{
        for(const events of chunks)this.emit('NodeTracing.dataCollected',{params:{value:events}});
        if(malformed)this.emit('NodeTracing.dataCollected',{params:{value:null}});
        if(complete)this.emit('NodeTracing.tracingComplete');
        if(stopError)throw new Error('stop unavailable');
      }
    }
    disconnect(){disconnected=true;calls.push('disconnect');}
  }
  let clock=100n;
  return {options:{SessionClass:Session,clock:()=>clock++},calls,disconnected:()=>disconnected};
}
function decode(trace){
  const data=Buffer.from(trace.data,'base64');
  assert.equal(trace.sha256,createHash('sha256').update(data).digest('hex'));
  return JSON.parse(data).traceEvents;
}
test('collector preserves native events and duplicates, exact clock bridge and workload result',async()=>{
  const f=fixture();let runs=0;
  const response=await traceRun(moduleHash,onEpoch=>{runs++;onEpoch('100');return {samples:[{verified:true}]};},f.options);
  assert.equal(runs,1);assert.deepEqual(response.samples,[{verified:true}]);
  assert.equal(response.engine_trace.status,'collected');
  assert.equal(response.engine_trace.start_clock_ns,'100');
  assert.equal(response.engine_trace.end_clock_ns,'101');
  assert.equal(response.engine_trace.trajectory_epoch_ns,'100');
  assert.deepEqual(decode(response.engine_trace),[event,event]);
  assert.deepEqual(f.calls,['connect','NodeTracing.start','NodeTracing.stop','disconnect']);
  assert.equal(f.disconnected(),true);
});
test('collector failures do not hide or repeat the workload',async()=>{
  for(const failure of ['construct','start','stop','completion','notification']){
    const f=fixture({startError:failure==='start',stopError:failure==='stop',complete:failure!=='completion',malformed:failure==='notification'});
    if(failure==='construct')f.options.SessionClass=class {constructor(){throw new Error('no inspector');}};
    let runs=0;
    const response=await traceRun(moduleHash,()=>{runs++;return {status:'error',reason:'guest failure',samples:[{verified:false}]};},f.options);
    assert.equal(runs,1);assert.equal(response.status,'error');assert.equal(response.reason,'guest failure');
    assert.equal(response.samples.length,1);
    assert.equal(response.engine_trace.status,['construct','start'].includes(failure)?'unavailable':'incomplete');
    assert.ok(response.engine_trace.reason);
    if(response.engine_trace.status==='unavailable'){
      for(const field of ['data','sha256','start_clock_ns','end_clock_ns','trajectory_epoch_ns'])assert.equal(response.engine_trace[field],undefined);
    }else assert.deepEqual(decode(response.engine_trace),[event,event]);
  }
});
test('throwing workload still stops collector and preserves evidence',async()=>{
  const f=fixture();
  const response=await traceRun(moduleHash,()=>{throw new Error('guest failed');},f.options);
  assert.equal(response.status,'error');assert.equal(response.reason,'guest failed');
  assert.equal(response.engine_trace.status,'collected');assert.equal(f.disconnected(),true);
  assert.deepEqual(decode(response.engine_trace),[event,event]);
});
test('byte budget retains only a prefix, never later small events',async()=>{
  const f=fixture({chunks:[[event,{...event,args:{large:'x'.repeat(2000)}},event],[event]]});
  f.options.byteLimit=500;
  const response=await traceRun(moduleHash,()=>({}),f.options);
  assert.equal(response.engine_trace.status,'incomplete');
  assert.match(response.engine_trace.reason,/500 byte budget/);
  assert.deepEqual(decode(response.engine_trace),[event]);
  assert.ok(Buffer.from(response.engine_trace.data,'base64').length<=500);
});
test('empty completed collection is not invented activity',async()=>{
  const f=fixture({chunks:[]});
  const response=await traceRun(moduleHash,()=>({}),f.options);
  assert.equal(response.engine_trace.status,'collected');assert.deepEqual(decode(response.engine_trace),[]);
});
test('installed inspector emits native compilation events without launch tracing flags',async()=>{
  const bytes=Buffer.from('0061736d010000000105016000017f030201000707010372756e00000a0601040041070b','hex');
  const hash=createHash('sha256').update(bytes).digest('hex');
  const probe=await traceProbe(bytes,hash);
  assert.equal(probe.compilation_category_verified,true);
  const result=await traceRun(hash,onEpoch=>{
    const fn=new WebAssembly.Instance(new WebAssembly.Module(bytes)).exports.run;
    onEpoch(process.hrtime.bigint().toString());
    for(let i=0;i<10;i++)assert.equal(fn(),7);
    return {verified:true};
  });
  assert.equal(result.verified,true);assert.equal(result.engine_trace.status,'collected');
  const events=decode(result.engine_trace);
  assert.ok(events.some(e=>e.cat==='v8.wasm'&&e.name==='wasm.SyncCompile'));
  assert.ok(events.every(e=>Number.isInteger(e.pid)&&Number.isInteger(e.tid)));
  assert.ok(BigInt(result.engine_trace.trajectory_epoch_ns)>=BigInt(result.engine_trace.start_clock_ns));
  assert.ok(BigInt(result.engine_trace.trajectory_epoch_ns)<=BigInt(result.engine_trace.end_clock_ns));
});
