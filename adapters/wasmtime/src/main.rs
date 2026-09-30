use serde_json::{Value, json};
#[cfg(feature = "native-allocator")]
mod allocator;
mod app_init;
#[cfg(all(feature = "native-code-lifetime", target_os = "linux"))]
mod code_lifetime;
mod commands;
mod component_calls;
mod components;
mod counters;
mod density;
mod emscripten;
mod features;
mod floats;
mod harness;
mod native_code;
mod p2commands;
mod pooling;
mod reactors;
mod teardown;
mod traps;
mod vectors;
use sha2::{Digest, Sha256};
use std::{
    fs,
    io::{self, BufRead, Write},
    time::Instant,
};
use wasmtime::{Config, Engine, Extern, Func, Instance, Module, Store, Strategy, Val, ValType};
use wasmtime::{Result, bail, format_err as anyhow};
#[cfg(all(feature = "native-allocator", feature = "native-code-lifetime"))]
compile_error!("allocator and code lifetime require separate diagnostic binaries");

const SCENARIOS: &[&str] = &[
    "harness-calibration",
    "density",
    "density-cycle",
    "engine-init",
    "compile",
    "compile-materialized",
    "instantiate",
    "app-init",
    "first-call",
    "steady",
    "trajectory",
    "aot-produce",
    "aot-load",
    "teardown",
];
struct Adapter {
    prep: Option<Value>,
    bytes: Vec<u8>,
    winch: bool,
}
struct State<T: 'static = ()> {
    store: Store<T>,
    instance: Instance,
    function: Func,
    args: Vec<Val>,
    results: Vec<Val>,
}
impl Adapter {
    fn engine(&self) -> Result<Engine> {
        self.engine_with_allocator(pooling::enabled())
    }
    fn engine_with_allocator(&self, pooled: bool) -> Result<Engine> {
        let mut config = Config::new();
        config.strategy(if self.winch {
            Strategy::Winch
        } else {
            Strategy::Cranelift
        });
        config.parallel_compilation(false);
        config.wasm_component_model(true);
        features::configure(&mut config, self.winch);
        if pooled {
            pooling::configure(&mut config);
        }
        Engine::new(&config)
    }
    fn workload(&self) -> Result<&Value> {
        Ok(&self
            .prep
            .as_ref()
            .ok_or_else(|| anyhow!("prepare required"))?["workload"])
    }
    fn prepare_state(&self, engine: &Engine, module: &Module) -> Result<State> {
        let mut store = Store::new(engine, ());
        let imports = self.imports(&mut store, module)?;
        let instance = Instance::new(&mut store, module, &imports)?;
        self.resolve_state(store, instance)
    }
    fn imports(&self, store: &mut Store<()>, module: &Module) -> Result<Vec<Extern>> {
        match self.workload()?["host_profile"].as_str().unwrap_or("") {
            "" => Ok(vec![]),
            "identity-v1" => {
                let identity = Func::wrap(&mut *store, |v: i32| v);
                module
                    .imports()
                    .map(|import| {
                        if import.module() != "wasmbench" || import.name() != "identity" {
                            bail!("unsupported identity-v1 import")
                        };
                        Ok(Extern::Func(identity))
                    })
                    .collect()
            }
            "assemblyscript-abort-v1" => {
                let abort = Func::wrap(
                    &mut *store,
                    |message: i32, file: i32, line: i32, column: i32| -> Result<()> {
                        bail!(
                            "AssemblyScript abort: message_ptr={} file_ptr={} line={} column={}",
                            message as u32,
                            file as u32,
                            line as u32,
                            column as u32
                        )
                    },
                );
                module
                    .imports()
                    .map(|import| {
                        if import.module() != "env" || import.name() != "abort" {
                            bail!("unsupported assemblyscript-abort-v1 import")
                        }
                        Ok(Extern::Func(abort))
                    })
                    .collect()
            }
            _ => bail!("unsupported host profile"),
        }
    }
    fn resolve_state(&self, store: Store<()>, instance: Instance) -> Result<State> {
        self.resolve_state_with_init(store, instance, true)
    }
    fn resolve_state_with_init<T: 'static>(
        &self,
        mut store: Store<T>,
        instance: Instance,
        initialize: bool,
    ) -> Result<State<T>> {
        let w = self.workload()?;
        if let Some(name) = w["initialize"]
            .as_str()
            .filter(|n| initialize && !n.is_empty())
        {
            instance
                .get_typed_func::<(), ()>(&mut store, name)?
                .call(&mut store, ())?;
        }
        if let Some(input) = w.get("input").filter(|v| !v.is_null()) {
            let data = hex::decode(
                input["hex"]
                    .as_str()
                    .ok_or_else(|| anyhow!("missing input hex"))?,
            )?;
            let mut base = 0u64;
            if let Some(name) = input["pointer_export"].as_str().filter(|s| !s.is_empty()) {
                base = instance
                    .get_typed_func::<(), i32>(&mut store, name)?
                    .call(&mut store, ())? as u32 as u64;
            }
            let offset = base
                .checked_add(
                    input["offset"]
                        .as_u64()
                        .ok_or_else(|| anyhow!("missing input offset"))?,
                )
                .filter(|n| *n <= u32::MAX as u64)
                .ok_or_else(|| anyhow!("input address overflow"))?;
            instance
                .get_memory(&mut store, "memory")
                .ok_or_else(|| anyhow!("missing input memory"))?
                .write(&mut store, usize::try_from(offset)?, &data)?;
        }
        let function = instance
            .get_func(
                &mut store,
                w["export"]
                    .as_str()
                    .ok_or_else(|| anyhow!("missing export"))?,
            )
            .ok_or_else(|| anyhow!("missing export"))?;
        let ty = function.ty(&store);
        let raw = w["args"]
            .as_array()
            .ok_or_else(|| anyhow!("missing arguments"))?;
        if raw.len() != ty.params().len() {
            bail!("argument arity mismatch")
        }
        let args: Vec<Val> = ty
            .params()
            .zip(raw)
            .map(|(t, v)| -> Result<Val> {
                let n = bits(v)?;
                Ok(match t {
                    ValType::I32 => Val::I32(n as i32),
                    ValType::I64 => Val::I64(n as i64),
                    ValType::F32 => Val::F32(n as u32),
                    ValType::F64 => Val::F64(n),
                    _ => bail!("unsupported argument type"),
                })
            })
            .collect::<Result<_>>()?;
        let results = ty
            .results()
            .map(|t| -> Result<Val> {
                Ok(match t {
                    ValType::I32 => Val::I32(0),
                    ValType::I64 => Val::I64(0),
                    ValType::F32 => Val::F32(0),
                    ValType::F64 => Val::F64(0),
                    _ => bail!("unsupported result type"),
                })
            })
            .collect::<Result<Vec<_>>>()?;
        Ok(State {
            store,
            instance,
            function,
            args,
            results,
        })
    }
    fn verify<T: 'static>(&self, state: &mut State<T>, results: &[Val]) -> Result<Vec<String>> {
        let w = self.workload()?;
        if w["oracle"]["kind"] != "exact_u64" && w["oracle"]["kind"] != "float_bits_v1" {
            bail!("unsupported oracle")
        }
        let got: Vec<u64> = results
            .iter()
            .map(|v| -> Result<u64> {
                Ok(match v {
                    Val::I32(n) => *n as u32 as u64,
                    Val::I64(n) => *n as u64,
                    Val::F32(n) => *n as u64,
                    Val::F64(n) => *n,
                    _ => bail!("unsupported result"),
                })
            })
            .collect::<Result<_>>()?;
        let want = w["oracle"]["expected"]
            .as_array()
            .ok_or_else(|| anyhow!("missing oracle"))?
            .iter()
            .map(bits)
            .collect::<Result<Vec<_>>>()?;
        if w["oracle"]["kind"] == "float_bits_v1" {
            floats::verify(&w["oracle"], results)?;
        } else if got != want {
            bail!("incorrect result: got {got:?}, want {want:?}")
        }
        let mut output_base = 0usize;
        if let Some(name) = w["oracle"]["output_pointer_export"]
            .as_str()
            .filter(|s| !s.is_empty())
        {
            let pointer = state
                .instance
                .get_typed_func::<(), i32>(&mut state.store, name)
                .map_err(|e| anyhow!("incorrect result: output pointer: {e}"))?;
            output_base = pointer.call(&mut state.store, ())? as u32 as usize;
        }
        if let Some(checks) = w["oracle"]["memory"].as_array() {
            for check in checks {
                let memory = state
                    .instance
                    .get_memory(&mut state.store, "memory")
                    .ok_or_else(|| anyhow!("missing memory"))?;
                let want = hex::decode(
                    check["hex"]
                        .as_str()
                        .ok_or_else(|| anyhow!("missing hex"))?,
                )?;
                let offset = check["offset"]
                    .as_u64()
                    .ok_or_else(|| anyhow!("missing offset"))?
                    as usize;
                let offset = output_base
                    .checked_add(offset)
                    .filter(|n| *n <= u32::MAX as usize)
                    .ok_or_else(|| anyhow!("incorrect result: output pointer overflow"))?;
                if memory.data(&state.store).get(
                    offset
                        ..offset
                            .checked_add(want.len())
                            .ok_or_else(|| anyhow!("offset overflow"))?,
                ) != Some(want.as_slice())
                {
                    bail!("incorrect result: memory oracle mismatch")
                }
            }
        }
        Ok(got.iter().map(u64::to_string).collect())
    }
    fn check(&self, engine: &Engine, module: &Module) -> Result<()> {
        let mut s = self.prepare_state(engine, module)?;
        s.function.call(&mut s.store, &s.args, &mut s.results)?;
        let results = s.results.clone();
        self.verify(&mut s, &results)?;
        Ok(())
    }
    fn compile_phases(
        &self,
        samples: u64,
        barrier: &mut dyn FnMut(u64, &str) -> Result<()>,
    ) -> Result<Value> {
        let profile = self
            .prep
            .as_ref()
            .ok_or_else(|| anyhow!("prepare required"))?["profile"]
            .as_str()
            .unwrap_or("");
        if !["memory", "counters"].contains(&profile) {
            bail!("phase barriers require memory or counters profile")
        }
        let engine = self.engine()?;
        let mut output = Vec::with_capacity(samples as usize);
        for index in 0..samples {
            barrier(index, "before_compile")?;
            let start = Instant::now();
            let module = Module::new(&engine, &self.bytes)?;
            let elapsed = start.elapsed().as_nanos() as u64;
            barrier(index, "compiled")?;
            self.check(&engine, &module)?;
            drop(module);
            barrier(index, "released")?;
            output.push(json!({"index":index,"warmup":false,"elapsed_ns":elapsed,"operations":1,"sample_type":"individual_operation","verified":true}));
        }
        Ok(json!({"samples":output}))
    }
    fn run(&self, r: &Value, barrier: &mut dyn FnMut(u64, &str) -> Result<()>) -> Result<Value> {
        #[cfg(all(feature = "native-code-lifetime", target_os = "linux"))]
        {
            if self.prep.as_ref().is_none_or(|p| p["profile"] != "code") {
                bail!("native code lifetime binary requires a dedicated code pass");
            }
            if r["scenario"] == "code-lifetime" {
                return code_lifetime::run(self, r);
            }
            if r["scenario"] != "first-call" {
                bail!("only code lifetime and sacrificial first-call verification supported");
            }
        }
        #[cfg(feature = "native-allocator")]
        {
            return allocator::run(self, r, barrier);
        }
        #[allow(unreachable_code)]
        if r["scenario"] == "harness-calibration" {
            let prep = self
                .prep
                .as_ref()
                .ok_or_else(|| anyhow!("prepare required"))?;
            let (samples, operations) = harness::validate(prep, r)?;
            let engine = self.engine()?;
            let module = Module::new(&engine, &self.bytes)?;
            self.check(&engine, &module)?;
            return harness::run(samples, operations);
        }
        #[allow(unreachable_code)]
        if r["scenario"] == "compile-materialized" {
            let prep = self
                .prep
                .as_ref()
                .ok_or_else(|| anyhow!("prepare required"))?;
            native_code::validate_materialized(prep, r)?;
            let engine = self.engine()?;
            let start = Instant::now();
            let module = Module::new(&engine, &self.bytes)?;
            let elapsed = start.elapsed().as_nanos() as u64;
            let mut result = native_code::materialized(&module, &self.bytes, self.winch, elapsed)?;
            self.check(&engine, &module)?; // Correctness outside compile timer.
            result["samples"] = json!([{"index":0,"warmup":false,"elapsed_ns":elapsed,"operations":1,"sample_type":"individual_operation","verified":true}]);
            return Ok(result);
        }
        if self.workload()?.get("abi").and_then(Value::as_str) == Some("component")
            && self.workload()?["oracle"]["kind"] == "exact_command"
        {
            return p2commands::run(self, r, barrier);
        }
        if self.workload()?.get("abi").and_then(Value::as_str) == Some("component") {
            let prep = self
                .prep
                .as_ref()
                .ok_or_else(|| anyhow!("prepare required"))?;
            if self.workload()?["host_profile"] == component_calls::POLICY {
                return component_calls::run(&self.engine()?, &self.bytes, prep, r, barrier);
            }
            return components::run(&self.engine()?, &self.bytes, prep, r);
        }
        if self.workload()?["abi"] == "wasi-reactor" {
            return self.run_reactor(r);
        }
        let prep = self
            .prep
            .as_ref()
            .ok_or_else(|| anyhow!("prepare required"))?;
        if prep["profile"] == "counters" {
            counters::validate(prep, r)?;
            if r["scenario"] == "first-call" {
                return self.first_call_counters(r, barrier);
            }
            if r["scenario"] == "steady" {
                return self.steady_counters(r, barrier);
            }
        }
        if r["scenario"] == "density"
            || r["scenario"] == "density-cycle"
            || !self.workload()?["density"].is_null()
        {
            return self.run_density(r, barrier);
        }
        if r["scenario"] == "trajectory" {
            if r["warmup"].as_u64().is_none() {
                bail!("invalid trajectory warmup");
            }
            let w = self.workload()?;
            if self.prep.as_ref().unwrap()["profile"] != "timing"
                || r["phase_barriers"] == true
                || w["abi"] != "core"
                || w["reset"] != "stateless"
                || (w["oracle"]["kind"] != "exact_u64" && w["oracle"]["kind"] != "float_bits_v1")
                || !w["command"].is_null()
                || !w["vectors"].is_null()
                || w["export"].as_str().unwrap_or("").is_empty()
            {
                bail!("unsupported trajectory contract");
            }
        }
        if self.workload()?["oracle"]["kind"] == "float_bits_v1" {
            let w = self.workload()?;
            let profile = self.prep.as_ref().unwrap()["profile"]
                .as_str()
                .unwrap_or("");
            if w["abi"] != "core"
                || !w["command"].is_null()
                || !w["vectors"].is_null()
                || !["timing", "memory"].contains(&profile)
                || (r["phase_barriers"] == true
                    && (profile != "memory"
                        || !["compile", "instantiate", "teardown"]
                            .contains(&r["scenario"].as_str().unwrap_or(""))))
                || ![
                    "compile",
                    "instantiate",
                    "first-call",
                    "steady",
                    "trajectory",
                    "teardown",
                ]
                .contains(&r["scenario"].as_str().unwrap_or(""))
            {
                bail!("unsupported float oracle scenario/profile");
            }
        }
        if self.workload()?["oracle"]["kind"] == "expected_trap" {
            return self.run_traps(r);
        }
        if r["scenario"] == "instantiate"
            && r["phase_barriers"].as_bool().unwrap_or(false)
            && self.workload()?["command"].is_null()
            && self.workload()?["vectors"].is_null()
        {
            return self.instantiate_phases(r, barrier);
        }
        if r["scenario"] == "teardown"
            && self.workload()?["vectors"].is_null()
            && self.workload()?["command"].is_null()
        {
            return self.run_teardown(r, barrier);
        }
        if r["scenario"] == "app-init" {
            return self.run_app_init(r, barrier);
        }
        if !self.workload()?["command"].is_null() {
            return self.run_command(r, barrier);
        }
        if !self.workload()?["vectors"].is_null() {
            return self.run_vectors(r, barrier);
        }
        let scenario = r["scenario"]
            .as_str()
            .ok_or_else(|| anyhow!("scenario required"))?;
        if !SCENARIOS.contains(&scenario) {
            return Ok(json!({"status":"unsupported","reason":"scenario not advertised"}));
        }
        let samples = r["samples"].as_u64().unwrap_or(0);
        let operations = r["operations"].as_u64().unwrap_or(0);
        let warmup = if scenario == "steady" || scenario == "trajectory" {
            r["warmup"].as_u64().unwrap_or(0)
        } else {
            0
        };
        if samples == 0
            || samples > 100000
            || operations == 0
            || operations > 1000000
            || warmup > 100000
        {
            bail!("invalid batch")
        }
        if r["phase_barriers"].as_bool().unwrap_or(false) {
            if scenario != "compile" {
                bail!("unsupported phase barrier scenario")
            }
            return self.compile_phases(samples, barrier);
        }
        let engine = self.engine()?;
        let module = Module::new(&engine, &self.bytes)?;
        let mut state = self.prepare_state(&engine, &module)?;
        let aot = if scenario == "aot-load" {
            module.serialize()?
        } else {
            vec![]
        };
        let reset = self.workload()?["reset"] == "fresh_instance_per_sample";
        let memory = self.prep.as_ref().unwrap()["profile"] == "memory";
        let mut output = Vec::with_capacity((samples + warmup) as usize);
        for index in 0..samples + warmup {
            let ops = if scenario == "first-call" || scenario == "trajectory" || reset {
                1
            } else {
                operations
            };
            if scenario == "first-call" || (scenario == "steady" && reset) {
                state = self.prepare_state(&engine, &module)?;
            }
            let mut elapsed = 0u64;
            let mut result = vec![];
            if scenario == "steady" || scenario == "trajectory" || scenario == "first-call" {
                let mut results = vec![state.results.clone(); ops as usize];
                let start = Instant::now();
                for values in &mut results {
                    state.function.call(&mut state.store, &state.args, values)?;
                }
                elapsed = start.elapsed().as_nanos() as u64;
                for values in results {
                    result = self.verify(&mut state, &values)?;
                }
            } else {
                for _ in 0..ops {
                    match scenario {
                        "engine-init" => {
                            let start = Instant::now();
                            let e = self.engine()?;
                            elapsed += start.elapsed().as_nanos() as u64;
                            // Establish usability on this measured engine. These
                            // operations and their release are outside init timing.
                            let m = Module::new(&e, &self.bytes)?;
                            self.check(&e, &m)?;
                            drop(m);
                            drop(e);
                        }
                        "compile" => {
                            let start = Instant::now();
                            let m = Module::new(&engine, &self.bytes)?;
                            elapsed += start.elapsed().as_nanos() as u64;
                            self.check(&engine, &m)?;
                        }
                        "instantiate" => {
                            let mut store = Store::new(&engine, ());
                            let imports = self.imports(&mut store, &module)?;
                            let start = Instant::now();
                            let instance = Instance::new(&mut store, &module, &imports)?;
                            elapsed += start.elapsed().as_nanos() as u64;
                            let mut s = self.resolve_state(store, instance)?;
                            s.function.call(&mut s.store, &s.args, &mut s.results)?;
                            let values = s.results.clone();
                            self.verify(&mut s, &values)?;
                        }
                        "aot-produce" => {
                            let start = Instant::now();
                            let m = Module::new(&engine, &self.bytes)?;
                            let artifact = m.serialize()?;
                            elapsed += start.elapsed().as_nanos() as u64;
                            // Only bytes just serialized by this engine are
                            // trusted here. Validate the produced artifact's
                            // behavior, not an unrelated precompiled module.
                            let restored = unsafe { Module::deserialize(&engine, &artifact) }?;
                            self.check(&engine, &restored)?;
                        }
                        "aot-load" => {
                            let start = Instant::now();
                            let m = unsafe { Module::deserialize(&engine, &aot) }?;
                            elapsed += start.elapsed().as_nanos() as u64;
                            self.check(&engine, &m)?;
                        }
                        _ => bail!("unsupported scenario"),
                    }
                }
            }
            let mut sample = json!({"index":index,"warmup":index<warmup,"elapsed_ns":elapsed,"operations":ops,"sample_type":if ops==1 {"individual_operation"} else {"batch_average"},"verified":true,"result":result});
            // Engine-init releases every verified engine and its instance;
            // state below belongs to untimed preparation, not that operation.
            if memory && scenario != "engine-init" {
                if let Some(mem) = state.instance.get_memory(&mut state.store, "memory") {
                    sample["observations"] = json!([observation(
                        "guest.memory.logical",
                        mem.data_size(&state.store),
                        "guest_linear_memory",
                        scenario,
                        "memory"
                    )]);
                }
            }
            output.push(sample);
        }
        Ok(json!({"samples":output}))
    }
    fn handle(
        &mut self,
        request: &Value,
        barrier: &mut dyn FnMut(u64, &str) -> Result<()>,
    ) -> Result<Value> {
        if request["version"] != 1 {
            bail!("unsupported protocol version")
        }
        match request["method"].as_str().unwrap_or("") {
            "describe" => Ok(
                json!({"description":{"runtime":"wasmtime","runtime_version":"46.0.1","backend":if self.winch {"winch"} else {"cranelift"},"embedding":"Rust API","build":"Cargo.lock / release","effective_configuration":{"cache":"disabled","parallel_compilation":"false","component_model":"enabled","materialization":"synchronous-api-and-complete-native-ranges-v1","strategy":if self.winch {"winch"} else {"cranelift"}},"capabilities":{"can_compile_separately":true,"can_instantiate_separately":true,"can_compile_materialized":true,"can_compile_components":true,"can_execute_components":false,"can_disable_code_cache":true,"can_observe_tiers":false,"can_measure_host_allocations":false,"can_export_native_code":true,"can_snapshot":false},"scenarios":SCENARIOS,"abis":["core","component","wasi-command","emscripten"],"features":["mvp","bulk-memory","simd","reference-types","multi-value"]}}),
            ),
            "prepare" => {
                let prep = request["prepare"].clone();
                let w = &prep["workload"];
                if w["abi"] == "component" && w["host_profile"] == component_calls::POLICY {
                    component_calls::validate(w)?;
                } else if w["abi"] == "component" && w["oracle"]["kind"] == "component_compile_only"
                {
                    components::validate_workload(w)?;
                } else if w["abi"] == "component" && w["oracle"]["kind"] == "exact_command" {
                    commands::validate(w)?;
                } else if w["abi"] == "wasi-command" {
                    commands::validate(w)?;
                } else if w["abi"] == "emscripten" {
                    commands::validate(w)?;
                } else if w["abi"] == "wasi-reactor" {
                    reactors::validate(w)?;
                } else if w["abi"] != "core" || !w["command"].is_null() {
                    bail!("unsupported ABI")
                };
                if w["reset"] != "stateless" && w["reset"] != "fresh_instance_per_sample" {
                    bail!("unsupported reset")
                };
                self.bytes = fs::read(
                    prep["artifact"]
                        .as_str()
                        .ok_or_else(|| anyhow!("artifact required"))?,
                )?;
                if hex::encode(Sha256::digest(&self.bytes))
                    != prep["artifact_sha256"].as_str().unwrap_or("")
                {
                    bail!("artifact digest mismatch")
                };
                self.prep = Some(prep);
                Ok(json!({}))
            }
            "run" => self.run(&request["run"], barrier),
            "inspect" => {
                let engine = self.engine()?;
                if self.workload()?.get("abi").and_then(Value::as_str) == Some("component") {
                    wasmtime::component::Component::new(&engine, &self.bytes)?;
                    return Ok(
                        json!({"diagnostics":[observation("artifact.component_bytes",self.bytes.len(),"component","compile","structure"),{"metric":"component.compiled","definition_version":1,"value":true,"unit":"boolean","scope":"component","phase":"compile","collector":"wasmtime","collector_version":"46.0.1","quality":"engine_reported","profile":"structure","status":"available","normalization_denominator":"component"},{"metric":"native.guest_code","definition_version":1,"unit":"bytes","scope":"guest_function_code","phase":"compile","collector":"wasmtime","collector_version":"46.0.1","quality":"engine_reported","profile":"code","status":"unsupported","reason":"function code extraction is not configured","normalization_denominator":"component"}]}),
                    );
                }
                let module = Module::new(&engine, &self.bytes)?;
                native_code::inspect(&module, &self.bytes, self.winch)
            }
            "close" => {
                self.prep = None;
                self.bytes.clear();
                Ok(json!({}))
            }
            _ => bail!("unknown method"),
        }
    }
}
fn bits(v: &Value) -> Result<u64> {
    if let Some(s) = v.as_str() {
        Ok(s.parse()?)
    } else {
        v.as_u64().ok_or_else(|| anyhow!("invalid integer bits"))
    }
}
fn observation(metric: &str, value: usize, scope: &str, phase: &str, profile: &str) -> Value {
    json!({"metric":metric,"definition_version":1,"value":value,"unit":"bytes","scope":scope,"phase":phase,"collector":"wasmtime Rust API","collector_version":"46.0.1","quality":"engine_reported","profile":profile,"status":"available","normalization_denominator":"module_or_instance"})
}
fn main() -> Result<()> {
    let mut adapter = Adapter {
        prep: None,
        bytes: vec![],
        winch: std::env::args().any(|s| s == "--winch"),
    };
    let input = io::stdin();
    let mut output = io::stdout().lock();
    let mut lines = input.lock().lines();
    while let Some(line) = lines.next() {
        let line = line?;
        let request = serde_json::from_str::<Value>(&line);
        let mut response = json!({"version":1,"id":0,"status":"ok"});
        let mut close = false;
        let result = match request {
            Ok(req) => {
                response["id"] = req["id"].clone();
                close = req["method"] == "close";
                let mut barrier = |index: u64, stage: &str| -> Result<()> {
                    serde_json::to_writer(
                        &mut output,
                        &json!({"version":1,"id":req["id"],"status":"phase","phase":{"sample_index":index,"stage":stage}}),
                    )?;
                    writeln!(&mut output)?;
                    output.flush()?;
                    let line = lines
                        .next()
                        .ok_or_else(|| anyhow!("phase acknowledgement missing"))??;
                    let ack: Value = serde_json::from_str(&line)?;
                    if ack["version"] != 1 || ack["id"] != req["id"] || ack["method"] != "continue"
                    {
                        bail!("invalid phase acknowledgement")
                    }
                    Ok(())
                };
                adapter.handle(&req, &mut barrier)
            }
            Err(e) => Err(e.into()),
        };
        match result {
            Ok(value) => {
                for (k, v) in value.as_object().unwrap() {
                    response[k] = v.clone();
                }
            }
            Err(e) => {
                response["status"] = json!("error");
                response["reason"] = json!(format!("{e:#}"));
            }
        }
        if response["description"].is_object() {
            response["description"]["capabilities"]["can_verify_invocation_traps"] = json!(true);
            response["description"]["capabilities"]["can_verify_float_bits_v1"] = json!(true);
            response["description"]["capabilities"]["can_float_phases"] = json!(true);
            response["description"]["capabilities"]["can_float_teardown"] = json!(true);
            response["description"]["capabilities"]["can_float_trajectory"] = json!(true);
            response["description"]["capabilities"]["can_measure_invocation_traps"] = json!(true);
            response["description"]["validator_features"] = features::describe(adapter.winch);
            response["description"]["effective_configuration"]["instance_allocation"] =
                json!(if pooling::enabled() {
                    pooling::POLICY
                } else {
                    "on-demand; Wasmtime 46.0.1 defaults"
                });
            response["description"]["effective_configuration"]["validator_feature_policy"] =
                json!("wasmtime-default-subset-v1");
            response["description"]["effective_configuration"]["aot_policy"] = json!(
                "produce: resident Wasm bytes through compile and serialize; deserialize locally produced immutable bytes and verify behavior outside timer; load: deserialize resident bytes serialized by the same configured engine before sampling, verify behavior outside timer; no arbitrary native artifact ingestion; release outside timers"
            );
            response["description"]["effective_configuration"]["engine_init_policy"] = json!(
                "time configured Engine construction only; compile resident workload bytes, instantiate and verify behavior using each measured engine after timer stop; release outside timer; no forced reclamation"
            );
            response["description"]["effective_configuration"]["harness_calibration_policy"] =
                json!(harness::POLICY);
            response["description"]["abis"] = json!([
                "core",
                "wasi-command",
                "wasi-reactor",
                "component",
                "emscripten"
            ]);
            response["description"]["capabilities"]["can_compile_components"] = json!(true);
            response["description"]["capabilities"]["can_execute_components"] = json!(false);
            response["description"]["capabilities"]["can_component_u64_calls_v1"] = json!(true);
            response["description"]["capabilities"]["can_component_u64_memory_v1"] = json!(true);
            response["description"]["effective_configuration"]["component_u64_call_policy"] =
                json!(component_calls::BOUNDARIES);
            response["description"]["capabilities"]["can_run_component_commands"] = json!(true);
            response["description"]["capabilities"]["can_component_command_lifecycle"] =
                json!(true);
            response["description"]["capabilities"]["can_component_command_phases"] = json!(true);
            response["description"]["effective_configuration"]["component_command_lifecycle_policy"] = json!(
                "compile: resident component bytes through Component::new including validation, engine/linker retained; instantiate: prepared Store/WASI/imports through typed Command::instantiate including nested core start functions but excluding wasi:cli/run; first-call: wasi:cli/run only; exact command verification outside compile/instantiate timers; fresh fixture root, WASI context and instance each sample; memory barriers before/returned/released; release drops Store/instance and staging root, compiled component retained except compile; capture buffers retained through sample evidence construction; no forced reclamation"
            );
            response["description"]["capabilities"]["can_run_reactors"] = json!(true);
            response["description"]["effective_configuration"]["wasi_reactor_host"] =
                json!(reactors::POLICY);
            response["description"]["capabilities"]["can_run_commands"] = json!(true);
            response["description"]["capabilities"]["can_command_compile_phases"] = json!(true);
            response["description"]["capabilities"]["can_command_instantiate_phases"] = json!(true);
            response["description"]["capabilities"]["can_command_first-call_phases"] = json!(true);
            response["description"]["effective_configuration"]["command_lifecycle_phases_policy"] = json!(
                "compile/instantiate/first-call memory API windows; prepared Store/WASI/imports and capture buffers excluded; instantiate includes Wasm start but excludes Emscripten constructors/argv; first-call includes bounded capture and host calls; output/exit verification before release; fresh Store/instance each sample; module/engine retained except compile module; no forced reclamation"
            );
            response["description"]["capabilities"]["can_command_teardown_phases"] = json!(true);
            response["description"]["capabilities"]["can_vector_teardown_phases"] = json!(true);
            response["description"]["effective_configuration"]["command_teardown_policy"] = json!(
                "after verified command termination drop store (WASI context, preopens, stdin), module, linker, and engine; fixture cleanup and capture-buffer reclamation excluded; no forced allocator reclamation"
            );
            response["description"]["effective_configuration"]["teardown_policy"] = json!(
                "verify each fresh instance before timing store, module, and engine drop; no forced allocator reclamation"
            );
            response["description"]["effective_configuration"]["wasi_host"] = json!(
                "wasmtime-wasi 46.0.1; readonly fixture root; empty env; zero random; synthetic clocks advancing 1ms per read; fresh owned WASI context; bounded capture included in call"
            );
            response["description"]["effective_configuration"]["emscripten_host"] = json!(
                "emscripten-stdio-v1; pinned argv/stdin, bounded output, deterministic clocks/randomness; filesystem syscalls fail closed; constructors then main(argc,argv); fresh instance per sample"
            );
            response["description"]["capabilities"]["can_run_vectors"] = json!(true);
            response["description"]["capabilities"]["can_density"] = json!(true);
            response["description"]["capabilities"]["can_counter_compile"] = json!(true);
            response["description"]["capabilities"]["can_counter_instantiate"] = json!(true);
            response["description"]["capabilities"]["can_counter_first-call"] = json!(true);
            response["description"]["capabilities"]["can_counter_steady"] = json!(true);
            response["description"]["effective_configuration"]["counter_steady_policy"] = json!(
                "stateless exact scalar; fresh engine/module/store/initialized instance per request retained across explicit warmup and measured batches; separate result buffers allocated before collection; every result verified afterward; no hidden pre-call, snapshots or allocator purge; raw counts per batch include embedding and barrier transport"
            );
            response["description"]["effective_configuration"]["counter_first_call_policy"] = json!(
                "fresh initialized store/instance per sample; engine/module retained; exactly one requested call; setup/imports/input/argument and result buffers prepared before collection; verification/result copy/store drop after; no memory snapshots or allocator purge"
            );
            response["description"]["effective_configuration"]["counter_policy"] = json!(
                "core exact scalar compile/instantiate phase handshakes; one operation, no warmup; no memory snapshots; perf collected externally over cgroup barrier window including transport/background work; verification and release excluded"
            );
            response["description"]["capabilities"]["can_density_cycle"] = json!(true);
            response["description"]["effective_configuration"]["density_cycle_policy"] = json!(
                "fresh engine/module resources retained across samples; no untimed instance prewarm; each sample creates and verifies fresh instances, then drops Stores; first cycle retained; engine/module construction excluded from timer; final batch resource drop follows final barrier; no forced allocator purge"
            );
            response["description"]["effective_configuration"]["density_policy"] = json!(
                "fresh simultaneous stores/instances; shared_module one engine/module; separate_engines fresh engine/compile per instance; timer includes engine construction, compile, store/instantiate/start, initialization/input and workload call; verification/release excluded; drop stores then modules then engines; no forced allocator reclamation"
            );
            response["description"]["capabilities"]["can_vector_compile_phases"] = json!(true);
            response["description"]["capabilities"]["can_vector_instantiate_phases"] = json!(true);
            response["description"]["capabilities"]["can_vector_first-call_phases"] = json!(true);
            response["description"]["effective_configuration"]["vector_first_call_phases_policy"] = json!(
                "fresh initialized instance; pointers and first input prepared before before_first_call; first_call_returned follows last ordered call before its output oracle; memory window includes intercase input writes and oracle checks, unlike sequence_call_sum timers; final oracle and verified instance release follow returned barrier; compiled module retained; no forced reclamation"
            );
            response["description"]["effective_configuration"]["vector_instantiate_phases_policy"] = json!(
                "fresh store/instance; prepared imports/module/engine outside instantiation window; includes Wasm start; explicit initialization and ordered vector oracle verification outside API window; drop verified store with module/engine retained; no forced reclamation"
            );
            response["description"]["phase_barrier_scenarios"] = json!([
                "steady",
                "first-call",
                "compile",
                "teardown",
                "app-init",
                "instantiate",
                "density",
                "density-cycle"
            ]);
            response["description"]["effective_configuration"]["instantiate_release_policy"] = json!(
                "drop verified store/instance; compiled module and engine retained; store/import preparation excluded from instantiation; no forced allocator reclamation"
            );
            response["description"]["effective_configuration"]["app_init_release_policy"] = json!(
                "drop verified store/instance; compiled module and engine retained; no forced allocator reclamation"
            );
            response["description"]["phase_release_policy"] = json!(
                "drop measured Store and Module; engine retained; no forced allocator reclamation"
            );
        }
        #[cfg(feature = "native-allocator")]
        if response["description"].is_object() {
            allocator::decorate(&mut response);
        }
        #[cfg(all(feature = "native-code-lifetime", target_os = "linux"))]
        if response["description"].is_object() {
            let d = &mut response["description"];
            d["scenarios"] = json!(["first-call", "code-lifetime"]);
            d["abis"] = json!(["core"]);
            d["capabilities"] = json!({"can_code_lifetime":true,"can_export_native_code":true,"can_disable_code_cache":true,"can_snapshot":false});
            d["effective_configuration"] = json!({"cache":"disabled","parallel_compilation":"false","strategy":if adapter.winch {"winch"} else {"cranelift"},"code_lifetime":"wasmtime-executable-publication-v1","code_lifetime_profile":"code only; first-call sacrificial verification uses ordinary engine","code_lifetime_protection":"Linux W^X; use_bti matched to detected arm64 host; pinned cache coherence 46.0.1","code_lifetime_release":"drop_module_handles_then_store_then_engine"});
            d["phase_barrier_scenarios"] = json!([]);
            d.as_object_mut().unwrap().remove("phase_release_policy");
        }
        serde_json::to_writer(&mut output, &response)?;
        writeln!(&mut output)?;
        output.flush()?;
        if close {
            break;
        }
    }
    Ok(())
}
