use super::*;

impl Adapter {
    pub(super) fn run_teardown(
        &self,
        r: &Value,
        barrier: &mut dyn FnMut(u64, &str) -> Result<()>,
    ) -> Result<Value> {
        let w = self.workload()?;
        let profile = self.prep.as_ref().unwrap()["profile"]
            .as_str()
            .unwrap_or("");
        let samples = r["samples"].as_u64().unwrap_or(0);
        if w["abi"] != "core"
            || (w["oracle"]["kind"] != "exact_u64" && w["oracle"]["kind"] != "float_bits_v1")
            || !w["vectors"].is_null()
            || !w["command"].is_null()
            || !["timing", "memory"].contains(&profile)
            || (r["phase_barriers"].as_bool().unwrap_or(false) && profile != "memory")
            || samples == 0
            || samples > 100000
            || r["operations"].as_u64().unwrap_or(0) == 0
        {
            bail!("unsupported teardown contract/profile/batch")
        }
        let mut out = Vec::with_capacity(samples as usize);
        for i in 0..samples {
            let engine = self.engine()?;
            let module = Module::new(&engine, &self.bytes)?;
            let mut state = self.prepare_state(&engine, &module)?;
            state
                .function
                .call(&mut state.store, &state.args, &mut state.results)?;
            let values = state.results.clone();
            let result = self.verify(&mut state, &values)?;
            let mut observations = Vec::new();
            if profile == "memory" {
                if let Some(mem) = state.instance.get_memory(&mut state.store, "memory") {
                    observations.push(observation(
                        "guest.memory.logical",
                        mem.data_size(&state.store),
                        "guest_linear_memory",
                        "teardown/before_release",
                        "memory",
                    ));
                }
            }
            if r["phase_barriers"].as_bool().unwrap_or(false) {
                barrier(i, "before_teardown")?;
            }
            let start = Instant::now();
            drop(state);
            drop(module);
            drop(engine);
            let elapsed = start.elapsed().as_nanos() as u64;
            if r["phase_barriers"].as_bool().unwrap_or(false) {
                barrier(i, "torn_down")?;
            }
            out.push(json!({"index":i,"warmup":false,"elapsed_ns":elapsed,"operations":1,"sample_type":"individual_operation","verified":true,"result":result,"observations":observations}));
        }
        Ok(json!({"samples":out}))
    }
}
