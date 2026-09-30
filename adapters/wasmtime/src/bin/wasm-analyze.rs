// Independent input analysis: no Wasmtime engine is constructed or consulted.
use anyhow::{Result, bail};
use serde_json::{Value, json};
use sha2::{Digest, Sha256};
use std::{collections::BTreeMap, fs};
use wasmparser::{
    CompositeInnerType, Encoding, Operator, Parser, Payload, TypeRef, Validator, WasmFeatures,
};
#[path = "../component_analysis.rs"]
mod component_analysis;
fn main() -> Result<()> {
    let mut args = std::env::args().skip(1);
    let path = args.next().ok_or_else(|| {
        anyhow::anyhow!("usage: wasm-analyze FILE.wasm [default|wasm1|wasm2|wasm3]")
    })?;
    let profile = args.next().unwrap_or_else(|| "default".into());
    if args.next().is_some() {
        bail!("unexpected analyzer argument")
    }
    let bytes = fs::read(path)?;
    println!(
        "{}",
        serde_json::to_string_pretty(&analyze(&bytes, &profile)?)?
    );
    Ok(())
}

fn feature_policy(profile: &str) -> Result<WasmFeatures> {
    Ok(match profile {
        "default" => WasmFeatures::default(),
        "wasm1" => WasmFeatures::WASM1,
        "wasm2" => WasmFeatures::WASM2,
        "wasm3" => WasmFeatures::WASM3,
        _ => bail!("unknown feature profile {profile}"),
    })
}

// Each result is conditional on the selected policy: all other enabled flags
// remain unchanged. Composite flags may remove multiple bits. This is not a
// claim of a unique minimal proposal set or an engine capability test.
fn feature_probes(bytes: &[u8], features: WasmFeatures) -> Value {
    let mut probes = Vec::new();
    for (name, flag) in features.iter_names() {
        let reduced = features.difference(flag);
        let error = Validator::new_with_features(reduced)
            .validate_all(bytes)
            .err();
        probes.push(json!({
            "feature": name,
            "disabled_bits": format!("{:x}", flag.bits()),
            "remaining_bits": format!("{:x}", reduced.bits()),
            "valid_without": error.is_none(),
            "failure": error.as_ref().map(ToString::to_string),
            "failure_offset": error.as_ref().map(|e| e.offset()),
        }));
    }
    json!({"method":"single-flag-removal-v1", "policy_bits":format!("{:x}",features.bits()), "interpretation":"necessity under the selected validator policy; composite flags can overlap; not a unique minimal proposal set or runtime support", "probes":probes})
}

