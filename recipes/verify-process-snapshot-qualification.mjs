// Qualification evidence only. This is not a run-bundle or latency validator.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import {createHash} from 'node:crypto';
import {pathToFileURL} from 'node:url';

export function canonicalProcessArchitecture(arch){
  if(arch==='x64')return 'amd64';
  assert.equal(arch,'arm64','unsupported qualifier architecture');
  return arch;
}

export function validateProcessSnapshotQualification(q, guard, structure, wasmSHA, arch){
  assert.equal(q.scope,'linux_process_cow_clone');assert.equal(q.runtime,'wasmtime');
  assert.equal(q.version,'46.0.1');assert.equal(q.qualification_only,true);
  assert.equal(q.headline_samples,false);assert.equal(Object.hasOwn(q,'samples'),false);
  assert.equal(q.pre_fork_threads,1);assert.equal(q.wasm_sha256,wasmSHA);
  assert.equal(q.architecture,arch==='arm64'?'aarch64':'x86_64');
  assert.ok(['arm64','amd64'].includes(arch));
  assert.equal(guard.qualification_only,true);assert.equal(guard.multithreaded_fork_rejected,true);
  assert.equal(structure.sha256,wasmSHA);assert.equal(structure.validated,true);
  assert.equal(structure.encoding,'core-module');assert.equal(structure.analysis_version,'core-structure-v3');
  assert.equal(structure.import_count,0);assert.equal(structure.defined_functions,8);
  assert.equal(structure.defined_globals,1);assert.equal(structure.defined_memories,1);
  assert.equal(structure.defined_tables,1);assert.equal(structure.data_segments,1);
  assert.equal(structure.element_segments,2);
  assert.deepEqual(q.records.map(r=>r.backend),['cranelift','winch']);
  for(const r of q.records){
    assert.equal(r.source_released_before_restore,true);assert.equal(r.source_after_mutation,125);
    assert.equal(r.restored_before_write,64);assert.equal(r.restored_after_write,125);
    assert.equal(r.passive_segment_probe,127);assert.equal(r.restored_memory_pages,3);
    assert.equal(r.restored_table_elements,3);assert.equal(r.independent_restorations,2);
  }
}

export function validateProcessSnapshotCleanup(cleanup){
  assert.equal(cleanup.version,'linux-process-snapshot-cleanup-v2');
  assert.equal(cleanup.qualification_only,true);
  assert.deepEqual(cleanup.cases.map(c=>c.mode),['valid','exit_without_proof','partial_proof','wrong_proof','proof_then_failure','proof_then_stall','stall_without_proof']);
  for(const c of cleanup.cases){assert.equal(c.accepted,c.mode==='valid');assert.equal(c.reaped,true);}
  assert.deepEqual(cleanup.parent_death,{signal:9,reaped:true});
}

if(process.argv[1]&&fs.existsSync(process.argv[1])&&import.meta.url===pathToFileURL(fs.realpathSync(process.argv[1])).href){
  assert.ok(process.argv.length===4||(process.argv.length===5&&process.argv[4]==='--verify-receipt'),'usage: verify-process-snapshot-qualification ROOT BINARY [--verify-receipt]');
  const root=process.argv[2],binary=process.argv[3],read=name=>JSON.parse(fs.readFileSync(path.join(root,name),'utf8'));
  const digest=p=>createHash('sha256').update(fs.readFileSync(p)).digest('hex');
  const files=['qualifier','fixture.wasm','structure.json','guard.json','cleanup.json','qualification.json','container.json'];
  for(const name of [...files,...(process.argv[4]?['receipt.json']:[])])assert.ok(fs.lstatSync(path.join(root,name)).isFile(),'qualification member must be a regular file: '+name);
  const hashes=Object.fromEntries(files.map(name=>[name,digest(path.join(root,name))]));
  const arch=canonicalProcessArchitecture(process.arch);
  const image=read('container.json');assert.equal(image.os,'linux');assert.equal(image.architecture,arch);
  assert.match(image.image_id,/^sha256:[a-f0-9]{64}$/);
  validateProcessSnapshotQualification(read('qualification.json'),read('guard.json'),read('structure.json'),hashes['fixture.wasm'],arch);
  validateProcessSnapshotCleanup(read('cleanup.json'));
  assert.equal(hashes.qualifier,digest(binary),'archived executable differs from mounted build');
  const receipt={version:'linux-process-snapshot-qualification-v3',qualification_only:true,image_id:image.image_id,architecture:arch,qualifier_sha256:digest(binary),files:hashes};
  if(process.argv[4])assert.deepEqual(read('receipt.json'),receipt,'retained receipt differs from current qualified evidence');
  console.log(JSON.stringify(receipt));
}
