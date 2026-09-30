// Dedicated diagnostic adapter: exposes native testing syntax without changing
// compiler/tiering/lazy flags. Not a headline timing or general ABI adapter.
import readline from 'node:readline';
import fs from 'node:fs';
import {createHash} from 'node:crypto';
import {numericSignature} from './floats.mjs';

const traced=process.argv.length===3&&process.argv[2]==='--trace-wasm-events';
if(JSON.stringify(process.execArgv)!==JSON.stringify(['--allow-natives-syntax'])||(!traced&&process.argv.length!==2)||process.env.NODE_OPTIONS)throw new Error('tier observation requires exact native-inspection flag, known options and no NODE_OPTIONS');
const hash=b=>createHash('sha256').update(b).digest('hex');
const isWasm=new Function('fn','return %IsWasmCode(fn);');
const states=[['uncompiled',new Function('fn','return %IsUncompiledWasmFunction(fn);')],['liftoff',new Function('fn','return %IsLiftoffFunction(fn);')],['optimizing',new Function('fn','return %IsTurboFanFunction(fn);')]];
function classify(fn) {
  if(typeof fn!=='function'||!isWasm(fn))return {state:'unavailable',reason:'export is not an inspectable Wasm function'};
  const seen=states.filter(([,inspect])=>inspect(fn));
  return seen.length===1?{state:seen[0][0]}:{state:'unavailable',reason:'non-atomic intrinsic queries were inconsistent or no known code tier was present'};
}
const probe=Buffer.from('0061736d010000000105016000017f030201000707010372756e00000a0601040041070b','hex');
const calibration=new WebAssembly.Instance(new WebAssembly.Module(probe)).exports.run;
classify(calibration); // Probe inspection before first call, without locking a scheduling-dependent state.
if(calibration()!==7||!['liftoff','optimizing'].includes(classify(calibration).state))throw new Error('installed V8 lacks usable code-tier inspection');
const tracing=traced?await import('./tracing.mjs'):undefined;
const traceCalibration=traced?await tracing.traceProbe(probe,hash(probe)):undefined;
let prep,bytes;
function validate(p,r) {
  const w=p?.workload;
  if(p?.profile!=='profiling'||r?.scenario!=='trajectory'||r.operations!==1||r.phase_barriers||!w||w.abi!=='core'||w.reset!=='stateless'||w.oracle?.kind!=='exact_u64'||w.oracle.float||w.command||w.vectors||w.density||w.checkpoint||w.guest_density||w.input||w.oracle.memory?.length||w.initialize||w.host_profile||!w.export)throw new Error('unsupported tier diagnostic contract/profile');
  for(const [key,min] of [['samples',1],['warmup',0]])if(!Number.isInteger(r[key])||r[key]<min||r[key]>100000)throw new Error('invalid tier trajectory budget');
}
function run(r,onEpoch) {
  const admission=prep?.profile==='timing'&&r?.scenario==='first-call'&&r.samples===1&&r.operations===1&&r.warmup===0&&!r.phase_barriers;
  validate(admission?{...prep,profile:'profiling'}:prep,admission?{...r,scenario:'trajectory'}:r);
  if(traced&&!admission&&(r.samples+r.warmup>10000||prep.workload.export.length>1024))throw new Error('traced trajectories require at most 10000 total calls and bounded export names');
  const w=prep.workload,signature=numericSignature(bytes,w.export);
  if(signature.params.some(t=>t!=='i32')||signature.results.length!==1||signature.results[0]!=='i32'||w.args.length!==signature.params.length||w.oracle.expected.length!==1)throw new Error('tier diagnostics support i32 parameters and one i32 result');
  const module=new WebAssembly.Module(bytes);
  if(WebAssembly.Module.imports(module).length)throw new Error('tier diagnostics require an import-free module');
  const instance=new WebAssembly.Instance(module),fn=instance.exports[w.export];
  const args=w.args.map(v=>{const n=Number(v);if(!Number.isInteger(n)||n<0||n>0xffffffff)throw new Error('invalid i32 argument');return n|0;});
  const expected=Number(w.oracle.expected[0]);if(!Number.isInteger(expected)||expected<0||expected>0xffffffff)throw new Error('invalid i32 oracle');
  const epoch=process.hrtime.bigint(),offset=()=>Number(process.hrtime.bigint()-epoch);
  onEpoch?.(epoch.toString());
  const reading=()=>{const start_ns=offset(),value=classify(fn),end_ns=offset();return {start_ns,end_ns,...value};};
  const samples=new Array(r.samples+r.warmup);
  for(let i=0;i<samples.length;i++) {
    const before=admission?undefined:reading(),start=process.hrtime.bigint();
    let result,callError;
    try {result=fn(...args);} catch(error) {callError=error;}
    const end=process.hrtime.bigint(),after=admission?undefined:reading();
    // These are ordinary exact-result workloads: an unexpected guest exception
    // is a failure, not successful expected-trap evidence.
    const verified=!callError&&(result>>>0)===expected;
    const failure=callError?'guest call trapped: '+String(callError.message):!verified?'incorrect result in tier trajectory: expected '+expected+', got '+(result>>>0):'';
    samples[i]={index:i,warmup:i<r.warmup,operations:1,sample_type:'individual_operation',elapsed_ns:Number(end-start),verified,result:callError?[]:[String(result>>>0)]};
    if(!admission)samples[i].tier_window={version:2,invocation_outcome:callError?'guest_trap':verified?'returned':'oracle_mismatch',...(failure?{failure_reason:failure}:{}),module_sha256:prep.artifact_sha256,export:w.export,collector:'V8/testing-code-tier-intrinsics',collector_version:process.versions.v8,
        scope:'exported_entry_code_nonatomic_boundary_snapshots',quality:'engine_reported',invocation:i+1,before,operation_start_ns:Number(start-epoch),operation_end_ns:Number(end-epoch),after};
    if(failure)return {status:'error',reason:failure,samples:samples.slice(0,i+1)};
  }
  return {samples};
}
for await(const line of readline.createInterface({input:process.stdin})) {
  let req,response;
  try {
    req=JSON.parse(line);if(req.version!==1)throw new Error('unsupported protocol');
    let result;
    switch(req.method) {
      case 'describe': result={description:{runtime:'v8',runtime_version:process.versions.v8,backend:traced?'production-default-tiering-traced':'production-default-tiering-observed',embedding:'Node.js WebAssembly API with diagnostic native inspection',build:process.version,
        effective_configuration:{flags:JSON.stringify(process.execArgv),tier_observation:'v8-entry-tier-boundaries-v2',probe_sha256:hash(probe),probe_inspection:'verified-supported',
          policy:'one retained module/instance/export; first workload invocation retained; explicit warmup only; individual calls bracketed by non-atomic code snapshots; not actual executed-tier, internal-function coverage, retirement or completion events',module_cache:'uncontrolled',tiering:'production-default-with-native-inspection',
          ...(traced?{engine_tracing:'v8-wasm-events-v1',trace_probe:JSON.stringify(traceCalibration),trace_policy:'process-wide category events including setup and flush; no module/function attribution; 4 MiB retained prefix; no sum of nested or overlapping durations; at most 10000 calls'}:{})},
        capabilities:{can_observe_tiers:true,can_profile_tier_trajectory:true,...(traced?{can_trace_v8_wasm_events:true}:{}),can_disable_code_cache:false,can_export_native_code:false},scenarios:['trajectory','first-call'],abis:['core'],features:['mvp','bulk-memory','simd','reference-types','multi-value']}};break;
      case 'prepare': prep=req.prepare;bytes=fs.readFileSync(prep.artifact);if(hash(bytes)!==prep.artifact_sha256)throw new Error('artifact digest mismatch');result={};break;
      case 'run': result=traced&&prep?.profile==='profiling'?await tracing.traceRun(prep.artifact_sha256,onEpoch=>run(req.run,onEpoch)):run(req.run);break;
      case 'close': prep=bytes=undefined;result={};break;
      default: throw new Error('unsupported method');
    }
    response={version:1,id:req.id,status:'ok',...result};
  } catch(error) {response={version:1,id:req?.id??0,status:'error',reason:String(error.message)};}
  process.stdout.write(JSON.stringify(response)+'\n');if(req?.method==='close')break;
}
