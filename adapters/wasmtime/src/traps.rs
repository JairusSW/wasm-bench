use crate::*;
use wasmtime::Trap;

impl Adapter {
    pub(crate) fn run_traps(&self, r: &Value) -> Result<Value> {
        let w = self.workload()?;
        let expected = w["oracle"]["expected_trap"].as_str().unwrap_or("");
        let codes = [
            "unreachable",
            "memory_out_of_bounds",
            "integer_divide_by_zero",
            "integer_overflow",
        ];
        if !codes.contains(&expected)
            || w["abi"] != "core"
            || w["reset"] != "fresh_instance_per_sample"
            || !w["input"].is_null()
            || !w["vectors"].is_null()
            || !w["command"].is_null()
            || ["initialize", "host_profile"]
                .iter()
                .any(|key| w[*key].as_str().is_some_and(|s| !s.is_empty()))
            || w["args"].as_array().is_some_and(|a| !a.is_empty())
            || ["expected", "memory"]
                .iter()
                .any(|key| w["oracle"][*key].as_array().is_some_and(|a| !a.is_empty()))
            || w["oracle"]["output_pointer_export"]
                .as_str()
                .is_some_and(|s| !s.is_empty())
        {
            bail!("unsupported or ambiguous invocation-trap contract");
        }
        let samples = r["samples"].as_u64().unwrap_or(0);
        let operations = r["operations"].as_u64().unwrap_or(0);
        let warmup = r["warmup"].as_u64().unwrap_or(0);
        let profile = self.prep.as_ref().unwrap()["profile"]
            .as_str()
            .unwrap_or("");
        if !["timing", "memory"].contains(&profile)
            || r["phase_barriers"] == true
            || !["first-call", "steady"].contains(&r["scenario"].as_str().unwrap_or(""))
            || samples == 0
            || samples > 100000
            || operations == 0
            || operations > 1000000
            || warmup > 100000
        {
            bail!("unsupported trap scenario/profile/batch");
        }
        let warmup = if r["scenario"] == "steady" { warmup } else { 0 };
        let engine = self.engine()?;
        let module = Module::new(&engine, &self.bytes)?;
        let mut samples_out = Vec::with_capacity((samples + warmup) as usize);
        for index in 0..samples + warmup {
            let mut state = self.prepare_state(&engine, &module)?;
            let start = Instant::now();
            let error = state
                .function
                .call(&mut state.store, &state.args, &mut state.results)
                .err();
            let elapsed = start.elapsed().as_nanos() as u64;
            let mut observations = Vec::new();
            if profile == "memory" {
                if let Some(memory) = state.instance.get_memory(&mut state.store, "memory") {
                    let mut obs = observation(
                        "guest.memory.logical",
                        memory.data_size(&state.store),
                        "guest_linear_memory",
                        &format!("{}/after_trap", r["scenario"].as_str().unwrap()),
                        "memory",
                    );
                    obs["normalization_denominator"] = json!("instance");
                    observations.push(obs);
                }
            }
            let code = error
                .as_ref()
                .and_then(|e| e.downcast_ref::<Trap>())
                .and_then(|trap| match trap {
                    Trap::UnreachableCodeReached => Some("unreachable"),
                    Trap::MemoryOutOfBounds => Some("memory_out_of_bounds"),
                    Trap::IntegerDivisionByZero => Some("integer_divide_by_zero"),
                    Trap::IntegerOverflow => Some("integer_overflow"),
                    _ => None,
                });
            if code != Some(expected) {
                bail!("incorrect result: expected invocation trap {expected}, got {error:?}");
            }
            samples_out.push(json!({"index":index,"warmup":index<warmup,"elapsed_ns":elapsed,"operations":1,"sample_type":"individual_operation","verified":true,"observations":observations,"trap_result":{"code":code.unwrap(),"source":"wasmtime/46.0.1/Trap-v1","message":format!("{:#}",error.unwrap())}}));
        }
        Ok(json!({"samples":samples_out}))
    }
}
