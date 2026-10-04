#[cfg(feature = "wavm")]
mod commands;
#[cfg(feature = "wavm")]
mod native_object;
use anyhow::{Result, bail, ensure};
use serde_json::{Value, json};
use sha2::{Digest, Sha256};
use std::{
    fs,
    io::{self, BufRead, Write},
    time::Instant,
};
#[cfg(feature = "wasmi")]
mod wasmi;
#[cfg(feature = "wasmi")]
use wasmi as embedding;
#[cfg(not(feature = "wasmi"))]
mod ffi;
#[cfg(not(feature = "wasmi"))]
use ffi as embedding;
#[cfg(any(
    feature = "wasmer_llvm",
    feature = "wasmer_singlepass",
    feature = "wavm"
))]
mod vectors;
const RUNTIME: &str = env!("WB_RUNTIME");
fn scenarios() -> Vec<&'static str> {
    if cfg!(feature = "wasm3") {
        vec!["compile", "first-call", "steady"]
    } else {
        vec!["compile", "instantiate", "first-call", "steady"]
    }
}
struct Adapter {
    prep: Option<Value>,
    bytes: Vec<u8>,
}
fn values(v: &Value) -> Result<Vec<u64>> {
    v.as_array()
        .ok_or_else(|| anyhow::anyhow!("decimal string array required"))?
        .iter()
        .map(|x| {
            Ok(x.as_str()
                .ok_or_else(|| anyhow::anyhow!("u64 string required"))?
                .parse()?)
        })
        .collect()
}
fn field<'a>(v: &'a Value, name: &str) -> Result<&'a str> {
    v[name]
        .as_str()
        .ok_or_else(|| anyhow::anyhow!("missing {name}"))
}
fn pointer(target: &mut embedding::Instance, w: &Value, name: &str) -> Result<usize> {
    match w[name].as_str().filter(|x| !x.is_empty()) {
        None => Ok(0),
        Some(export) => {
            let p = target.call(export, &[])?;
            ensure!(
                p.len() == 1 && p[0] <= u32::MAX as u64,
                "invalid wasm32 pointer"
            );
            Ok(p[0] as usize)
        }
    }
}
fn range(target: &mut embedding::Instance, offset: usize, n: usize) -> Result<&mut [u8]> {
    let end = offset
        .checked_add(n)
        .ok_or_else(|| anyhow::anyhow!("memory range overflow"))?;
    let memory = target.memory()?;
    ensure!(
        offset <= u32::MAX as usize && end <= memory.len(),
        "memory range out of bounds"
    );
    Ok(&mut memory[offset..end])
}
fn initialize(target: &mut embedding::Instance, w: &Value) -> Result<()> {
    if let Some(name) = w["initialize"].as_str().filter(|x| !x.is_empty()) {
        ensure!(
            target.call(name, &[])?.is_empty(),
            "unsupported: initializer must return no values"
        );
    }
    if let Some(input) = w.get("input").filter(|x| !x.is_null()) {
        let data = hex::decode(field(input, "hex")?)?;
        let base = pointer(target, input, "pointer_export")?;
        let offset = input["offset"]
            .as_u64()
            .filter(|x| *x <= u32::MAX as u64)
            .ok_or_else(|| anyhow::anyhow!("invalid input offset"))? as usize;
        range(
            target,
            base.checked_add(offset)
                .ok_or_else(|| anyhow::anyhow!("offset overflow"))?,
            data.len(),
        )?
        .copy_from_slice(&data);
    }
    Ok(())
}
fn verify(target: &mut embedding::Instance, w: &Value, result: &[u64]) -> Result<()> {
    ensure!(
        result == values(&w["oracle"]["expected"])?,
        "incorrect result: scalar oracle mismatch"
    );
    let base = pointer(target, &w["oracle"], "output_pointer_export")?;
    if let Some(checks) = w["oracle"]["memory"].as_array() {
        for check in checks {
            let expected = hex::decode(field(check, "hex")?)?;
            let offset = check["offset"]
                .as_u64()
                .filter(|x| *x <= u32::MAX as u64)
                .ok_or_else(|| anyhow::anyhow!("invalid memory offset"))?
                as usize;
            ensure!(
                range(
                    target,
                    base.checked_add(offset)
                        .ok_or_else(|| anyhow::anyhow!("offset overflow"))?,
                    expected.len()
                )? == expected,
                "incorrect result: memory oracle mismatch"
            );
        }
    }
    Ok(())
}
fn write(value: &Value) -> Result<()> {
    let mut out = io::stdout().lock();
    serde_json::to_writer(&mut out, value)?;
    writeln!(out)?;
    out.flush()?;
    Ok(())
}
fn barrier(input: &mut impl BufRead, id: &Value, index: usize, stage: &str) -> Result<()> {
    write(
        &json!({"version":1,"id":id,"status":"phase","phase":{"sample_index":index,"stage":stage}}),
    )?;
    let mut line = String::new();
    ensure!(
        input.read_line(&mut line)? > 0,
        "missing phase acknowledgement"
    );
    let ack: Value = serde_json::from_str(&line)?;
    ensure!(
        ack["version"] == 1 && ack["id"] == *id && ack["method"] == "continue",
        "invalid phase acknowledgement"
    );
    Ok(())
}
impl Adapter {
    fn handle(&mut self, req: &Value, input: &mut impl BufRead) -> Result<Value> {
        ensure!(
            req["version"] == 1 && req["id"].as_u64().is_some(),
            "invalid protocol envelope"
        );
        match req["method"].as_str().unwrap_or("") {
            "describe" => {
                let mut description = json!({"description":{
                    "runtime":RUNTIME,"runtime_version":embedding::version(),"backend":if cfg!(feature="wavm") || cfg!(feature="wasmer_llvm"){"llvm-jit"}else if cfg!(feature="wasmer_singlepass"){"singlepass-jit"}else{"interpreter"},"embedding":"standalone Rust/C embedding","build":format!("adapter-native/{}; {}",env!("CARGO_PKG_VERSION"),RUNTIME),
                    "effective_configuration":{"compile_policy":if cfg!(feature="wasmer_llvm") || cfg!(feature="wasmer_singlepass"){"explicit Wasmer compiler; fresh compilation store and eager module compilation; engine creation excluded; instance creates a fresh store from the same engine"}else if cfg!(feature="wamr"){"C-API store allocation and module loading/validation; classic interpreter with GC and exception handling enabled, software stack bounds, 1MiB Wasm stack and no injected application heap; no native compilation"}else if cfg!(feature="wasmedge"){"loader parse and validator; interpreter explicitly selected; no native compilation"}else if cfg!(feature="wasm3"){"parse, 1MiB stack runtime allocation, module load and eager bytecode compilation; module tied to runtime"}else{"eager module API; engine creation excluded"},"call_policy":"embedding export lookup, integer argument/result marshalling and result allocation included; every result verified outside timed batch","wasmi_dispatch":if cfg!(feature="wasmi"){"portable-dispatch; eager bytecode; no fuel"}else{"not applicable; external SDK build identity pinned"},"reset_policy":"fresh lifecycle instance; stateless steady retained instance; wasm3 recompiles outside first-call timing","release_policy":"native instance/store disposal; no physical RSS reclamation claim"},
                    "capabilities":{"can_compile_separately":true,"can_instantiate_separately":!cfg!(feature="wasm3"),"can_host_function_calls_v1":cfg!(feature="wasmi") || cfg!(feature="wasmer_llvm") || cfg!(feature="wasmer_singlepass") || cfg!(feature="wasm3") || cfg!(feature="wamr") || cfg!(feature="wavm")},"scenarios":scenarios(),"abis":["core"],"features":["mvp"],"phase_barrier_scenarios":scenarios(),"phase_release_policy":"release native instance; compiled module/engine retained except compile samples; no RSS reclamation claim"
                }});
                if cfg!(feature = "wasmer_llvm") || cfg!(feature = "wasmer_singlepass") {
                    description["description"]["validator_features"] = json!({"namespace":"wasmparser/0.251.0","evidence":"Pinned Wasmer 7.3.0 compiler configuration and explicit C API feature subset; Singlepass SIMD disabled after corpus instruction lowering failures; enabled validation is not a guarantee of complete backend support","supported":{"GC":false,"FUNCTION_REFERENCES":false,"MEMORY64":false,"STACK_SWITCHING":false,"SIMD":cfg!(feature="wasmer_llvm"),"RELAXED_SIMD":cfg!(feature="wasmer_llvm"),"EXCEPTIONS":cfg!(feature="wasmer_llvm"),"LEGACY_EXCEPTIONS":cfg!(feature="wasmer_llvm"),"TAIL_CALL":cfg!(feature="wasmer_llvm")}});
                }
                if cfg!(feature = "wasmer_llvm")
                    || cfg!(feature = "wasmer_singlepass")
                    || cfg!(feature = "wavm")
                {
                    #[cfg(any(feature = "wasmer_singlepass", feature = "wasmer_llvm"))]
                    {
                        if let Some(metadata) =
                            option_env!("WB_SDK_RECEIPT_JSON").filter(|s| !s.is_empty())
                        {
                            description["description"]["effective_configuration"]["native_sdk_receipt"] =
                                json!(serde_json::from_str::<Value>(metadata)?.to_string());
                        }
                        description["description"]["capabilities"]["can_code_profile"] =
                            json!(embedding::can_native_size());
                        description["description"]["capabilities"]["can_measure_native_code_size"] =
                            json!(embedding::can_native_size());
                        description["description"]["effective_configuration"]["native_code_policy"] = json!(
                            "Optional SDK C ABI getter delegates to public sys_artifact/finished_function_extents; complete defined-function coverage required; excludes call trampolines; serialized artifact bytes are not reported as native code"
                        );
                    }
                    description["description"]["capabilities"]["can_run_vectors"] = json!(true);
                    for name in ["compile", "instantiate", "first-call"] {
                        description["description"]["capabilities"]
                            [format!("can_vector_{name}_phases")] = json!(true);
                    }
                    description["description"]["effective_configuration"]["vector_policy"] = json!(
                        "Fresh instance per ordered vector sequence; input writes and exact output verification excluded from sum of timed export lookups, integer marshalling and embedding calls; one operation per complete sequence; lifecycle windows match scalar API phases"
                    );
                    description["description"]["effective_configuration"]["assemblyscript_abort_policy"] = json!(
                        "Only env.abort with four i32 parameters and no results; imported callback creates a real guest trap; identity host imports use their separately validated profile"
                    );
                }
                if cfg!(feature = "wavm") {
                    description["description"]["abis"] = json!(["core", "wasi-command"]);
                    description["description"]["effective_configuration"]["command_policy"] = json!(
                        "Public WAVM WASI embedding; fresh process and instance per operation; fixture staging excluded; instantiation includes compartment, context, WASI process/resolver and guest instantiation; call includes only _start invocation and exit capture; output verification excluded; bounded stdio; read-only verified fixture directory; networking disabled; missing sock_accept receives ENOTCAPABLE rather than success; engine clocks and random source retained"
                    );
                    description["description"]["capabilities"]["can_run_commands"] = json!(true);
                    for name in ["compile", "instantiate", "first-call"] {
                        description["description"]["capabilities"]
                            [format!("can_command_{name}_phases")] = json!(true);
                    }
                    description["description"]["capabilities"]["can_measure_native_code_size"] =
                        json!(true);
                }
                Ok(description)
            }
            #[cfg(any(feature = "wasmer_singlepass", feature = "wasmer_llvm"))]
            "inspect" => {
                ensure!(self.prep.is_some(), "prepare required");
                ensure!(
                    embedding::can_native_size(),
                    "unsupported: SDK native-size getter unavailable"
                );
                let engine = embedding::Engine::new()?;
                let module = engine.compile(&self.bytes)?;
                let size = module.native_size()?;
                ensure!(
                    size as u64 <= (1u64 << 53),
                    "native size exceeds exact numeric range"
                );
                Ok(
                    json!({"diagnostics":[{"metric":"native.code_size","definition_version":1,"status":"available","value":size,"unit":"bytes","scope":"compiled_module","phase":"compile","collector":"Wasmer/sys_artifact/finished_function_extents","collector_version":embedding::version(),"quality":"engine_reported","profile":"code","normalization_denominator":"module","reason":"Sum of all defined-function native extents; excludes call trampolines, metadata and serialized artifact headers. Complete byte export unavailable."},{"metric":"native.code_export","definition_version":1,"status":"unavailable","unit":"bytes","scope":"compiled_module","phase":"compile","collector":"Wasmer/sys_artifact/finished_function_extents","collector_version":embedding::version(),"quality":"engine_reported","profile":"code","normalization_denominator":"module","reason":"Native function lengths captured; native image byte export is not configured"}]}),
                )
            }
            #[cfg(feature = "wavm")]
            "inspect" => {
                ensure!(self.prep.is_some(), "prepare required");
                native_object::inspect(&embedding::object_code(&self.bytes)?, &embedding::version())
            }
            "prepare" => {
                self.prep = None;
                self.bytes.clear();
                let p = &req["prepare"];
                let w = &p["workload"];
                #[cfg(feature = "wavm")]
                if w["abi"] == "wasi-command" {
                    commands::validate(w)?;
                    ensure!(
                        ["timing", "memory", "code"].contains(&field(p, "profile")?),
                        "unsupported: command profile"
                    );
                    let bytes = fs::read(field(p, "artifact")?)?;
                    ensure!(
                        hex::encode(Sha256::digest(&bytes)) == field(p, "artifact_sha256")?,
                        "artifact digest mismatch"
                    );
                    self.bytes = bytes;
                    self.prep = Some(p.clone());
                    return Ok(json!({}));
                }
                let vector_contract = (cfg!(feature = "wasmer_llvm")
                    || cfg!(feature = "wasmer_singlepass")
                    || cfg!(feature = "wavm"))
                    && w["oracle"]["kind"] == "exact_vectors";
                let assemblyscript = (cfg!(feature = "wasmer_llvm")
                    || cfg!(feature = "wasmer_singlepass")
                    || cfg!(feature = "wavm"))
                    && w["host_profile"] == "assemblyscript-abort-v1";
                let identity_host = (cfg!(feature = "wasmi")
                    || cfg!(feature = "wasmer_llvm")
                    || cfg!(feature = "wasmer_singlepass")
                    || cfg!(feature = "wasm3")
                    || cfg!(feature = "wamr")
                    || cfg!(feature = "wavm"))
                    && w["host_profile"] == "identity-v1";
                #[cfg(any(feature = "wasmer_singlepass", feature = "wasmer_llvm"))]
                let native_size_profile = embedding::can_native_size() && p["profile"] == "code";
                #[cfg(not(any(feature = "wasmer_singlepass", feature = "wasmer_llvm")))]
                let native_size_profile = false;
                ensure!(
                    (["timing", "memory"].contains(&field(p, "profile")?)
                        || (cfg!(feature = "wavm") && p["profile"] == "code")
                        || native_size_profile)
                        && w["abi"] == "core"
                        && ["stateless", "fresh_instance_per_sample"].contains(&field(w, "reset")?)
                        && (w["oracle"]["kind"] == "exact_u64" || vector_contract)
                        && !field(w, "export")?.is_empty(),
                    "unsupported: core integer scalar timing/memory only"
                );
                for key in [
                    "host_profile",
                    "command",
                    "vectors",
                    "density",
                    "checkpoint",
                    "continuation",
                    "process_snapshot",
                    "guest_density",
                    "snapshot_density",
                ] {
                    if key == "host_profile" && (assemblyscript || identity_host)
                        || key == "vectors" && vector_contract
                    {
                        continue;
                    }
                    ensure!(
                        w[key].is_null() || w[key] == "",
                        "unsupported: extended workload contract {key}"
                    );
                }
                ensure!(
                    w["oracle"]["float"].is_null()
                        && (w["oracle"]["expected_trap"].is_null()
                            || w["oracle"]["expected_trap"] == ""),
                    "unsupported: extended oracle"
                );
                values(&w["args"])?;
                values(&w["oracle"]["expected"])?;
                #[cfg(any(
                    feature = "wasmer_llvm",
                    feature = "wasmer_singlepass",
                    feature = "wavm"
                ))]
                if vector_contract {
                    vectors::validate(w)?;
                }
                let bytes = fs::read(field(p, "artifact")?)?;
                ensure!(
                    hex::encode(Sha256::digest(&bytes)) == field(p, "artifact_sha256")?,
                    "artifact digest mismatch"
                );
                for payload in wasmparser::Parser::new(0).parse_all(&bytes) {
                    match payload? {
                        wasmparser::Payload::Version { encoding, .. } => ensure!(
                            encoding == wasmparser::Encoding::Module,
                            "unsupported: core binary required"
                        ),
                        wasmparser::Payload::ImportSection(imports) => ensure!(
                            imports.count() == 0 || assemblyscript || identity_host,
                            "unsupported: imports require an explicitly supported host profile"
                        ),
                        _ => {}
                    }
                }
                self.bytes = bytes;
                self.prep = Some(p.clone());
                Ok(json!({}))
            }
            "run" => {
                #[cfg(feature = "wavm")]
                if self
                    .prep
                    .as_ref()
                    .is_some_and(|p| p["workload"]["abi"] == "wasi-command")
                {
                    return self.run_command(req, input);
                }
                #[cfg(any(
                    feature = "wasmer_llvm",
                    feature = "wasmer_singlepass",
                    feature = "wavm"
                ))]
                if self
                    .prep
                    .as_ref()
                    .is_some_and(|p| p["workload"]["oracle"]["kind"] == "exact_vectors")
                {
                    return self.run_vectors(req, input);
                }
                let p = self
                    .prep
                    .as_ref()
                    .ok_or_else(|| anyhow::anyhow!("prepare required"))?;
                let w = &p["workload"];
                let r = &req["run"];
                let scenario = field(r, "scenario")?;
                ensure!(
                    scenarios().contains(&scenario),
                    "unsupported: scenario not advertised"
                );
                let phases = r["phase_barriers"].as_bool().unwrap_or(false);
                ensure!(
                    !phases || p["profile"] == "memory",
                    "unsupported: barriers require memory profile"
                );
                let bounded = |name: &str, min: u64, max: u64| -> Result<usize> {
                    Ok(r[name]
                        .as_u64()
                        .filter(|x| *x >= min && *x <= max)
                        .ok_or_else(|| anyhow::anyhow!("invalid bounded {name}"))?
                        as usize)
                };
                let samples = bounded("samples", 1, 100000)?;
                let operations = bounded("operations", 1, 1000000)?;
                let requested_warmup = bounded("warmup", 0, 100000)?;
                // The harness protocol applies warmups only to steady samples.
                // Fresh lifecycle samples must remain cold even when collection
                // options also request warmups for steady measurements.
                let warmup = if scenario != "steady" {
                    0
                } else {
                    requested_warmup
                };
                ensure!(
                    scenario == "steady" || (operations == 1 && warmup == 0),
                    "unsupported: lifecycle requires one operation and zero warmup"
                );
                ensure!(
                    scenario != "steady"
                        || w["reset"] == "stateless"
                        || (operations == 1 && !cfg!(feature = "wasm3")),
                    "unsupported: steady requires stateless reset, or one operation under fresh-instance policy"
                );
                let engine = embedding::Engine::new()?;
                let shared = if scenario == "compile" || cfg!(feature = "wasm3") {
                    None
                } else {
                    Some(engine.compile(&self.bytes)?)
                };
                let mut steady_module = None;
                if scenario == "steady" && shared.is_none() {
                    steady_module = Some(engine.compile(&self.bytes)?);
                }
                let mut steady = if scenario == "steady" && w["reset"] == "stateless" {
                    let mut target =
                        engine.instantiate(shared.as_ref().or(steady_module.as_ref()).unwrap())?;
                    initialize(&mut target, w)?;
                    Some(target)
                } else {
                    None
                };
                let args = values(&w["args"])?;
                let export = field(w, "export")?;
                if let Some(target) = steady.as_mut() {
                    let result = target.call(export, &args)?;
                    verify(target, w, &result)?;
                }
                let stages = match scenario {
                    "compile" => ["before_compile", "compiled", "released"],
                    "instantiate" => ["before_instantiate", "instantiated", "instance_released"],
                    "first-call" => [
                        "before_first_call",
                        "first_call_returned",
                        "first_call_released",
                    ],
                    _ => [
                        "before_steady_batch",
                        "steady_batch_returned",
                        "steady_batch_verified",
                    ],
                };
                let mut out = Vec::new();
                for i in 0..samples + warmup {
                    let mut local = if cfg!(feature = "wasm3") && scenario == "first-call" {
                        Some(engine.compile(&self.bytes)?)
                    } else {
                        None
                    };
                    let mut instance = if scenario == "first-call"
                        || (scenario == "steady" && w["reset"] == "fresh_instance_per_sample")
                    {
                        let mut target =
                            engine.instantiate(shared.as_ref().or(local.as_ref()).unwrap())?;
                        initialize(&mut target, w)?;
                        Some(target)
                    } else {
                        None
                    };
                    let mut results = Vec::with_capacity(operations);
                    if phases {
                        barrier(input, &req["id"], i, stages[0])?;
                    }
                    let start = Instant::now();
                    match scenario {
                        "compile" => local = Some(engine.compile(&self.bytes)?),
                        "instantiate" => {
                            instance = Some(engine.instantiate(shared.as_ref().unwrap())?)
                        }
                        _ => {
                            let target = instance.as_mut().or(steady.as_mut()).unwrap();
                            for _ in 0..operations {
                                results.push(target.call(export, &args)?);
                            }
                        }
                    }
                    let elapsed = start.elapsed().as_nanos();
                    ensure!(elapsed <= i64::MAX as u128, "elapsed overflow");
                    if phases {
                        barrier(input, &req["id"], i, stages[1])?;
                    }
                    if scenario == "compile" {
                        instance = Some(engine.instantiate(local.as_ref().unwrap())?);
                    }
                    let target = instance.as_mut().or(steady.as_mut()).unwrap();
                    if scenario == "compile" || scenario == "instantiate" {
                        initialize(target, w)?;
                        results.push(target.call(export, &args)?);
                    }
                    for result in &results {
                        verify(target, w, result)?;
                    }
                    let observations = if p["profile"] == "memory" {
                        if let Ok(memory) = target.memory() {
                            json!([{"metric":"guest.memory.logical","definition_version":1,"value":memory.len(),"unit":"bytes","scope":"guest_linear_memory","phase":scenario,"collector":"runtime memory API","collector_version":embedding::version(),"quality":"engine_reported","profile":"memory","status":"available","normalization_denominator":"instance"}])
                        } else {
                            json!([])
                        }
                    } else {
                        json!([])
                    };
                    let result = results
                        .last()
                        .unwrap()
                        .iter()
                        .map(u64::to_string)
                        .collect::<Vec<_>>();
                    drop(instance);
                    drop(local);
                    if phases {
                        barrier(input, &req["id"], i, stages[2])?;
                    }
                    out.push(json!({"index":i,"warmup":i<warmup,"elapsed_ns":elapsed as u64,"operations":if scenario=="steady"{operations}else{1},"sample_type":if scenario=="steady"&&operations>1{"batch_average"}else{"individual_operation"},"verified":true,"result":result,"observations":observations}));
                }
                Ok(json!({"samples":out}))
            }
            "close" => {
                self.prep = None;
                self.bytes.clear();
                Ok(json!({}))
            }
            _ => bail!("unsupported: unknown method"),
        }
    }
}
fn main() -> Result<()> {
    let mut input = io::stdin().lock();
    let mut adapter = Adapter {
        prep: None,
        bytes: Vec::new(),
    };
    loop {
        let mut line = String::new();
        if input.read_line(&mut line)? == 0 {
            break;
        }
        let req: Value = match serde_json::from_str(&line) {
            Ok(v) => v,
            Err(e) => {
                write(&json!({"version":1,"id":0,"status":"error","reason":e.to_string()}))?;
                continue;
            }
        };
        let mut response = json!({"version":1,"id":req["id"],"status":"ok"});
        match adapter.handle(&req, &mut input) {
            Ok(value) => {
                for (k, v) in value.as_object().unwrap() {
                    response[k] = v.clone();
                }
            }
            Err(e) => {
                let reason = e.to_string();
                response["status"] = json!(if reason.starts_with("unsupported:") {
                    "unsupported"
                } else {
                    "error"
                });
                response["reason"] = json!(reason);
            }
        }
        write(&response)?;
        if req["method"] == "close" {
            break;
        }
    }
    Ok(())
}
