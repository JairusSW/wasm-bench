// Dedicated fixed-fixture adapter. No guest imports or generic snapshots.
use super::*;
use serde_json::{Value, json};
use std::io::BufRead;

const MODE: &str = "linux-process-cow-clone-v1";
const SHA: &str = "7a828c361cd0790044ac23e97c5c93838605c8f202088cabe990c4c1eb6d3381";
const STAGES: [&str; 5] = [
    "process-snapshot-capture",
    "process-snapshot-restore",
    "process-snapshot-first-write",
    "process-snapshot-execute",
    "process-snapshot-density",
];

fn prepare(p: &Value) -> Result<()> {
    if !p["workload"]["snapshot_density"].is_null() {
        let w = &p["workload"];
        let instances = w["snapshot_density"]["instances"].as_u64().unwrap_or(0);
        ensure!(p["profile"] == "memory" && (1..=32).contains(&instances));
        ensure!(w["process_snapshot"].is_null());
        ensure!(w["snapshot_density"]["mode"] == "simultaneous_linux_process_cow");
        ensure!(w["dimension"] == "instances" && w["size"] == instances);
        ensure!(w["generator"] == "wasmbench-process-snapshot-density-v1");
        ensure!(
            w["work_unit"] == "restored_process_group"
                && w["reset"] == "fresh_process_snapshot_group_per_sample"
        );
        ensure!(w["oracle"]["kind"] == "linux_process_snapshot_density_v1");
        let mut base = p.clone();
        base["workload"]["snapshot_density"] = Value::Null;
        base["workload"]["process_snapshot"] = json!({"mode":MODE});
        base["workload"]["dimension"] = Value::Null;
        base["workload"]["size"] = Value::Null;
        base["workload"]["generator"] = json!("wasmbench-process-snapshot-v1");
        base["workload"]["work_unit"] = json!("process_snapshot_stage");
        base["workload"]["reset"] = json!("fresh_process_snapshot_per_sample");
        base["workload"]["oracle"]["kind"] = json!("linux_process_snapshot_v1");
        return prepare(&base);
    }
    let w = &p["workload"];
    ensure!(
        p["profile"] == "timing" || p["profile"] == "memory",
        "unsupported snapshot profile"
    );
    ensure!(p["artifact_sha256"] == SHA && w["sha256"] == SHA);
    ensure!(w["process_snapshot"]["mode"] == MODE && w["schema"] == 1);
    ensure!(w["abi"] == "core" && w["generator"] == "wasmbench-process-snapshot-v1");
    ensure!(w["export"] == "check" && w["initialize"] == "seed");
    ensure!(w["reset"] == "fresh_process_snapshot_per_sample");
    ensure!(w["work_unit"] == "process_snapshot_stage" && w["units_per_invocation"] == 1);
    ensure!(
        w["oracle"]["kind"] == "linux_process_snapshot_v1"
            && w["oracle"]["expected"] == json!(["64"])
    );
    for field in [
        "input",
        "command",
        "vectors",
        "density",
        "checkpoint",
        "guest_density",
        "continuation",
        "host_profile",
        "dimension",
        "snapshot_density",
    ] {
        ensure!(w[field].is_null(), "foreign workload field: {field}");
    }
    ensure!(w["args"].as_array().is_none_or(|a| a.is_empty()));
    for field in ["float", "memory", "expected_trap", "output_pointer_export"] {
        ensure!(
            w["oracle"][field].is_null(),
            "foreign oracle field: {field}"
        );
    }
    let path = p["artifact"]
        .as_str()
        .ok_or_else(|| anyhow::anyhow!("missing artifact"))?;
    let bytes = std::fs::read(path)?;
    ensure!(
        bytes == wat::parse_str(FIXTURE)?,
        "noncanonical snapshot artifact"
    );
    single_threaded()?;
    Ok(())
}

fn request(r: &Value) -> Result<(&str, usize)> {
    let stage = r["scenario"]
        .as_str()
        .ok_or_else(|| anyhow::anyhow!("missing stage"))?;
    ensure!(STAGES.contains(&stage), "unsupported stage");
    let samples = r["samples"]
        .as_u64()
        .ok_or_else(|| anyhow::anyhow!("missing sample count"))?;
    ensure!((1..=1000).contains(&samples));
    ensure!(r["operations"] == 1 && r["warmup"] == 0);
    ensure!(r["phase_barriers"].is_null() || r["phase_barriers"] == false);
    ensure!(r["sustained_post_collection"].is_null() || r["sustained_post_collection"] == false);
    ensure!(r["sustained_duration_ns"].is_null() || r["sustained_duration_ns"] == 0);
    Ok((stage, samples as usize))
}