fn analyze(bytes: &[u8], profile: &str) -> Result<Value> {
    let features = feature_policy(profile)?;
    Validator::new_with_features(features).validate_all(bytes)?;
    if matches!(
        Parser::new(0).parse_all(bytes).next().transpose()?,
        Some(Payload::Version {
            encoding: Encoding::Component,
            ..
        })
    ) {
        return component_analysis::analyze(bytes, profile, features);
    }
    let mut sections = BTreeMap::<u8, usize>::new();
    let mut histogram = BTreeMap::<String, u64>::new();
    let mut bodies = vec![];
    let (
        mut functions,
        mut imports,
        mut exports,
        mut globals,
        mut tables,
        mut memories,
        mut elements,
        mut data_segments,
    ) = (0, 0, 0, 0, 0, 0, 0, 0);
    let (mut data_bytes, mut custom_bytes, mut debug_bytes) = (0, 0, 0);
    let mut max_depth = 0;
    let mut branch_tables = vec![];
    let mut type_groups = 0;
    let mut types = vec![];
    let mut function_types = vec![];
    let mut import_details = vec![];
    let mut export_details = vec![];
    let mut imported_functions = 0u32;
    let mut imported_memories = 0u32;
    let mut imported_tables = 0u32;
    let mut imported_globals = 0u32;
    let mut imported_tags = 0u32;
    let mut start_function = None;
    for payload in Parser::new(0).parse_all(&bytes) {
        let payload = payload?;
        if let Some((id, range)) = payload.as_section() {
            *sections.entry(id).or_default() += range.len();
        }
        match payload {
            Payload::Version { encoding, .. } if encoding != Encoding::Module => bail!(
                "component structure analysis is not yet supported; refusing flattened module statistics"
            ),
            Payload::TypeSection(s) => {
                type_groups += s.count();
                for group in s {
                    for ty in group?.types() {
                        let index = types.len();
                        let (kind, params, results) = match &ty.composite_type.inner {
                            CompositeInnerType::Func(f) => (
                                "function",
                                f.params()
                                    .iter()
                                    .map(ToString::to_string)
                                    .collect::<Vec<_>>(),
                                f.results()
                                    .iter()
                                    .map(ToString::to_string)
                                    .collect::<Vec<_>>(),
                            ),
                            CompositeInnerType::Struct(_) => ("struct", vec![], vec![]),
                            CompositeInnerType::Array(_) => ("array", vec![], vec![]),
                            CompositeInnerType::Cont(_) => ("continuation", vec![], vec![]),
                        };
                        types.push(json!({"type_index":index,"kind":kind,"params":params,"results":results,"final":ty.is_final,"shared":ty.composite_type.shared}));
                    }
                }
            }
            Payload::ImportSection(s) => {
                imports += s.count();
                for import in s.into_imports() {
                    let import = import?;
                    let (kind, index, type_index) = match import.ty {
                        TypeRef::Func(t) | TypeRef::FuncExact(t) => {
                            let i = imported_functions;
                            imported_functions += 1;
                            ("function", i, Some(t))
                        }
                        TypeRef::Memory(_) => {
                            let i = imported_memories;
                            imported_memories += 1;
                            ("memory", i, None)
                        }
                        TypeRef::Table(_) => {
                            let i = imported_tables;
                            imported_tables += 1;
                            ("table", i, None)
                        }
                        TypeRef::Global(_) => {
                            let i = imported_globals;
                            imported_globals += 1;
                            ("global", i, None)
                        }
                        TypeRef::Tag(t) => {
                            let i = imported_tags;
                            imported_tags += 1;
                            ("tag", i, Some(t.func_type_idx))
                        }
                    };
                    import_details.push(json!({"module":import.module,"name":import.name,"kind":kind,"index":index,"type_index":type_index}));
                }
            }
            Payload::FunctionSection(s) => {
                for t in s {
                    function_types.push(t?);
                }
            }
            Payload::ExportSection(s) => {
                exports += s.count();
                for e in s {
                    let e = e?;
                    export_details.push(json!({"name":e.name,"kind":format!("{:?}",e.kind).to_lowercase(),"index":e.index}));
                }
            }
            Payload::StartSection { func, .. } => start_function = Some(func),
            Payload::GlobalSection(s) => globals += s.count(),
            Payload::TableSection(s) => tables += s.count(),
            Payload::MemorySection(s) => memories += s.count(),
            Payload::ElementSection(s) => elements += s.count(),
            Payload::DataSection(s) => {
                data_segments += s.count();
                for segment in s {
                    data_bytes += segment?.data.len();
                }
            }
            Payload::CustomSection(s) => {
                custom_bytes += s.data().len();
                if s.name().starts_with(".debug") || s.name() == "name" {
                    debug_bytes += s.data().len();
                }
            }
            Payload::CodeSectionEntry(body) => {
                functions += 1;
                let mut locals = 0u64;
                for local in body.get_locals_reader()? {
                    locals += local?.0 as u64;
                }
                let mut reader = body.get_operators_reader()?;
                let (mut count, mut depth, mut body_depth) = (0, 0usize, 0usize);
                while !reader.eof() {
                    let op = reader.read()?;
                    let label = format!("{op:?}")
                        .split([' ', '{'])
                        .next()
                        .unwrap()
                        .to_string();
                    *histogram.entry(label).or_default() += 1;
                    count += 1;
                    match op {
                        Operator::Block { .. }
                        | Operator::Loop { .. }
                        | Operator::If { .. }
                        | Operator::TryTable { .. } => {
                            depth += 1;
                            body_depth = body_depth.max(depth)
                        }
                        Operator::End => depth = depth.saturating_sub(1),
                        Operator::BrTable { targets } => branch_tables.push(targets.len()),
                        _ => {}
                    }
                }
                max_depth = max_depth.max(body_depth);
                let type_index = function_types[(functions - 1) as usize];
                let signature = &types[type_index as usize];
                bodies.push(json!({"defined_function_index":functions-1,"function_index":imported_functions+functions-1,"type_index":type_index,"params":signature["params"],"results":signature["results"],"body_bytes":body.range().len(),"locals":locals,"operators":count,"max_control_depth":body_depth}));
            }
            _ => {}
        }
    }
    let mut report = json!({"schema":2,"analyzer":"wasmparser","analyzer_version":"0.251.0","analysis_version":"core-structure-v3","encoding":"core-module","validation_profile":profile,"validation_features":features.iter_names().map(|(name,_)|name).collect::<Vec<_>>(),"feature_evidence":"validation policy, not inferred minimal requirements","sha256":hex::encode(Sha256::digest(bytes)),"bytes":bytes.len(),"validated":true,"section_payload_bytes":sections,"custom_data_bytes":custom_bytes,"debug_data_bytes":debug_bytes,"data_bytes":data_bytes,"defined_functions":functions,"imported_functions":imported_functions,"total_functions":imported_functions+functions,"type_groups":type_groups,"types":types,"import_groups":imports,"imports":import_details,"import_count":import_details.len(),"exports":exports,"export_details":export_details,"start_function":start_function,"defined_globals":globals,"imported_globals":imported_globals,"defined_tables":tables,"imported_tables":imported_tables,"defined_memories":memories,"imported_memories":imported_memories,"element_segments":elements,"data_segments":data_segments,"max_control_depth":max_depth,"branch_table_entries":branch_tables,"functions":bodies,"opcode_histogram":histogram});
    report["feature_probes"] = feature_probes(bytes, features);
    Ok(report)
}

