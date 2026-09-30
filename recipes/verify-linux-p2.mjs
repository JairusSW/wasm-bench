import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import {pathToFileURL} from 'node:url';

const digest = bytes => crypto.createHash('sha256').update(bytes).digest('hex');
const stages = {compile:['before_compile','compiled','released'],instantiate:['before_instantiate','instantiated','instance_released'],'first-call':['before_first_call','first_call_returned','first_call_released']};
const read = file => JSON.parse(fs.readFileSync(file,'utf8'));

export function createP2Suite(root) {
  const artifact = path.join(root,'fixture.component.wasm');
  const source = fs.readFileSync(path.join(root,'fixture.rs'));
  const compiler = fs.readFileSync(path.join(root,'compiler.txt'));
  assert.match(compiler.toString(), /rustc 1\.98\.1/);
  const w = {schema:1,id:'p2/filesystem-reset',family:'applications',artifact,sha256:digest(fs.readFileSync(artifact)),abi:'component',host_profile:'wasi-preview2-temporary-filesystem-v1',features:['component-model'],export:'_start',work_unit:'command',units_per_invocation:1,reset:'fresh_instance_per_sample',oracle:{kind:'exact_command'},license:'Apache-2.0',source:`fixture.rs sha256:${digest(source)}`,generator:`rustc 1.98.1 --edition 2024 --target wasm32-wasip2 -O; compiler.txt sha256:${digest(compiler)}`,command:{argv:['p2-filesystem-reset'],exit_code:0,output_limit_bytes:4096,stdout_sha256:digest('filesystem reset verified\n'),stderr_sha256:digest(''),files:{'data/input.txt':{data:Buffer.from('pristine\n').toString('base64'),sha256:digest('pristine\n')}}}};
  fs.writeFileSync(path.join(root,'suite.json'),JSON.stringify([w],null,2),{flag:'wx'});
}

