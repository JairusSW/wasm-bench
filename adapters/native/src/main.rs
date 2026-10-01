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
            "describe" => Ok(json!({"description":{
                "runtime":RUNTIME,"runtime_version":embedding::version(),"backend":if cfg!(feature="wavm"){"llvm-jit"}else{"interpreter"},"embedding":"standalone Rust/C embedding","build":format!("adapter-native/{}; {}",env!("CARGO_PKG_VERSION"),RUNTIME),
                "effective_configuration":{"compile_policy":if cfg!(feature="wasmedge"){"loader parse and validator; interpreter explicitly selected; no native compilation"}else if cfg!(feature="wasm3"){"parse, 1MiB stack runtime allocation, module load and eager bytecode compilation; module tied to runtime"}else{"eager module API; engine creation excluded"},"call_policy":"embedding export lookup, integer argument/result marshalling and result allocation included; every result verified outside timed batch","wasmi_dispatch":if cfg!(feature="wasmi"){"portable-dispatch; eager bytecode; no fuel"}else{"not applicable; external SDK build identity pinned"},"reset_policy":"fresh lifecycle instance; stateless steady retained instance; wasm3 recompiles outside first-call timing","release_policy":"native instance/store disposal; no physical RSS reclamation claim"},
                "capabilities":{"can_compile_separately":true,"can_instantiate_separately":!cfg!(feature="wasm3")},"scenarios":scenarios(),"abis":["core"],"features":["mvp"],"phase_barrier_scenarios":scenarios(),"phase_release_policy":"release native instance; compiled module/engine retained except compile samples; no RSS reclamation claim"
            }})),
            "prepare" => {
                self.prep = None;
                self.bytes.clear();
                let p = &req["prepare"];
                let w = &p["workload"];
                ensure!(
                    ["timing", "memory"].contains(&field(p, "profile")?)
                        && w["abi"] == "core"
                        && ["stateless", "fresh_instance_per_sample"].contains(&field(w, "reset")?)
                        && w["oracle"]["kind"] == "exact_u64"
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
                            imports.count() == 0,
                            "unsupported: import-free modules only"
                        ),
                        _ => {}
                    }
                }
                self.bytes = bytes;
                self.prep = Some(p.clone());
                Ok(json!({}))
            }
            "run" => {
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
                let warmup = bounded("warmup", 0, 100000)?;
                ensure!(
                    scenario == "steady" || (operations == 1 && warmup == 0),
                    "unsupported: lifecycle requires one operation and zero warmup"
                );
                ensure!(
                    scenario != "steady" || w["reset"] == "stateless",
                    "unsupported: steady requires stateless reset"
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
                let mut steady = if scenario == "steady" {
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
                    let mut instance = if scenario == "first-call" {
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
