import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {spawnSync} from 'node:child_process';
function fixture(t) {
  const root=fs.mkdtempSync(path.join(os.tmpdir(),'wasmbench-command-recipe-'));t.after(()=>fs.rmSync(root,{recursive:true,force:true}));
  for(const dir of ['recipes','runs','bin','.wasmbench/wasmtime-linux-target/release'])fs.mkdirSync(path.join(root,dir),{recursive:true});
  const script=path.join(root,'recipes/test-linux-command-lifecycle.sh');fs.copyFileSync(new URL('./test-linux-command-lifecycle.sh',import.meta.url),script);
  for(const file of ['wasmbench-linux-command','adapter-wazero-linux-command','wasmtime-linux-target/release/adapter-wasmtime','wasmtime-linux-target/release/wasm-analyze'])fs.writeFileSync(path.join(root,'.wasmbench',file),'fixture');
  const log=path.join(root,'calls');
  fs.writeFileSync(path.join(root,'bin/docker'),`#!/bin/sh
printf '%s\\n' "$*" >> "$WASMBENCH_TEST_CALL_LOG"
case "$1" in
info) printf 'x86_64\\n';;
image) case "$4" in
'{{.Id}}') printf '%s\\n' "$WASMBENCH_TEST_IMAGE_ID";;
'{{.Os}}') printf '%s\\n' "$WASMBENCH_TEST_OS";;
'{{.Architecture}}') printf 'arm64\\n';;
*) exit 90;; esac;;
*) exit 91;; esac
`,{mode:0o755});
  const run=(name,extra={})=>spawnSync('sh',[script,'image',name],{encoding:'utf8',env:{...process.env,PATH:path.join(root,'bin')+path.delimiter+process.env.PATH,WASMBENCH_TEST_CALL_LOG:log,WASMBENCH_TEST_IMAGE_ID:'sha256:'+'a'.repeat(64),WASMBENCH_TEST_OS:'linux',...extra}});return {root,log,run};
}
test('command recipe preserves existing evidence and dangling aliases before Docker',t=>{
  const f=fixture(t);fs.mkdirSync(path.join(f.root,'runs/existing'));fs.writeFileSync(path.join(f.root,'runs/existing/marker'),'keep');fs.symlinkSync('missing',path.join(f.root,'runs/alias'));
  assert.equal(f.run('existing').status,2);assert.equal(f.run('alias').status,2);assert.equal(fs.existsSync(f.log),false);assert.equal(fs.readFileSync(path.join(f.root,'runs/existing/marker'),'utf8'),'keep');assert.equal(fs.readlinkSync(path.join(f.root,'runs/alias')),'missing');
});
test('command recipe refuses traversal before Docker',t=>{const f=fixture(t);assert.equal(f.run('../escape').status,2);assert.equal(fs.existsSync(f.log),false);});
for(const [name,env] of Object.entries({emulation:{},platform:{WASMBENCH_TEST_OS:'darwin'},mutable:{WASMBENCH_TEST_IMAGE_ID:'image:latest'},short:{WASMBENCH_TEST_IMAGE_ID:'sha256:abc'},invalid:{WASMBENCH_TEST_IMAGE_ID:'sha256:'+'g'.repeat(64)}}))test('command recipe refuses '+name+' before evidence/container creation',t=>{
  const f=fixture(t),r=f.run('new',env);assert.notEqual(r.status,0);assert.equal(fs.existsSync(path.join(f.root,'runs/new')),false);assert.ok(!fs.readFileSync(f.log,'utf8').split('\n').some(s=>s.startsWith('run ')));
});
