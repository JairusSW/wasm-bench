//! Component containers retain independent index spaces and byte ranges.
//! Validation is whole-artifact: nested components can refer to outer types.
use anyhow::{Result, anyhow, bail};
use serde_json::{Value, json};
use sha2::{Digest, Sha256};
use wasmparser::{Encoding, Parser, Payload, WasmFeatures};

pub fn analyze(bytes: &[u8], profile: &str, features: WasmFeatures) -> Result<Value> {
    let mut nodes: Vec<Value> = Vec::new();
    let mut stack: Vec<usize> = Vec::new();
    for payload in Parser::new(0).parse_all(bytes) {
        let payload = payload?;
        if let Payload::Version {
            encoding, range, ..
        } = &payload
        {
            let parent = stack.last().copied();
            let id = nodes.len();
            nodes.push(json!({
                "id":id,"parent":parent,"encoding":if *encoding==Encoding::Component {"component"} else {"core-module"},
                "byte_start":range.start,"byte_end":null,"bytes":null,"sha256":null,
                "sections":[],"imports":[],"exports":[],"core_functions":[],
                "canonical_functions":0,"component_types":0,"core_types":0,
                "component_instances":0,"core_instances":0,"aliases":0,
                "start":null
            }));
            stack.push(id);
            continue;
        }
        let id = *stack
            .last()
            .ok_or_else(|| anyhow!("payload outside a container"))?;
        if let Some((section, range)) = payload.as_section() {
            nodes[id]["sections"].as_array_mut().unwrap().push(json!({
                "id":section,"payload_start":range.start,"payload_end":range.end,"payload_bytes":range.len()
            }));
        }
        match payload {
            Payload::End(end) => {
                let start = nodes[id]["byte_start"].as_u64().unwrap() as usize;
                let raw = bytes
                    .get(start..end)
                    .ok_or_else(|| anyhow!("container range out of bounds"))?;
                nodes[id]["byte_end"] = json!(end);
                nodes[id]["bytes"] = json!(raw.len());
                nodes[id]["sha256"] = json!(hex::encode(Sha256::digest(raw)));
                stack.pop();
            }
            Payload::ComponentImportSection(reader) => {
                for import in reader {
                    let import = import?;
                    nodes[id]["imports"].as_array_mut().unwrap().push(json!({"name":import.name.name,"implements":import.name.implements,"type_reference":format!("{:?}",import.ty)}));
                }
            }
            Payload::ComponentExportSection(reader) => {
                for export in reader {
                    let export = export?;
                    nodes[id]["exports"].as_array_mut().unwrap().push(json!({"name":export.name.name,"implements":export.name.implements,"kind":export.kind.desc(),"index":export.index}));
                }
            }
            Payload::ImportSection(reader) => {
                for import in reader.into_imports() {
                    let import = import?;
                    nodes[id]["imports"].as_array_mut().unwrap().push(json!({"module":import.module,"name":import.name,"type_reference":format!("{:?}",import.ty)}));
                }
            }
            Payload::ExportSection(reader) => {
                for export in reader {
                    let export = export?;
                    nodes[id]["exports"].as_array_mut().unwrap().push(json!({"name":export.name,"kind":format!("{:?}",export.kind),"index":export.index}));
                }
            }
            Payload::ComponentCanonicalSection(s) => {
                add(&mut nodes[id], "canonical_functions", s.count())
            }
            Payload::ComponentTypeSection(s) => add(&mut nodes[id], "component_types", s.count()),
            Payload::CoreTypeSection(s) => add(&mut nodes[id], "core_types", s.count()),
            Payload::ComponentInstanceSection(s) => {
                add(&mut nodes[id], "component_instances", s.count())
            }
            Payload::InstanceSection(s) => add(&mut nodes[id], "core_instances", s.count()),
            Payload::ComponentAliasSection(s) => add(&mut nodes[id], "aliases", s.count()),
            Payload::StartSection { func, .. } => {
                nodes[id]["start"] = json!({"core_function_index":func})
            }
            Payload::ComponentStartSection { start, .. } => {
                nodes[id]["start"] = json!({"function_index":start.func_index,"arguments":start.arguments,"results":start.results})
            }
            Payload::CodeSectionEntry(body) => {
                let mut locals = 0u64;
                for local in body.get_locals_reader()? {
                    locals += u64::from(local?.0);
                }
                let mut operators = 0u64;
                let mut reader = body.get_operators_reader()?;
                while !reader.eof() {
                    reader.read()?;
                    operators += 1;
                }
                let functions = nodes[id]["core_functions"].as_array_mut().unwrap();
                functions.push(json!({"defined_function_index":functions.len(),"body_start":body.range().start,"body_end":body.range().end,"body_bytes":body.range().len(),"locals":locals,"operators":operators}));
            }
            _ => {}
        }
    }
    if !stack.is_empty() || nodes.first().is_none_or(|n| n["encoding"] != "component") {
        bail!("incomplete component tree")
    }
    Ok(
        json!({"schema":2,"analyzer":"wasmparser","analyzer_version":"0.251.0","analysis_version":"component-structure-v1","encoding":"component","validation_profile":profile,"validation_features":features.iter_names().map(|(name,_)|name).collect::<Vec<_>>(),"validated":true,"sha256":hex::encode(Sha256::digest(bytes)),"bytes":bytes.len(),"nodes":nodes,"feature_probes":super::feature_probes(bytes,features),"interpretation":"Whole-artifact validation; each node has its own index spaces. Parent container and section sizes include nested bytes: do not sum node sizes. Ranges are absolute byte offsets, end-exclusive. Type references use the pinned wasmparser representation. This is structure, not ABI compatibility or runtime support."}),
    )
}

