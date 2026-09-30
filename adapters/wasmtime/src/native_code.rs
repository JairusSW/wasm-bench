use base64::{Engine as _, engine::general_purpose::STANDARD};
use serde_json::{Value, json};
use sha2::{Digest, Sha256};
use wasmtime::{Module, Result};

pub fn validate_materialized(prep: &Value, r: &Value) -> Result<()> {
    let w = &prep["workload"];
    if prep["profile"] != "code"
        || r["scenario"] != "compile-materialized"
        || r["samples"] != 1
        || r["operations"] != 1
        || r["warmup"] != 0
        || r["phase_barriers"] == true
        || w["abi"] != "core"
        || w["reset"] != "stateless"
        || w["export"].as_str().unwrap_or("").is_empty()
        || w["oracle"]["kind"] != "exact_u64"
        || !w["oracle"]["float"].is_null()
        || [
            "command",
            "vectors",
            "density",
            "checkpoint",
            "guest_density",
            "input",
        ]
        .iter()
        .any(|k| !w[*k].is_null())
        || w["oracle"]["memory"]
            .as_array()
            .is_some_and(|a| !a.is_empty())
        || w["initialize"].as_str().is_some_and(|s| !s.is_empty())
    {
        wasmtime::bail!(
            "materialized compile requires one code-pass compile and a stateless exact-result core workload without compound fixtures"
        );
    }
    Ok(())
}

pub fn materialized(module: &Module, wasm: &[u8], winch: bool, elapsed: u64) -> Result<Value> {
    use wasmparser::{Encoding, Parser, Payload, TypeRef};
    let mut imported = 0u32;
    let mut defined = 0u32;
    for payload in Parser::new(0).parse_all(wasm) {
        match payload? {
            Payload::Version {
                encoding: Encoding::Component,
                ..
            } => wasmtime::bail!("materialization supports core modules only"),
            Payload::ImportSection(reader) => {
                for import in reader.into_imports() {
                    if matches!(import?.ty, TypeRef::Func(_) | TypeRef::FuncExact(_)) {
                        imported = imported
                            .checked_add(1)
                            .ok_or_else(|| wasmtime::format_err!("import count overflow"))?;
                    }
                }
            }
            Payload::FunctionSection(reader) => defined = reader.count(),
            _ => (),
        }
    }
    if defined > 10000 || elapsed > i64::MAX as u64 {
        wasmtime::bail!("materialization evidence exceeds bounded range/clock contract");
    }
    let mut result = inspect(module, wasm, winch)?;
    let image = &mut result["code_image"];
    if image.is_null() {
        wasmtime::bail!("materialized native image exceeds transport budget");
    }
    let functions = image["functions"].as_array().unwrap();
    let text_len = module.text().len() as u64;
    if functions.len() != defined as usize {
        wasmtime::bail!("incomplete native function coverage");
    }
    for (i, f) in functions.iter().enumerate() {
        let offset = f["offset"].as_u64().unwrap();
        let length = f["length"].as_u64().unwrap();
        if f["wasm_index"].as_u64() != Some(imported as u64 + i as u64)
            || length == 0
            || offset > text_len
            || length > text_len - offset
        {
            wasmtime::bail!("invalid defined-function native range");
        }
    }
    image["version"] = json!(3);
    image["materialization"] = json!({"version":1,"collector":"Wasmtime/Module::functions","collector_version":"46.0.1","parser":"wasmparser/0.251.0","scope":"all_defined_function_ranges_at_synchronous_compile_return","completion":"documented_synchronous_api_with_verified_native_coverage","quality":"engine_reported","imported_functions":imported,"defined_functions":defined,"compile_elapsed_ns":elapsed});
    Ok(result)
}

