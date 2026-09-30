use super::*;

#[cfg(test)]
mod tests {
    use super::*;
    fn workload() -> Value {
        json!({"abi":"core","export":"benchmark","args":["65536"],"oracle":{"kind":"exact_u64","expected":["65537"]},"reset":"fresh_instance_per_sample","work_unit":"instance_group","units_per_invocation":1,"density":{"instances":4,"sharing":"shared_module"}})
    }
    #[test]
    fn groups_are_distinct_and_verified() -> Result<()> {
        for winch in [false, true] {
            for separate in [false, true] {
                let a = Adapter {
                    prep: Some(json!({"workload":workload(),"profile":"memory"})),
                    bytes: include_bytes!("../../../corpus/testdata/density.wasm").to_vec(),
                    winch,
                };
                let mut group = a.density_group(4, separate)?;
                assert_eq!(group.states.len(), 4);
                assert_eq!(group.engines.len(), if separate { 4 } else { 1 });
                assert_eq!(group.modules.len(), group.engines.len());
                let mut addresses = Vec::new();
                for state in &mut group.states {
                    let values = state.results.clone();
                    a.verify(state, &values)?;
                    let mem = state
                        .instance
                        .get_memory(&mut state.store, "memory")
                        .unwrap();
                    assert!(mem.data(&state.store).iter().all(|v| *v == 1));
                    addresses.push(mem.data_ptr(&state.store) as usize);
                    state
                        .function
                        .call(&mut state.store, &state.args, &mut state.results)?;
                    let values = state.results.clone();
                    assert!(
                        a.verify(state, &values).is_err(),
                        "instance reuse must fail"
                    );
                }
                addresses.sort();
                addresses.dedup();
                assert_eq!(addresses.len(), 4);
            }
        }
        Ok(())
    }
    #[test]
    fn invalid_density_contracts_fail_closed() {
        let w = workload();
        let r = json!({"scenario":"density","samples":2,"operations":1,"warmup":0});
        assert!(contract(&w, "timing", &r).is_ok());
        for count in [0, 129] {
            let mut x = w.clone();
            x["density"]["instances"] = json!(count);
            assert!(contract(&x, "timing", &r).is_err());
        }
        for mode in ["pooled", "separate_modules", ""] {
            let mut x = w.clone();
            x["density"]["sharing"] = json!(mode);
            assert!(contract(&x, "timing", &r).is_err());
        }
        for (key, value) in [
            ("warmup", json!(1)),
            ("operations", json!(2)),
            ("samples", json!(0)),
            ("scenario", json!("steady")),
            ("phase_barriers", json!(true)),
        ] {
            let mut x = r.clone();
            x[key] = value;
            assert!(contract(&w, "timing", &x).is_err());
        }
        assert!(contract(&w, "counters", &r).is_err());
    }

    #[test]
    fn cycles_keep_resources_but_reset_instances() -> Result<()> {
        let a = Adapter {
            prep: Some(json!({"workload":workload(),"profile":"memory"})),
            bytes: include_bytes!("../../../corpus/testdata/density.wasm").to_vec(),
            winch: false,
        };
        for separate in [false, true] {
            let mut group = a.density_resources(4, separate)?;
            assert!(group.states.is_empty(), "no hidden instance prewarm");
            let engine_address = &group.engines[0] as *const Engine;
            let module_address = &group.modules[0] as *const Module;
            for _ in 0..3 {
                a.density_refill(&mut group, 4, separate)?;
                assert_eq!(group.states.len(), 4);
                for state in &mut group.states {
                    let values = state.results.clone();
                    a.verify(state, &values)?;
                }
                group.states.clear();
                assert_eq!(&group.engines[0] as *const Engine, engine_address);
                assert_eq!(&group.modules[0] as *const Module, module_address);
            }
        }
        Ok(())
    }
}

struct Group {
    // Drop stores before modules and engines, including on error paths.
    states: Vec<State>,
    modules: Vec<Module>,
    engines: Vec<Engine>,
}

fn contract(w: &Value, profile: &str, r: &Value) -> Result<(usize, bool, u64)> {
    let count = w["density"]["instances"].as_u64().unwrap_or(0);
    let mode = w["density"]["sharing"].as_str().unwrap_or("");
    let samples = r["samples"].as_u64().unwrap_or(0);
    if !(1..=128).contains(&count)
        || !["shared_module", "separate_engines"].contains(&mode)
        || w["abi"] != "core"
        || w["host_profile"].as_str().unwrap_or("") != ""
        || !w["command"].is_null()
        || !w["vectors"].is_null()
        || w["oracle"]["kind"] != "exact_u64"
        || !w["oracle"]["float"].is_null()
        || w["export"].as_str().unwrap_or("").is_empty()
        || w["reset"] != "fresh_instance_per_sample"
        || w["work_unit"] != "instance_group"
        || w["units_per_invocation"].as_u64() != Some(1)
        || (r["scenario"] != "density" && r["scenario"] != "density-cycle")
        || r["operations"].as_u64() != Some(1)
        || r["warmup"].as_u64() != Some(0)
        || !(1..=100000).contains(&samples)
        || !["timing", "memory"].contains(&profile)
        || (r["phase_barriers"] == true && profile != "memory")
    {
        bail!("unsupported density contract/profile/batch")
    }
    Ok((count as usize, mode == "separate_engines", samples))
}

