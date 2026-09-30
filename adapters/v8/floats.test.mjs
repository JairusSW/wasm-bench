import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import {encodeFloats,verifyFloats,validateFloat,numericSignature,floatArguments,decodeFloat} from './floats.mjs';
const oracle=want=>({kind:'float_bits_v1',expected:encodeFloats(want,['f64']),float:{types:['f64'],absolute_tolerance:1e-12,relative_tolerance:1e-9,nan:'reject',signed_zero:'match'}});
test('float parity edge cases',()=>{
  for(const [got,want,ok] of [[1+1e-10,1,true],[1.01,1,false],[1e-13,0,true],[Number.MAX_VALUE,-Number.MAX_VALUE,false],[Infinity,Infinity,true],[-Infinity,Infinity,false],[-0,0,false],[NaN,1,false]]){
    const run=()=>verifyFloats(oracle(want),encodeFloats(got,['f64']),['f64']);if(ok)assert.doesNotThrow(run);else assert.throws(run);
  }
  const o=oracle(NaN);o.float.nan='any_nan';assert.doesNotThrow(()=>verifyFloats(o,['9221120237041090626'],['f64']));
  assert.throws(()=>verifyFloats(o,encodeFloats(1,['f64']),['f64']));
  for(const tolerance of [-1,NaN,Infinity]){const o=oracle(1);o.float.absolute_tolerance=tolerance;assert.throws(()=>validateFloat(o,['f64']));}
  assert.throws(()=>decodeFloat('f32','4294967296'));
  assert.throws(()=>decodeFloat('f64',9007199254740992));
  assert.throws(()=>validateFloat(oracle(1),['i64']));
});
test('binary signature and argument decoding',()=>{
  const b=fs.readFileSync(new URL('../../corpus/testdata/floats.wasm',import.meta.url));
  assert.deepEqual(numericSignature(b,'sum_f64'),{params:[],results:['f64']});
  assert.deepEqual(numericSignature(b,'wrong_type'),{params:[],results:['i64']});
  assert.throws(()=>numericSignature(b,'missing'));
  assert.throws(()=>numericSignature(b.subarray(0,b.length-1),'sum_f64'));
  const imported=fs.readFileSync(new URL('../../corpus/testdata/float-import-signature.wasm',import.meta.url));
  assert(WebAssembly.validate(imported));
  assert.deepEqual(numericSignature(imported,'benchmark'),{params:['f32','i64'],results:['f64']});
  assert.deepEqual(numericSignature(b,'mixed_results'),{params:[],results:['f32','f64']});
  assert.deepEqual(floatArguments(['4294967295','18446744073709551615','1069547520','4612248968380809216'],['i32','i64','f32','f64']),[-1,-1n,1.5,2.25]);
});
