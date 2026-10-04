// JSON control only on stdout. Guest imports must route output to stderr.
import readline from 'node:readline';
import fs from 'node:fs';
import crypto from 'node:crypto';
import os from 'node:os';
import path from 'node:path';
import {WASI} from 'node:wasi';
import {readonlyWasiImports,normalizeWasiStdout} from './wasi-readonly.mjs';
import {profileRun} from './profiling.mjs';
import * as harness from './harness.mjs';
import {encodeFloats, verifyFloats, validateFloat, numericSignature, floatArguments} from './floats.mjs';
import {parseCompilerMode,verifyCompilerMode} from './compiler-mode.mjs';

const compilerMode=parseCompilerMode(process.argv.slice(2),process.execArgv,process.env);
let compilerModeProbe;
function compilerDescription(description) {
  if(!compilerModeProbe)return description;
  const configuration=description.effective_configuration;
  description.capabilities.can_control_compiler_mode=true;
  configuration.compiler_mode=configuration.tiering=compilerMode;
  if(process.execArgv.includes('--no-wasm-native-module-cache')){
    configuration.module_cache='disabled';
    description.capabilities.can_disable_code_cache=true;
  }
  configuration.lazy_compilation='disabled';
  configuration.compiler_mode_probe=JSON.stringify(compilerModeProbe);
  configuration.compiler_mode_policy='explicit eager compiler mode verified using a separate fixed calibration module before workloads; no per-workload tier inspection or fully materialized whole-module claim';
  configuration.sustained_policy=configuration.sustained_policy.replace('production-default tiering','explicit eager '+compilerMode+' compilation with tier-up disabled');
  return description;
}

let prep, bytes, module, instance, imports;
const featureProbes = {"MEMORY64": "AGFzbQEAAAABBgFgAX8BfwMCAQAFBQEFAoACBxYCBm1lbW9yeQIACWJlbmNobWFyawAACj0BOwECfwNAIAICfyABQf8/cUECdK0gATYCACABQf8/cUECdK0oAgALaiECIAFBAWohASABIABJDQALIAILAB4EbmFtZQIMAQADAAFuAQFpAgFhAwkBAAEABGxvb3A=", "EXCEPTIONS": "AGFzbQEAAAABCgJgAX8AYAF/AX8DAgEBDQMBAAAHDQEJYmVuY2htYXJrAAAKLQErAQJ/A0AgAgJ/H0ABAAAAIAEIAAsAC2ohAiABQQFqIQEgASAASQ0ACyACCwApBG5hbWUCDAEAAwABbgEBaQIBYQMOAQACAARsb29wAQNvdXQLBAEAAXQ=", "GC": "AGFzbQEAAAABBgFgAX8BfwMCAQAHDQEJYmVuY2htYXJrAAAKJAEiAQJ/A0AgAiAB+xz7HmohAiABQQFqIQEgASAASQ0ACyACCwAeBG5hbWUCDAEAAwABbgEBaQIBYQMJAQABAARsb29w", "RELAXED_SIMD": "AGFzbQEAAAABBgFgAX8BfwMCAQAHDQEJYmVuY2htYXJrAAAKOAE2AQJ/A0AgAiABs/0TQwAAAED9E0MAAIA//RP9hQL9HwKpaiECIAFBAWohASABIABJDQALIAILAB4EbmFtZQIMAQADAAFuAQFpAgFhAwkBAAEABGxvb3A=", "STACK_SWITCHING": "AGFzbQEAAAABCAJgAX8Bf10AAwMCAAAHDQEJYmVuY2htYXJrAAEJBQEDAAEAChUCBwAgAEEHagsLACAA0gDgAeMBAAsAFwRuYW1lAQcBAAR0YXNrBAcCAAFmAQFj", "MULTI_MEMORY": "AGFzbQEAAAABBgFgAX8BfwMCAQAFBwIBAgIBAgIHDQEJYmVuY2htYXJrAAAKWwFZAQJ/A0AgAgJ/IAFB/z9xQQJ0IAE2AgAgAUH/P3FBAnQgAUEHajZCAQAgAUH/P3FBAnQoQgEAIAFB/z9xQQJ0KAIAawtqIQIgAUEBaiEBIAEgAEkNAAsgAgsAJwRuYW1lAgwBAAMAAW4BAWkCAWEDCQEAAQAEbG9vcAYHAgABYQEBYg=="};
const validatorFeatures = {namespace: "wasmparser/0.251.0", evidence: "Representative valid proposal modules tested with WebAssembly.validate under the recorded Node flags; this gates unavailable configurations, not proposal conformance", supported: Object.fromEntries(Object.entries(featureProbes).map(([name, encoded]) => [name, WebAssembly.validate(Buffer.from(encoded, "base64"))]))};

function stringBuiltinsAvailable() {
  try {
    const bytes=Buffer.from("AGFzbQEAAAABDAJgAm9/AX9gAX8BfwJDAgdzdHJpbmdzGmFiY2RlZmdoaWprbG1ub3BxcnN0dXZ3eHl6A28ADndhc206anMtc3RyaW5nCmNoYXJDb2RlQXQAAAMCAQEHDQEJYmVuY2htYXJrAAEKJwElAQJ/A0AgAiMAIAFBGnAQAGohAiABQQFqIQEgASAASQ0ACyACCwAtBG5hbWUBBwEABGNoYXICDAEBAwABbgEBaQIBYQMJAQEBAARsb29wBwQBAAFz","base64");
    const module=new WebAssembly.Module(bytes,{builtins:["js-string"],importedStringConstants:"strings"});
    return new WebAssembly.Instance(module,{}).exports.benchmark(1)===97;
  } catch { return false; }
}
function compileModule(bytes) { return prep?.workload.host_profile === "js-string-builtins-v1" ? new WebAssembly.Module(bytes, {builtins: ["js-string"], importedStringConstants: "strings"}) : new WebAssembly.Module(bytes); }

