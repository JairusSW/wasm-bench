// Ordered vector calls use one fresh instance per sequence. Input writes and
// exact output checks stay outside the sum of timed embedding calls.
use super::*;

fn number(v: &Value, name: &str, maximum: u64) -> Result<usize> {
    let value = v[name]
        .as_u64()
        .ok_or_else(|| anyhow::anyhow!("invalid vector {name}"))?;
    ensure!(value <= maximum, "vector {name} exceeds bound");
    Ok(usize::try_from(value)?)
}
fn address(
    target: &mut embedding::Instance,
    v: &Value,
    key: &str,
    fallback: usize,
) -> Result<usize> {
    match v[key].as_str().filter(|s| !s.is_empty()) {
        None => Ok(fallback),
        Some(name) => {
            let (params, results) = target.signature(name)?;
            ensure!(
                params.is_empty() && results == [0x7f],
                "unsupported: vector pointer export must be () -> i32"
            );
            let result = target.call(name, &[])?;
            Ok(usize::try_from(result[0])?)
        }
    }
}
pub(super) fn validate(w: &Value) -> Result<Vec<(Vec<u8>, Vec<u8>)>> {
    ensure!(
        w["oracle"]["kind"] == "exact_vectors"
            && w["reset"] == "fresh_instance_per_sample"
            && w["input"].is_null()
            && w["args"].as_array().is_some_and(|a| a.is_empty())
            && w["oracle"]["expected"]
                .as_array()
                .is_some_and(|a| a.is_empty())
            && w["oracle"]["memory"]
                .as_array()
                .is_none_or(|a| a.is_empty())
            && w["oracle"]["output_pointer_export"]
                .as_str()
                .unwrap_or("")
                .is_empty()
            && w["host_profile"].as_str().unwrap_or("").is_empty(),
        "unsupported: ambiguous ordered vector contract"
    );
    let v = &w["vectors"];
    number(v, "input_offset", u32::MAX as u64)?;
    number(v, "output_offset", u32::MAX as u64)?;
    let output = number(v, "output_len", u32::MAX as u64)?;
    ensure!(output > 0, "empty vector output");
    let budget = number(w, "vector_byte_budget", 64 * 1024 * 1024)?;
    let modulus = if v["mod"].is_null() {
        0
    } else {
        number(v, "mod", u32::MAX as u64)?
    };
    let cases = v["cases"]
        .as_array()
        .ok_or_else(|| anyhow::anyhow!("vector cases required"))?;
    ensure!(!cases.is_empty(), "empty vector sequence");
    let mut total = 0usize;
    let mut prepared = Vec::new();
    for case in cases {
        let length = number(case, "len", u32::MAX as u64)?;
        total = total
            .checked_add(length)
            .and_then(|n| n.checked_add(output))
            .ok_or_else(|| anyhow::anyhow!("vector budget overflow"))?;
        ensure!(total <= budget, "vector input/oracle byte budget exceeded");
        let text = field(case, "out")?;
        ensure!(
            text.len()
                == output
                    .checked_mul(2)
                    .ok_or_else(|| anyhow::anyhow!("vector hex length overflow"))?,
            "vector oracle length mismatch"
        );
        let expected = hex::decode(text)?;
        let input = (0..length)
            .map(|i| if modulus == 0 { 0 } else { (i % modulus) as u8 })
            .collect();
        prepared.push((input, expected));
    }
    Ok(prepared)
}
impl Adapter {
    pub(super) fn run_vectors(&self, req: &Value, input: &mut impl BufRead) -> Result<Value> {
        let prep = self
            .prep
            .as_ref()
            .ok_or_else(|| anyhow::anyhow!("prepare required"))?;
        let w = &prep["workload"];
        let v = &w["vectors"];
        let r = &req["run"];
        let prepared = validate(w)?;
        let scenario = field(r, "scenario")?;
        let profile = field(prep, "profile")?;
        let phased = r["phase_barriers"].as_bool().unwrap_or(false);
        ensure!(
            ["compile", "instantiate", "first-call", "steady"].contains(&scenario)
                && ["timing", "memory"].contains(&profile),
            "unsupported: vector scenario/profile"
        );
        ensure!(
            !phased || profile == "memory" && scenario != "steady",
            "unsupported: vector barriers require memory lifecycle scenario"
        );
        let samples = number(r, "samples", 100000)?;
        ensure!(samples > 0, "empty vector sample batch");
        let requested_warmup = number(r, "warmup", 100000)?;
        let warmup = if scenario == "steady" {
            requested_warmup
        } else {
            0
        };
        ensure!(
            number(r, "operations", 1000000)? > 0,
            "empty vector operation batch"
        );
        let engine = embedding::Engine::new()?;
        let shared = if scenario == "compile" {
            None
        } else {
            Some(engine.compile(&self.bytes)?)
        };
        let stages = match scenario {
            "compile" => ["before_compile", "compiled", "released"],
            "instantiate" => ["before_instantiate", "instantiated", "instance_released"],
            _ => [
                "before_first_call",
                "first_call_returned",
                "first_call_released",
            ],
        };
        let mut out = Vec::new();
        for index in 0..samples + warmup {
            let mut elapsed = 0u128;
            let mut owned = None;
            if scenario == "compile" {
                if phased {
                    barrier(input, &req["id"], index, stages[0])?;
                }
                let start = Instant::now();
                owned = Some(engine.compile(&self.bytes)?);
                elapsed = start.elapsed().as_nanos();
                if phased {
                    barrier(input, &req["id"], index, stages[1])?;
                }
            }
            if phased && scenario == "instantiate" {
                barrier(input, &req["id"], index, stages[0])?;
            }
            let start = Instant::now();
            let mut target = engine.instantiate(shared.as_ref().or(owned.as_ref()).unwrap())?;
            if scenario == "instantiate" {
                elapsed = start.elapsed().as_nanos();
            }
            if phased && scenario == "instantiate" {
                barrier(input, &req["id"], index, stages[1])?;
            }
            initialize(&mut target, w)?;
            let export = field(w, "export")?;
            let (params, results) = target.signature(export)?;
            ensure!(
                params == [0x7f, 0x7f, 0x7f] && results.iter().all(|t| [0x7f, 0x7e].contains(t)),
                "unsupported: vector export requires three i32 arguments and integer results"
            );
            let from = address(
                &mut target,
                v,
                "input_ptr_export",
                number(v, "input_offset", u32::MAX as u64)?,
            )?;
            let to = address(
                &mut target,
                v,
                "output_ptr_export",
                number(v, "output_offset", u32::MAX as u64)?,
            )?;
            for (case, (data, expected)) in prepared.iter().enumerate() {
                ensure!(
                    (from as u64) + (data.len() as u64) <= 1u64 << 32
                        && (to as u64) + (expected.len() as u64) <= 1u64 << 32,
                    "vector address overflow"
                );
                range(&mut target, from, data.len())?.copy_from_slice(data);
                if phased && scenario == "first-call" && case == 0 {
                    barrier(input, &req["id"], index, stages[0])?;
                }
                if ["first-call", "steady"].contains(&scenario) {
                    let start = Instant::now();
                    target.call(export, &[from as u64, data.len() as u64, to as u64])?;
                    elapsed = elapsed
                        .checked_add(start.elapsed().as_nanos())
                        .ok_or_else(|| anyhow::anyhow!("elapsed overflow"))?;
                } else {
                    target.call(export, &[from as u64, data.len() as u64, to as u64])?;
                }
                if phased && scenario == "first-call" && case + 1 == prepared.len() {
                    barrier(input, &req["id"], index, stages[1])?;
                }
                ensure!(
                    range(&mut target, to, expected.len())? == expected,
                    "incorrect result: vector {case} memory mismatch"
                );
            }
            let mut sample = json!({"index":index,"warmup":index<warmup,"elapsed_ns":u64::try_from(elapsed)?,"operations":1,"sample_type":if ["first-call","steady"].contains(&scenario){"sequence_call_sum"}else{"individual_operation"},"verified":true});
            if profile == "memory" {
                sample["observations"] = json!([{"metric":"guest.memory.logical","definition_version":1,"value":target.memory()?.len(),"unit":"bytes","scope":"guest_linear_memory","phase":format!("{scenario}/vector_verified"),"collector":"runtime memory API","collector_version":embedding::version(),"quality":"engine_reported","profile":"memory","status":"available","normalization_denominator":"instance"}]);
            }
            drop(target);
            drop(owned);
            if phased {
                barrier(input, &req["id"], index, stages[2])?;
            }
            out.push(sample);
        }
        Ok(json!({"samples":out}))
    }
}
