// Run only after the sealed-bundle CLI validator. This additionally requires
// complete native compiler coverage and actually available Linux residency.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';

const root=process.argv[2], arch=process.argv[3];
assert.ok(root, 'pass the paired evidence directory');
assert.ok(['arm64','amd64'].includes(arch), 'declare native Linux architecture');
const stages=['continuation-create','continuation-resume','continuation-first-write','continuation-execute'];
const footprints=['process.rss','process.pss','process.private','process.virtual'];
const read=p=>JSON.parse(fs.readFileSync(p,'utf8'));
const timingHost=read(path.join(root,'timing','manifest.json')).host;
assert.deepEqual(timingHost,read(path.join(root,'memory','manifest.json')).host,'paired passes must retain identical observed host identity/policy');
for(const profile of ['timing','memory']){
  const dir=path.join(root,profile), m=read(path.join(dir,'manifest.json')), o=m.lock.options;
  assert.equal(m.host.os,'linux');assert.equal(m.host.arch,arch);
  assert.equal(o.correctness_only,false);assert.equal(o.profile,profile);
  assert.equal(o.phase_barriers,profile==='memory');assert.equal(o.launches,3);
  assert.equal(o.samples,2);assert.equal(o.operations,1);assert.equal(o.warmup,0);
  assert.deepEqual(o.scenarios,stages);
  assert.deepEqual(m.lock.workloads.map(w=>w.continuation.depth),[0,1,8,32,128]);
  assert.deepEqual(m.lock.runtime_configurations.map(r=>r.id),['wazero','wazero-interpreter']);
  const compiler=m.lock.runtime_configurations[0].description;
  assert.equal(compiler.runtime,'wazero');assert.equal(compiler.runtime_version,'1.12.0');
  assert.equal(compiler.backend,'compiler');assert.equal(compiler.capabilities.can_native_continuation,true);
  assert.equal(compiler.capabilities.can_snapshot,false);
  let ok=0,unsupported=0,samples=0,boundaries=0,residency=0,unavailablePeaks=0;
  const cells=new Set();
  for(const name of fs.readdirSync(path.join(dir,'trials'))){
    const t=read(path.join(dir,'trials',name));if(t.block<0)continue;
    assert.equal(t.profile,profile);assert.ok(stages.includes(t.scenario));
    const key=[t.runtime_configuration,t.workload,t.scenario,t.block].join('/');
    assert.ok(!cells.has(key),'duplicate measured cell');cells.add(key);
    if(t.runtime_configuration==='wazero-interpreter'){
      assert.equal(t.status,'unsupported');assert.ok(!t.samples?.length);unsupported++;continue;
    }
    assert.equal(t.runtime_configuration,'wazero');assert.equal(t.status,'ok');ok++;
    assert.equal(t.samples.length,2);samples+=t.samples.length;
    for(const s of t.samples){assert.equal(s.verified,true);assert.equal(s.continuation_result.guest_resumed,true);}
    if(profile==='timing'){assert.ok(!t.phase_events?.length);continue;}
    assert.equal(t.phase_events.length,6);boundaries+=t.phase_events.length;
    for(const e of t.phase_events){
      for(const metric of footprints){
        const obs=e.observations.filter(x=>x.metric===metric);
        assert.equal(obs.length,1);const x=obs[0];
        assert.equal(x.status,'available');assert.equal(x.quality,'boundary_snapshot_only');
        assert.equal(x.scope,'adapter_process');assert.equal(x.collector,'procfs');
        assert.ok(Number.isSafeInteger(x.value)&&x.value>=0);residency++;
      }
    }
    // This unprivileged recipe deliberately has no per-adapter cgroup. Never
    // present a sampled peak or whole-container peak as a kernel phase peak.
    for(const s of t.samples){
      const peaks=s.observations.filter(x=>x.metric==='cgroup.memory.phase_peak');
      assert.equal(peaks.length,1);assert.equal(peaks[0].status,'unavailable');
      assert.equal(peaks[0].value,undefined);assert.ok(peaks[0].reason);unavailablePeaks++;
    }
  }
  assert.equal(cells.size,120);assert.equal(ok,60);assert.equal(unsupported,60);assert.equal(samples,120);
  assert.equal(boundaries,profile==='memory'?360:0);assert.equal(residency,profile==='memory'?1440:0);
  assert.equal(unavailablePeaks,profile==='memory'?120:0);
  console.log(JSON.stringify({profile,arch,ok,unsupported,samples,boundaries,residency,unavailablePeaks}));
}
