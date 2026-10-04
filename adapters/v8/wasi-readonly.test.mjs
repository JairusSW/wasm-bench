import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readonlyWasiImports } from './wasi-readonly.mjs';
test('read-only opens retain read/seek/stat rights without requesting a writable host descriptor', () => {
  let received;
  const host = { path_open: (...args) => { received=args; return 0; } };
  const imports=readonlyWasiImports({ getImportObject:()=>({wasi_snapshot_preview1:host}) });
  const read=(1n<<1n)|(1n<<2n)|(1n<<21n), write=(1n<<0n)|(1n<<6n)|(1n<<8n)|(1n<<22n)|(1n<<23n);
  assert.equal(imports.wasi_snapshot_preview1.path_open(3,1,16,8,0,read|write,read|write,0,32),0);
  assert.deepEqual(received,[3,1,16,8,0,read,read,0,32]);
  for(const oflags of [1,4,8])assert.equal(host.path_open(3,1,16,8,oflags,read,0n,0,32),76);
  for(const fdflags of [1,2,8,16])assert.equal(host.path_open(3,1,16,8,0,read,0n,fdflags,32),76);
});