pub(super) fn serve(backend: &str) -> Result<()> {
    ensure!(
        matches!(backend, "cranelift" | "winch"),
        "unsupported backend"
    );
    ensure!(matches!(std::env::consts::ARCH, "aarch64" | "x86_64"));
    single_threaded()?;
    let stdin = std::io::stdin();
    let mut input = stdin.lock();
    let stdout = std::io::stdout();
    let mut output = stdout.lock();
    let mut prepared = false;
    let mut profile = String::new();
    let mut density_instances = None;
    loop {
        let mut line = String::new();
        if input.read_line(&mut line)? == 0 {
            break;
        }
        let req: Value = serde_json::from_str(&line)?;
        let id = req["id"].clone();
        let method = req["method"].as_str().unwrap_or("");
        let result = (|| -> Result<Value> {
            ensure!(
                req["version"] == 1 && id.as_u64().is_some(),
                "invalid protocol envelope"
            );
            match method {
                "describe" => Ok(json!({"description":{
                    "runtime":"wasmtime","runtime_version":"46.0.1","backend":backend,
                    "embedding":"Rust API","build":"Cargo.lock / release / process-snapshot",
                    "effective_configuration":{"cache":"disabled","parallel_compilation":"false",
                        "process_snapshot_protocol":MODE,"process_snapshot_os":"linux",
                        "process_snapshot_spawn":"single_threaded_owned_process","strategy":backend},
                    "capabilities":{"can_linux_process_snapshot":true,"can_inspect_linux_snapshot_lineage":true,"can_inspect_linux_snapshot_density":true,"can_snapshot":false,"can_disable_code_cache":true},
                    "scenarios":STAGES,"abis":["core"],"features":["bulk-memory","reference-types"]}})),
                "prepare" => {
                    prepared = false;
                    profile.clear();
                    density_instances = None;
                    prepare(&req["prepare"])?;
                    density_instances = req["prepare"]["workload"]["snapshot_density"]["instances"]
                        .as_u64()
                        .map(|n| n as usize);
                    profile = req["prepare"]["profile"].as_str().unwrap().to_owned();
                    prepared = true;
                    Ok(json!({}))
                }
                "run" => {
                    ensure!(prepared, "prepare canonical workload first");
                    ensure!(
                        profile == "timing",
                        "memory inspection requires inspect and explicit barriers"
                    );
                    let (stage, samples) = request(&req["run"])?;
                    ensure!(density_instances.is_none() && stage != "process-snapshot-density");
                    prepared = false;
                    let record = timing::collect(backend, stage, samples)?;
                    prepared = true;
                    Ok(json!({"samples":record["samples"]}))
                }
                "inspect" => {
                    ensure!(
                        prepared && profile == "memory",
                        "prepare memory inspection first"
                    );
                    ensure!(
                        req["run"]["phase_barriers"] == true,
                        "inspection requires explicit barriers"
                    );
                    let mut r = req["run"].clone();
                    r["phase_barriers"] = json!(false);
                    let (stage, count) = request(&r)?;
                    let is_density = stage == "process-snapshot-density";
                    ensure!(
                        is_density == density_instances.is_some(),
                        "scenario differs from prepared contract"
                    );
                    if is_density {
                        ensure!(count <= 32, "too many density groups");
                    }
                    prepared = false;
                    let mut observe = |event: Value| -> Result<()> {
                        writeln!(
                            output,
                            "{}",
                            if is_density {
                                json!({"version":1,"id":id,"status":"snapshot_density_boundary","snapshot_density_boundary":event})
                            } else {
                                json!({"version":1,"id":id,"status":"snapshot_boundary","snapshot_boundary":event})
                            }
                        )?;
                        output.flush()?;
                        let mut line = String::new();
                        ensure!(
                            input.read_line(&mut line)? > 0,
                            "controller closed during inspection"
                        );
                        let release: Value = serde_json::from_str(&line)?;
                        ensure!(
                            release["version"] == 1
                                && release["id"] == id
                                && release["method"] == "continue",
                            "invalid inspection release"
                        );
                        Ok(())
                    };
                    if is_density {
                        let mut proofs = Vec::with_capacity(count);
                        for index in 0..count {
                            let mut indexed = |mut event: Value| -> Result<()> {
                                event["sample_index"] = json!(index);
                                observe(event)
                            };
                            proofs.push(density::collect(
                                backend,
                                density_instances.unwrap(),
                                &mut indexed,
                            )?);
                        }
                        prepared = true;
                        return Ok(
                            json!({"snapshot_density_diagnostics":{"version":"linux-process-snapshot-density-inspection-v1","profile":"memory","latency_eligible":false,"samples":proofs}}),
                        );
                    }
                    let record =
                        timing::collect_observed(backend, stage, count, Some(&mut observe))?;
                    prepared = true;
                    Ok(
                        json!({"snapshot_diagnostics":{"version":"linux-process-snapshot-live-inspection-v1",
                        "profile":"memory","latency_eligible":false,"samples":record["samples"]}}),
                    )
                }
                "close" => {
                    prepared = false;
                    Ok(json!({}))
                }
                _ => bail!("unsupported method"),
            }
        })();
        let mut response = match result {
            Ok(value) => value,
            Err(error) => json!({"status":"error","reason":format!("{error:#}")}),
        };
        response["version"] = json!(1);
        response["id"] = id;
        if response["status"].is_null() {
            response["status"] = json!("ok");
        }
        writeln!(output, "{response}")?;
        output.flush()?;
        if method == "close" {
            break;
        }
    }
    Ok(())
}
