export const policy='empty-local-harness-v1: timing only; resident stateless core scalar workload checked before sampling; preallocated uint64-equivalent slots; timed local loop writes iteration+1 to every slot without Wasm/embedding calls; every slot verified after timer; no warmup, barriers or memory instrumentation; sample result is iteration count, not guest output; no automatic subtraction or performance scoring';
export function validate(prep,r){
  const w=prep?.workload;
  if(!w||r?.scenario!=='harness-calibration'||prep.profile!=='timing'||r.phase_barriers||r.warmup!==0||w.abi!=='core'||w.reset!=='stateless'||w.oracle?.kind!=='exact_u64'||w.oracle.float||!w.export||['command','vectors','density','checkpoint','continuation','guest_density','process_snapshot','snapshot_density'].some(k=>w[k]))throw Error('unsupported harness calibration contract');
  if(!Number.isSafeInteger(r.samples)||r.samples<1||r.samples>100000||!Number.isSafeInteger(r.operations)||r.operations<1||r.operations>1000000)throw Error('invalid harness calibration budget');
}
export function run(r){
  // All admitted indices fit exactly in a Float64 slot; eight bytes per slot.
  const slots=new Float64Array(r.operations),samples=new Array(r.samples);
  for(let i=0;i<samples.length;i++){
    slots.fill(0);
    const start=process.hrtime.bigint();
    for(let j=0;j<slots.length;j++)slots[j]=j+1;
    const elapsed=Number(process.hrtime.bigint()-start);
    for(let j=0;j<slots.length;j++)if(slots[j]!==j+1)throw Error('incorrect harness calibration bookkeeping');
    samples[i]={index:i,warmup:false,elapsed_ns:elapsed,operations:r.operations,sample_type:r.operations===1?'individual_operation':'batch_average',verified:true,result:[String(r.operations)]};
  }
  return {samples};
}