let floatSignature;
const scenarios = ['harness-calibration', 'compile', 'instantiate', 'app-init', 'first-call', 'steady', 'trajectory', 'teardown', 'density', 'sustained'];
const hash = b => crypto.createHash('sha256').update(b).digest('hex');
const now = () => process.hrtime.bigint();
function construct() { return new WebAssembly.Instance(module, imports); }
function initialize(target) {
  if(prep.workload.initialize)target.exports[prep.workload.initialize]();
  return applyInput(target);
}
function applyInput(target) {
  const input=prep.workload.input;
  if(input){
    if(typeof input.hex!=='string'||!/^(?:[0-9a-fA-F]{2})*$/.test(input.hex))throw new Error('invalid input hex');
    let base=0;
    if(input.pointer_export){
      const f=target.exports[input.pointer_export];
      if(typeof f!=='function')throw new Error('missing input pointer export');
      const value=f();
      if(typeof value!=='number'||!Number.isInteger(value)||value < -2147483648||value>4294967295)throw new Error('invalid wasm32 input pointer');
      base=value>>>0;
    }
    const offset=base+input.offset, data=Buffer.from(input.hex,'hex'), memory=target.exports.memory;
    if(!Number.isSafeInteger(offset)||offset<0||offset>0xffffffff||!memory||offset+data.length>memory.buffer.byteLength)throw new Error('input write outside guest memory');
    new Uint8Array(memory.buffer,offset,data.length).set(data);
  }
  return target;
}
function normalize(value) {if(floatSignature)return encodeFloats(value,floatSignature.results);return value === undefined ? [] : (Array.isArray(value) ? value : [value]).map(x => typeof x === 'bigint' ? BigInt.asUintN(64,x).toString() : String(x >>> 0));}
function argumentsForCall() {if(floatSignature)return floatArguments(prep.workload.args,floatSignature.params);return prep.workload.args.map(x=>{const n=Number(x);if(!Number.isSafeInteger(n))throw new Error('i64 arguments require typed argument support');return n;});}
function invoke(target = instance) {
  const value = target.exports[prep.workload.export](...argumentsForCall());
  return normalize(value);
}
function verify(result,target=instance) {
  if(floatSignature)verifyFloats(prep.workload.oracle,result,floatSignature.results);
  else if (prep.workload.oracle.kind !== 'exact_u64' || JSON.stringify(result) !== JSON.stringify(prep.workload.oracle.expected.map(String))) throw new Error(`incorrect result: ${JSON.stringify(result)}`);
  let base=0;
  const pointer=prep.workload.oracle.output_pointer_export;
  if(pointer){
    if(typeof target.exports[pointer]!=='function')throw new Error('incorrect result: missing output pointer');
    const value=target.exports[pointer]();
    if(typeof value!=='number'||!Number.isInteger(value)||value < -2147483648||value>4294967295)throw new Error('incorrect result: invalid wasm32 output pointer');
    base=value>>>0;
  }
  for(const check of prep.workload.oracle.memory||[]){const want=Buffer.from(check.hex,'hex');const memory=target.exports.memory;const offset=base+check.offset;if(!Number.isSafeInteger(offset)||offset<0||offset>0xffffffff||!memory||offset+want.length>memory.buffer.byteLength||!Buffer.from(memory.buffer,offset,want.length).equals(want))throw new Error('incorrect result: memory oracle mismatch');}
}
function wasiFixture(command) {
  if(command.stdout_normalize && command.stdout_normalize!=='llvm-ir-preds')throw new Error('unsupported WASI stdout normalizer');
  const root=fs.mkdtempSync(path.join(os.tmpdir(),'wasmbench-v8-wasi-'));
  const streams=fs.mkdtempSync(path.join(os.tmpdir(),'wasmbench-v8-wasi-streams-'));
  const staged=new Map();
  const readonlyDirs=new Set([root]);
  let stdinFd,stdoutFd,stderrFd;
  try {
    for(const [name,file] of Object.entries(command.files||{})){
      if(!name||name.startsWith('/')||name.split('/').some(part=>!part||part==='.'||part==='..'))throw new Error('invalid WASI fixture path');
      const target=path.join(root,...name.split('/'));
      fs.mkdirSync(path.dirname(target),{recursive:true,mode:0o700});
      for(let directory=path.dirname(target);directory===root||directory.startsWith(root+path.sep);directory=path.dirname(directory)){readonlyDirs.add(directory);if(directory===root)break;}
      const contents=file.path?fs.readFileSync(file.path):Buffer.from(file.data||'','base64');
      if(file.size!==undefined&&contents.length!==Number(file.size))throw new Error(`WASI fixture size mismatch: ${name}`);
      if(file.sha256&&hash(contents)!==file.sha256)throw new Error(`WASI fixture digest mismatch: ${name}`);
      // Node's WASI Preview 1 guest does not see the owner-only host fixture
      // permissions as readable. Keep fixtures immutable but world-readable.
      fs.writeFileSync(target,contents,{mode:0o444,flag:'wx'});staged.set(name,target);
    }
    let input=Buffer.from(command.stdin||'','base64');
    if(command.stdin_file){const file=staged.get(command.stdin_file);if(!file)throw new Error(`WASI stdin fixture missing: ${command.stdin_file}`);input=fs.readFileSync(file);}
    const stdinPath=path.join(streams,'stdin'),stdoutPath=path.join(streams,'stdout'),stderrPath=path.join(streams,'stderr');
    fs.writeFileSync(stdinPath,input,{mode:0o600});fs.writeFileSync(stdoutPath,'',{mode:0o600});fs.writeFileSync(stderrPath,'',{mode:0o600});
    for(const directory of [...readonlyDirs].sort((a,b)=>b.length-a.length))fs.chmodSync(directory,0o555);
    stdinFd=fs.openSync(stdinPath,'r');stdoutFd=fs.openSync(stdoutPath,'r+');stderrFd=fs.openSync(stderrPath,'r+');
    // This harness only exposes pinned fixtures; Node documents that its WASI
    // preopens are not a security boundary for untrusted guest modules.
    return {root,stdinFd,stdoutFd,stderrFd,stdoutPath,stderrPath,close(){for(const fd of [stdinFd,stdoutFd,stderrFd])try{fs.closeSync(fd)}catch{};for(const directory of readonlyDirs)try{fs.chmodSync(directory,0o700)}catch{};fs.rmSync(root,{recursive:true,force:true});fs.rmSync(streams,{recursive:true,force:true});}};
  } catch(error) {
    for(const fd of [stdinFd,stdoutFd,stderrFd])if(fd!==undefined)try{fs.closeSync(fd)}catch{}
    for(const directory of readonlyDirs)try{fs.chmodSync(directory,0o700)}catch{};fs.rmSync(root,{recursive:true,force:true});fs.rmSync(streams,{recursive:true,force:true});throw error;
  }
}
function runWasiCommand(compiled,scenario) {
  const w=prep.workload,c=w.command,fixture=wasiFixture(c);
  try {
    const wasi=new WASI({version:'preview1',args:c.argv,env:{},preopens:{'/':fixture.root},stdin:fixture.stdinFd,stdout:fixture.stdoutFd,stderr:fixture.stderrFd,returnOnExit:true});
    let created,exitCode,elapsed;
    if(scenario==='instantiate'){
      const start=now();created=new WebAssembly.Instance(compiled,readonlyWasiImports(wasi));elapsed=Number(now()-start);
      exitCode=wasi.start(created);
    }else if(scenario==='first-call'){
      created=new WebAssembly.Instance(compiled,readonlyWasiImports(wasi));const start=now();exitCode=wasi.start(created);elapsed=Number(now()-start);
    }else if(scenario==='steady'){
      const start=now();created=new WebAssembly.Instance(compiled,readonlyWasiImports(wasi));exitCode=wasi.start(created);elapsed=Number(now()-start);
    }else throw new Error(`unsupported WASI command scenario: ${scenario}`);
    const stdout=fs.readFileSync(fixture.stdoutPath),stderr=fs.readFileSync(fixture.stderrPath);
    if(stdout.length>c.output_limit_bytes||stderr.length>c.output_limit_bytes)throw new Error(`WASI output exceeds ${c.output_limit_bytes} bytes per stream`);
    const result={exit_code:exitCode>>>0,stdout_sha256:hash(normalizeWasiStdout(c.stdout_normalize,stdout)),stderr_sha256:hash(stderr),stdout_bytes:stdout.length,stderr_bytes:stderr.length};
    if(c.stdout_sha256&&result.stdout_sha256!==c.stdout_sha256)throw new Error('incorrect WASI stdout digest');
    if(c.stderr_sha256&&result.stderr_sha256!==c.stderr_sha256)throw new Error('incorrect WASI stderr digest');
    if(result.exit_code!==(c.exit_code>>>0))throw new Error(`incorrect WASI exit code: ${result.exit_code}`);
    return {elapsed_ns:elapsed,verified:true,sample_type:'individual_operation',operations:1,result,command_result:result};
  } finally {fixture.close();}
}
function runWasi(r) {
  const w=prep.workload;
  if(prep.profile!=='timing'||r.phase_barriers||!Number.isInteger(r.samples)||r.samples<1||r.samples>100000)
    throw new Error('WASI Preview 1 command supports unbarriered timing batches only');
  if(!['compile','instantiate','first-call','steady'].includes(r.scenario))return {status:'unsupported',reason:'Node WASI Preview 1 exposes command execution only'};
  if(r.scenario!=='steady'&&(r.operations!==1||r.warmup!==0))throw new Error('WASI compile, instantiate and first-call require one operation and no warmup');
  if(!Number.isInteger(r.operations)||r.operations<1||r.operations>1000000||!Number.isInteger(r.warmup)||r.warmup<0||r.warmup>100000)throw new Error('invalid WASI command batch');
  const samples=[];
  if(r.scenario==='steady'){
    const compiled=module||compileModule(bytes);
    for(let i=0;i<r.warmup;i++){
      const warm=runWasiCommand(compiled,'steady');
      samples.push({index:i,warmup:true,elapsed_ns:warm.elapsed_ns,operations:1,sample_type:'individual_operation',verified:true,command_result:warm.command_result});
    }
  }
  for(let i=0;i<r.samples;i++){
    let elapsed_ns=0,command_result;
    if(r.scenario==='compile'){
      const start=now(),compiled=compileModule(bytes);elapsed_ns=Number(now()-start);
      const verification=runWasiCommand(compiled,'first-call');if(!verification.verified)throw new Error('WASI compile verification failed');command_result=verification.command_result;
    }else if(r.scenario==='steady'){
      const compiled=module||compileModule(bytes);
      for(let n=0;n<r.operations;n++){const result=runWasiCommand(compiled,'steady');elapsed_ns+=result.elapsed_ns;command_result=result.command_result;}
    }else{
      const compiled=module||compileModule(bytes),result=runWasiCommand(compiled,r.scenario);elapsed_ns=result.elapsed_ns;command_result=result.command_result;
    }
    samples.push({index:i+(r.scenario==='steady'?r.warmup:0),warmup:false,elapsed_ns,operations:r.scenario==='steady'?r.operations:1,sample_type:r.scenario==='steady'&&r.operations>1?'batch_average':'individual_operation',verified:true,command_result});
  }
  return {samples};
}
function setup() { module ??= compileModule(bytes); instance ??= initialize(construct()); }
function observation(metric, value, scope, phase) {
  return {metric, value, scope, phase, definition_version: 1, unit: 'bytes', collector: 'node:process.memoryUsage', collector_version: process.version, quality: 'engine_reported', profile: 'memory', status: 'available', normalization_denominator: 'process_snapshot'};
}
async function barrier(req,index,stage) {
  process.stdout.write(JSON.stringify({version:1,id:req.id,status:'phase',phase:{sample_index:index,stage}})+'\n');
  const next=await lines.next();
  if(next.done)throw new Error('phase acknowledgement missing');
  const ack=JSON.parse(next.value);
  if(ack.version!==1||ack.id!==req.id||ack.method!=='continue')throw new Error('invalid phase acknowledgement');
}
function runSustained(r) {
  const w=prep.workload;
  if(!r||r.scenario!=='sustained'||!['timing','memory'].includes(prep.profile)||r.phase_barriers||r.sustained_post_collection||w.abi!=='core'||w.reset!=='stateless'||w.oracle.kind!=='exact_u64'||w.oracle.float||w.command||w.vectors||w.density||w.checkpoint||!w.export||!w.oracle.expected?.length)throw new Error('unsupported sustained stateless scalar contract/profile/collection');
  for(const [key,min,max] of [['samples',2,100000],['operations',1,1000000],['warmup',0,10000],['sustained_duration_ns',1000000,3600000000000]])if(!Number.isSafeInteger(r[key])||r[key]<min||r[key]>max)throw new Error('invalid sustained fixed budget/duration');
  module=instance=undefined;
  setup();
  let fn=instance.exports[w.export];
  const args=argumentsForCall(),values=new Array(r.operations),samples=new Array(r.samples+r.warmup);
  if(typeof fn!=='function')throw new Error('missing sustained workload export');
  const epoch=now(),offset=()=>Number(now()-epoch);
  const heap=(snapshot,which,phase)=>({...observation('host.js_heap.'+which,snapshot.heapUsed,'adapter_process_v8_heap',phase),normalization_denominator:phase==='sustained'?'batch_operation_window_including_heap_snapshots_excluding_verification':'js_module_instance_export_reference_release_without_forced_gc'});
  try {
    for(let i=0;i<samples.length;i++){
      const window={start_ns:offset()};
      const before=prep.profile==='memory'?process.memoryUsage():undefined;
      const start=now();window.operation_start_ns=Number(start-epoch);
      for(let j=0;j<r.operations;j++)values[j]=fn(...args);
      const end=now();window.operation_end_ns=Number(end-epoch);
      const after=before?process.memoryUsage():undefined;window.end_ns=offset();
      let result;
      for(const value of values){result=normalize(value);verify(result);}
      const sample={index:i,warmup:i<r.warmup,elapsed_ns:Number(end-start),operations:r.operations,sample_type:r.operations===1?'individual_operation':'batch_average',verified:true,result,sustained_window:window};
      if(before){
        sample.observations=[heap(before,'start','sustained'),heap(after,'end','sustained')];
        if(instance.exports.memory)sample.observations.push({...observation('guest.memory.logical',instance.exports.memory.buffer.byteLength,'guest_linear_memory','sustained'),collector:'WebAssembly.Memory.buffer',normalization_denominator:'instance'});
      }
      samples[i]=sample;
    }
    const before=prep.profile==='memory'?process.memoryUsage():undefined,start=offset();
    fn=instance=module=undefined;
    const end=offset(),after=before?process.memoryUsage():undefined,last=samples.at(-1);
    last.sustained_release={start_ns:start,end_ns:end,closed:false,policy:'js_references_dropped'};
    if(before)last.observations.push(heap(before,'start','sustained/release_window'),heap(after,'end','sustained/release_window'));
    return {samples};
  } finally {fn=instance=module=undefined;}
}
async function runDensity(r,req) {
  const w=prep.workload,d=w.density;
  if(!d||!Number.isInteger(d.instances)||d.instances<1||d.instances>128||d.sharing!=='shared_module'||w.abi!=='core'||w.host_profile||w.command||w.vectors||w.oracle.kind!=='exact_u64'||w.oracle.float||!w.export||w.reset!=='fresh_instance_per_sample'||w.work_unit!=='instance_group'||w.units_per_invocation!==1||r.scenario!=='density'||r.operations!==1||r.warmup!==0||!Number.isInteger(r.samples)||r.samples<1||r.samples>100000||!['timing','memory'].includes(prep.profile)||(r.phase_barriers&&prep.profile!=='memory'))throw new Error('unsupported density contract: Node exposes shared_module only, no independently constructed engines');
  module=instance=undefined;
  const samples=[];
  for(let i=0;i<r.samples;i++){
    if(r.phase_barriers)await barrier(req,i,'before_density');
    const before=prep.profile==='memory'?process.memoryUsage():undefined;
    let held=[],compiled;
    const results=[];
    try {
      const start=now();
      compiled=compileModule(bytes);
      if(WebAssembly.Module.imports(compiled).length)throw new Error('density requires import-free modules');
      for(let j=0;j<d.instances;j++){
        held.push(initialize(new WebAssembly.Instance(compiled,{})));
        results.push(invoke(held[j]));
      }
      const elapsed=Number(now()-start),after=before?process.memoryUsage():undefined;
      for(let j=0;j<held.length;j++)verify(results[j],held[j]);
      const observations=[];
      if(before){
        observations.push(observation('host.js_heap.start',before.heapUsed,'adapter_process_v8_heap','density/provision_window'),observation('host.js_heap.end',after.heapUsed,'adapter_process_v8_heap','density/provision_window'));
        observations.push({...observation('density.guest_memory.logical',held.reduce((sum,m)=>sum+(m.exports.memory?.buffer.byteLength||0),0),'instance_group_linear_memory','density_ready'),collector:'WebAssembly.Memory.buffer.byteLength',normalization_denominator:'instance_group'});
      }
      if(r.phase_barriers)await barrier(req,i,'density_ready');
      held.length=0;compiled=undefined;
      if(r.phase_barriers)await barrier(req,i,'density_released');
      samples.push({index:i,warmup:false,elapsed_ns:elapsed,operations:1,sample_type:'individual_operation',verified:true,result:results[0],observations});
    } finally {held.length=0;compiled=undefined;}
  }
  return {samples};
}
async function compilePhases(req) {
  if(!['memory','counters'].includes(prep.profile)||req.run.scenario!=='compile')throw new Error('unsupported phase barrier request');
  module=instance=undefined;
  const samples=[];
  for(let i=0;i<req.run.samples;i++) {
    await barrier(req,i,'before_compile');
    const before=prep.profile==='memory'?process.memoryUsage():undefined;
    const start=now();
    let compiled=compileModule(bytes);
    const elapsed=now()-start;
    const after=prep.profile==='memory'?process.memoryUsage():undefined;
    await barrier(req,i,'compiled');
    let created=initialize(new WebAssembly.Instance(compiled,imports));
    const result=invoke(created);verify(result,created);
    created=compiled=undefined; // Drop handles; collection/reclamation is uncontrolled.
    await barrier(req,i,'released');
    samples.push({index:i,warmup:false,elapsed_ns:Number(elapsed),operations:1,sample_type:'individual_operation',verified:true,result,observations:before?[observation('host.js_heap.start',before.heapUsed,'adapter_process_v8_heap','compile/api_window'),observation('host.js_heap.end',after.heapUsed,'adapter_process_v8_heap','compile/api_window')]:[]});
  }
  return {samples};
}
async function runVectors(r,req) {
  const w=prep.workload,v=w.vectors;
  if(w.oracle.kind!=='exact_vectors'||w.reset!=='fresh_instance_per_sample'||w.input||(w.args||[]).length||(w.oracle.memory||[]).length||(w.oracle.expected||[]).length||w.oracle.output_pointer_export||w.host_profile)throw new Error('unsupported or ambiguous vector contract');
  if(!['timing','memory'].includes(prep.profile)||(r.phase_barriers&&(prep.profile!=='memory'||!['compile','instantiate','first-call','teardown'].includes(r.scenario)))||!['compile','instantiate','first-call','steady','teardown'].includes(r.scenario))throw new Error('unsupported vector scenario/profile');
  for(const [name,min,max] of [['samples',1,100000],['warmup',0,100000],['operations',1,Number.MAX_SAFE_INTEGER]])if(!Number.isSafeInteger(r[name])||r[name]<min||r[name]>max)throw new Error('invalid vector batch');
  const mod=v.mod||0;
  if(!Array.isArray(v.cases)||!v.cases.length||!Number.isInteger(v.output_len)||v.output_len<1||v.output_len>0xffffffff||!Number.isSafeInteger(mod)||mod<0||!Number.isSafeInteger(w.vector_byte_budget)||w.vector_byte_budget<0)throw new Error('invalid vector dimensions/budget');
  for(const offset of [v.input_offset,v.output_offset])if(!Number.isInteger(offset)||offset<0||offset>0xffffffff)throw new Error('invalid vector offset');
  let total=0;
  for(const c of v.cases){
    if(!Number.isInteger(c.len)||c.len<0||c.len>0xffffffff||typeof c.out!=='string'||c.out.length!==v.output_len*2||!/^(?:[0-9a-fA-F]{2})+$/.test(c.out))throw new Error('invalid vector case');
    total+=c.len+v.output_len;
    if(!Number.isSafeInteger(total)||total>w.vector_byte_budget)throw new Error('vector input/oracle byte budget exceeded');
  }
  const cases=v.cases.map(c=>{const input=Buffer.alloc(c.len);if(mod>0)for(let j=0;j<input.length;j++)input[j]=(j%mod)&255;return {input,expected:Buffer.from(c.out,'hex')};});
  const shared=['compile','teardown'].includes(r.scenario)?undefined:compileModule(bytes);
  const timedCalls=['first-call','steady'].includes(r.scenario),warmup=r.scenario==='steady'?r.warmup:0,samples=[];
  for(let i=-warmup;i<r.samples;i++){
    if(r.phase_barriers&&r.scenario==='compile')await barrier(req,i,'before_compile');
    let before=prep.profile==='memory'?process.memoryUsage():undefined;
    let compiled=shared,elapsed=0n;
    if(['compile','teardown'].includes(r.scenario)){const start=now();compiled=compileModule(bytes);elapsed=now()-start;}
    const compileAfter=r.phase_barriers&&r.scenario==='compile'?process.memoryUsage():undefined;
    if(r.phase_barriers&&r.scenario==='compile')await barrier(req,i,'compiled');
    if(r.phase_barriers&&r.scenario==='instantiate'){
      await barrier(req,i,'before_instantiate');
      before=process.memoryUsage();
    }
    const start=now();
    let target=new WebAssembly.Instance(compiled,imports);
    if(r.scenario==='instantiate')elapsed=now()-start;
    const instantiateAfter=r.phase_barriers&&r.scenario==='instantiate'?process.memoryUsage():undefined;
    if(r.phase_barriers&&r.scenario==='instantiate')await barrier(req,i,'instantiated');
    initialize(target);
    let fn=target.exports[w.export];
    if(typeof fn!=='function'||!target.exports.memory)throw new Error('missing vector export/memory');
    const pointer=(name,offset)=>{
      if(!name)return offset;
      const f=target.exports[name];if(typeof f!=='function')throw new Error('missing vector pointer');
      const value=f();
      if(typeof value!=='number'||!Number.isInteger(value)||value < -2147483648||value>4294967295)throw new Error('invalid wasm32 vector pointer');
      return value>>>0;
    };
    const input=pointer(v.input_ptr_export,v.input_offset),output=pointer(v.output_ptr_export,v.output_offset);
    let callAfter;
    for(const [index,c] of cases.entries()){
      let memory=target.exports.memory.buffer;
      if(input+c.input.length>0x100000000||output+c.expected.length>0x100000000||input+c.input.length>memory.byteLength)throw new Error('vector input outside memory');
      new Uint8Array(memory,input,c.input.length).set(c.input);
      if(r.phase_barriers&&r.scenario==='first-call'&&index===0){
        await barrier(req,i,'before_first_call');
        before=process.memoryUsage();
      }
      if(timedCalls){const start=now();fn(input,c.input.length,output);elapsed+=now()-start;}else fn(input,c.input.length,output);
      if(r.phase_barriers&&r.scenario==='first-call'&&index===cases.length-1){
        callAfter=process.memoryUsage();
        await barrier(req,i,'first_call_returned');
      }
      memory=target.exports.memory.buffer;
      if(output+c.expected.length>memory.byteLength||!Buffer.from(memory,output,c.expected.length).equals(c.expected))throw new Error(`incorrect result: vector ${index} memory mismatch`);
    }
    const sample={index:i+warmup,warmup:i<0,elapsed_ns:Number(elapsed),operations:1,sample_type:timedCalls?'sequence_call_sum':'individual_operation',verified:true};
    if(before){
      const after=compileAfter||instantiateAfter||callAfter||process.memoryUsage(),phase=r.phase_barriers?(r.scenario==='first-call'?'first-call/vector_sequence_call_window':r.scenario+'/api_window'):r.scenario+'/vector_lifecycle';
      sample.observations=[observation('host.js_heap.start',before.heapUsed,'adapter_process_v8_heap',phase),observation('host.js_heap.end',after.heapUsed,'adapter_process_v8_heap',phase),{...observation('guest.memory.logical',target.exports.memory.buffer.byteLength,'guest_linear_memory',r.scenario+'/vector_verified'),collector:'WebAssembly.Memory.buffer',normalization_denominator:'instance'}];
      if(r.phase_barriers&&r.scenario==='first-call')for(const o of sample.observations)if(o.scope==='adapter_process_v8_heap')o.normalization_denominator='ordered_calls_with_intercase_input_and_verification_excluding_last_oracle_release_and_barriers';
    }
    if(r.scenario==='teardown') {
      if(r.phase_barriers)await barrier(req,i,'before_teardown');
      before=prep.profile==='memory'?process.memoryUsage():undefined;
      const start=now();
      target=fn=compiled=undefined;
      sample.elapsed_ns=Number(now()-start);
      if(before) {
        const after=process.memoryUsage();
        sample.observations=sample.observations.filter(o=>o.metric==='guest.memory.logical');
        sample.observations.push(observation('host.js_heap.start',before.heapUsed,'adapter_process_v8_heap','teardown/api_window'),observation('host.js_heap.end',after.heapUsed,'adapter_process_v8_heap','teardown/api_window'));
      }
    } else target=fn=compiled=undefined; // No forced GC or cache reclamation.
    if(r.phase_barriers)await barrier(req,i,r.scenario==='teardown'?'torn_down':r.scenario==='instantiate'?'instance_released':r.scenario==='first-call'?'first_call_released':'released');
    samples.push(sample);
  }
  return {samples};
}
async function runAppInit(r,req) {
  const w=prep.workload;
  if(!w.initialize||w.abi!=='core'||w.oracle.kind!=='exact_u64'||w.vectors||w.command||(r.phase_barriers&&prep.profile!=='memory')||!['timing','memory'].includes(prep.profile)||!Number.isInteger(r.samples)||r.samples>100000)throw new Error('unsupported app-init contract/profile/batch');
  const compiled=compileModule(bytes),samples=[];
  for(let i=0;i<r.samples;i++) {
    let target=new WebAssembly.Instance(compiled,imports),init=target.exports[w.initialize];
    if(typeof init!=='function'||init.length!==0)throw new Error('initializer must be () -> ()');
    if(r.phase_barriers)await barrier(req,i,'before_app_init');
    const before=prep.profile==='memory'?process.memoryUsage():undefined;
    const start=now();
    const value=init();
    const elapsed=now()-start;
    const after=before?process.memoryUsage():undefined;
    if(value!==undefined)throw new Error('initializer must be () -> ()');
    const observations=before?[observation('host.js_heap.start',before.heapUsed,'adapter_process_v8_heap','app-init/api_window'),observation('host.js_heap.end',after.heapUsed,'adapter_process_v8_heap','app-init/api_window')]:[];
    if(before&&target.exports.memory)observations.push({...observation('guest.memory.logical',target.exports.memory.buffer.byteLength,'guest_linear_memory','app-init/initialized'),collector:'WebAssembly.Memory.buffer',normalization_denominator:'instance'});
    if(r.phase_barriers)await barrier(req,i,'app_initialized');
    applyInput(target);
    const result=invoke(target);verify(result,target);
    target=init=undefined;
    if(r.phase_barriers)await barrier(req,i,'app_released');
    samples.push({index:i,warmup:false,elapsed_ns:Number(elapsed),operations:1,sample_type:'individual_operation',verified:true,result,observations});
  }
  return {samples};
}
async function steadyCounters(r,req) {
  const compiled=compileModule(bytes),samples=[];
  const target=initialize(new WebAssembly.Instance(compiled,imports));
  const fn=target.exports[prep.workload.export],args=argumentsForCall();
  if(typeof fn!=='function')throw new Error('missing workload export');
  for(let i=0;i<r.samples+r.warmup;i++) {
    const values=new Array(r.operations);
    let callError;
    await barrier(req,i,'before_steady_batch');
    const start=now();
    try { for(let j=0;j<r.operations;j++)values[j]=fn(...args); } catch(error) { callError=error; }
    const elapsed=now()-start;
    await barrier(req,i,'steady_batch_returned');
    if(callError)throw callError;
    let result;
    for(const value of values){result=normalize(value);verify(result,target);}
    await barrier(req,i,'steady_batch_verified');
    samples.push({index:i,warmup:i<r.warmup,elapsed_ns:Number(elapsed),operations:r.operations,sample_type:r.operations===1?'individual_operation':'batch_average',verified:true,result});
  }
  return {samples};
}
async function firstCallCounters(r,req) {
  const compiled=compileModule(bytes),samples=[];
  for(let i=0;i<r.samples;i++) {
    let target=initialize(new WebAssembly.Instance(compiled,imports));
    let fn=target.exports[prep.workload.export];
    if(typeof fn!=='function')throw new Error('missing workload export');
    const args=argumentsForCall();
    let value,callError;
    await barrier(req,i,'before_first_call');
    const start=now();
    try { value=fn(...args); } catch(error) { callError=error; }
    const elapsed=now()-start;
    await barrier(req,i,'first_call_returned');
    if(callError)throw callError;
    const result=normalize(value);verify(result,target);
    target=fn=undefined;
    await barrier(req,i,'first_call_released');
    samples.push({index:i,warmup:false,elapsed_ns:Number(elapsed),operations:1,sample_type:'individual_operation',verified:true,result});
  }
  return {samples};
}
async function instantiatePhases(r,req) {
  const w=prep.workload;
  if(w.abi!=='core'||!['exact_u64','float_bits_v1'].includes(w.oracle.kind)||w.vectors||w.command||!['memory','counters'].includes(prep.profile)||!['stateless','fresh_instance_per_sample'].includes(w.reset)||!Number.isInteger(r.samples)||r.samples<1||r.samples>100000)throw new Error('unsupported instantiation barriers');
  const compiled=compileModule(bytes),samples=[];
  for(let i=0;i<r.samples;i++) {
    await barrier(req,i,'before_instantiate');
    const before=prep.profile==='memory'?process.memoryUsage():undefined,start=now();
    let target=new WebAssembly.Instance(compiled,imports);
    const elapsed=now()-start,after=prep.profile==='memory'?process.memoryUsage():undefined;
    const observations=before?[observation('host.js_heap.start',before.heapUsed,'adapter_process_v8_heap','instantiate/api_window'),observation('host.js_heap.end',after.heapUsed,'adapter_process_v8_heap','instantiate/api_window')]:[];
    if(prep.profile==='memory'&&target.exports.memory)observations.push({...observation('guest.memory.logical',target.exports.memory.buffer.byteLength,'guest_linear_memory','instantiate/instantiated'),collector:'WebAssembly.Memory.buffer',normalization_denominator:'instance'});
    await barrier(req,i,'instantiated');
    initialize(target);
    const result=invoke(target);verify(result,target);
    target=undefined;
    await barrier(req,i,'instance_released');
    samples.push({index:i,warmup:false,elapsed_ns:Number(elapsed),operations:1,sample_type:'individual_operation',verified:true,result,observations});
  }
  return {samples};
}
function runTraps(r) {
  const w=prep.workload, codes={unreachable:'unreachable','memory access out of bounds':'memory_out_of_bounds','divide by zero':'integer_divide_by_zero','divide result unrepresentable':'integer_overflow'};
  if(!Object.values(codes).includes(w.oracle.expected_trap)||w.abi!=='core'||w.reset!=='fresh_instance_per_sample'||!w.export||w.host_profile||w.initialize||w.input||w.command||w.vectors||(w.args||[]).length||(w.oracle.expected||[]).length||(w.oracle.memory||[]).length||w.oracle.output_pointer_export)throw new Error('unsupported or ambiguous invocation-trap contract');
  if(!['timing','memory'].includes(prep.profile)||r.phase_barriers||!['first-call','steady'].includes(r.scenario)||!Number.isInteger(r.samples)||r.samples<1||r.samples>100000||!Number.isInteger(r.warmup)||r.warmup<0||r.warmup>100000||!Number.isInteger(r.operations)||r.operations<1||r.operations>1000000)throw new Error('unsupported trap scenario/profile/batch');
  const compiled=compileModule(bytes),warmup=r.scenario==='steady'?r.warmup:0,samples=[];
  for(let i=0;i<r.samples+warmup;i++){
    const target=new WebAssembly.Instance(compiled,Object.create(null)),f=target.exports[w.export];
    if(typeof f!=='function'||f.length!==0)throw new Error('missing or parameterized trap export');
    let error;
    const before=prep.profile==='memory'?process.memoryUsage():null;
    const start=now();try{f();}catch(e){error=e;}const elapsed=now()-start;
    const after=before?process.memoryUsage():null,observations=[];
    if(before){
      observations.push(observation('host.js_heap.start',before.heapUsed,'adapter_process_v8_heap',r.scenario+'/trap_api_window'),observation('host.js_heap.end',after.heapUsed,'adapter_process_v8_heap',r.scenario+'/trap_api_window'));
      if(target.exports.memory instanceof WebAssembly.Memory)observations.push({...observation('guest.memory.logical',target.exports.memory.buffer.byteLength,'guest_linear_memory',r.scenario+'/after_trap'),collector:'WebAssembly.Memory.buffer',normalization_denominator:'instance'});
    }
    const code=error instanceof WebAssembly.RuntimeError?codes[error.message]:undefined;
    if(code!==w.oracle.expected_trap)throw new Error(`incorrect result: expected invocation trap ${w.oracle.expected_trap}, got ${error}`);
    samples.push({index:i,warmup:i<warmup,elapsed_ns:Number(elapsed),operations:1,sample_type:'individual_operation',verified:true,observations,trap_result:{code,source:`v8/${process.versions.v8}/RuntimeError-exact-message-v1`,message:error.message}});
  }
  return {samples};
}
async function handle(req,profiled=false) {
  if (req.version !== 1) throw new Error('unsupported protocol version');
  if(req.method==='run'&&prep?.profile==='profiling'&&!profiled)return profileRun(prep,req.run,()=>handle(req,true));
  switch (req.method) {
    case 'describe': {
      if(compilerMode!=='production-default')compilerModeProbe??=verifyCompilerMode(compilerMode);
      return {description: compilerDescription({
      validator_features: validatorFeatures, runtime: 'v8', runtime_version: process.versions.v8, backend: compilerMode==='production-default'?'production-default-tiering':compilerMode, embedding: 'Node.js WebAssembly API', build: process.version,
      effective_configuration: {
        wasi_preview1_policy:'Node node:wasi Preview 1 host; command argv and pinned files only; fresh per-sample instance; output captured through adapter-owned files; timing excludes fixture staging and verification; steady samples use a fresh instance per command operation; Node does not promise a secure WASI filesystem sandbox',
        vector_instantiate_phases_policy:'fresh JS instance; prepared module/imports retained; Wasm start included; explicit initialization and ordered vector verification outside API window; release drops instance/export references, not physical reclamation; no forced GC',
        vector_first_call_phases_policy:'fresh initialized instance; pointers and first input prepared before before_first_call; first_call_returned follows last ordered call before its output oracle; memory window includes intercase input writes and oracle checks, unlike sequence_call_sum timers; final oracle and verified instance release follow returned barrier; compiled module retained; no forced reclamation',
        harness_calibration_policy:harness.policy,
        sustained_policy:'fixed batches on one fresh initialized JS module/instance; invocation one retained, explicit warmup only; arguments/results prepared before timers; every result verified after timers and heap snapshots; monotonic session windows; cumulative measured non-warmup API target; production-default tiering, engine/code cache/background compilation uncontrolled and tiers unobserved; final release drops module/instance/export JS references only, not engine disposal or physical reclamation; no forced GC',
        cpu_profile_policy:'Node inspector local session, no listening port; 1000us requested sampling interval; V8 isolate stacks over complete run request including setup/warmup/verification; not all-process threads or guest-only CPU; native profile JSON retained with no time-delta conversion to CPU totals',
        counter_steady_policy:'stateless exact scalar; fresh module wrapper/initialized instance per request retained across explicit warmup and measured batches; process engine/cache/GC uncontrolled; arguments and result container prepared before collection, every result normalized/verified afterward; no hidden pre-call or snapshots; raw counts per batch include embedding, background compiler and barrier transport',
        counter_first_call_policy:'fresh initialized instance per sample; module retained, engine/cache/GC uncontrolled; export and arguments resolved before collection; exactly one requested call; result normalization/verification/reference release after; no memory snapshots; background compiler and transport work included in cgroup window',
        counter_policy:'core exact scalar compile/instantiate phase handshakes; one operation, no warmup; no memory snapshots; external cgroup window includes transport, adapter and background compiler work; verification and reference release excluded; API return is not proof of final tier completion; engine/cache/GC remain uncontrolled',
        density_policy:'shared_module only; one new JS Module wrapper and fresh simultaneous instances per sample; process V8 engine reused, internal code cache uncontrolled; timer includes compile/instantiate/start/init/input/invoke, not engine construction; release drops JS references only, no forced GC or engine disposal',node: process.version, flags: JSON.stringify(process.execArgv), module_cache: 'uncontrolled', tiering: 'production-default', instantiate_release_policy: 'drop verified instance references; compiled module retained; no engine disposal or forced GC', app_init_release_policy: 'drop verified instance and initializer references; compiled module retained; no engine disposal or forced GC', teardown_policy: 'verify each fresh instance before timing JS instance/module reference release; no engine disposal or forced GC; reclamation unobserved'},
      capabilities: {'can_host_profile_js-string-builtins-v1':stringBuiltinsAvailable(),'can_host_profile_threads-defined-v1':true,'can_vector_first-call_phases':true,can_vector_instantiate_phases:true,can_control_compiler_mode:false,can_sustained_execution:true,can_sustained_post_collection:false,can_profile_v8_cpu_steady:true,can_counter_steady:true,'can_counter_first-call':true,can_counter_compile:true,can_counter_instantiate:true,can_density:true,can_density_separate_engines:false,can_float_teardown: true, can_float_phases: true, can_float_trajectory: true, can_verify_float_bits_v1: true, can_verify_invocation_traps: true, can_measure_invocation_traps: true, can_run_vectors: true, can_run_commands:true, can_vector_compile_phases: true, can_vector_teardown_phases: true, can_compile_separately: true, can_instantiate_separately: true, can_disable_code_cache: false, can_observe_tiers: false, can_export_native_code: false, can_measure_host_allocations: false, can_snapshot: false},
      scenarios, phase_barrier_scenarios:['compile','teardown','app-init','instantiate','density','first-call','steady'], phase_release_policy:'drop module and instance JS references; engine caches and garbage collection uncontrolled; no forced GC', abis: ['core','wasi-command'], features: ['mvp', 'bulk-memory', 'simd', 'reference-types', 'multi-value','wasi-preview1']
      })}; }
    case 'prepare':
      if(compilerMode!=='production-default')compilerModeProbe??=verifyCompilerMode(compilerMode);
      prep = req.prepare;
      floatSignature=undefined; module=instance=undefined;
      const wasiWorkload=prep.workload.abi==='wasi-command';
      if(prep.workload.host_profile&&!['identity-v1','assemblyscript-abort-v1','js-string-builtins-v1','threads-defined-v1','wasi-preview1-readonly-v1'].includes(prep.workload.host_profile))throw new Error('unsupported host profile');
      imports=Object.create(null);
      if(prep.workload.host_profile==='identity-v1')imports.wasmbench=Object.assign(Object.create(null),{identity:v=>v});
      if(prep.workload.host_profile==='assemblyscript-abort-v1')imports.env=Object.assign(Object.create(null),{abort:(message,file,line,column)=>{throw new Error(`AssemblyScript abort: message_ptr=${message>>>0} file_ptr=${file>>>0} line=${line>>>0} column=${column>>>0}`);}});
      if(wasiWorkload){
        if(prep.profile!=='timing'||prep.workload.host_profile!=='wasi-preview1-readonly-v1'||prep.workload.oracle.kind!=='exact_command'||!prep.workload.command||prep.workload.reset!=='fresh_instance_per_sample'||!['','llvm-ir-preds'].includes(prep.workload.command.stdout_normalize || ''))
          return {status:'unsupported',reason:'Node WASI Preview 1 adapter supports timing only for exact-command, fresh-instance workloads without stdout normalization'};
      }else if (prep.workload.abi !== 'core' || !['stateless','fresh_instance_per_sample'].includes(prep.workload.reset)) throw new Error('unsupported ABI or reset policy');
      bytes = fs.readFileSync(prep.artifact);
      if (hash(bytes) !== prep.artifact_sha256) throw new Error('artifact digest mismatch');
      if(prep.workload.oracle.kind==='float_bits_v1') {
        floatSignature=numericSignature(bytes,prep.workload.export);
        validateFloat(prep.workload.oracle,floatSignature.results);
      }
      module = instance = undefined;
      return {};
    case 'run': {
      if (!prep) throw new Error('prepare required');
      const r = req.run;
      if(r?.scenario==='harness-calibration'){harness.validate(prep,r);setup();verify(invoke());return harness.run(r);}
      if(r?.scenario==='sustained')return runSustained(r);
      if(prep.workload.abi==='wasi-command')return runWasi(r);
      if(prep.profile==='counters') {
        const w=prep.workload;
        if(w.abi!=='core'||w.command||w.vectors||w.density||w.oracle.kind!=='exact_u64'||!['stateless','fresh_instance_per_sample'].includes(w.reset)||!w.export)throw new Error('counters require a core scalar exact oracle');
        if(!r||!['compile','instantiate','first-call','steady'].includes(r.scenario)||r.phase_barriers!==true||!Number.isInteger(r.samples)||r.samples<1||r.samples>100000||!Number.isInteger(r.operations)||r.operations<1||r.operations>1000000||!Number.isInteger(r.warmup)||r.warmup<0||r.warmup>100000)throw new Error('counter batches require phase barriers and bounded budgets');
        if(r.scenario==='steady'&&w.reset!=='stateless')throw new Error('steady counters require stateless repeated invocations');
        if(r.scenario!=='steady'&&(r.operations!==1||r.warmup!==0))throw new Error('non-steady counters require one operation and no warmup');
        if(r.scenario==='first-call')return firstCallCounters(r,req);
        if(r.scenario==='steady')return steadyCounters(r,req);
      }
      if(r.scenario==='density'||prep.workload.density)return runDensity(r,req);
      if(floatSignature&&(prep.workload.command||prep.workload.vectors||(r.phase_barriers&&(prep.profile!=='memory'||!['compile','instantiate','teardown'].includes(r.scenario)))||!['timing','memory'].includes(prep.profile)||!['compile','instantiate','first-call','steady','trajectory','teardown'].includes(r.scenario)))throw new Error('unsupported float scenario/profile');
      if(r.scenario==='trajectory'){
        const w=prep.workload;
        if(prep.profile!=='timing'||r.phase_barriers||w.abi!=='core'||w.reset!=='stateless'||!['exact_u64','float_bits_v1'].includes(w.oracle.kind)||w.command||w.vectors||!w.export)throw new Error('unsupported trajectory contract');
        if(!Number.isInteger(r.samples)||r.samples<1||r.samples>100000||!Number.isInteger(r.warmup)||r.warmup<0||r.warmup>100000||!Number.isInteger(r.operations)||r.operations<1||r.operations>1000000)throw new Error('invalid trajectory batch');
        module=instance=undefined;
      }
      if(prep.workload.oracle.kind==='expected_trap')return runTraps(r);
      if (!scenarios.includes(r.scenario)) return {status: 'unsupported', reason: 'scenario not exposed by Node embedding'};
      if (r.samples < 1 || r.operations < 1 || r.warmup < 0) throw new Error('invalid batch');
      if(r.scenario==='app-init')return runAppInit(r,req);
      if(prep.workload.vectors)return runVectors(r,req);
      if(r.scenario==='instantiate'&&r.phase_barriers)return instantiatePhases(r,req);
      if(r.phase_barriers&&r.scenario!=='teardown')return compilePhases(req);
      if(r.phase_barriers&&prep.profile!=='memory')throw new Error('unsupported teardown barriers');
      setup();
      if (!['first-call', 'trajectory', 'teardown'].includes(r.scenario)) verify(invoke());
      const samples = [], warmup = ['steady','trajectory'].includes(r.scenario) ? r.warmup : 0;
      for (let i = -warmup; i < r.samples; i++) {
        const operations = ['first-call', 'trajectory', 'teardown'].includes(r.scenario) || prep.workload.reset==='fresh_instance_per_sample' ? 1 : r.operations;
        if (r.scenario === 'first-call'||(r.scenario==='steady'&&prep.workload.reset==='fresh_instance_per_sample')) instance = initialize(construct());
        let elapsed = 0n, result;
        if (r.scenario === 'teardown') { setup(); result = invoke(); verify(result); }
        if(r.phase_barriers)await barrier(req,i,'before_teardown');
        const before = prep.profile === 'memory' ? process.memoryUsage() : undefined;
        if (['steady', 'trajectory', 'first-call'].includes(r.scenario)) {
          const results = new Array(operations);
          const fn = instance.exports[prep.workload.export], args = argumentsForCall();
          const start = now();
          for (let j = 0; j < operations; j++) results[j] = fn(...args);
          elapsed = now() - start;
          for (const v of results) {
            result = normalize(v);
            verify(result);
          }
        } else {
          for (let j = 0; j < operations; j++) {
            const start = now();
            switch (r.scenario) {
              case 'compile': { const compiled = compileModule(bytes); elapsed += now() - start; const created=initialize(new WebAssembly.Instance(compiled,imports));verify(invoke(created),created); break; }
              case 'instantiate': { const created = construct(); elapsed += now() - start; initialize(created);verify(invoke(created),created); break; }
              case 'teardown': instance = module = undefined; elapsed += now() - start; break;
            }
          }
        }
        const sample = {index: i + warmup, warmup: i < 0, elapsed_ns: Number(elapsed), operations, sample_type: operations === 1 ? 'individual_operation' : 'batch_average', verified: true, result};
        if (before) {
          const after = process.memoryUsage();
          sample.observations = [observation('host.js_heap.start', before.heapUsed, 'adapter_process_v8_heap', r.scenario), observation('host.js_heap.end', after.heapUsed, 'adapter_process_v8_heap', r.scenario)];
          if (instance?.exports.memory) sample.observations.push({...observation('guest.memory.logical', instance.exports.memory.buffer.byteLength, 'guest_linear_memory', r.scenario), collector: 'WebAssembly.Memory.buffer', normalization_denominator: 'instance'});
        }
        if(r.phase_barriers)await barrier(req,i,'torn_down');
        samples.push(sample);
      }
      return {samples};
    }
    case 'inspect': return {diagnostics: [{metric: 'native.guest_code', definition_version: 1, unit: 'bytes', scope: 'guest_function_code', phase: 'compile', collector: 'Node.js', collector_version: process.version, quality: 'engine_reported', profile: 'code', status: 'unsupported', reason: 'standard JS API exposes neither tier nor native code', normalization_denominator: 'module'}]};
    case 'close': module = instance = bytes = prep = undefined; return {};
    default: throw new Error(`unknown method ${req.method}`);
  }
}
const lines=readline.createInterface({input:process.stdin})[Symbol.asyncIterator]();
for (;;) {
  const {value:line,done}=await lines.next();if(done)break;
  let req, response;
  try { req = JSON.parse(line); response = {version: 1, id: req.id, status: 'ok', ...await handle(req)}; }
  catch (error) { response = {version: 1, id: req?.id ?? 0, status: 'error', reason: String(error.message)}; }
  process.stdout.write(JSON.stringify(response) + '\n');
  if (req?.method === 'close') break;
}
