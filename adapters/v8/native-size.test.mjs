import{test}from'node:test';import assert from'node:assert/strict';
import{mkdtempSync,writeFileSync,rmSync}from'node:fs';import{tmpdir}from'node:os';import{join}from'node:path';import{createHash}from'node:crypto';
import{definedFunctionCount,parseNativeSizes,inspectNativeSize,nativeSizeStream}from'./native-size.mjs';
const bytes=Buffer.from('0061736d010000000105016000017f030201000707010372756e00000a0601040041070b','hex');
test('sizes require complete unique optimizing function coverage',()=>{
 const block='--- WebAssembly code ---\nindex: 0\nkind: wasm function\ncompiler: TurboFan\nBody (size = 64 = 48 + 16 padding)\nInstructions (size = 36)\n--- End code ---';
 assert.equal(definedFunctionCount(bytes),1);assert.equal(parseNativeSizes(block,1),64);
 for(const text of ['',block+block,block.replace('TurboFan','Liftoff'),block.replace('--- End code ---',''),block.replace('36','65')])assert.throws(()=>parseNativeSizes(text,1));
 assert.throws(()=>definedFunctionCount(bytes.subarray(0,bytes.length-1)));
});
test('installed engine reports a deterministic complete body size without inventing bytes',async()=>{
 const dir=mkdtempSync(join(tmpdir(),'wasmbench-v8-native-'));
 try{const file=join(dir,'probe.wasm');writeFileSync(file,bytes);const hash=createHash('sha256').update(bytes).digest('hex');
 const a=await inspectNativeSize(file,bytes,hash,'optimizing-only'),b=await inspectNativeSize(file,bytes,hash,'optimizing-only');
 assert.equal(a.diagnostics[0].status,'available');assert(a.diagnostics[0].value>0);assert.equal(a.diagnostics[0].value,b.diagnostics[0].value);assert.equal(a.diagnostics[1].status,'unavailable');assert.equal(a.code_image,undefined);
 assert.equal((await inspectNativeSize(file,bytes,'0'.repeat(64),'optimizing-only')).diagnostics[0].status,'unavailable');
 }finally{rmSync(dir,{recursive:true,force:true});}
});

test('streaming diagnostics discard large disassembly while validating every body',()=>{
 const parser=nativeSizeStream(1);
 parser.write('--- WebAssembly code ---\nindex: 0\nkind: wasm function\ncompiler: TurboFan\nBody (size = 64)\nInstructions (size = 36)\n');
 const disassembly='0x00000000  00  instruction\n'.repeat(1000);
 for(let i=0;i<3000;i++)parser.write(disassembly);
 parser.write('--- End co');parser.write('de ---\n');assert.equal(parser.finish(),64);
 const incomplete=nativeSizeStream(1);incomplete.write('--- WebAssembly code ---\n');assert.throws(()=>incomplete.finish());
 const huge=nativeSizeStream(0);huge.write('x'.repeat(65537));assert.throws(()=>huge.finish());
});

test('streamed metadata rejects duplicate, wrong-tier, and impossible bodies',()=>{
 const block='--- WebAssembly code ---\nindex: 0\nkind: wasm function\ncompiler: TurboFan\nBody (size = 64)\nInstructions (size = 36)\n--- End code ---\n';
 for(const text of [block+block,block.replace('TurboFan','Liftoff'),block.replace('36','65')]) {
  const parser=nativeSizeStream(1);for(let i=0;i<text.length;i+=7)parser.write(text.slice(i,i+7));assert.throws(()=>parser.finish());
 }
});
