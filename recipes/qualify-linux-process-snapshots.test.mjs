import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {spawnSync} from 'node:child_process';

function isolatedRecipe(t, overrides={}){
  const root=fs.mkdtempSync(path.join(os.tmpdir(),'wasmbench-process-recipe-'));
  t.after(()=>fs.rmSync(root,{recursive:true,force:true}));
  for(const dir of ['recipes','runs','bin','.wasmbench/wasmtime-linux-target/process-snapshot/release'])fs.mkdirSync(path.join(root,dir),{recursive:true});
  const script=path.join(root,'recipes','qualify-linux-process-snapshots.sh');
  fs.copyFileSync(new URL('./qualify-linux-process-snapshots.sh',import.meta.url),script);
  fs.writeFileSync(path.join(root,'.wasmbench/wasmtime-linux-target/process-snapshot/release/qualify-process-snapshot'),'test qualifier');
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
  const env={...process.env,PATH:path.join(root,'bin')+path.delimiter+process.env.PATH,WASMBENCH_TEST_CALL_LOG:log,WASMBENCH_TEST_IMAGE_ID:'sha256:'+'a'.repeat(64),WASMBENCH_TEST_IMAGE_ARCH:'arm64',WASMBENCH_TEST_DAEMON_ARCH:'aarch64',WASMBENCH_TEST_IMAGE_OS:'linux',...overrides};
  return {root,log,run:(...args)=>spawnSync('sh',[script,...args],{encoding:'utf8',env})};
}

test('process qualification preserves existing evidence before Docker calls',t=>{
  const f=isolatedRecipe(t),dir=path.join(f.root,'runs','existing');fs.mkdirSync(dir);
  fs.writeFileSync(path.join(dir,'marker'),'preserve');
  assert.equal(f.run('image','existing').status,2);
  assert.equal(fs.readFileSync(path.join(dir,'marker'),'utf8'),'preserve');assert.equal(fs.existsSync(f.log),false);
});
test('process qualification preserves dangling evidence aliases',t=>{
  const f=isolatedRecipe(t),alias=path.join(f.root,'runs','alias');fs.symlinkSync('absent',alias);
  assert.equal(f.run('image','alias').status,2);assert.equal(fs.readlinkSync(alias),'absent');assert.equal(fs.existsSync(f.log),false);
});
test('process qualification rejects traversal before Docker calls',t=>{
  const f=isolatedRecipe(t);assert.equal(f.run('image','../escaped').status,2);
  assert.equal(fs.existsSync(path.join(f.root,'escaped')),false);assert.equal(fs.existsSync(f.log),false);
});
for(const [name,env,reason] of [
  ['emulation',{WASMBENCH_TEST_DAEMON_ARCH:'x86_64'},/not emulation/],
  ['non-Linux',{WASMBENCH_TEST_IMAGE_OS:'windows'},/Linux image/],
  ['mutable image identity',{WASMBENCH_TEST_IMAGE_ID:'image:latest'},/immutable image/],
  ['short image digest',{WASMBENCH_TEST_IMAGE_ID:'sha256:abc'},/immutable image/],
  ['non-hex image digest',{WASMBENCH_TEST_IMAGE_ID:'sha256:'+'z'.repeat(64)},/immutable image/],
])test('process qualification rejects '+name+' before evidence creation',t=>{
  const f=isolatedRecipe(t,env),r=f.run('image','rejected');assert.equal(r.status,2);assert.match(r.stderr,reason);
  assert.equal(fs.existsSync(path.join(f.root,'runs','rejected')),false);
  assert.ok(!fs.readFileSync(f.log,'utf8').split('\n').some(line=>line.startsWith('run ')));
});
