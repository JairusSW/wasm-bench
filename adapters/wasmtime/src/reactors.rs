use super::*;
use commands::{Capture, Clock};
use std::sync::atomic::AtomicU64;
use wasmtime::Linker;
use wasmtime_wasi::{
    WasiCtxBuilder,
    p1::{self, WasiP1Ctx},
    p2::pipe::MemoryInputPipe,
};

#[cfg(test)]
mod tests {
    use super::*;
    fn adapter(winch: bool, fresh: bool, profile: &str) -> Adapter {
        Adapter {
            winch,
            bytes: include_bytes!("../../../corpus/testdata/wasi-reactor.wasm").to_vec(),
            prep: Some(json!({"profile":profile,"workload":{
                "abi":"wasi-reactor","host_profile":"wasi-preview1-reactor-noio-v1","initialize":"_initialize",
                "export":if fresh {"once"} else {"benchmark"},"reset":if fresh {"fresh_instance_per_sample"} else {"stateless"},
                "args":[35],"input":{"offset":0,"pointer_export":"input_ptr","hex":"07000000"},
                "oracle":{"kind":"exact_u64","expected":[42],"memory":[{"offset":64,"hex":"01000000"}]}
            }})),
        }
    }
    fn request(scenario: &str) -> Value {
        json!({"scenario":scenario,"samples":3,"operations":4,"warmup":2})
    }
    #[test]
    fn lifecycle_and_profiles() -> Result<()> {
        for winch in [false, true] {
            for fresh in [false, true] {
                for profile in ["timing", "memory"] {
                    for scenario in [
                        "compile",
                        "instantiate",
                        "app-init",
                        "first-call",
                        "steady",
                        "teardown",
                    ] {
                        let a = adapter(winch, fresh, profile);
                        let result = a.run_reactor(&request(scenario))?;
                        let samples = result["samples"].as_array().unwrap();
                        assert_eq!(samples.len(), if scenario == "steady" { 5 } else { 3 });
                        for sample in samples {
                            assert_eq!(sample["verified"], true);
                            assert_eq!(sample["result"], json!(["42"]));
                            assert_eq!(
                                sample["operations"],
                                if scenario == "steady" && !fresh { 4 } else { 1 }
                            );
                            if profile == "memory" && scenario != "teardown" {
                                assert_eq!(sample["observations"][0]["value"], 65536);
                            }
                        }
                    }
                }
            }
        }
        Ok(())
    }
    #[test]
    fn rejects_bad_results_state_and_io() {
        for winch in [false, true] {
            for export in ["once", "wrong_middle", "write_output"] {
                let mut a = adapter(winch, false, "timing");
                a.prep.as_mut().unwrap()["workload"]["export"] = json!(export);
                assert!(a.run_reactor(&request("steady")).is_err(), "{export}");
            }
            let mut a = adapter(winch, false, "timing");
            a.prep.as_mut().unwrap()["workload"]["oracle"]["expected"] = json!([99]);
            assert!(a.run_reactor(&request("first-call")).is_err());
            let mut a = adapter(winch, false, "timing");
            a.prep.as_mut().unwrap()["workload"]["oracle"]["output_pointer_export"] =
                json!("write_pointer");
            a.prep.as_mut().unwrap()["workload"]["oracle"]["memory"][0]["offset"] = json!(0);
            assert!(a.run_reactor(&request("first-call")).is_err());
            for replacement in ["missinginit", "param_init_", "result_init"] {
                let mut a = adapter(winch, false, "timing");
                let at = a
                    .bytes
                    .windows(11)
                    .position(|b| b == b"_initialize")
                    .unwrap();
                a.bytes[at..at + 11].copy_from_slice(b"unused_init");
                if replacement != "missinginit" {
                    let at = a
                        .bytes
                        .windows(11)
                        .position(|b| b == replacement.as_bytes())
                        .unwrap();
                    a.bytes[at..at + 11].copy_from_slice(b"_initialize");
                }
                assert!(a.run_reactor(&request("app-init")).is_err());
            }
        }
    }
}

