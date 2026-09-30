import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {spawnSync} from 'node:child_process';
function fixture(t,recipe='test-linux-vector-lifecycle.sh') {
  const root=fs.mkdtempSync(path.join(os.tmpdir(),'wasmbench-vector-recipe-'));t.after(()=>fs.rmSync(root,{recursive:true,force:true}));
  for(const dir of ['recipes','runs','bin','.wasmbench/wasmtime-linux-target/release'])fs.mkdirSync(path.join(root,dir),{recursive:true});
  const script=path.join(root,'recipes',recipe);fs.copyFileSync(new URL('./'+recipe,import.meta.url),script);
  for(const file of ['wasmbench-linux-vector','adapter-wazero-linux-vector','adapter-wago-linux-vector','agent-cgroup-vector.test','wasmtime-linux-target/release/adapter-wasmtime','wasmtime-linux-target/release/wasm-analyze'])fs.writeFileSync(path.join(root,'.wasmbench',file),'fixture');
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
test('vector recipe preserves existing evidence and dangling aliases before Docker',t=>{
  const f=fixture(t);fs.mkdirSync(path.join(f.root,'runs/existing'));fs.writeFileSync(path.join(f.root,'runs/existing/marker'),'keep');fs.symlinkSync('missing',path.join(f.root,'runs/alias'));
  assert.equal(f.run('existing').status,2);assert.equal(f.run('alias').status,2);assert.equal(fs.existsSync(f.log),false);assert.equal(fs.readFileSync(path.join(f.root,'runs/existing/marker'),'utf8'),'keep');assert.equal(fs.readlinkSync(path.join(f.root,'runs/alias')),'missing');
});
test('vector recipe refuses traversal before Docker',t=>{const f=fixture(t);assert.equal(f.run('../escape').status,2);assert.equal(fs.existsSync(f.log),false);});
for(const [name,env] of Object.entries({emulation:{},platform:{WASMBENCH_TEST_OS:'darwin'},mutable:{WASMBENCH_TEST_IMAGE_ID:'image:latest'},short:{WASMBENCH_TEST_IMAGE_ID:'sha256:abc'},invalid:{WASMBENCH_TEST_IMAGE_ID:'sha256:'+'g'.repeat(64)}}))test('vector recipe refuses '+name+' before evidence/container creation',t=>{
  const f=fixture(t),r=f.run('new',env);assert.notEqual(r.status,0);assert.equal(fs.existsSync(path.join(f.root,'runs/new')),false);assert.ok(!fs.readFileSync(f.log,'utf8').split('\n').some(s=>s.startsWith('run ')));
});
test('Docker package includes each local V8 adapter import',()=>{
  const source=fs.readFileSync(new URL('../adapters/v8/adapter.mjs',import.meta.url),'utf8'),docker=fs.readFileSync(new URL('../Dockerfile',import.meta.url),'utf8');
  for(const match of source.matchAll(/from ['"]\.\/([^'"]+)['"]/g))assert.ok(docker.includes('COPY adapters/v8/'+match[1]+' ./adapters/v8/'+match[1]),'missing packaged dependency '+match[1]);
});
test('private cgroup recipe requires explicit opt-in before Docker or evidence creation',t=>{
  const f=fixture(t,'test-linux-vector-cgroup.sh');
  assert.equal(f.run('new',{WASMBENCH_EPHEMERAL_CGROUP_TEST:''}).status,2);
  assert.equal(fs.existsSync(f.log),false);assert.equal(fs.existsSync(path.join(f.root,'runs/new')),false);
});
test('private cgroup recipe preserves existing directories and dangling aliases',t=>{
  const f=fixture(t,'test-linux-vector-cgroup.sh');fs.mkdirSync(path.join(f.root,'runs/existing'));fs.writeFileSync(path.join(f.root,'runs/existing/marker'),'keep');fs.symlinkSync('missing',path.join(f.root,'runs/alias'));
  for(const name of ['existing','alias','../escape'])assert.equal(f.run(name,{WASMBENCH_EPHEMERAL_CGROUP_TEST:'1'}).status,2);
  assert.equal(fs.existsSync(f.log),false);assert.equal(fs.readFileSync(path.join(f.root,'runs/existing/marker'),'utf8'),'keep');
});
for(const [name,env] of Object.entries({emulation:{},platform:{WASMBENCH_TEST_OS:'darwin'},mutable:{WASMBENCH_TEST_IMAGE_ID:'image:latest'},short:{WASMBENCH_TEST_IMAGE_ID:'sha256:abc'},invalid:{WASMBENCH_TEST_IMAGE_ID:'sha256:'+'g'.repeat(64)}}))test('private cgroup recipe refuses '+name+' before container/evidence creation',t=>{
  const f=fixture(t,'test-linux-vector-cgroup.sh'),r=f.run('new',{WASMBENCH_EPHEMERAL_CGROUP_TEST:'1',...env});assert.notEqual(r.status,0);assert.equal(fs.existsSync(path.join(f.root,'runs/new')),false);assert.ok(!fs.readFileSync(f.log,'utf8').split('\n').some(s=>s.startsWith('run ')));
});
test('private cgroup recipe scopes privilege and retains replay/report validation',()=>{
  const script=fs.readFileSync(new URL('./test-linux-vector-cgroup.sh',import.meta.url),'utf8');
  assert.match(script,/--privileged --cgroupns=private --read-only/);assert.match(script,/--network none/);
  assert.doesNotMatch(script,/src=\/sys\/fs\/cgroup|--pid[ =]host|--cgroupns[ =]host|docker.sock/);
  assert.match(script,/WASMBENCH_REPLAY_VECTOR_PHASE_SMOKE=1/);assert.match(script,/sha256sum -c checksums.sha256/);
  const setup=fs.readFileSync(new URL('../agent/test-cgroup-container.sh',import.meta.url),'utf8');
  assert.match(setup,/test -f \/.dockerenv/);assert.match(setup,/0::\//);assert.match(setup,/restore-tools/);assert.match(setup,/--recorded-builder/);
});
