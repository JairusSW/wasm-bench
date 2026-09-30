import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {createHash} from 'node:crypto';
import {spawnSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';
import {validateProcessSnapshotQualification as validate,validateProcessSnapshotCleanup,canonicalProcessArchitecture} from './verify-process-snapshot-qualification.mjs';

test('Node x64 maps to Docker amd64 and unsupported architectures fail',()=>{
  assert.equal(canonicalProcessArchitecture('x64'),'amd64');
  assert.equal(canonicalProcessArchitecture('arm64'),'arm64');
  assert.throws(()=>canonicalProcessArchitecture('ia32'));
});

function fixture(){
  const q={scope:'linux_process_cow_clone',runtime:'wasmtime',version:'46.0.1',qualification_only:true,headline_samples:false,pre_fork_threads:1,wasm_sha256:'fixture',architecture:'aarch64',records:['cranelift','winch'].map(backend=>({backend,source_released_before_restore:true,source_after_mutation:125,restored_before_write:64,restored_after_write:125,passive_segment_probe:127,restored_memory_pages:3,restored_table_elements:3,independent_restorations:2}))};
  const guard={qualification_only:true,multithreaded_fork_rejected:true};
  const structure={sha256:'fixture',validated:true,encoding:'core-module',analysis_version:'core-structure-v3',import_count:0,defined_functions:8,defined_globals:1,defined_memories:1,defined_tables:1,data_segments:1,element_segments:2};
  return {q,guard,structure};
}
test('qualified process snapshot evidence passes without being a timing result',()=>{
  const f=fixture();validate(f.q,f.guard,f.structure,'fixture','arm64');
  f.q.architecture='x86_64';validate(f.q,f.guard,f.structure,'fixture','amd64');
});
for(const mode of ['scope','headline','samples','threads','guard','fixture','unvalidated','imports','tables','segments','backend','source_alive','wrong_restore','wrong_probe','growth','repeated_restore'])test('reject '+mode+' qualification',()=>{
  const f=fixture(),r=f.q.records[0];
  switch(mode){
    case 'scope':f.q.scope='whole_instance_engine_api';break;
    case 'headline':f.q.headline_samples=true;break;
    case 'samples':f.q.samples=[];break;
    case 'threads':f.q.pre_fork_threads=2;break;
    case 'guard':f.guard.multithreaded_fork_rejected=false;break;
    case 'fixture':f.q.wasm_sha256='other';break;
    case 'unvalidated':f.structure.validated=false;break;
    case 'imports':f.structure.import_count=1;break;
    case 'tables':f.structure.defined_tables=0;break;
    case 'segments':f.structure.element_segments=0;break;
    case 'backend':f.q.records[1].backend='cranelift';break;
    case 'source_alive':r.source_released_before_restore=false;break;
    case 'wrong_restore':r.restored_before_write=125;break;
    case 'wrong_probe':r.passive_segment_probe=0;break;
    case 'growth':r.restored_memory_pages=4;break;
    case 'repeated_restore':r.independent_restorations=1;break;
  }
  assert.throws(()=>validate(f.q,f.guard,f.structure,'fixture','arm64'));
});

function cleanupFixture(){return {version:'linux-process-snapshot-cleanup-v2',qualification_only:true,parent_death:{signal:9,reaped:true},cases:['valid','exit_without_proof','partial_proof','wrong_proof','proof_then_failure','proof_then_stall','stall_without_proof'].map(mode=>({mode,accepted:mode==='valid',reaped:true}))};}
test('cleanup proof requires valid positive control and all rejected children reaped',()=>validateProcessSnapshotCleanup(cleanupFixture()));
for(const mode of ['version','scope','missing','duplicate','false_success','unreaped','no_positive_control','parent_alive','orphan_unreaped'])test('reject cleanup '+mode,()=>{
  const f=cleanupFixture();
  switch(mode){
    case 'version':f.version='other';break;
    case 'scope':f.qualification_only=false;break;
    case 'missing':f.cases.pop();break;
    case 'duplicate':f.cases[1].mode='valid';break;
    case 'false_success':f.cases[1].accepted=true;break;
    case 'unreaped':f.cases[5].reaped=false;break;
    case 'no_positive_control':f.cases[0].accepted=false;break;
    case 'parent_alive':f.parent_death.signal=0;break;
    case 'orphan_unreaped':f.parent_death.reaped=false;break;
  }
  assert.throws(()=>validateProcessSnapshotCleanup(f));
});

// Synthetic evidence checks the receipt boundary, not engine correctness.
function receiptFixture(t){
  const root=fs.mkdtempSync(path.join(os.tmpdir(),'wasmbench-process-receipt-'));
  t.after(()=>fs.rmSync(root,{recursive:true,force:true}));
  const f=fixture(),arch=canonicalProcessArchitecture(process.arch),bytes=Buffer.from('synthetic fixture');
  const hash=createHash('sha256').update(bytes).digest('hex');
  f.q.wasm_sha256=hash;f.structure.sha256=hash;f.q.architecture=arch==='arm64'?'aarch64':'x86_64';
  fs.writeFileSync(path.join(root,'fixture.wasm'),bytes);
  for(const [name,data] of Object.entries({'structure.json':f.structure,'guard.json':f.guard,'cleanup.json':cleanupFixture(),'qualification.json':f.q,'container.json':{os:'linux',architecture:arch,image_id:'sha256:'+'a'.repeat(64)}}))fs.writeFileSync(path.join(root,name),JSON.stringify(data));
  fs.writeFileSync(path.join(root,'qualifier'),'synthetic executable');
  const build=path.join(root,'mounted-build');fs.copyFileSync(path.join(root,'qualifier'),build);
  return {root,run:(...args)=>spawnSync(process.execPath,[fileURLToPath(new URL('./verify-process-snapshot-qualification.mjs',import.meta.url)),root,build,...args],{encoding:'utf8'})};
}
test('receipt CLI binds archived binary and all required qualification files',t=>{
  const f=receiptFixture(t),r=f.run();assert.equal(r.status,0,r.stderr);
  const receipt=JSON.parse(r.stdout);assert.equal(receipt.version,'linux-process-snapshot-qualification-v3');
  assert.deepEqual(Object.keys(receipt.files),['qualifier','fixture.wasm','structure.json','guard.json','cleanup.json','qualification.json','container.json']);
  assert.equal(receipt.files.qualifier,receipt.qualifier_sha256);
});
test('receipt CLI rejects an archived executable different from the mounted build',t=>{
  const f=receiptFixture(t);fs.writeFileSync(path.join(f.root,'qualifier'),'different executable');
  const r=f.run();assert.notEqual(r.status,0);assert.match(r.stderr,/archived executable differs/);assert.equal(r.stdout,'');
});
test('verifier entry-point aliases execute the checks rather than silently succeeding',t=>{
  const f=receiptFixture(t),alias=path.join(f.root,'verifier-alias.mjs');
  fs.symlinkSync(fileURLToPath(new URL('./verify-process-snapshot-qualification.mjs',import.meta.url)),alias);
  const r=spawnSync(process.execPath,[alias,f.root,path.join(f.root,'mounted-build')],{encoding:'utf8'});
  assert.equal(r.status,0,r.stderr);assert.equal(JSON.parse(r.stdout).version,'linux-process-snapshot-qualification-v3');
});
test('receipt CLI refuses missing cleanup proof',t=>{
  const f=receiptFixture(t);fs.unlinkSync(path.join(f.root,'cleanup.json'));
  const r=f.run();assert.notEqual(r.status,0);assert.equal(r.stdout,'');
});
test('receipt CLI refuses incomplete parent-death proof',t=>{
  const f=receiptFixture(t),proof=cleanupFixture();delete proof.parent_death;
  fs.writeFileSync(path.join(f.root,'cleanup.json'),JSON.stringify(proof));
  const r=f.run();assert.notEqual(r.status,0);assert.equal(r.stdout,'');
});
test('retained receipt verification accepts unchanged evidence',t=>{
  const f=receiptFixture(t),r=f.run();assert.equal(r.status,0,r.stderr);
  fs.writeFileSync(path.join(f.root,'receipt.json'),r.stdout);
  const verified=f.run('--verify-receipt');assert.equal(verified.status,0,verified.stderr);assert.equal(verified.stdout,r.stdout);
});
for(const mode of ['binary','input','receipt','symlink','missing_receipt','unknown_argument'])test('retained receipt verification rejects '+mode,t=>{
  const f=receiptFixture(t),r=f.run();assert.equal(r.status,0,r.stderr);
  fs.writeFileSync(path.join(f.root,'receipt.json'),r.stdout);
  switch(mode){
    case 'binary':fs.writeFileSync(path.join(f.root,'qualifier'),'changed');break;
    case 'input':fs.writeFileSync(path.join(f.root,'fixture.wasm'),'changed');break;
    case 'receipt':{const q=JSON.parse(r.stdout);q.qualifier_sha256='0'.repeat(64);fs.writeFileSync(path.join(f.root,'receipt.json'),JSON.stringify(q));break;}
    case 'symlink':fs.renameSync(path.join(f.root,'guard.json'),path.join(f.root,'guard-real.json'));fs.symlinkSync('guard-real.json',path.join(f.root,'guard.json'));break;
    case 'missing_receipt':fs.unlinkSync(path.join(f.root,'receipt.json'));break;
  }
  const result=f.run(mode==='unknown_argument'?'--unknown':'--verify-receipt');assert.notEqual(result.status,0);assert.equal(result.stdout,'');
});