pub(super) const POLICY: &str = "wasi-preview1-reactor-noio-v1; wasmtime-wasi 46.0.1; empty args/env/stdin, no preopens/network, zero random; wall clock starts 1640995200000000000ns, monotonic starts zero, both advance 1ms/read; reject nonempty stdout/stderr including ignored write errors; Wasm start inside instantiation, explicit _initialize once before input; fresh WASI context per instance; stateless steady retains instance across explicit warmup and samples without hidden pre-call; teardown drops store/module/linker/engine after verification, no forced allocator reclamation; logical memory post-verification, no native allocator instrumentation";

pub(super) fn validate(w: &Value) -> Result<()> {
    if w["abi"] != "wasi-reactor"
        || w["host_profile"] != "wasi-preview1-reactor-noio-v1"
        || w["initialize"] != "_initialize"
        || ["", "_start", "_initialize"].contains(&w["export"].as_str().unwrap_or(""))
        || ["command", "vectors", "density"]
            .iter()
            .any(|k| !w[k].is_null())
        || w["oracle"]["kind"] != "exact_u64"
        || !w["oracle"]["float"].is_null()
        || !w["oracle"]["expected_trap"]
            .as_str()
            .unwrap_or("")
            .is_empty()
        || !["stateless", "fresh_instance_per_sample"].contains(&w["reset"].as_str().unwrap_or(""))
    {
        bail!("unsupported WASI reactor contract")
    }
    Ok(())
}

fn context(engine: &Engine) -> (Store<WasiP1Ctx>, Capture) {
    let output = Capture::new(0);
    let mut builder = WasiCtxBuilder::new();
    builder
        .stdin(MemoryInputPipe::new(Vec::<u8>::new()))
        .stdout(output.clone())
        .stderr(output.clone())
        .secure_random(wasmtime_wasi::random::Deterministic::new(vec![0]))
        .wall_clock(Clock(AtomicU64::new(1_640_995_200_000_000_000)))
        .monotonic_clock(Clock(AtomicU64::new(0)));
    (Store::new(engine, builder.build_p1()), output)
}

impl Adapter {
    fn reactor_state(
        &self,
        mut store: Store<WasiP1Ctx>,
        instance: Instance,
        output: &Capture,
        timed_init: bool,
    ) -> Result<(State<WasiP1Ctx>, u64)> {
        if instance.get_export(&mut store, "_start").is_some() {
            bail!("reactor must not export _start")
        }
        let init = instance.get_typed_func::<(), ()>(&mut store, "_initialize")?;
        let start = Instant::now();
        init.call(&mut store, ())?;
        let elapsed = if timed_init {
            start.elapsed().as_nanos() as u64
        } else {
            0
        };
        output.evidence()?;
        let state = self.resolve_state_with_init(store, instance, false)?;
        output.evidence()?;
        Ok((state, elapsed))
    }

    fn reactor_verify(
        &self,
        state: &mut State<WasiP1Ctx>,
        output: &Capture,
        values: &[Val],
    ) -> Result<Vec<String>> {
        output.evidence()?;
        let result = self.verify(state, values)?;
        output.evidence()?;
        Ok(result)
    }

