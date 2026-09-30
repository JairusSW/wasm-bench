use super::*;

fn unsigned(value: &Value, name: &str) -> Result<u64> {
    value
        .as_u64()
        .ok_or_else(|| anyhow!("invalid vector {name}"))
}
fn offset(value: &Value) -> Result<u32> {
    Ok(u32::try_from(unsigned(value, "offset")?)?)
}
fn pointer(instance: &Instance, store: &mut Store<()>, name: &Value, fallback: u32) -> Result<u32> {
    match name.as_str().filter(|s| !s.is_empty()) {
        Some(name) => Ok(instance
            .get_typed_func::<(), i32>(&mut *store, name)?
            .call(store, ())? as u32),
        None => Ok(fallback),
    }
}

impl Adapter {
    pub(super) fn run_vectors(
        &self,
        r: &Value,
        barrier: &mut dyn FnMut(u64, &str) -> Result<()>,
    ) -> Result<Value> {
        let w = self.workload()?;
        let v = &w["vectors"];
        if w["oracle"]["kind"] != "exact_vectors"
            || w["reset"] != "fresh_instance_per_sample"
            || !w["input"].is_null()
            || w["args"].as_array().is_some_and(|a| !a.is_empty())
            || w["oracle"]["expected"]
                .as_array()
                .is_some_and(|a| !a.is_empty())
            || w["oracle"]["memory"]
                .as_array()
                .is_some_and(|a| !a.is_empty())
            || !w["oracle"]["output_pointer_export"]
                .as_str()
                .unwrap_or("")
                .is_empty()
            || !w["host_profile"].as_str().unwrap_or("").is_empty()
        {
            bail!("unsupported or ambiguous vector contract")
        }
        let scenario = r["scenario"].as_str().unwrap_or("");
        let profile = self.prep.as_ref().unwrap()["profile"]
            .as_str()
            .unwrap_or("");
        let phased = r["phase_barriers"].as_bool().unwrap_or(false);
        if !["timing", "memory"].contains(&profile)
            || (phased
                && (profile != "memory"
                    || !["compile", "instantiate", "first-call", "teardown"].contains(&scenario)))
            || !["compile", "instantiate", "first-call", "steady", "teardown"].contains(&scenario)
        {
            bail!("unsupported vector scenario/profile")
        }
        let samples = unsigned(&r["samples"], "samples")?;
        let requested_warmup = unsigned(&r["warmup"], "warmup")?;
        if samples == 0
            || samples > 100000
            || requested_warmup > 100000
            || unsigned(&r["operations"], "operations")? == 0
        {
            bail!("invalid vector batch")
        }
        let warmup = if scenario == "steady" {
            requested_warmup
        } else {
            0
        };
        let input_offset = offset(&v["input_offset"])?;
        let output_offset = offset(&v["output_offset"])?;
        let output_len = unsigned(&v["output_len"], "output length")?;
        if output_len == 0 || output_len > u32::MAX as u64 {
            bail!("invalid vector output length")
        }
        let modulus = if v["mod"].is_null() {
            0
        } else {
            unsigned(&v["mod"], "modulus")?
        };
        let budget = unsigned(&w["vector_byte_budget"], "budget")?;
        let cases = v["cases"]
            .as_array()
            .ok_or_else(|| anyhow!("missing vector cases"))?;
        if cases.is_empty() {
            bail!("empty vector cases")
        }
        let mut total = 0u64;
        for case in cases {
            let len = unsigned(&case["len"], "input length")?;
            let hex = case["out"]
                .as_str()
                .ok_or_else(|| anyhow!("missing vector digest"))?;
            if len > u32::MAX as u64 || hex.len() as u64 != output_len * 2 {
                bail!("invalid vector dimensions")
            }
            total = total
                .checked_add(len + output_len)
                .ok_or_else(|| anyhow!("vector budget overflow"))?;
            if total > budget {
                bail!("vector input/oracle byte budget exceeded")
            }
        }
        let prepared = cases
            .iter()
            .map(|case| -> Result<(Vec<u8>, Vec<u8>)> {
                let len = usize::try_from(unsigned(&case["len"], "input length")?)?;
                let mut input = vec![0u8; len];
                if modulus > 0 {
                    for (i, b) in input.iter_mut().enumerate() {
                        *b = (i as u64 % modulus) as u8
                    }
                }
                Ok((input, hex::decode(case["out"].as_str().unwrap())?))
            })
            .collect::<Result<Vec<_>>>()?;
        let engine = if scenario == "teardown" {
            None
        } else {
            Some(self.engine()?)
        };
        let shared = if scenario == "compile" || scenario == "teardown" {
            None
        } else {
            Some(Module::new(engine.as_ref().unwrap(), &self.bytes)?)
        };
        let timed_calls = ["first-call", "steady"].contains(&scenario);
        let mut output = Vec::with_capacity((samples + warmup) as usize);
        for index in 0..samples + warmup {
            let owned_engine = if scenario == "teardown" {
                Some(self.engine()?)
            } else {
                None
            };
            let current_engine = owned_engine.as_ref().or(engine.as_ref()).unwrap();
            if phased && scenario == "compile" {
                barrier(index, "before_compile")?;
            }
            let mut elapsed = 0u128;
            let mut compiled = None;
            let module = match &shared {
                Some(m) => m,
                None => {
                    let start = Instant::now();
                    compiled = Some(Module::new(current_engine, &self.bytes)?);
                    elapsed = start.elapsed().as_nanos();
                    compiled.as_ref().unwrap()
                }
            };
            if phased && scenario == "compile" {
                barrier(index, "compiled")?;
            }
            let mut store = Store::new(current_engine, ());
            if phased && scenario == "instantiate" {
                barrier(index, "before_instantiate")?;
            }
            let start = Instant::now();
            let instance = Instance::new(&mut store, module, &[])?;
            if scenario == "instantiate" {
                elapsed = start.elapsed().as_nanos()
            }
            if phased && scenario == "instantiate" {
                barrier(index, "instantiated")?;
            }
            if let Some(name) = w["initialize"].as_str().filter(|s| !s.is_empty()) {
                instance
                    .get_typed_func::<(), ()>(&mut store, name)?
                    .call(&mut store, ())?;
            }
            let function = instance
                .get_func(
                    &mut store,
                    w["export"]
                        .as_str()
                        .ok_or_else(|| anyhow!("missing vector export"))?,
                )
                .ok_or_else(|| anyhow!("missing vector export"))?;
            let ty = function.ty(&store);
            if ty.params().len() != 3 || !ty.params().all(|t| matches!(t, ValType::I32)) {
                bail!("unsupported vector function signature")
            }
            let mut results = ty
                .results()
                .map(|t| {
                    Val::default_for_ty(&t).ok_or_else(|| anyhow!("unsupported vector result type"))
                })
                .collect::<Result<Vec<_>>>()?;
            let memory = instance
                .get_memory(&mut store, "memory")
                .ok_or_else(|| anyhow!("missing vector memory"))?;
            let input = pointer(&instance, &mut store, &v["input_ptr_export"], input_offset)?;
            let out = pointer(
                &instance,
                &mut store,
                &v["output_ptr_export"],
                output_offset,
            )?;
            for (case, (data, want)) in prepared.iter().enumerate() {
                if input as u64 + data.len() as u64 > 1u64 << 32
                    || out as u64 + want.len() as u64 > 1u64 << 32
                {
                    bail!("vector address overflow")
                }
                memory.write(&mut store, input as usize, data)?;
                let args = [
                    Val::I32(input as i32),
                    Val::I32(data.len() as i32),
                    Val::I32(out as i32),
                ];
                if phased && scenario == "first-call" && case == 0 {
                    barrier(index, "before_first_call")?;
                }
                if timed_calls {
                    let start = Instant::now();
                    function.call(&mut store, &args, &mut results)?;
                    elapsed += start.elapsed().as_nanos();
                } else {
                    function.call(&mut store, &args, &mut results)?;
                }
                if phased && scenario == "first-call" && case + 1 == prepared.len() {
                    barrier(index, "first_call_returned")?;
                }
                let end = (out as usize)
                    .checked_add(want.len())
                    .ok_or_else(|| anyhow!("vector output overflow"))?;
                if memory.data(&store).get(out as usize..end) != Some(want.as_slice()) {
                    bail!("incorrect result: vector {case} memory mismatch")
                }
            }
            let mut sample = json!({"index":index,"warmup":index<warmup,"elapsed_ns":u64::try_from(elapsed)?,"operations":1,"sample_type":if timed_calls {"sequence_call_sum"} else {"individual_operation"},"verified":true});
            if profile == "memory" {
                let mut logical = observation(
                    "guest.memory.logical",
                    memory.data_size(&store),
                    "guest_linear_memory",
                    &format!("{scenario}/vector_verified"),
                    "memory",
                );
                logical["normalization_denominator"] = json!("instance");
                sample["observations"] = json!([logical]);
            }
            if phased && scenario == "teardown" {
                barrier(index, "before_teardown")?;
            }
            let release_start = Instant::now();
            drop(store);
            drop(compiled);
            drop(owned_engine);
            if scenario == "teardown" {
                sample["elapsed_ns"] = json!(u64::try_from(release_start.elapsed().as_nanos())?);
            }
            if phased {
                barrier(
                    index,
                    if scenario == "teardown" {
                        "torn_down"
                    } else if scenario == "instantiate" {
                        "instance_released"
                    } else if scenario == "first-call" {
                        "first_call_released"
                    } else {
                        "released"
                    },
                )?;
            }
            output.push(sample);
        }
        Ok(json!({"samples":output}))
    }
}
