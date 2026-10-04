import { spawn } from 'node:child_process';

export function definedFunctionCount(bytes) {
  if(bytes.length<8 || bytes.subarray(0,8).toString('hex')!=='0061736d01000000')throw Error('Core module required');
  let position=8,functions=0,bodies=0;
  function leb(end){let value=0;for(let i=0;i<5;i++){if(position>=end)throw Error('Truncated section length');const byte=bytes[position++];if(i===4&&(byte&0xf0))throw Error('Invalid u32 length');value+=(byte&127)*2**(7*i);if(!(byte&128))return value;}throw Error('Invalid section length');}
  while(position<bytes.length){const id=bytes[position++],size=leb(bytes.length),end=position+size;if(end>bytes.length)throw Error('Truncated section');if(id===3)functions=leb(end);if(id===10)bodies=leb(end);position=end;}
  if(functions!==bodies)throw Error('Function/body count mismatch');
  return functions;
}

export function parseNativeSizes(text, expectedFunctions) {
  const blocks=text.split('--- WebAssembly code ---').slice(1),indices=new Set();let size=0;
  for(const block of blocks){
    if(!/^kind: wasm function$/m.test(block))continue;
    if(!/^compiler: TurboFan$/m.test(block))throw Error('Non-optimizing code body');
    const index=/^index: (\d+)$/m.exec(block),body=/^Body \(size = (\d+)(?: = [^\n]+)?\)$/m.exec(block),instructions=/^Instructions \(size = (\d+)\)$/m.exec(block);
    if(!index || !body || !instructions || !block.includes('--- End code ---'))throw Error('Incomplete code diagnostic');
    if(indices.has(index[1]))throw Error('Duplicate function generation');indices.add(index[1]);
    const bytes=Number(body[1]);if(!Number.isSafeInteger(bytes)||Number(instructions[1])>bytes)throw Error('Invalid code body size');size+=bytes;
  }
  if(indices.size!==expectedFunctions || !Number.isSafeInteger(size))throw Error('Incomplete defined-function coverage');
  return size;
}

export function nativeSizeStream(expectedFunctions) {
  let pending='',block=null,total=0,failed=null;
  const indices=new Set();
  function line(value) {
    if(value==='--- WebAssembly code ---') {
      if(block)throw Error('Incomplete code diagnostic');
      block=[value];
    } else if(block && value==='--- End code ---') {
      block.push(value);
      const text=block.join('\n');block=null;
      if(!/^kind: wasm function$/m.test(text))return;
      const size=parseNativeSizes(text,1),index=/^index: (\d+)$/m.exec(text)[1];
      if(indices.has(index))throw Error('Duplicate function generation');
      indices.add(index);total+=size;
      if(indices.size>expectedFunctions || !Number.isSafeInteger(total))throw Error('Invalid defined-function coverage');
    } else if(block && /^(index:|kind:|compiler:|Body \(|Instructions \()/.test(value)) {
      block.push(value);
      if(block.length>16)throw Error('Invalid code diagnostic metadata');
    }
  }
  return {
    write(chunk) {
      if(failed)return;
      try {
        pending+=chunk;
        let end;
        while((end=pending.indexOf('\n'))!==-1) {
          if(end>65536)throw Error('Oversized diagnostic line');
          line(pending.slice(0,end).replace(/\r$/,''));pending=pending.slice(end+1);
        }
        if(pending.length>65536)throw Error('Oversized diagnostic line');
      } catch(error) {failed=error;pending='';block=null;}
    },
    finish() {
      if(failed)throw failed;
      if(pending)line(pending);
      if(block || indices.size!==expectedFunctions)throw Error('Incomplete defined-function coverage');
      return total;
    }
  };
}

export async function inspectNativeSize(artifact, bytes, moduleSha256, mode) {
  const definition={definition_version:1,unit:'bytes',scope:'compiled_module',phase:'compile',collector:'V8/print-wasm-code/body-size',collector_version:process.versions.v8,quality:'engine_reported',profile:'code',normalization_denominator:'module'};
  const unavailable=reason=>({diagnostics:[{...definition,metric:'native.code_export',status:'unavailable',reason}]});
  if(mode!=='optimizing-only')return unavailable('Native size collection requires the locked eager optimizing tier');
  const program='const fs=require("node:fs"),crypto=require("node:crypto");const bytes=fs.readFileSync(process.argv[1]);if(crypto.createHash("sha256").update(bytes).digest("hex")!==process.argv[2])throw Error("artifact changed");new WebAssembly.Module(bytes);';
  const parser=nativeSizeStream(definedFunctionCount(bytes));
  const result=await new Promise(resolve=>{
    const child=spawn(process.execPath,['--no-liftoff','--no-wasm-tier-up','--no-wasm-lazy-compilation','--no-wasm-native-module-cache','--print-wasm-code','-e',program,artifact,moduleSha256],{stdio:['ignore','pipe','ignore'],env:{...process.env,NODE_OPTIONS:''}});
    const timeout=setTimeout(()=>child.kill('SIGKILL'),240000);
    let failure=null;
    child.stdout.setEncoding('utf8');child.stdout.on('data',chunk=>parser.write(chunk));
    child.on('error',error=>{failure=error;});
    child.on('close',(status,signal)=>{clearTimeout(timeout);resolve({status,signal,error:failure});});
  });
  if(result.error || result.status!==0)return unavailable('Isolated native diagnostic did not complete within its time budget');
  try{
    const value=parser.finish();
    return {diagnostics:[{...definition,metric:'native.code_size',status:'available',value,reason:'Sum of all defined-function code bodies, including per-body metadata and padding; excludes shared engine and wrapper code. Byte export unavailable.'},{...definition,metric:'native.code_export',status:'unavailable',reason:'Engine-reported body sizes are captured; complete native image bytes are not exported'}]};
  }catch(error){return unavailable(error.message);}
}