    pub(super) fn run_reactor(&self, r: &Value) -> Result<Value> {
        validate(self.workload()?)?;
        let scenario = r["scenario"].as_str().unwrap_or("");
        let profile = self.prep.as_ref().unwrap()["profile"]
            .as_str()
            .unwrap_or("");
        if !["timing", "memory"].contains(&profile)
            || r["phase_barriers"] == true
            || ![
                "compile",
                "instantiate",
                "app-init",
                "first-call",
                "steady",
                "teardown",
            ]
            .contains(&scenario)
        {
            return Ok(
                json!({"status":"unsupported", "reason":"reactor scenario/profile not qualified; cold-process is controller-owned"}),
            );
        }
        let samples = r["samples"].as_u64().unwrap_or(0);
        let operations = r["operations"].as_u64().unwrap_or(0);
        let requested_warmup = r["warmup"].as_u64().unwrap_or(0);
        if !(1..=100000).contains(&samples)
            || !(1..=1000000).contains(&operations)
            || requested_warmup > 100000
        {
            bail!("invalid reactor batch")
        }
        let warmup = if scenario == "steady" {
            requested_warmup
        } else {
            0
        };
        let retained = scenario == "steady" && self.workload()?["reset"] == "stateless";
        let runtime = || -> Result<(Engine, Linker<WasiP1Ctx>)> {
            let engine = self.engine()?;
            let mut linker = Linker::new(&engine);
            p1::add_to_linker_sync(&mut linker, |ctx| ctx)?;
            Ok((engine, linker))
        };
        // Teardown owns a fresh complete runtime per sample. No shared engine clone survives its timer.
        let shared = if scenario == "teardown" {
            None
        } else {
            Some(runtime()?)
        };
        let module = if !["compile", "teardown"].contains(&scenario) {
            Some(Module::new(&shared.as_ref().unwrap().0, &self.bytes)?)
        } else {
            None
        };
        let mut steady = None;
        let mut samples_out = Vec::new();
        for index in 0..samples + warmup {
            let owned = if scenario == "teardown" {
                Some(runtime()?)
            } else {
                None
            };
            let (engine, linker) = owned.as_ref().or(shared.as_ref()).unwrap();
            let mut elapsed = 0;
            let fresh_module = if module.is_none() {
                let start = Instant::now();
                let m = Module::new(engine, &self.bytes)?;
                if scenario == "compile" {
                    elapsed = start.elapsed().as_nanos() as u64;
                }
                Some(m)
            } else {
                None
            };
            if steady.is_none() {
                let (mut store, output) = context(engine);
                let start = Instant::now();
                let instance = linker.instantiate(
                    &mut store,
                    module.as_ref().or(fresh_module.as_ref()).unwrap(),
                )?;
                if scenario == "instantiate" {
                    elapsed = start.elapsed().as_nanos() as u64;
                }
                let (state, init_ns) =
                    self.reactor_state(store, instance, &output, scenario == "app-init")?;
                if scenario == "app-init" {
                    elapsed = init_ns;
                }
                steady = Some((state, output));
            }
            let (state, output) = steady.as_mut().unwrap();
            let ops = if retained { operations } else { 1 };
            let mut values = vec![state.results.clone(); ops as usize];
            let start = Instant::now();
            for v in &mut values {
                state.function.call(&mut state.store, &state.args, v)?;
            }
            if ["first-call", "steady"].contains(&scenario) {
                elapsed = start.elapsed().as_nanos() as u64;
            }
            let mut result = vec![];
            for v in values {
                result = self.reactor_verify(state, output, &v)?;
            }
            let mut observations = vec![];
            if profile == "memory" && scenario != "teardown" {
                if let Some(memory) = state.instance.get_memory(&mut state.store, "memory") {
                    observations.push(observation(
                        "guest.memory.logical",
                        memory.data_size(&state.store),
                        "guest_linear_memory",
                        "post_verification",
                        "memory",
                    ));
                }
            }
            if scenario == "teardown" {
                let released = steady.take();
                let start = Instant::now();
                drop(released);
                drop(fresh_module);
                drop(owned);
                elapsed = start.elapsed().as_nanos() as u64;
            } else if !retained {
                steady = None;
            }
            samples_out.push(json!({"index":index,"warmup":index<warmup,"elapsed_ns":elapsed,"operations":ops,"sample_type":if ops==1 {"individual_operation"} else {"batch_average"},"verified":true,"result":result,"observations":observations}));
        }
        Ok(json!({"samples":samples_out}))
    }
}
