import {Session} from 'node:inspector/promises';
import {createHash} from 'node:crypto';

export const traceByteLimit=4*1024*1024;
export async function traceRun(moduleSHA256,run,{SessionClass=Session,clock=()=>process.hrtime.bigint(),byteLimit=traceByteLimit}={}) {
  const artifact={version:1,module_sha256:moduleSHA256,format:'chrome-trace-event-json',collector:'node:inspector/NodeTracing',collector_version:process.versions.v8,embedding_version:process.version,
    scope:'adapter_process_v8_wasm_events_without_module_attribution',window:'run_request_including_setup_warmup_verification_and_trace_flush',quality:'engine_reported',categories:['v8.wasm'],status:'unavailable'};
  let session;const events=[];let bytes=18,started=false,complete=false,truncated=false,collectionError,result;
  const post=async(method,params)=>session.post(method,params);
  try {
    try {
      session=new SessionClass();
      session.on('NodeTracing.dataCollected',chunk=>{
        // Retain a prefix, including duplicate metadata and unknown native fields.
        // A malformed notification must not throw through the workload's callback.
        if(truncated||collectionError)return;
        try {
          if(!Array.isArray(chunk.params?.value))throw new Error('missing event array');
          for(const event of chunk.params.value){const size=Buffer.byteLength(JSON.stringify(event))+1;if(bytes+size>byteLimit){truncated=true;break;}events.push(event);bytes+=size;}
        }catch(error){collectionError='Malformed trace notification: '+String(error.message);}
      });
      session.on('NodeTracing.tracingComplete',()=>{complete=true;});
      session.connect();artifact.start_clock_ns=clock().toString();await post('NodeTracing.start',{traceConfig:{includedCategories:artifact.categories}});started=true;
    }
    catch(error){artifact.reason='Trace start failed: '+String(error.message);delete artifact.start_clock_ns;}
    try {result=await run(epoch=>{if(started)artifact.trajectory_epoch_ns=epoch;});}catch(error){result={status:'error',reason:String(error.message)};}
    if(started){
      try {await post('NodeTracing.stop');artifact.status=complete&&!truncated&&!collectionError?'collected':'incomplete';if(!complete)artifact.reason='Trace completion notification was not received';if(truncated)artifact.reason='Trace retained prefix reached the '+byteLimit+' byte budget';if(collectionError)artifact.reason=collectionError;}
      catch(error){artifact.status='incomplete';artifact.reason='Trace stop failed: '+String(error.message);}
      artifact.end_clock_ns=clock().toString();
      const data=Buffer.from(JSON.stringify({traceEvents:events}));
      if(data.length>byteLimit){artifact.status='unavailable';artifact.reason='Encoded trace exceeded byte budget';delete artifact.start_clock_ns;delete artifact.end_clock_ns;delete artifact.trajectory_epoch_ns;}
      else {artifact.sha256=createHash('sha256').update(data).digest('hex');artifact.data=data.toString('base64');}
    }
  }finally{try{session?.disconnect();}catch{}}
  return {...result,engine_trace:artifact};
}

export async function traceProbe(bytes,moduleSHA256) {
  const result=await traceRun(moduleSHA256,()=>{const fn=new WebAssembly.Instance(new WebAssembly.Module(bytes)).exports.run;if(fn()!==7)throw new Error('incorrect trace calibration');return {};});
  const trace=result.engine_trace;
  if(result.status==='error'||trace.status!=='collected')throw new Error('installed NodeTracing calibration failed: '+(trace.reason||result.reason||'unavailable'));
  const events=JSON.parse(Buffer.from(trace.data,'base64')).traceEvents;
  if(!events.some(e=>e.cat==='v8.wasm'&&e.name==='wasm.SyncCompile'))throw new Error('installed engine did not emit requested Wasm compilation category');
  return {version:'v8-wasm-trace-probe-v1',module_sha256:moduleSHA256,collector:trace.collector,categories:trace.categories,compilation_category_verified:true};
}
