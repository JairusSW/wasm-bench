import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {spawnSync} from 'node:child_process';

function fixture(t,overrides={}){
  const root=fs.mkdtempSync(path.join(os.tmpdir(),'wasmbench-density-recipe-'));
  t.after(()=>fs.rmSync(root,{recursive:true,force:true}));
  for(const dir of ['recipes','runs','bin','.wasmbench/wasmtime-linux-target/process-snapshot/release'])fs.mkdirSync(path.join(root,dir),{recursive:true});
  const script=path.join(root,'recipes','test-linux-snapshot-density.sh');
  fs.copyFileSync(new URL('./test-linux-snapshot-density.sh',import.meta.url),script);
  fs.writeFileSync(path.join(root,'.wasmbench/snapshot-density.test'),'test executable');
  fs.writeFileSync(path.join(root,'.wasmbench/wasmtime-linux-target/process-snapshot/release/qualify-process-snapshot'),'worker');
  const log=path.join(root,'docker-calls');
  fs.writeFileSync(path.join(root,'bin/docker'),`#!/bin/sh
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
  const env={...process.env,PATH:path.join(root,'bin')+path.delimiter+process.env.PATH,WASMBENCH_TEST_CALL_LOG:log,WASMBENCH_TEST_IMAGE_ID:'sha256:'+'a'.repeat(64),WASMBENCH_TEST_IMAGE_ARCH:'arm64',WASMBENCH_TEST_DAEMON_ARCH:'aarch64',WASMBENCH_TEST_IMAGE_OS:'linux',...overrides};
  return {root,log,run:(name)=>spawnSync('sh',[script,'image',name],{encoding:'utf8',env})};
}

test('density recipe preserves existing evidence before Docker calls',t=>{
  const f=fixture(t),dir=path.join(f.root,'runs/existing');fs.mkdirSync(dir);fs.writeFileSync(path.join(dir,'marker'),'preserve');
  assert.equal(f.run('existing').status,2);assert.equal(fs.readFileSync(path.join(dir,'marker'),'utf8'),'preserve');assert.equal(fs.existsSync(f.log),false);
});
test('density recipe preserves dangling aliases before Docker calls',t=>{
  const f=fixture(t),alias=path.join(f.root,'runs/alias');fs.symlinkSync('absent',alias);
  assert.equal(f.run('alias').status,2);assert.equal(fs.readlinkSync(alias),'absent');assert.equal(fs.existsSync(f.log),false);
});
test('density recipe refuses traversal before Docker calls',t=>{
  const f=fixture(t);assert.equal(f.run('../escape').status,2);assert.equal(fs.existsSync(path.join(f.root,'escape')),false);assert.equal(fs.existsSync(f.log),false);
});
for(const [name,env] of [
  ['emulation',{WASMBENCH_TEST_DAEMON_ARCH:'x86_64'}],
  ['non-Linux',{WASMBENCH_TEST_IMAGE_OS:'windows'}],
  ['mutable image',{WASMBENCH_TEST_IMAGE_ID:'image:latest'}],
  ['short digest',{WASMBENCH_TEST_IMAGE_ID:'sha256:abc'}],
  ['invalid digest',{WASMBENCH_TEST_IMAGE_ID:'sha256:'+'z'.repeat(64)}],
  ['unknown daemon',{WASMBENCH_TEST_DAEMON_ARCH:'unknown'}],
])test('density recipe rejects '+name+' before evidence creation',t=>{
  const f=fixture(t,env);assert.notEqual(f.run('rejected').status,0);assert.equal(fs.existsSync(path.join(f.root,'runs/rejected')),false);
  assert.ok(!fs.readFileSync(f.log,'utf8').split('\n').some(line=>line.startsWith('run ')));
});
