import {createHash} from 'node:crypto';

export const compilerModeFlags = Object.freeze({
  'liftoff-only': Object.freeze(['--allow-natives-syntax','--liftoff-only','--no-wasm-tier-up','--no-wasm-lazy-compilation']),
  'optimizing-only': Object.freeze(['--allow-natives-syntax','--no-liftoff','--no-wasm-tier-up','--no-wasm-lazy-compilation']),
});

// No imports/start function; this distinct calibration module is not a workload
// and never executes any workload export. Native intrinsics inspect its code.
const probeBytes = Buffer.from('0061736d010000000105016000017f030201000707010372756e00000a0601040041070b','hex');

export function parseCompilerMode(argv, flags, environment) {
  if(argv.length===0)return 'production-default';
  if(argv.length!==1||!argv[0].startsWith('--compiler-mode='))throw new Error('expected one --compiler-mode option');
  const mode=argv[0].slice('--compiler-mode='.length),expected=compilerModeFlags[mode];
  if(!expected)throw new Error('unknown V8 compiler mode');
  if(environment.NODE_OPTIONS)throw new Error('controlled compiler mode refuses NODE_OPTIONS overrides');
  if(JSON.stringify(flags)!==JSON.stringify(expected))throw new Error('controlled compiler mode requires the exact verified Node/V8 flag sequence');
  return mode;
}

export function verifyCompilerMode(mode) {
  if(mode==='production-default')return null;
  if(!compilerModeFlags[mode])throw new Error('unknown V8 compiler mode');
  let baseline,optimized;
  try {
    baseline=new Function('fn','return %IsLiftoffFunction(fn);');
    optimized=new Function('fn','return %IsTurboFanFunction(fn);');
  } catch(error) {throw new Error('installed V8 lacks compiler-mode inspection intrinsics: '+error.message);}
  const module=new WebAssembly.Module(probeBytes),instance=new WebAssembly.Instance(module);
  const fn=instance.exports.run;
  const liftoff=baseline(fn),optimizing=optimized(fn);
  if(typeof liftoff!=='boolean'||typeof optimizing!=='boolean'||liftoff==optimizing||liftoff!==(mode==='liftoff-only'))throw new Error('installed V8 did not establish requested eager compiler mode');
  if(fn()!==7)throw new Error('compiler-mode probe returned an incorrect result');
  return {version:'v8-compiler-mode-probe-v1',module_sha256:createHash('sha256').update(probeBytes).digest('hex'),
    collector:'V8/%IsLiftoffFunction/%IsTurboFanFunction',collector_version:process.versions.v8,
    scope:'separate_calibration_module_export_before_first_call',liftoff,optimizing,
    policy:'eager compilation; tier-up disabled; calibration only, not workload tier coverage or background-code completion'};
}
