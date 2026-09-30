use serde_json::{Value, json};
use wasmtime::{Result, bail};

pub const POLICY: &str = "empty-local-harness-v1: timing only; resident stateless core scalar workload checked before sampling; preallocated uint64-equivalent slots; timed local loop writes iteration+1 to every slot without Wasm/embedding calls; every slot verified after timer; no warmup, barriers or memory instrumentation; sample result is iteration count, not guest output; no automatic subtraction or performance scoring";

pub fn validate(prep: &Value, r: &Value) -> Result<(usize, usize)> {
    let w = &prep["workload"];
    if r["scenario"] != "harness-calibration"
        || prep["profile"] != "timing"
        || (!r["phase_barriers"].is_null() && r["phase_barriers"] != false)
        || (!r["warmup"].is_null() && r["warmup"].as_u64() != Some(0))
        || w["abi"] != "core"
        || w["reset"] != "stateless"
        || w["oracle"]["kind"] != "exact_u64"
        || !w["oracle"]["float"].is_null()
        || w["export"].as_str().is_none_or(str::is_empty)
        || [
            "command",
            "vectors",
            "density",
            "checkpoint",
            "continuation",
            "guest_density",
            "process_snapshot",
            "snapshot_density",
        ]
        .iter()
        .any(|k| !w[k].is_null())
    {
        bail!("unsupported harness calibration contract");
    }
    let samples = r["samples"].as_u64().filter(|n| (1..=100000).contains(n));
    let ops = r["operations"]
        .as_u64()
        .filter(|n| (1..=1000000).contains(n));
    match (samples, ops) {
        (Some(s), Some(o)) => Ok((s as usize, o as usize)),
        _ => bail!("invalid harness calibration budget"),
    }
}

// Caller verifies workload behavior once outside calibration sampling.
pub fn run(samples: usize, operations: usize) -> Result<Value> {
    let mut slots = vec![0u64; operations];
    let mut output = Vec::with_capacity(samples);
    for index in 0..samples {
        slots.fill(0);
        let start = std::time::Instant::now();
        for (i, slot) in slots.iter_mut().enumerate() {
            *slot = (i + 1) as u64;
        }
        // Escape stores before reading the timer: calibration retains real bookkeeping.
        std::hint::black_box(&slots);
        let elapsed = start.elapsed().as_nanos() as u64;
        if slots.iter().enumerate().any(|(i, v)| *v != (i + 1) as u64) {
            bail!("incorrect harness calibration bookkeeping");
        }
        output.push(json!({"index":index,"warmup":false,"elapsed_ns":elapsed,"operations":operations,"sample_type":if operations==1{"individual_operation"}else{"batch_average"},"verified":true,"result":[operations.to_string()]}));
    }
    Ok(json!({"samples":output}))
}
