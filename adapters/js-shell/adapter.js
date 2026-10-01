// Direct engine adapter. No Node, npm, jsvu, subprocess, or browser dependency.
// Only JSON control goes to stdout; import-free guests cannot print there.
(async function () {
  'use strict';
  const argv = typeof Deno !== 'undefined' ? Deno.args :
    typeof scriptArgs !== 'undefined' ? scriptArgs : globalThis.arguments || [];
  const option = name => argv.find(x => x.startsWith(name + '='))?.slice(name.length + 1);
  const runtime = option('runtime');
  if (!['v8-shell', 'spidermonkey', 'jsc', 'deno'].includes(runtime)) throw Error('explicit runtime identity required');
  const deno = runtime === 'deno';
  let pending = '', eof = false;
  const decoder = deno ? new TextDecoder() : null;
  const encoder = deno ? new TextEncoder() : null;
  async function line() {
    if (!deno) return readline();
    while (!pending.includes('\n') && !eof) {
      const buffer = new Uint8Array(4096), n = await Deno.stdin.read(buffer);
      if (n === null) { pending += decoder.decode(); eof = true; }
      else pending += decoder.decode(buffer.subarray(0, n), {stream: true});
    }
    const end = pending.indexOf('\n');
    if (end < 0) { const rest = pending; pending = ''; return rest || null; }
    const result = pending.slice(0, end); pending = pending.slice(end + 1); return result;
  }
  async function write(value) {
    const text = JSON.stringify(value);
    if (!deno) { print(text); return; }
    const data = encoder.encode(text + '\n');
    for (let offset = 0; offset < data.length;) offset += await Deno.stdout.write(data.subarray(offset));
  }
  const readBytes = path => deno ? Deno.readFileSync(path) :
    new Uint8Array(runtime === 'v8-shell' ? readbuffer(path) : read(path, 'binary'));
  // Never substitute Date.now / JSC preciseTime wall time for a monotonic timer.
  const hostClock = typeof benchNow === 'function';
  const monotonic = typeof performance !== 'undefined' && typeof performance.now === 'function';
  if (!hostClock && !monotonic) throw Error('monotonic clock unavailable; JSC on macOS requires the raw native host');
  const now = hostClock ? () => benchNow() * 1e6 : () => performance.now() * 1e6;
  const scenarios = ['compile', 'instantiate', 'first-call', 'steady'];
  let prep, bytes, signature;
  function unsupported(message) { const error = Error(message); error.unsupported = true; throw error; }
  function hex(text) {
    if (typeof text !== 'string' || !/^(?:[0-9a-fA-F]{2})*$/.test(text)) throw Error('invalid hex');
    return Uint8Array.from(text.match(/../g) || [], x => parseInt(x, 16));
  }
  // SHA-256 implemented over bytes, including the complete padding length.
  function sha256(data) {
    const rotr = (x, n) => x >>> n | x << (32 - n);
    const primes = [], initial = [], constants = [];
    for (let p = 2; primes.length < 64; p++) {
      if (primes.some(q => p % q === 0)) continue;
      primes.push(p);
      if (initial.length < 8) initial.push(Math.sqrt(p) * 4294967296 | 0);
      constants.push(Math.cbrt(p) * 4294967296 | 0);
    }
    const padded = new Uint8Array(Math.ceil((data.length + 9) / 64) * 64);
    padded.set(data); padded[data.length] = 128;
    const view = new DataView(padded.buffer), bitLength = BigInt(data.length) * 8n;
    view.setUint32(padded.length - 8, Number(bitLength >> 32n));
    view.setUint32(padded.length - 4, Number(bitLength & 0xffffffffn));
    const h = initial, w = new Int32Array(64);
    for (let offset = 0; offset < padded.length; offset += 64) {
      for (let i = 0; i < 16; i++) w[i] = view.getInt32(offset + 4 * i);
      for (let i = 16; i < 64; i++) {
        const x = w[i - 15], y = w[i - 2];
        w[i] = (rotr(x, 7) ^ rotr(x, 18) ^ x >>> 3) + w[i - 16] +
          (rotr(y, 17) ^ rotr(y, 19) ^ y >>> 10) + w[i - 7];
      }
      let [a, b, c, d, e, f, g, hh] = h;
      for (let i = 0; i < 64; i++) {
        const t1 = (hh + (rotr(e, 6) ^ rotr(e, 11) ^ rotr(e, 25)) +
          (e & f ^ ~e & g) + constants[i] + w[i]) | 0;
        const t2 = ((rotr(a, 2) ^ rotr(a, 13) ^ rotr(a, 22)) + (a & b ^ a & c ^ b & c)) | 0;
        hh = g; g = f; f = e; e = d + t1 | 0; d = c; c = b; b = a; a = t1 + t2 | 0;
      }
      [a,b,c,d,e,f,g,hh].forEach((x, i) => { h[i] = h[i] + x | 0; });
    }
    return h.map(x => (x >>> 0).toString(16).padStart(8, '0')).join('');
  }
  // Read just numeric function signatures; the runtime still validates the module.
  function numericSignature(data, name) {
    let pos = 8, limit = data.length, types = [], functions = [], target;
    function byte() { if (pos >= limit) throw Error('truncated Wasm'); return data[pos++]; }
    function u32() {
      let n = 0;
      for (let i = 0; i < 5; i++) { const b = byte(); if (i === 4 && b > 15) throw Error('invalid u32'); n += (b & 127) * 2 ** (7 * i); if (b < 128) return n; }
      throw Error('invalid u32');
    }
    function vector(fn) { const n = u32(); if (n > limit - pos) throw Error('invalid vector'); return Array.from({length: n}, fn); }
    function text() { return vector(byte).map(x => String.fromCharCode(x)).join(''); }
    if (data.length < 8 || data.slice(0,8).some((x,i) => x !== [0,97,115,109,1,0,0,0][i])) unsupported('core Wasm binary required');
    while (pos < data.length) {
      limit = data.length;
      const id = byte(), length = u32(), end = pos + length;
      if (end > data.length) throw Error('truncated section');
      limit = end;
      if (id === 1) types = vector(() => { if (byte() !== 96) unsupported('non-function type unsupported'); return {params: vector(byte), results: vector(byte)}; });
      if (id === 2 && u32() !== 0) unsupported('import-free core modules only');
      if (id === 3) functions = vector(u32);
      if (id === 7) vector(() => { const exportName = text(), kind = byte(), index = u32(); if (exportName === name && kind === 0) target = index; return null; });
      pos = end;
    }
    const sig = types[functions[target]];
    if (!sig || [...sig.params, ...sig.results].some(x => ![127,126].includes(x))) unsupported('i32/i64 function signature required');
    return sig;
  }
  function canonical(value) {
    if (typeof value !== 'string' || !/^(0|[1-9][0-9]*)$/.test(value) || BigInt(value) > 0xffffffffffffffffn) throw Error('invalid u64 decimal string');
    return value;
  }
  function callArgs() {
    const args = prep.workload.args || [];
    if (args.length !== signature.params.length) throw Error('argument count mismatch');
    return args.map((x, i) => signature.params[i] === 126 ? BigInt.asIntN(64, BigInt(canonical(x))) : Number(BigInt.asIntN(32, BigInt(canonical(x)))));
  }
  function invoke(target, args) {
    const value = target.exports[prep.workload.export](...args);
    const values = value === undefined ? [] : Array.isArray(value) ? value : [value];
    if (values.length !== signature.results.length) throw Error('result count mismatch');
    return values.map((x, i) => signature.results[i] === 126 ? BigInt.asUintN(64, x).toString() : String(x >>> 0));
  }
  function pointer(target, name) {
    if (!name) return 0;
    const fn = target.exports[name];
    if (typeof fn !== 'function') throw Error('missing pointer export');
    const p = fn();
    if (!Number.isInteger(p) || p < -2147483648 || p > 4294967295) throw Error('invalid wasm32 pointer');
    return p >>> 0;
  }
  function memory(target, offset, length) {
    const mem = target.exports.memory;
    if (!Number.isSafeInteger(offset) || offset < 0 || offset > 0xffffffff || !mem || offset + length > mem.buffer.byteLength) throw Error('memory range out of bounds');
    return new Uint8Array(mem.buffer, offset, length);
  }
  function initialize(target) {
    const w = prep.workload;
    if (w.initialize) { if (typeof target.exports[w.initialize] !== 'function') throw Error('missing initializer'); target.exports[w.initialize](); }
    if (w.input) { const data = hex(w.input.hex); memory(target, pointer(target,w.input.pointer_export) + w.input.offset, data.length).set(data); }
    return target;
  }
  function verify(result, target) {
    const w = prep.workload;
    if (JSON.stringify(result) !== JSON.stringify(w.oracle.expected.map(canonical))) throw Error('incorrect result: scalar oracle mismatch');
    const base = pointer(target, w.oracle.output_pointer_export);
    for (const check of w.oracle.memory || []) { const expected = hex(check.hex), actual = memory(target, base + check.offset, expected.length); if (actual.some((x,i) => x !== expected[i])) throw Error('incorrect result: memory oracle mismatch'); }
  }
  async function barrier(req, index, stage) {
    await write({version:1,id:req.id,status:'phase',phase:{sample_index:index,stage}});
    const raw = await line(); if (!raw) throw Error('missing phase acknowledgement');
    const ack = JSON.parse(raw);
    if (ack.version !== 1 || ack.id !== req.id || ack.method !== 'continue') throw Error('invalid phase acknowledgement');
  }
  async function handle(req) {
    if (req.version !== 1 || !Number.isSafeInteger(req.id)) throw Error('invalid protocol envelope');
    if (req.method === 'describe') return {description:{
      runtime: runtime === 'v8-shell' || deno ? 'v8' : runtime,
      runtime_version: deno ? Deno.version.v8 : option('binary-sha256') ? 'binary-sha256:' + option('binary-sha256') : 'unreported',
      embedding: deno ? 'Deno WebAssembly API' : runtime + ' standalone shell WebAssembly API',
      backend:'production-default-tiering', build: deno ? 'Deno ' + Deno.version.deno : 'binary-sha256:' + option('binary-sha256'),
      effective_configuration:{clock:hostClock?'std::chrono::steady_clock':'performance.now monotonic', compilation_policy:'WebAssembly.Module API return; lazy/background compilation and engine caches uncontrolled; no materialization claim', reset_policy:'fresh instance per lifecycle sample; stateless steady reuses one instance', release_policy:'drop JS references only; GC and physical reclamation uncontrolled; no forced GC'},
      capabilities:{can_compile_separately:true,can_instantiate_separately:true},
      scenarios,abis:['core'],features:['mvp'],phase_barrier_scenarios:scenarios,
      phase_release_policy:'drop JS references only; no physical reclamation or engine disposal claim'
    }};
    if (req.method === 'prepare') {
      prep = bytes = signature = undefined;
      const p = req.prepare, w = p?.workload;
      if (!w || !['timing','memory'].includes(p.profile) || w.abi !== 'core' || !['stateless','fresh_instance_per_sample'].includes(w.reset) || w.oracle?.kind !== 'exact_u64' || !Array.isArray(w.oracle.expected) || !w.export || w.host_profile || w.command || w.vectors || w.density || w.checkpoint || w.continuation || w.process_snapshot || w.guest_density || w.snapshot_density || w.oracle.float || w.oracle.expected_trap) unsupported('only import-free core integer scalar timing/memory contracts supported');
      const data = readBytes(p.artifact);
      if (sha256(data) !== p.artifact_sha256) throw Error('artifact digest mismatch');
      const sig = numericSignature(data, w.export);
      if (w.initialize) {
        const init = numericSignature(data,w.initialize);
        if (init.params.length || init.results.length) unsupported('initializer must have () -> () signature');
      }
      for (const name of [w.input?.pointer_export,w.oracle.output_pointer_export].filter(Boolean)) {
        const ptr = numericSignature(data,name);
        if (ptr.params.length || ptr.results.length !== 1 || ptr.results[0] !== 127) unsupported('pointer export must have () -> i32 signature');
      }
      if (w.oracle.expected.length !== sig.results.length) throw Error('oracle result count mismatch');
      w.oracle.expected.forEach(canonical);
      prep = p; bytes = data; signature = sig;
      try { callArgs(); } catch(error) { prep = bytes = signature = undefined; throw error; }
      return {};
    }
    if (req.method === 'run') {
      if (!prep) throw Error('prepare required');
      const r = req.run;
      if (!r || !scenarios.includes(r.scenario) || (r.phase_barriers && prep.profile !== 'memory')) unsupported('unsupported scenario/profile/barriers');
      for (const [key,min,max] of [['samples',1,100000],['operations',1,1000000],['warmup',0,100000]]) if (!Number.isSafeInteger(r[key]) || r[key] < min || r[key] > max) throw Error('invalid bounded batch');
      if (r.scenario === 'steady' && prep.workload.reset !== 'stateless') unsupported('steady batches require stateless reset');
      if (r.scenario !== 'steady' && (r.operations !== 1 || r.warmup !== 0)) unsupported('lifecycle samples require one operation and zero warmup');
      const args = callArgs(), warmup = r.scenario === 'steady' ? r.warmup : 0, samples = [];
      let compiled = r.scenario === 'compile' ? undefined : new WebAssembly.Module(bytes);
      let shared = r.scenario === 'steady' ? initialize(new WebAssembly.Instance(compiled, {})) : undefined;
      if (shared) verify(invoke(shared,args),shared);
      const stages = {compile:['before_compile','compiled','released'],instantiate:['before_instantiate','instantiated','instance_released'],'first-call':['before_first_call','first_call_returned','first_call_released'],steady:['before_steady_batch','steady_batch_returned','steady_batch_verified']}[r.scenario];
      for (let i = 0; i < warmup + r.samples; i++) {
        let target = shared, module = compiled;
        if (r.scenario === 'first-call') target = initialize(new WebAssembly.Instance(module, {}));
        const results = new Array(r.scenario === 'steady' ? r.operations : 1);
        if (r.phase_barriers) await barrier(req,i,stages[0]);
        const start = now();
        if (r.scenario === 'compile') module = new WebAssembly.Module(bytes);
        else if (r.scenario === 'instantiate') target = new WebAssembly.Instance(module, {});
        else for (let j = 0; j < results.length; j++) results[j] = target.exports[prep.workload.export](...args);
        const elapsed = Math.round(now() - start);
        if (!Number.isSafeInteger(elapsed) || elapsed < 0) throw Error('invalid elapsed clock interval');
        if (r.phase_barriers) await barrier(req,i,stages[1]);
        let result;
        if (r.scenario === 'compile' || r.scenario === 'instantiate') { target = initialize(target || new WebAssembly.Instance(module, {})); result = invoke(target,args); verify(result,target); }
        else for (const value of results) {
          const values = value === undefined ? [] : Array.isArray(value) ? value : [value];
          result = values.map((x,k) => signature.results[k] === 126 ? BigInt.asUintN(64,x).toString() : String(x >>> 0));
          verify(result,target);
        }
        const observations = prep.profile === 'memory' && target.exports.memory ? [{metric:'guest.memory.logical',definition_version:1,value:target.exports.memory.buffer.byteLength,unit:'bytes',scope:'guest_linear_memory',phase:r.scenario,collector:'WebAssembly.Memory.buffer',collector_version:'1',quality:'engine_reported',profile:'memory',status:'available',normalization_denominator:'instance'}] : [];
        target = module = undefined;
        if (r.phase_barriers) await barrier(req,i,stages[2]);
        samples.push({index:i,warmup:i<warmup,elapsed_ns:elapsed,operations:results.length,sample_type:results.length===1?'individual_operation':'batch_average',verified:true,result,observations});
      }
      shared = compiled = undefined;
      return {samples};
    }
    if (req.method === 'close') { prep = bytes = signature = undefined; return {}; }
    unsupported('unknown method');
  }
  while (true) {
    const raw = await line(); if (!raw) break;
    let req;
    try { req = JSON.parse(raw); await write({version:1,id:req.id,status:'ok',...await handle(req)}); }
    catch (error) { await write({version:1,id:req?.id || 0,status:error.unsupported?'unsupported':'error',reason:String(error.message || error)}); }
    if (req?.method === 'close') break;
  }
})();