#[cfg(test)]
mod tests {
    use super::*;
    const SIGNATURES: &[u8] =
        include_bytes!("../../../../corpus/testdata/analyzer-signatures.wasm");
    const LIFECYCLE: &[u8] = include_bytes!("../../../../corpus/testdata/app-init.wasm");

    #[test]
    fn indices_and_signatures_include_imports() {
        let report = analyze(SIGNATURES, "wasm2").unwrap();
        assert_eq!(report["schema"], 2);
        assert_eq!(report["import_count"], 4);
        assert_eq!(report["imported_functions"], 1);
        assert_eq!(report["total_functions"], 2);
        assert_eq!(report["imported_memories"], 1);
        assert_eq!(report["imported_tables"], 1);
        assert_eq!(report["imported_globals"], 1);
        assert_eq!(report["functions"][0]["function_index"], 1);
        assert_eq!(report["functions"][0]["defined_function_index"], 0);
        assert_eq!(report["functions"][0]["params"], json!(["i32", "i64"]));
        assert_eq!(report["functions"][0]["results"], json!(["f64", "i32"]));
        assert_eq!(report["functions"][0]["locals"], 1);
        assert_eq!(report["export_details"][0]["index"], 1);
        assert_eq!(report["imports"][0]["module"], "env");
        assert_eq!(report["imports"][0]["name"], "function");
        assert_eq!(report["validation_profile"], "wasm2");
        assert!(report["start_function"].is_null());
    }

    #[test]
    fn feature_policy_is_enforced_not_just_labeled() {
        assert!(analyze(SIGNATURES, "wasm1").is_err()); // Multi-value results.
        assert!(analyze(SIGNATURES, "wasm2").is_ok());
        assert!(analyze(SIGNATURES, "wasm3").is_ok());
        assert!(analyze(SIGNATURES, "default").is_ok());
        assert!(analyze(SIGNATURES, "typo").is_err());
        let report = analyze(LIFECYCLE, "wasm1").unwrap();
        assert!(report["start_function"].is_number());
        assert!(
            !report["validation_features"]
                .as_array()
                .unwrap()
                .contains(&json!("SIMD"))
        );
    }

    #[test]
    fn removal_probes_distinguish_policy_from_requirements() {
        let report = analyze(SIGNATURES, "wasm2").unwrap();
        assert_eq!(report["analysis_version"], "core-structure-v3");
        let probes = report["feature_probes"]["probes"].as_array().unwrap();
        let multi = probes
            .iter()
            .find(|p| p["feature"] == "MULTI_VALUE")
            .unwrap();
        assert_eq!(multi["valid_without"], false);
        assert!(multi["failure"].as_str().unwrap().contains("multi-value"));
        assert!(multi["failure_offset"].is_number());
        let simd = probes.iter().find(|p| p["feature"] == "SIMD").unwrap();
        assert_eq!(simd["valid_without"], true);
        assert!(simd["failure"].is_null());
        assert!(simd["failure_offset"].is_null());
        assert_eq!(
            probes.len(),
            report["validation_features"].as_array().unwrap().len()
        );
        // Composite flags retain explicit bit masks, not an invented proposal count.
        let bulk = probes
            .iter()
            .find(|p| p["feature"] == "BULK_MEMORY")
            .unwrap();
        let bits = u64::from_str_radix(bulk["disabled_bits"].as_str().unwrap(), 16).unwrap();
        assert!(bits.count_ones() > 1);
        let code = include_bytes!("../../../../corpus/testdata/analyzer-features.wasm");
        let report = analyze(code, "wasm2").unwrap();
        let probes = report["feature_probes"]["probes"].as_array().unwrap();
        for feature in ["SIMD", "BULK_MEMORY"] {
            let probe = probes.iter().find(|p| p["feature"] == feature).unwrap();
            assert_eq!(probe["valid_without"], false);
            assert!(probe["failure_offset"].is_number());
        }
    }

    #[test]
    fn invalid_and_component_inputs_are_not_flattened() {
        assert!(analyze(b"not wasm", "default").is_err());
        let report = analyze(&[0, 97, 115, 109, 13, 0, 1, 0], "default").unwrap();
        assert_eq!(report["encoding"], "component");
        assert_eq!(report["analysis_version"], "component-structure-v1");
        assert!(report.get("total_functions").is_none());
        assert!(analyze(&SIGNATURES[..SIGNATURES.len() - 1], "wasm2").is_err());
    }
}