fn add(node: &mut Value, field: &str, count: u32) {
    node[field] = json!(node[field].as_u64().unwrap() + u64::from(count));
}

#[cfg(test)]
mod tests {
    use super::*;
    const COMPONENT: &[u8] = &[0, 97, 115, 109, 13, 0, 1, 0];
    const MODULE: &[u8] = &[0, 97, 115, 109, 1, 0, 0, 0];
    fn section(target: &mut Vec<u8>, id: u8, bytes: &[u8]) {
        target.push(id);
        let mut len = bytes.len();
        loop {
            let mut byte = (len & 127) as u8;
            len >>= 7;
            if len != 0 {
                byte |= 128
            }
            target.push(byte);
            if len == 0 {
                break;
            }
        }
        target.extend(bytes);
    }
    #[test]
    fn nonempty_modules_keep_imports_exports_and_function_bodies_local() {
        let module = include_bytes!("../../../corpus/testdata/analyzer-signatures.wasm");
        let mut root = COMPONENT.to_vec();
        section(&mut root, 1, module);
        section(&mut root, 1, module);
        let report = super::super::analyze(&root, "default").unwrap();
        let nodes = report["nodes"].as_array().unwrap();
        assert_eq!(nodes.len(), 3);
        assert!(nodes[0]["core_functions"].as_array().unwrap().is_empty());
        for node in &nodes[1..] {
            assert_eq!(node["imports"].as_array().unwrap().len(), 4);
            assert!(!node["exports"].as_array().unwrap().is_empty());
            let functions = node["core_functions"].as_array().unwrap();
            assert!(!functions.is_empty());
            assert_eq!(functions[0]["defined_function_index"], 0);
            for function in functions {
                assert!(function["operators"].as_u64().unwrap() > 0);
                assert!(
                    function["body_start"].as_u64().unwrap()
                        >= node["byte_start"].as_u64().unwrap()
                );
                assert!(
                    function["body_end"].as_u64().unwrap() <= node["byte_end"].as_u64().unwrap()
                );
            }
        }
        assert_eq!(nodes[1]["sha256"], nodes[2]["sha256"]);
        assert_ne!(nodes[1]["byte_start"], nodes[2]["byte_start"]);
    }
    #[test]
    fn nested_containers_keep_ranges_and_identity() {
        let mut nested = COMPONENT.to_vec();
        section(&mut nested, 1, MODULE);
        let mut root = COMPONENT.to_vec();
        section(&mut root, 1, MODULE);
        section(&mut root, 4, &nested);
        let report = super::super::analyze(&root, "default").unwrap();
        let nodes = report["nodes"].as_array().unwrap();
        assert_eq!(nodes.len(), 4);
        assert_eq!(nodes[0]["parent"], Value::Null);
        assert_eq!(nodes[1]["parent"], 0);
        assert_eq!(nodes[2]["parent"], 0);
        assert_eq!(nodes[3]["parent"], 2);
        assert_eq!(nodes[1]["encoding"], "core-module");
        assert_eq!(nodes[2]["encoding"], "component");
        for node in nodes {
            let start = node["byte_start"].as_u64().unwrap() as usize;
            let end = node["byte_end"].as_u64().unwrap() as usize;
            assert_eq!(
                node["sha256"],
                hex::encode(Sha256::digest(&root[start..end]))
            );
            assert_eq!(node["bytes"], end - start);
        }
        assert!(report.get("total_functions").is_none());
        assert!(super::super::analyze(&root, "wasm1").is_err());
        assert!(super::super::analyze(&root[..root.len() - 1], "default").is_err());
    }
}
