import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {spawnSync} from 'node:child_process';

function isolatedRecipe(t){
  const root=fs.mkdtempSync(path.join(os.tmpdir(),'wasmbench-code-recipe-'));
  t.after(()=>fs.rmSync(root,{recursive:true,force:true}));
  fs.mkdirSync(path.join(root,'recipes'));fs.mkdirSync(path.join(root,'runs'));
  const script=path.join(root,'recipes','test-linux-code-lifetime.sh');
  fs.copyFileSync(new URL('./test-linux-code-lifetime.sh',import.meta.url),script);
  fs.mkdirSync(path.join(root,'bin'));
  const log=path.join(root,'docker-calls');
  fs.writeFileSync(path.join(root,'bin','docker'),`#!/bin/sh
printf '%s\\n' "$*" >> "$WASMBENCH_TEST_CALL_LOG"
case "$1" in
  info) printf 'x86_64\\n';;
  image) case "$4" in
    '{{.Id}}') printf 'sha256:test\\n';;
    '{{.Architecture}}') printf 'arm64\\n';;
    '{{.Os}}') printf 'linux\\n';;
    *) exit 90;;
  esac;;
  *) exit 91;;
esac
`,{mode:0o755});
  const run=(...args)=>spawnSync('sh',[script,...args],{encoding:'utf8',env:{...process.env,PATH:path.join(root,'bin')+path.delimiter+process.env.PATH,WASMBENCH_TEST_CALL_LOG:log}});
  return {root,log,run};
}

test('existing evidence is preserved before Docker is contacted',t=>{
  const f=isolatedRecipe(t),dir=path.join(f.root,'runs','existing');fs.mkdirSync(dir);
  fs.writeFileSync(path.join(dir,'marker'),'preserve');
  const r=f.run('image','existing');assert.equal(r.status,2);assert.match(r.stderr,/Preserving existing evidence/);
  assert.equal(fs.readFileSync(path.join(dir,'marker'),'utf8'),'preserve');assert.equal(fs.existsSync(f.log),false);
});
test('dangling evidence aliases are preserved',t=>{
  const f=isolatedRecipe(t),alias=path.join(f.root,'runs','alias');fs.symlinkSync('absent',alias);
  assert.equal(f.run('image','alias').status,2);assert.equal(fs.readlinkSync(alias),'absent');assert.equal(fs.existsSync(f.log),false);
});
test('path traversal is rejected before Docker or evidence creation',t=>{
  const f=isolatedRecipe(t);assert.equal(f.run('image','../escaped').status,2);
  assert.equal(fs.existsSync(path.join(f.root,'escaped')),false);assert.equal(fs.existsSync(f.log),false);
});
test('emulated image is rejected before creating evidence or launching a container',t=>{
  const f=isolatedRecipe(t),r=f.run('image','native-only');assert.equal(r.status,2);assert.match(r.stderr,/not emulation/);
  assert.equal(fs.existsSync(path.join(f.root,'runs','native-only')),false);
  assert.ok(!fs.readFileSync(f.log,'utf8').split('\n').some(line=>line.startsWith('run ')));
});
