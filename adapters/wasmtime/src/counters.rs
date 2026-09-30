use super::*;

// Keep aligned with protocol.ValidateCounterRun. This validates the complete
// request before dispatch so no unsupported path emits a counter barrier.
pub(super) fn validate(prep: &Value, r: &Value) -> Result<()> {
    let w = &prep["workload"];
    if prep["profile"] != "counters"
        || w["abi"] != "core"
        || !w["command"].is_null()
        || !w["vectors"].is_null()
        || !w["density"].is_null()
        || w["oracle"]["kind"] != "exact_u64"
        || !["stateless", "fresh_instance_per_sample"].contains(&w["reset"].as_str().unwrap_or(""))
        || w["export"].as_str().unwrap_or("").is_empty()
    {
        bail!("counters require a core scalar exact oracle");
    }
    let steady = r["scenario"] == "steady";
    if steady && w["reset"] != "stateless" {
        bail!("steady counters require stateless repeated invocations");
    }
    if !["compile", "instantiate", "first-call", "steady"]
        .contains(&r["scenario"].as_str().unwrap_or(""))
        || r["phase_barriers"] != true
        || !matches!(r["samples"].as_u64(), Some(1..=100000))
        || !matches!(r["operations"].as_u64(), Some(1..=1000000))
        || !matches!(r["warmup"].as_u64(), Some(0..=100000))
        || (!steady && (r["operations"].as_u64() != Some(1) || r["warmup"].as_u64() != Some(0)))
    {
        bail!(
            "counter batches require phase barriers and bounded budgets; non-steady requires one operation and no warmup"
        );
    }
    Ok(())
}

impl Adapter {
    pub(super) fn steady_counters(
        &self,
        r: &Value,
        barrier: &mut dyn FnMut(u64, &str) -> Result<()>,
    ) -> Result<Value> {
        validate(
            self.prep
                .as_ref()
                .ok_or_else(|| anyhow!("prepare required"))?,
            r,
        )?;
        let engine = self.engine()?;
        let module = Module::new(&engine, &self.bytes)?;
        let mut state = self.prepare_state(&engine, &module)?;
        let warmup = r["warmup"].as_u64().unwrap();
        let samples = r["samples"].as_u64().unwrap();
        let operations = r["operations"].as_u64().unwrap();
        let mut out = Vec::with_capacity((samples + warmup) as usize);
        for i in 0..samples + warmup {
            let mut results = vec![state.results.clone(); operations as usize];
            barrier(i, "before_steady_batch")?;
            let mut call = Ok(());
            let start = Instant::now();
            for values in &mut results {
                call = state.function.call(&mut state.store, &state.args, values);
                if call.is_err() {
                    break;
                }
            }
            let elapsed = start.elapsed().as_nanos() as u64;
            barrier(i, "steady_batch_returned")?;
            call?;
            let mut result = vec![];
            for values in &results {
                result = self.verify(&mut state, values)?;
            }
            barrier(i, "steady_batch_verified")?;
            let kind = if operations == 1 {
                "individual_operation"
            } else {
                "batch_average"
            };
            out.push(json!({"index":i,"warmup":i<warmup,"elapsed_ns":elapsed,"operations":operations,"sample_type":kind,"verified":true,"result":result}));
        }
        Ok(json!({"samples":out}))
    }
    pub(super) fn first_call_counters(
        &self,
        r: &Value,
        barrier: &mut dyn FnMut(u64, &str) -> Result<()>,
    ) -> Result<Value> {
        validate(
            self.prep
                .as_ref()
                .ok_or_else(|| anyhow!("prepare required"))?,
            r,
        )?;
        let engine = self.engine()?;
        let module = Module::new(&engine, &self.bytes)?;
        let samples = r["samples"].as_u64().unwrap();
        let mut out = Vec::with_capacity(samples as usize);
        for i in 0..samples {
            let mut state = self.prepare_state(&engine, &module)?;
            barrier(i, "before_first_call")?;
            let start = Instant::now();
            let call = state
                .function
                .call(&mut state.store, &state.args, &mut state.results);
            let elapsed = start.elapsed().as_nanos() as u64;
            barrier(i, "first_call_returned")?;
            call?;
            let values = state.results.clone();
            let result = self.verify(&mut state, &values)?;
            drop(state);
            barrier(i, "first_call_released")?;
            out.push(json!({"index":i,"warmup":false,"elapsed_ns":elapsed,"operations":1,"sample_type":"individual_operation","verified":true,"result":result}));
        }
        Ok(json!({"samples":out}))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn exact_counter_contract() {
        let prep = json!({"profile":"counters","workload":{"abi":"core","oracle":{"kind":"exact_u64"},"reset":"stateless","export":"benchmark"}});
        let request = json!({"scenario":"compile","samples":2,"operations":1,"warmup":0,"phase_barriers":true});
        for scenario in ["compile", "instantiate", "first-call"] {
            for reset in ["stateless", "fresh_instance_per_sample"] {
                let mut p = prep.clone();
                let mut r = request.clone();
                p["workload"]["reset"] = json!(reset);
                r["scenario"] = json!(scenario);
                assert!(validate(&p, &r).is_ok());
            }
        }
        for (key, value) in [
            ("scenario", json!("teardown")),
            ("phase_barriers", json!(false)),
            ("samples", json!(0)),
            ("samples", json!(100001)),
            ("samples", json!(-1)),
            ("operations", json!(2)),
            ("warmup", json!(1)),
            ("warmup", json!(-1)),
            ("warmup", Value::Null),
        ] {
            let mut r = request.clone();
            r[key] = value;
            assert!(validate(&prep, &r).is_err(), "{r}");
        }
        for (key, value) in [
            ("abi", json!("wasi-command")),
            ("oracle", json!({"kind":"float_bits_v1"})),
            ("reset", json!("reused")),
            ("export", json!("")),
            ("command", json!({})),
            ("vectors", json!({})),
            ("density", json!({})),
        ] {
            let mut p = prep.clone();
            p["workload"][key] = value;
            assert!(validate(&p, &request).is_err(), "{p}");
        }
        let mut p = prep.clone();
        p["profile"] = json!("memory");
        assert!(validate(&p, &request).is_err());
        let mut steady = request.clone();
        steady["scenario"] = json!("steady");
        steady["warmup"] = json!(2);
        steady["operations"] = json!(5);
        assert!(validate(&prep, &steady).is_ok());
        for (key, value) in [
            ("warmup", json!(-1)),
            ("warmup", json!(100001)),
            ("operations", json!(0)),
            ("operations", json!(1000001)),
        ] {
            let mut bad = steady.clone();
            bad[key] = value;
            assert!(validate(&prep, &bad).is_err());
        }
        let mut p = prep.clone();
        p["workload"]["reset"] = json!("fresh_instance_per_sample");
        assert!(validate(&p, &steady).is_err());
    }
}
