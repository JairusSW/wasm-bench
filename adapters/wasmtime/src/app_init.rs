use super::*;

impl Adapter {
    pub(super) fn instantiate_phases(
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
            || !["stateless", "fresh_instance_per_sample"]
                .contains(&w["reset"].as_str().unwrap_or(""))
            || !["memory", "counters"].contains(&profile)
            || samples == 0
            || samples > 100000
            || r["operations"].as_u64().unwrap_or(0) == 0
        {
            bail!("unsupported instantiation barriers")
        }
        let engine = self.engine()?;
        let module = Module::new(&engine, &self.bytes)?;
        let mut out = Vec::with_capacity(samples as usize);
        for i in 0..samples {
            let mut store = Store::new(&engine, ());
            let imports = self.imports(&mut store, &module)?;
            barrier(i, "before_instantiate")?;
            let start = Instant::now();
            let instance = Instance::new(&mut store, &module, &imports)?;
            let elapsed = start.elapsed().as_nanos() as u64;
            let mut observations = Vec::new();
            if profile == "memory"
                && let Some(mem) = instance.get_memory(&mut store, "memory")
            {
                observations.push(observation(
                    "guest.memory.logical",
                    mem.data_size(&store),
                    "guest_linear_memory",
                    "instantiate/instantiated",
                    "memory",
                ));
            }
            barrier(i, "instantiated")?;
            let mut state = self.resolve_state_with_init(store, instance, true)?;
            state
                .function
                .call(&mut state.store, &state.args, &mut state.results)?;
            let values = state.results.clone();
            let result = self.verify(&mut state, &values)?;
            drop(state);
            barrier(i, "instance_released")?;
            out.push(json!({"index":i,"warmup":false,"elapsed_ns":elapsed,"operations":1,"sample_type":"individual_operation","verified":true,"result":result,"observations":observations}));
        }
        Ok(json!({"samples":out}))
    }

    pub(super) fn run_app_init(
        &self,
        r: &Value,
        barrier: &mut dyn FnMut(u64, &str) -> Result<()>,
    ) -> Result<Value> {
        let w = self.workload()?;
        let name = w["initialize"]
            .as_str()
            .filter(|s| !s.is_empty())
            .ok_or_else(|| anyhow!("missing initializer"))?;
        let profile = self.prep.as_ref().unwrap()["profile"]
            .as_str()
            .unwrap_or("");
        let samples = r["samples"].as_u64().unwrap_or(0);
        let phased = r["phase_barriers"].as_bool().unwrap_or(false);
        if w["abi"] != "core"
            || w["oracle"]["kind"] != "exact_u64"
            || !w["vectors"].is_null()
            || !w["command"].is_null()
            || !["stateless", "fresh_instance_per_sample"]
                .contains(&w["reset"].as_str().unwrap_or(""))
            || !["timing", "memory"].contains(&profile)
            || (phased && profile != "memory")
            || samples == 0
            || samples > 100000
            || r["operations"].as_u64().unwrap_or(0) == 0
        {
            bail!("unsupported app-init contract/profile/batch")
        }
        let engine = self.engine()?;
        let module = Module::new(&engine, &self.bytes)?;
        let mut out = Vec::with_capacity(samples as usize);
        for i in 0..samples {
            let mut store = Store::new(&engine, ());
            let imports = self.imports(&mut store, &module)?;
            let instance = Instance::new(&mut store, &module, &imports)?;
            let init = instance.get_typed_func::<(), ()>(&mut store, name)?;
            if phased {
                barrier(i, "before_app_init")?;
            }
            let start = Instant::now();
            init.call(&mut store, ())?;
            let elapsed = start.elapsed().as_nanos() as u64;
            let mut observations = Vec::new();
            if profile == "memory" {
                if let Some(mem) = instance.get_memory(&mut store, "memory") {
                    observations.push(observation(
                        "guest.memory.logical",
                        mem.data_size(&store),
                        "guest_linear_memory",
                        "app-init/initialized",
                        "memory",
                    ));
                }
            }
            if phased {
                barrier(i, "app_initialized")?;
            }
            // Apply input and resolve the workload without calling init twice.
            let mut state = self.resolve_state_with_init(store, instance, false)?;
            state
                .function
                .call(&mut state.store, &state.args, &mut state.results)?;
            let values = state.results.clone();
            let result = self.verify(&mut state, &values)?;
            drop(state);
            if phased {
                barrier(i, "app_released")?;
            }
            out.push(json!({"index":i,"warmup":false,"elapsed_ns":elapsed,"operations":1,"sample_type":"individual_operation","verified":true,"result":result,"observations":observations}));
        }
        Ok(json!({"samples":out}))
    }
}
