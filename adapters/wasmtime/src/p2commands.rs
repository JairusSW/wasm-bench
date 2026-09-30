use crate::{Adapter, commands};
use serde_json::{Value, json};
use std::sync::atomic::AtomicU64;
use wasmtime::{
    Store,
    component::{Component, Linker, ResourceTable},
    format_err as anyhow,
};
use wasmtime_wasi::{
    DirPerms, FilePerms, WasiCtx, WasiCtxBuilder, WasiCtxView, WasiView,
    cli::InputFile,
    p2::{self, bindings::sync::Command, pipe::MemoryInputPipe},
};

struct CommandCtx {
    table: ResourceTable,
    wasi: WasiCtx,
}

impl WasiView for CommandCtx {
    fn ctx(&mut self) -> WasiCtxView<'_> {
        WasiCtxView {
            ctx: &mut self.wasi,
            table: &mut self.table,
        }
    }
}

pub(super) fn run(
    adapter: &Adapter,
    request: &Value,
    barrier: &mut dyn FnMut(u64, &str) -> wasmtime::Result<()>,
) -> wasmtime::Result<Value> {
    let workload = adapter.workload()?;
    commands::validate(workload)?;
    let command = &workload["command"];
    let scenario = commands::string(&request["scenario"]);
    let profile = commands::string(&adapter.prep.as_ref().unwrap()["profile"]);
    let phased = request["phase_barriers"].as_bool().unwrap_or(false);
    if !commands::string(&command["stdout_normalize"]).is_empty() {
        return Ok(json!({
            "status": "unsupported",
            "reason": "stdout normalization is not supported for this Component Model command adapter"
        }));
    }
    if workload["abi"] != "component"
        || ![
            "wasi-preview2-readonly-v1",
            "wasi-preview2-temporary-filesystem-v1",
        ]
        .contains(&workload["host_profile"].as_str().unwrap_or(""))
        || !["compile", "instantiate", "first-call"].contains(&scenario)
        || !["timing", "memory"].contains(&profile)
        || (phased && profile != "memory")
        || request["operations"] != 1
    {
        return Ok(
            json!({"status":"unsupported","reason":"WASI Preview 2 command requires compile/instantiate/first-call timing or memory, one operation; barriers require memory"}),
        );
    }
    let samples = request["samples"]
        .as_u64()
        .ok_or_else(|| anyhow!("invalid sample count"))?;
    if samples == 0 || samples > 100_000 {
        return Err(anyhow!("invalid sample count"));
    }
    let engine = adapter.engine()?;
    let shared_component = if scenario == "compile" {
        None
    } else {
        Some(Component::new(&engine, &adapter.bytes)?)
    };
    let mut linker = Linker::<CommandCtx>::new(&engine);
    p2::add_to_linker_sync(&mut linker)?;
    let argv: Vec<&str> = command["argv"]
        .as_array()
        .ok_or_else(|| anyhow!("missing argv"))?
        .iter()
        .map(commands::string)
        .collect();
    let mut output = Vec::with_capacity(samples as usize);
    for index in 0..samples {
        if phased && scenario == "compile" {
            barrier(index, "before_compile")?;
        }
        let mut elapsed = 0;
        let component = if let Some(component) = &shared_component {
            component.clone()
        } else {
            let start = std::time::Instant::now();
            let compiled = Component::new(&engine, &adapter.bytes)?;
            elapsed = u64::try_from(start.elapsed().as_nanos()).unwrap_or(u64::MAX);
            compiled
        };
        if phased && scenario == "compile" {
            barrier(index, "compiled")?;
        }
        // Host filesystem state is part of fresh_instance_per_sample too.
        // Recopy locked fixtures before instantiation and outside the call
        // timer; writable guest mutations must never reach later samples.
        let staging = commands::stage(command)?;
        let stdout =
            commands::Capture::new(command["output_limit_bytes"].as_u64().unwrap_or(0) as usize);
        let stderr =
            commands::Capture::new(command["output_limit_bytes"].as_u64().unwrap_or(0) as usize);
        let mut builder = WasiCtxBuilder::new();
        builder
            .args(&argv)
            .stdout(stdout.clone())
            .stderr(stderr.clone())
            .secure_random(wasmtime_wasi::random::Deterministic::new(vec![0]))
            .wall_clock(commands::Clock(AtomicU64::new(1_640_995_200_000_000_000)))
            .monotonic_clock(commands::Clock(AtomicU64::new(0)));
        if commands::string(&command["stdin_file"]).is_empty() {
            builder.stdin(MemoryInputPipe::new(commands::data(&command["stdin"])?));
        } else {
            builder.stdin(InputFile::new(std::fs::File::open(
                staging
                    .path()
                    .join(commands::string(&command["stdin_file"])),
            )?));
        }
        let writable = workload["host_profile"] == "wasi-preview2-temporary-filesystem-v1";
        if writable
            || command["files"]
                .as_object()
                .is_some_and(|files| !files.is_empty())
        {
            let dir_perms = if writable {
                DirPerms::all()
            } else {
                DirPerms::READ
            };
            let file_perms = if writable {
                FilePerms::all()
            } else {
                FilePerms::READ
            };
            builder.preopened_dir(staging.path(), "/", dir_perms, file_perms)?;
        }
        let ctx = CommandCtx {
            table: ResourceTable::new(),
            wasi: builder.build(),
        };
        let mut store = Store::new(&engine, ctx);
        if phased && scenario == "instantiate" {
            barrier(index, "before_instantiate")?;
        }
        let start = std::time::Instant::now();
        let command_instance = Command::instantiate(&mut store, &component, &linker)?;
        if scenario == "instantiate" {
            elapsed = u64::try_from(start.elapsed().as_nanos()).unwrap_or(u64::MAX);
        }
        if phased && scenario == "instantiate" {
            barrier(index, "instantiated")?;
        }
        if phased && scenario == "first-call" {
            barrier(index, "before_first_call")?;
        }
        let start = std::time::Instant::now();
        let exit_code = match command_instance.wasi_cli_run().call_run(&mut store)? {
            Ok(()) => 0,
            Err(()) => 1,
        };
        if scenario == "first-call" {
            elapsed = u64::try_from(start.elapsed().as_nanos()).unwrap_or(u64::MAX);
        }
        if phased && scenario == "first-call" {
            barrier(index, "first_call_returned")?;
        }
        let (stdout_hash, stdout_bytes) = stdout.evidence()?;
        let (stderr_hash, stderr_bytes) = stderr.evidence()?;
        if exit_code as u64 != command["exit_code"].as_u64().unwrap_or(u64::MAX)
            || (!commands::string(&command["stdout_sha256"]).is_empty()
                && stdout_hash != command["stdout_sha256"])
            || (!commands::string(&command["stderr_sha256"]).is_empty()
                && stderr_hash != command["stderr_sha256"])
        {
            return Err(anyhow!(
                "incorrect result: Preview 2 command exit or output mismatch"
            ));
        }
        drop(command_instance);
        drop(store);
        drop(staging);
        drop(component);
        if phased {
            barrier(
                index,
                match scenario {
                    "compile" => "released",
                    "instantiate" => "instance_released",
                    "first-call" => "first_call_released",
                    _ => unreachable!(),
                },
            )?;
        }
        let result = json!({"exit_code":exit_code,"stdout_sha256":stdout_hash,"stderr_sha256":stderr_hash,"stdout_bytes":stdout_bytes,"stderr_bytes":stderr_bytes});
        output.push(json!({"index":index,"warmup":false,"elapsed_ns":elapsed,"operations":1,"sample_type":"individual_operation","verified":true,"command_result":result}));
    }
    drop(shared_component);
    drop(linker);
    drop(engine);
    Ok(json!({"samples":output}))
}