impl Adapter {
    fn density_resources(&self, count: usize, separate: bool) -> Result<Group> {
        let mut g = Group {
            states: Vec::with_capacity(count),
            modules: Vec::new(),
            engines: Vec::new(),
        };
        for _ in 0..if separate { count } else { 1 } {
            let engine = self.engine()?;
            let module = Module::new(&engine, &self.bytes)?;
            if module.imports().next().is_some() {
                bail!("density requires import-free modules")
            }
            g.engines.push(engine);
            g.modules.push(module);
        }
        Ok(g)
    }

    fn density_refill(&self, group: &mut Group, count: usize, separate: bool) -> Result<()> {
        if !group.states.is_empty() {
            bail!("density refill requires released stores")
        }
        for i in 0..count {
            let j = if separate { i } else { 0 };
            let mut state = self.prepare_state(&group.engines[j], &group.modules[j])?;
            state
                .function
                .call(&mut state.store, &state.args, &mut state.results)?;
            group.states.push(state);
        }
        Ok(())
    }
    fn density_group(&self, count: usize, separate: bool) -> Result<Group> {
        let mut g = Group {
            states: Vec::with_capacity(count),
            modules: Vec::new(),
            engines: Vec::new(),
        };
        for i in 0..count {
            if i == 0 || separate {
                let engine = self.engine()?;
                let module = Module::new(&engine, &self.bytes)?;
                if module.imports().next().is_some() {
                    bail!("density requires import-free modules")
                }
                g.engines.push(engine);
                g.modules.push(module);
            }
            let mut state =
                self.prepare_state(g.engines.last().unwrap(), g.modules.last().unwrap())?;
            state
                .function
                .call(&mut state.store, &state.args, &mut state.results)?;
            g.states.push(state);
        }
        Ok(g)
    }

    pub(super) fn run_density(
        &self,
        r: &Value,
        barrier: &mut dyn FnMut(u64, &str) -> Result<()>,
    ) -> Result<Value> {
        let profile = self
            .prep
            .as_ref()
            .ok_or_else(|| anyhow!("prepare required"))?["profile"]
            .as_str()
            .unwrap_or("");
        let (count, separate, samples) = contract(self.workload()?, profile, r)?;
        let phased = r["phase_barriers"] == true;
        let cycle = r["scenario"] == "density-cycle";
        let stages = if cycle {
            [
                "before_density_cycle",
                "density_cycle_ready",
                "density_cycle_released",
            ]
        } else {
            ["before_density", "density_ready", "density_released"]
        };
        let mut retained = if cycle {
            Some(self.density_resources(count, separate)?)
        } else {
            None
        };
        let mut out = Vec::with_capacity(samples as usize);
        for i in 0..samples {
            if phased {
                barrier(i, stages[0])?;
            }
            let start = Instant::now();
            let mut group = if cycle {
                let mut group = retained.take().unwrap();
                self.density_refill(&mut group, count, separate)?;
                group
            } else {
                self.density_group(count, separate)?
            };
            let elapsed = start.elapsed().as_nanos() as u64;
            let mut result = Vec::new();
            let mut logical = 0usize;
            for state in &mut group.states {
                let values = state.results.clone();
                result = self.verify(state, &values)?;
                if let Some(memory) = state.instance.get_memory(&mut state.store, "memory") {
                    logical += memory.data_size(&state.store);
                }
            }
            let mut observations = Vec::new();
            if profile == "memory" {
                let mut o = observation(
                    "density.guest_memory.logical",
                    logical,
                    "instance_group_linear_memory",
                    stages[1],
                    "memory",
                );
                o["normalization_denominator"] = json!("instance_group");
                observations.push(o);
            }
            if phased {
                barrier(i, stages[1])?;
            }
            if cycle {
                group.states.clear();
                retained = Some(group);
            } else {
                drop(group);
            }
            if phased {
                barrier(i, stages[2])?;
            }
            out.push(json!({"index":i,"warmup":false,"elapsed_ns":elapsed,"operations":1,"sample_type":"individual_operation","verified":true,"result":result,"observations":observations}));
        }
        Ok(json!({"samples":out}))
    }
}