pub fn inspect(module: &Module, wasm: &[u8], winch: bool) -> Result<Value> {
    let text = module.text();
    let backend = if winch { "winch" } else { "cranelift" };
    let functions: Vec<Value> = module
        .functions()
        .map(|f| {
            json!({
                "module_index":0,"wasm_index":f.index.as_u32(),"name":f.name,
                "offset":f.offset,"length":f.len,"tier":backend,"generation":0
            })
        })
        .collect();
    let bytes: usize = module.functions().map(|f| f.len).sum();
    let mut result = json!({"diagnostics":[
        super::observation("artifact.serialized",module.serialize()?.len(),"compiled_module","compile","code"),
        super::observation("native.function_range_bytes",bytes,"guest_function_ranges","compile","code"),
        {"metric":"native.guest_code","definition_version":1,"unit":"bytes","scope":"guest_function_code","phase":"compile","collector":"wasmtime","collector_version":"46.0.1","quality":"engine_reported","profile":"code","status":"unsupported","reason":"function ranges can include embedded constants and padding; instruction-only bytes are not exposed","normalization_denominator":"module"}
    ]});
    if text.len() > 16 << 20 {
        result["diagnostics"].as_array_mut().unwrap().push(json!({
            "metric":"native.code_export","definition_version":1,"unit":"bytes","scope":"compiled_module","phase":"compile","collector":"wasmtime","collector_version":"46.0.1","quality":"engine_reported","profile":"code","status":"unavailable","reason":"native text exceeds 16 MiB transport budget","normalization_denominator":"module"
        }));
    } else {
        result["code_image"] = json!({
            "version":2,"module_sha256":hex::encode(Sha256::digest(wasm)),
            "sha256":hex::encode(Sha256::digest(text)),
            "architecture":if cfg!(target_arch="aarch64") {"arm64"} else {"amd64"},
            "backend":backend,"format":"raw-native-image",
            "section_kind":"mixed_code_and_embedded_data","event":"compiled_snapshot",
            "function_attribution":"engine_reported","functions":functions,"data":STANDARD.encode(text)
        });
    }
    Ok(result)
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn both_backends_export_defined_indices_not_imports() -> Result<()> {
        let wasm=hex::decode("0061736d010000000105016000017f020701016d0166000003020100070501016700010a0601040041070b").unwrap();
        for winch in [false, true] {
            let adapter = crate::Adapter {
                prep: None,
                bytes: wasm.clone(),
                winch,
            };
            let module = Module::new(&adapter.engine()?, &wasm)?;
            let result = inspect(&module, &wasm, winch)?;
            let image = &result["code_image"];
            assert_eq!(image["version"], 2);
            let functions = image["functions"].as_array().unwrap();
            assert_eq!(functions.len(), 1);
            assert_eq!(functions[0]["wasm_index"], 1);
            let offset = functions[0]["offset"].as_u64().unwrap() as usize;
            let length = functions[0]["length"].as_u64().unwrap() as usize;
            assert!(length > 0 && offset + length <= module.text().len());
            let decoded = STANDARD.decode(image["data"].as_str().unwrap()).unwrap();
            assert_eq!(decoded, module.text());
            assert_eq!(image["sha256"], hex::encode(Sha256::digest(&decoded)));
            assert_eq!(result["diagnostics"][1]["value"], length);
            assert!(image["materialization"].is_null());
            let completed = materialized(&module, &wasm, winch, 10)?;
            let completed = &completed["code_image"];
            assert_eq!(completed["version"], 3);
            assert_eq!(completed["materialization"]["imported_functions"], 1);
            assert_eq!(completed["materialization"]["defined_functions"], 1);
            assert_eq!(completed["materialization"]["compile_elapsed_ns"], 10);
            assert_eq!(completed["functions"], image["functions"]);
            assert_eq!(completed["data"], image["data"]);
            assert!(materialized(&module, &wasm, winch, u64::MAX).is_err());
        }
        Ok(())
    }

    #[test]
    fn materialization_includes_unexported_functions() -> Result<()> {
        // Two defined functions, neither exported: coverage is not export-only.
        let wasm =
            hex::decode("0061736d010000000105016000017f03030200000a0b02040041070b040041080b")
                .unwrap();
        for winch in [false, true] {
            let adapter = crate::Adapter {
                prep: None,
                bytes: wasm.clone(),
                winch,
            };
            let module = Module::new(&adapter.engine()?, &wasm)?;
            let result = materialized(&module, &wasm, winch, 0)?;
            assert_eq!(
                result["code_image"]["materialization"]["defined_functions"],
                2
            );
            let functions = result["code_image"]["functions"].as_array().unwrap();
            assert_eq!(functions.len(), 2);
            assert_eq!(functions[0]["wasm_index"], 0);
            assert_eq!(functions[1]["wasm_index"], 1);
        }
        Ok(())
    }

    #[test]
    fn materialization_rejects_non_diagnostic_batches() {
        let prep = json!({"profile":"code","workload":{"abi":"core","reset":"stateless","export":"run","oracle":{"kind":"exact_u64"}}});
        let request =
            json!({"scenario":"compile-materialized","samples":1,"operations":1,"warmup":0});
        assert!(validate_materialized(&prep, &request).is_ok());
        for (key, value) in [
            ("samples", json!(2)),
            ("operations", json!(2)),
            ("warmup", json!(1)),
            ("phase_barriers", json!(true)),
        ] {
            let mut invalid = request.clone();
            invalid[key] = value;
            assert!(validate_materialized(&prep, &invalid).is_err());
        }
        let mut timing = prep.clone();
        timing["profile"] = json!("timing");
        assert!(validate_materialized(&timing, &request).is_err());
    }
}
