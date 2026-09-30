import {Session} from 'node:inspector/promises';
import {createHash} from 'node:crypto';

export async function profileRun(prep,r,run) {
  const w=prep?.workload;
  if(prep?.profile!=='profiling'||!w||w.abi!=='core'||w.command||w.vectors||w.density||w.oracle?.kind!=='exact_u64'||w.reset!=='stateless'||!w.export||r?.scenario!=='steady'||r.phase_barriers)throw new Error('CPU profiling requires stateless core exact-scalar steady execution without barriers');
  if(!Number.isInteger(r.samples)||r.samples<1||r.samples>100000||!Number.isInteger(r.operations)||r.operations<1||r.operations>1000000||!Number.isInteger(r.warmup)||r.warmup<0||r.warmup>100000)throw new Error('profiling budget outside protocol bounds');
  const artifact={version:1,module_sha256:prep.artifact_sha256,format:'v8-cpuprofile-json',collector:'node:inspector/Profiler',collector_version:process.version+'/v8-'+process.versions.v8,scope:'adapter_v8_isolate_sampled_stacks',window:'run_request_including_setup_warmup_verification',quality:'sampled',status:'unavailable'};
  const session=new Session();let started=false,result;
  try {
    try {session.connect();await session.post('Profiler.enable');await session.post('Profiler.setSamplingInterval',{interval:1000});await session.post('Profiler.start');started=true;}
    catch(error){artifact.reason=String(error.message);}
    try {result=await run();}catch(error){result={status:'error',reason:String(error.message)};}
    if(started){
      try {
        const {profile}=await session.post('Profiler.stop');started=false;
        const data=Buffer.from(JSON.stringify(profile));
        if(data.length>16*1024*1024)artifact.reason='CPU profile exceeded 16 MiB budget; no partial artifact retained';
        else {artifact.status='collected';artifact.sha256=createHash('sha256').update(data).digest('hex');artifact.data=data.toString('base64');}
      }catch(error){artifact.reason=String(error.message);}
    }
  }finally{session.disconnect();}
  return {...result,cpu_profile:artifact};
}
