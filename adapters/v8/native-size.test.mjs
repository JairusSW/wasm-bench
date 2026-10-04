import{test}from'node:test';import assert from'node:assert/strict';
import{mkdtempSync,writeFileSync,rmSync}from'node:fs';import{tmpdir}from'node:os';import{join}from'node:path';import{createHash}from'node:crypto';
import{definedFunctionCount,parseNativeSizes,inspectNativeSize}from'./native-size.mjs';
const bytes=Buffer.from('0061736d010000000105016000017f030201000707010372756e00000a0601040041070b','hex');
test('sizes require complete unique optimizing function coverage',()=>{
 const block='--- WebAssembly code ---\nindex: 0\nkind: wasm function\ncompiler: TurboFan\nBody (size = 64 = 48 + 16 padding)\nInstructions (size = 36)\n--- End code ---';
 assert.equal(definedFunctionCount(bytes),1);assert.equal(parseNativeSizes(block,1),64);
 for(const text of ['',block+block,block.replace('TurboFan','Liftoff'),block.replace('--- End code ---',''),block.replace('36','65')])assert.throws(()=>parseNativeSizes(text,1));
 assert.throws(()=>definedFunctionCount(bytes.subarray(0,bytes.length-1)));
});
test('installed engine reports a deterministic complete body size without inventing bytes',()=>{
 const dir=mkdtempSync(join(tmpdir(),'wasmbench-v8-native-'));
 try{const file=join(dir,'probe.wasm');writeFileSync(file,bytes);const hash=createHash('sha256').update(bytes).digest('hex');
 const a=inspectNativeSize(file,bytes,hash,'optimizing-only'),b=inspectNativeSize(file,bytes,hash,'optimizing-only');
 assert.equal(a.diagnostics[0].status,'available');assert(a.diagnostics[0].value>0);assert.equal(a.diagnostics[0].value,b.diagnostics[0].value);assert.equal(a.diagnostics[1].status,'unavailable');assert.equal(a.code_image,undefined);
 assert.equal(inspectNativeSize(file,bytes,'0'.repeat(64),'optimizing-only').diagnostics[0].status,'unavailable');
 }finally{rmSync(dir,{recursive:true,force:true});}
});
