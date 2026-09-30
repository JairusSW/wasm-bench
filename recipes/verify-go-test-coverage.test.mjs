import test from 'node:test';
import assert from 'node:assert/strict';
import {Readable} from 'node:stream';
import {spawnSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';
import {GoTestCoverage, verifyStream} from './verify-go-test-coverage.mjs';

const pkg='example/experiment';
const event=(Action,Test)=>({Action,Package:pkg,...(Test===undefined?{}:{Test})});
const passing=()=>[event('start'),event('run','TestOne'),event('output','TestOne'),event('pass','TestOne'),event('pass')];
const check=events=>{const gate=new GoTestCoverage(pkg,['TestOne']);events.forEach(e=>gate.accept(e));return gate.finish();};
test('requires actual named execution and completed package, not output text',()=>{
  assert.deepEqual(check(passing()).passed,['TestOne']);
  assert.throws(()=>check([event('start'),{...event('output'),Output:'PASS TestOne'},event('pass')]));
});
for(const [name,mutate] of Object.entries({
  empty:()=>[],
  missingRun:e=>e.filter(x=>x.Action!=='run'),
  skipped:e=>e.map(x=>x.Action==='pass'&&x.Test?{...x,Action:'skip'}:x),
  failed:e=>e.map(x=>x.Action==='pass'&&x.Test?{...x,Action:'fail'}:x),
  truncated:e=>e.slice(0,-1),
  packageNotStarted:e=>e.slice(1),
  packageFailed:e=>e.map(x=>x.Action==='pass'&&!x.Test?{...x,Action:'fail'}:x),
  anotherPackageFailed:e=>[{Action:'fail',Package:'other'},...e],
  skippedChild:e=>[...e.slice(0,3),event('skip','TestOne/backend'),...e.slice(3)],
  duplicate:e=>[...e.slice(0,3),event('run','TestOne'),...e.slice(3)],
  passAfterCompletion:e=>[...e,event('pass','TestOne')],
  wrongPackage:e=>e.map(x=>({...x,Package:'other'})),
  malformed:e=>[...e,null],
}))test('rejects '+name,()=>assert.throws(()=>check(mutate(passing()))));
test('rejects duplicate, empty, or subtest requirements',()=>{
  for(const names of [[],['TestOne','TestOne'],['TestOne/sub'],['']])assert.throws(()=>new GoTestCoverage(pkg,names));
});
test('stream verifies structured JSON and rejects log-only/malformed evidence',async()=>{
  const input=events=>Readable.from(events.map(x=>JSON.stringify(x)+'\n'));
  assert.equal((await verifyStream(input(passing()),pkg,['TestOne'])).version,'go-required-test-coverage-v1');
  await assert.rejects(verifyStream(Readable.from(['ok example/experiment\n']),pkg,['TestOne']));
});
test('CLI exits nonzero for skipped or missing acceptance and refuses unknown arguments',()=>{
  const script=fileURLToPath(new URL('./verify-go-test-coverage.mjs',import.meta.url));
  const args=[script,'--package',pkg,'--require','TestOne'];
  const invoke=(events,argv=args)=>spawnSync(process.execPath,argv,{input:events.map(x=>JSON.stringify(x)+'\n').join(''),encoding:'utf8'});
  const good=invoke(passing());
  assert.equal(good.status,0,good.stderr);
  assert.deepEqual(JSON.parse(good.stdout).passed,['TestOne']);
  assert.notEqual(invoke([event('start'),event('pass')]).status,0);
  assert.notEqual(invoke(passing().map(x=>x.Test&&x.Action==='pass'?{...x,Action:'skip'}:x)).status,0);
  assert.notEqual(invoke(passing(),[...args,'--unknown']).status,0);
});
