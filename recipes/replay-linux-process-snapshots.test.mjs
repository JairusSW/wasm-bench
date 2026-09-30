import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {spawnSync} from 'node:child_process';
import {createHash} from 'node:crypto';

function isolatedRecipe(t,overrides={}){
  const root=fs.mkdtempSync(path.join(os.tmpdir(),'wasmbench-process-replay-'));
  t.after(()=>fs.rmSync(root,{recursive:true,force:true}));
  for(const dir of ['recipes','runs','bin'])fs.mkdirSync(path.join(root,dir));
  const script=path.join(root,'recipes','replay-linux-process-snapshots.sh');
  for(const file of ['replay-linux-process-snapshots.sh','verify-process-snapshot-qualification.mjs'])fs.copyFileSync(new URL('./'+file,import.meta.url),path.join(root,'recipes',file));
  const log=path.join(root,'docker-calls');
  fs.writeFileSync(path.join(root,'bin','docker'),`#!/bin/sh
printf '%s\\n' "$*" >> "$WASMBENCH_TEST_CALL_LOG"
case "$1" in
  info) printf '%s\\n' "$WASMBENCH_TEST_DAEMON_ARCH";;
  image) case "$4" in
    '{{.Id}}') printf '%s\\n' "$WASMBENCH_TEST_IMAGE_ID";;
    '{{.Architecture}}') printf '%s\\n' "$WASMBENCH_TEST_IMAGE_ARCH";;
    '{{.Os}}') printf '%s\\n' "$WASMBENCH_TEST_IMAGE_OS";;
    *) exit 90;;
  esac;;
  *) exit 91;;
esac
`,{mode:0o755});
  const arch=process.arch==='x64'?'amd64':'arm64';
  return {root,log,run:(...args)=>spawnSync('sh',[script,...args],{encoding:'utf8',env:{...process.env,PATH:path.join(root,'bin')+path.delimiter+process.env.PATH,WASMBENCH_TEST_CALL_LOG:log,WASMBENCH_TEST_IMAGE_ID:'sha256:'+'a'.repeat(64),WASMBENCH_TEST_IMAGE_ARCH:arch,WASMBENCH_TEST_DAEMON_ARCH:arch,WASMBENCH_TEST_IMAGE_OS:'linux',...overrides}})};
}

function seedSyntheticSource(f){
  const source=path.join(f.root,'runs','source');fs.mkdirSync(source);
  const arch=process.arch==='x64'?'amd64':'arm64',bytes=Buffer.from('synthetic fixture'),sha=createHash('sha256').update(bytes).digest('hex');
  fs.writeFileSync(path.join(source,'fixture.wasm'),bytes);fs.writeFileSync(path.join(source,'qualifier'),'synthetic executable');
  const q={scope:'linux_process_cow_clone',runtime:'wasmtime',version:'46.0.1',qualification_only:true,headline_samples:false,pre_fork_threads:1,wasm_sha256:sha,architecture:arch==='arm64'?'aarch64':'x86_64',records:['cranelift','winch'].map(backend=>({backend,source_released_before_restore:true,source_after_mutation:125,restored_before_write:64,restored_after_write:125,passive_segment_probe:127,restored_memory_pages:3,restored_table_elements:3,independent_restorations:2}))};
  const structure={sha256:sha,validated:true,encoding:'core-module',analysis_version:'core-structure-v3',import_count:0,defined_functions:8,defined_globals:1,defined_memories:1,defined_tables:1,data_segments:1,element_segments:2};
  const cleanup={version:'linux-process-snapshot-cleanup-v2',qualification_only:true,parent_death:{signal:9,reaped:true},cases:['valid','exit_without_proof','partial_proof','wrong_proof','proof_then_failure','proof_then_stall','stall_without_proof'].map(mode=>({mode,accepted:mode==='valid',reaped:true}))};
  for(const [name,data] of Object.entries({'structure.json':structure,'guard.json':{qualification_only:true,multithreaded_fork_rejected:true},'cleanup.json':cleanup,'qualification.json':q,'container.json':{os:'linux',architecture:arch,image_id:'sha256:'+'a'.repeat(64)}}))fs.writeFileSync(path.join(source,name),JSON.stringify(data));
  const sealed=spawnSync(process.execPath,[path.join(f.root,'recipes','verify-process-snapshot-qualification.mjs'),source,path.join(source,'qualifier')],{encoding:'utf8'});assert.equal(sealed.status,0,sealed.stderr);fs.writeFileSync(path.join(source,'receipt.json'),sealed.stdout);
}
test('replay preserves existing output before checking source or contacting Docker',t=>{
  const f=isolatedRecipe(t),dir=path.join(f.root,'runs','existing');fs.mkdirSync(dir);fs.writeFileSync(path.join(dir,'marker'),'preserve');
  assert.equal(f.run('missing','existing').status,2);assert.equal(fs.readFileSync(path.join(dir,'marker'),'utf8'),'preserve');assert.equal(fs.existsSync(f.log),false);
});
test('replay preserves dangling output aliases',t=>{
  const f=isolatedRecipe(t),alias=path.join(f.root,'runs','alias');fs.symlinkSync('absent',alias);
  assert.equal(f.run('missing','alias').status,2);assert.equal(fs.readlinkSync(alias),'absent');assert.equal(fs.existsSync(f.log),false);
});
for(const args of [['../escape','output'],['source','../escape']])test('replay rejects traversal in '+args.join(' '),t=>{
  const f=isolatedRecipe(t);assert.equal(f.run(...args).status,2);assert.equal(fs.existsSync(f.log),false);assert.equal(fs.existsSync(path.join(f.root,'escape')),false);
});
test('invalid source fails before Docker, output creation or executable invocation',t=>{
  const f=isolatedRecipe(t),source=path.join(f.root,'runs','source');fs.mkdirSync(source);
  const marker=path.join(f.root,'should-not-execute');
  fs.writeFileSync(path.join(source,'qualifier'),`#!/bin/sh\ntouch '${marker}'\n`,{mode:0o755});
  assert.notEqual(f.run('source','output').status,0);assert.equal(fs.existsSync(marker),false);
  assert.equal(fs.existsSync(f.log),false);assert.equal(fs.existsSync(path.join(f.root,'runs','output')),false);
});
test('replay rejects source directory aliases before Docker',t=>{
  const f=isolatedRecipe(t),outside=path.join(f.root,'outside');fs.mkdirSync(outside);
  fs.symlinkSync(outside,path.join(f.root,'runs','source'));
  assert.notEqual(f.run('source','output').status,0);assert.equal(fs.existsSync(f.log),false);assert.equal(fs.existsSync(path.join(f.root,'runs','output')),false);
});
for(const [name,env] of [
  ['image substitution',{WASMBENCH_TEST_IMAGE_ID:'sha256:'+'b'.repeat(64)}],
  ['non-Linux image',{WASMBENCH_TEST_IMAGE_OS:'windows'}],
  ['emulation',{WASMBENCH_TEST_DAEMON_ARCH:process.arch==='x64'?'arm64':'amd64'}],
])test('replay rejects '+name+' before creating output or starting container',t=>{
  const f=isolatedRecipe(t,env);seedSyntheticSource(f);
  const result=f.run('source','output');assert.notEqual(result.status,0);assert.equal(fs.existsSync(path.join(f.root,'runs','output')),false);
  assert.ok(fs.existsSync(f.log),result.stderr);
  assert.ok(!fs.readFileSync(f.log,'utf8').split('\n').some(line=>line.startsWith('run ')));
});
