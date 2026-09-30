import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {spawnSync} from 'node:child_process';

function recipe(t) {
  const root=fs.mkdtempSync(path.join(os.tmpdir(),'wasmbench-p2-recipe-'));
  t.after(()=>fs.rmSync(root,{recursive:true,force:true}));
  for(const dir of ['recipes','runs','bin','.wasmbench/wasmtime-linux-target/release','.wasmbench/p2-reset-fixture'])fs.mkdirSync(path.join(root,dir),{recursive:true});
  const script=path.join(root,'recipes','test-linux-p2.sh');fs.copyFileSync(new URL('./test-linux-p2.sh',import.meta.url),script);
  for(const file of ['.wasmbench/wasmbench-linux-p2','.wasmbench/wasmtime-linux-target/release/adapter-wasmtime','.wasmbench/wasmtime-linux-target/release/wasm-analyze','.wasmbench/p2-reset-fixture/p2-filesystem-reset.component.wasm','.wasmbench/p2-reset-fixture/compiler.txt'])fs.writeFileSync(path.join(root,file),'fixture');
  const log=path.join(root,'docker-calls');
  fs.writeFileSync(path.join(root,'bin','docker'),`#!/bin/sh
printf '%s\\n' "$*" >> "$WASMBENCH_TEST_CALL_LOG"
case "$1" in
  info) printf 'x86_64\\n';;
  image) case "$4" in
    '{{.Id}}') printf 'sha256:fixture\\n';;
    '{{.Architecture}}') printf 'arm64\\n';;
    '{{.Os}}') printf 'linux\\n';;
    *) exit 90;;
  esac;;
  *) exit 91;;
esac
`,{mode:0o755});
  const run=name=>spawnSync('sh',[script,'image',name],{encoding:'utf8',env:{...process.env,PATH:path.join(root,'bin')+path.delimiter+process.env.PATH,WASMBENCH_TEST_CALL_LOG:log}});
  return {root,log,run};
}
test('existing evidence is preserved before any Docker calls',t=>{
  const f=recipe(t);fs.mkdirSync(path.join(f.root,'runs','existing'));fs.writeFileSync(path.join(f.root,'runs','existing','marker'),'preserve');
  assert.equal(f.run('existing').status,2);assert.equal(fs.existsSync(f.log),false);assert.equal(fs.readFileSync(path.join(f.root,'runs','existing','marker'),'utf8'),'preserve');
});
test('dangling evidence symlinks are preserved',t=>{
  const f=recipe(t);fs.symlinkSync('absent',path.join(f.root,'runs','alias'));
  assert.equal(f.run('alias').status,2);assert.equal(fs.existsSync(f.log),false);assert.equal(fs.readlinkSync(path.join(f.root,'runs','alias')),'absent');
});
test('path traversal is refused before any Docker calls',t=>{
  const f=recipe(t);assert.equal(f.run('../escape').status,2);assert.equal(fs.existsSync(f.log),false);
});
test('emulation is refused before creating evidence or running Docker',t=>{
  const f=recipe(t),r=f.run('native-only');assert.equal(r.status,2);assert.match(r.stderr,/not emulation/);
  assert.equal(fs.existsSync(path.join(f.root,'runs','native-only')),false);assert.ok(!fs.readFileSync(f.log,'utf8').split('\n').some(s=>s.startsWith('run ')));
});
