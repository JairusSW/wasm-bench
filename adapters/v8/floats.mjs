// float_bits_v1: compare observed IEEE values, without inventing NaN payload
// preservation across the JavaScript Number boundary.
function wireBits(bits) {
  if(typeof bits==='number'?!Number.isSafeInteger(bits)||bits<0:typeof bits!=='string'||!/^\d+$/.test(bits))throw new Error('lossy or invalid integer bits');
  const n=BigInt(bits);if(n<0n||n>0xffffffffffffffffn)throw new Error('invalid uint64 bits');return n;
}
export function decodeFloat(type, bits) {
  const n=wireBits(bits), b=Buffer.alloc(8);
  if(n<0n||n>0xffffffffffffffffn||(type==='f32'&&n>0xffffffffn))throw new Error('noncanonical float bits');
  b.writeBigUInt64LE(n);
  if(type==='f32')return b.readFloatLE();
  if(type==='f64')return b.readDoubleLE();
  throw new Error('unsupported float type');
}
export function encodeFloats(value, types) {
  const values=value===undefined?[]:Array.isArray(value)?value:[value];
  if(values.length!==types.length)throw new Error('float result count mismatch');
  return values.map((v,i)=>{
    if(typeof v!=='number')throw new Error('float result is not a Number');
    const b=Buffer.alloc(8);
    if(types[i]==='f32')b.writeFloatLE(v);else if(types[i]==='f64')b.writeDoubleLE(v);else throw new Error('invalid float type');
    return b.readBigUInt64LE().toString();
  });
}
export function validateFloat(o, types) {
  const p=o.float;
  if(o.kind!=='float_bits_v1'||!p||!Array.isArray(o.expected)||!o.expected.length||JSON.stringify(p.types)!==JSON.stringify(types)||types.length!==o.expected.length||o.expected_trap)throw new Error('invalid float oracle/signature');
  if(!['reject','any_nan'].includes(p.nan)||!['match','ignore'].includes(p.signed_zero))throw new Error('explicit float policies required');
  for(const v of [p.absolute_tolerance??0,p.relative_tolerance??0])if(!Number.isFinite(v)||v<0)throw new Error('invalid float tolerance');
  o.expected.forEach((v,i)=>{if(Number.isNaN(decodeFloat(types[i],v))&&p.nan!=='any_nan')throw new Error('expected NaN requires any_nan');});
}
export function verifyFloats(o, bits, types) {
  validateFloat(o,types);
  if(bits.length!==types.length)throw new Error('float result count mismatch');
  const p=o.float, abs=p.absolute_tolerance??0, rel=p.relative_tolerance??0;
  bits.forEach((v,i)=>{
    const got=decodeFloat(types[i],v),want=decodeFloat(types[i],o.expected[i]);
    let ok;
    if(Number.isNaN(want))ok=p.nan==='any_nan'&&Number.isNaN(got);
    else if(Number.isNaN(got))ok=false;
    else if(!Number.isFinite(got)||!Number.isFinite(want))ok=got===want;
    else if(got===0&&want===0)ok=p.signed_zero==='ignore'||Object.is(got,want);
    else if(got===want)ok=true;
    else {const delta=Math.abs(got-want),scale=Math.max(Math.abs(got),Math.abs(want));ok=delta<=abs||(Number.isFinite(delta)?delta/scale:Math.abs(got/scale-want/scale))<=rel;}
    if(!ok)throw new Error('incorrect floating result '+i);
  });
}

// Signature extraction only. Engine validation remains authoritative for the
// module. This bounded reader fails closed on GC types and memory64 imports;
// it never guesses a signature for an encoding it does not understand.
export function numericSignature(bytes, name) {
  function reader(data) {
    let pos=0;
    const r={byte(){if(pos>=data.length)throw new Error('truncated signature metadata');return data[pos++];},
      uint(){let n=0;for(let i=0;i<5;i++){const b=r.byte();if(i===4&&(b&0xf0))throw new Error('invalid u32');n+=(b&127)*2**(7*i);if(!(b&128))return n;}throw new Error('invalid u32');},
      take(n){if(n>data.length-pos)throw new Error('truncated signature section');const v=data.subarray(pos,pos+n);pos+=n;return v;},
      name(){return new TextDecoder('utf-8',{fatal:true}).decode(r.take(r.uint()));},
      end(){return pos===data.length;}};
    return r;
  }
  const r=reader(bytes);
  if(Buffer.from(r.take(8)).toString('hex')!=='0061736d01000000')throw new Error('not a core module');
  const types=[], functions=[];let index;
  const val=r=>{const v=r.byte();const t={127:'i32',126:'i64',125:'f32',124:'f64',123:'v128',112:'funcref',111:'externref'}[v];if(!t)throw new Error('unsupported signature value type');return t;};
  const limits=r=>{const flags=r.uint();if(flags>3)throw new Error('unsupported wide/custom memory or table import');r.uint();if(flags&1)r.uint();};
  while(!r.end()) {
    const id=r.byte(),s=reader(r.take(r.uint()));
    if(id===1){const count=s.uint();for(let i=0;i<count;i++){if(s.byte()!==0x60)throw new Error('unsupported GC signature encoding');const params=[],results=[];for(let n=s.uint();n>0;n--)params.push(val(s));for(let n=s.uint();n>0;n--)results.push(val(s));types.push({params,results});}}
    else if(id===2){for(let n=s.uint();n>0;n--){s.name();s.name();switch(s.byte()){case 0:functions.push(s.uint());break;case 1:val(s);limits(s);break;case 2:limits(s);break;case 3:val(s);s.byte();break;case 4:s.byte();s.uint();break;default:throw new Error('unsupported import kind');}}}
    else if(id===3){for(let n=s.uint();n>0;n--)functions.push(s.uint());}
    else if(id===7){for(let n=s.uint();n>0;n--){const exported=s.name(),kind=s.byte(),idx=s.uint();if(exported===name){if(kind!==0)throw new Error('export is not a function');index=idx;}}}
    else continue;
    if(!s.end())throw new Error('signature section trailing bytes');
  }
  const signature=types[functions[index]];
  if(!signature||!signature.params.every(t=>['i32','i64','f32','f64'].includes(t)))throw new Error('missing or nonnumeric signature');
  return signature;
}
export function floatArguments(bits,types) {
  if(bits.length!==types.length)throw new Error('argument count mismatch');
  return bits.map((b,i)=>types[i]==='i64'?BigInt.asIntN(64,wireBits(b)):types[i]==='i32'?Number(BigInt.asIntN(32,wireBits(b))):decodeFloat(types[i],b));
}
