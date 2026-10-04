import {test} from 'node:test';
import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
import {compilerModeFlags,parseCompilerMode} from './compiler-mode.mjs';

test('controlled compiler mode requires exact flags and no environment overrides',()=>{
  assert.equal(parseCompilerMode([],[],{}),'production-default');
  for(const [mode,flags] of Object.entries(compilerModeFlags)) {
    const argv=['--compiler-mode='+mode];
    assert.equal(parseCompilerMode(argv,flags,{}),mode);
    assert.equal(parseCompilerMode(argv,[...flags,'--no-wasm-native-module-cache'],{}),mode);
    for(const invalid of [[],flags.slice(1),[...flags,'--wasm-tier-up'],[...flags,'--liftoff'],[...flags].reverse()])assert.throws(()=>parseCompilerMode(argv,invalid,{}));
    assert.throws(()=>parseCompilerMode(argv,flags,{NODE_OPTIONS:'--no-liftoff'}));
    assert.throws(()=>parseCompilerMode([...argv,...argv],flags,{}));
  }
  assert.throws(()=>parseCompilerMode(['--compiler-mode=unknown'],[],{}));
  assert.throws(()=>parseCompilerMode(['--unknown'],[],{}));
});

test('installed V8 proves requested code tier before invoking the calibration export',()=>{
  for(const [mode,flags] of Object.entries(compilerModeFlags)) {
    const program=`import {verifyCompilerMode} from ${JSON.stringify(new URL('./compiler-mode.mjs',import.meta.url).href)}; console.log(JSON.stringify(verifyCompilerMode(${JSON.stringify(mode)})));`;
    const child=spawnSync(process.execPath,[...flags,'--input-type=module','-e',program],{encoding:'utf8',timeout:10000,env:{...process.env,NODE_OPTIONS:''}});
    assert.equal(child.status,0,child.stderr);
    const evidence=JSON.parse(child.stdout);
    assert.equal(evidence.version,'v8-compiler-mode-probe-v1');
    assert.match(evidence.module_sha256,/^[a-f0-9]{64}$/);
    assert.equal(evidence.liftoff,mode==='liftoff-only');
    assert.equal(evidence.optimizing,mode==='optimizing-only');
    assert.equal(evidence.scope,'separate_calibration_module_export_before_first_call');
    assert.equal(evidence.collector_version,process.versions.v8);
  }
});

test('missing and contradicted engine inspection fails rather than trusting flag labels',()=>{
  const moduleURL=JSON.stringify(new URL('./compiler-mode.mjs',import.meta.url).href);
  for(const flags of [[],compilerModeFlags['optimizing-only']]) {
    const child=spawnSync(process.execPath,[...flags,'--input-type=module','-e',`import {verifyCompilerMode} from ${moduleURL}; verifyCompilerMode('liftoff-only');`],{encoding:'utf8',timeout:10000,env:{...process.env,NODE_OPTIONS:''}});
    assert.notEqual(child.status,0);
    assert.match(child.stderr,/inspection intrinsics|did not establish requested eager compiler mode/);
  }
});