// The CLI verifies bundle seals first. This adds native coverage and collector
// semantics; it deliberately does not claim cgroup or dedicated-host isolation.
export function verifyP2Coverage(root, arch, profile, originalRoot, restored=false) {
  assert.ok(['arm64','amd64'].includes(arch));
  const manifest=read(path.join(root,'manifest.json')), lock=manifest.lock, o=lock.options;
  assert.equal(manifest.host.os,'linux');assert.equal(manifest.host.arch,arch);
  assert.equal(o.correctness_only,false);assert.equal(o.profile,profile);
  assert.deepEqual(o.scenarios,Object.keys(stages));assert.equal(o.launches,1);
  assert.equal(o.samples,2);assert.equal(o.operations,1);assert.equal(o.warmup,0);
  assert.equal(o.phase_barriers,profile==='memory');assert.equal(lock.archive_tools,true);
  assert.deepEqual(lock.runtime_configurations.map(r=>r.id),['wasmtime','wasmtime-winch']);
  for(const [i,r] of lock.runtime_configurations.entries()) {
    assert.equal(r.description.backend,i===0?'cranelift':'winch');
    assert.equal(r.description.capabilities.can_component_command_lifecycle,true);
    assert.equal(r.description.capabilities.can_component_command_phases,true);
    assert.ok(r.description.effective_configuration.component_command_lifecycle_policy);
  }
  assert.equal(lock.workloads.length,1);const w=lock.workloads[0];
  assert.equal(w.id,'p2/filesystem-reset');assert.equal(w.abi,'component');
  assert.equal(w.reset,'fresh_instance_per_sample');assert.equal(w.oracle.kind,'exact_command');
  assert.equal(w.command.stdout_sha256,digest('filesystem reset verified\n'));
  assert.equal(w.command.stderr_sha256,digest(''));
  if(originalRoot) {
    const old=read(path.join(originalRoot,'manifest.json'));
    assert.deepEqual(manifest.host,old.host,'same native observed host');
    assert.equal(lock.runner_sha256,old.lock.runner_sha256);
    assert.equal(w.sha256,old.lock.workloads[0].sha256);
    assert.deepEqual(w.command,old.lock.workloads[0].command);
    for(const [i,r] of lock.runtime_configurations.entries()) {
      if(restored)assert.notEqual(r.command[0],old.lock.runtime_configurations[i].command[0],'use relocated archived adapter');
      assert.deepEqual(Object.values(r.file_sha256).sort(),Object.values(old.lock.runtime_configurations[i].file_sha256).sort());
      assert.deepEqual(r.description.effective_configuration,old.lock.runtime_configurations[i].description.effective_configuration);
    }
  }
  const expected=new Set(lock.runtime_configurations.flatMap(r=>Object.keys(stages).map(s=>`${r.id}/${s}`)));
  let checks=0,samples=0,boundaries=0;
  const trials=fs.readdirSync(path.join(root,'trials')).map(f=>read(path.join(root,'trials',f)));
  assert.equal(trials.length,8);
  for(const t of trials) {
    assert.equal(t.status,'ok');assert.equal(t.workload,w.id);assert.equal(t.samples.length,2);
    for(const [i,s] of t.samples.entries()) {
      assert.equal(s.index,i);assert.equal(s.verified,true);assert.equal(s.warmup,false);
      assert.equal(s.operations,1);assert.equal(s.sample_type,'individual_operation');
      assert.equal(s.command_result.exit_code,0);
      assert.equal(s.command_result.stdout_sha256,w.command.stdout_sha256);
      assert.equal(s.command_result.stderr_sha256,w.command.stderr_sha256);
      samples++;
    }
    if(t.block<0) {checks++;assert.equal(t.scenario,'first-call');assert.equal(t.phase_events?.length||0,0);continue;}
    assert.equal(t.block,0);assert.ok(expected.delete(`${t.runtime_configuration}/${t.scenario}`));
    const events=t.phase_events||[];
    assert.equal(events.length,profile==='memory'?6:0);
    if(profile==='timing')assert.equal(t.observations?.length||0,0);
    else {
      const peak=t.observations.filter(x=>x.metric==='process.peak_rss');assert.equal(peak.length,1);
      assert.equal(peak[0].status,'available');assert.ok(peak[0].value>0);
      assert.equal(peak[0].phase,t.scenario+'/process_lifetime');
      assert.equal(peak[0].quality,'kernel_accounted_peak');assert.equal(peak[0].collector,'wait4_rusage');
    }
    for(const [i,event] of events.entries()) {
      const stage=stages[t.scenario][i%3];assert.equal(event.event.stage,stage);assert.equal(event.event.sample_index,Math.floor(i/3));
      for(const metric of ['process.rss','process.pss','process.private','process.virtual']) {
        const obs=event.observations.filter(x=>x.metric===metric);assert.equal(obs.length,1);
        const x=obs[0];assert.equal(x.status,'available');assert.ok(Number.isSafeInteger(x.value)&&x.value>=0);
        assert.equal(x.unit,'bytes');assert.equal(x.scope,'adapter_process');assert.equal(x.phase,`${t.scenario}/${stage}`);
        assert.equal(x.collector,'procfs');assert.equal(x.quality,'boundary_snapshot_only');assert.equal(x.profile,'memory');
        const sample=t.samples[Math.floor(i/3)];assert.ok(sample.observations.some(y=>JSON.stringify(y)===JSON.stringify(x)),'boundary evidence attached to exact sample');
      }
      boundaries++;
    }
  }
  assert.equal(checks,2);assert.equal(samples,16);assert.equal(expected.size,0);
  return {arch,profile,trials:trials.length,samples,boundaries};
}

if(process.argv[1]&&import.meta.url===pathToFileURL(path.resolve(process.argv[1])).href) {
  if(process.argv[2]==='suite')createP2Suite(process.argv[3]);
  else console.log(JSON.stringify(verifyP2Coverage(...process.argv.slice(2))));
}
