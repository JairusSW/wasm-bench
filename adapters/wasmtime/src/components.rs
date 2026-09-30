//! Component Model compilation. Execution is intentionally gated until a
//! component world, host policy, and behavioral oracle are locked.
use serde_json::{Value, json};
use wasmtime::{Engine, Result, component::Component, format_err};

pub fn validate_workload(w: &Value) -> Result<()> {
    if w["abi"] != "component"
        || w["oracle"]["kind"] != "component_compile_only"
        || !w["command"].is_null()
        || !w["vectors"].is_null()
        || !w["input"].is_null()
        || w["reset"] != "stateless"
        || w["work_unit"] != "component"
        || w["units_per_invocation"] != 1
        || w["host_profile"].as_str().is_some_and(|s| !s.is_empty())
    {
        return Err(format_err!("invalid component compile-only contract"));
    }
    Ok(())
}

pub fn run(engine: &Engine, bytes: &[u8], prep: &Value, request: &Value) -> Result<Value> {
    let w = &prep["workload"];
    validate_workload(w)?;
    let scenario = request["scenario"].as_str().unwrap_or("");
    if scenario != "compile"
        || prep["profile"] != "timing"
        || request["phase_barriers"] == true
        || request["operations"] != 1
    {
        return Ok(
            json!({"status":"unsupported","reason":"only uninstrumented single component compilation is supported"}),
        );
    }
    let samples = request["samples"]
        .as_u64()
        .ok_or_else(|| format_err!("invalid component sample count"))?;
    if samples == 0 || samples > 100_000 {
        return Err(format_err!("invalid component sample count"));
    }
    let mut output = Vec::with_capacity(samples as usize);
    for index in 0..samples {
        let start = std::time::Instant::now();
        let compiled = Component::new(engine, bytes)?;
        let elapsed = u64::try_from(start.elapsed().as_nanos()).unwrap_or(u64::MAX);
        drop(compiled);
        output.push(json!({"index":index,"warmup":false,"elapsed_ns":elapsed,"operations":1,"sample_type":"individual_operation","verified":true,"result":[]}));
    }
    Ok(json!({"samples":output}))
}
