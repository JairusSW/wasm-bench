// Private native timing worker. It is not yet a registered product adapter.
// Reuses the qualifier's process ownership and parent-death cleanup machinery.
use super::*;
use serde_json::{Value, json};

const MODE: &str = "linux-process-cow-clone-v1";
const ARTIFACT: &str = "7a828c361cd0790044ac23e97c5c93838605c8f202088cabe990c4c1eb6d3381";

fn boundary(stage: &str) -> Result<&'static str> {
    Ok(match stage {
        "process-snapshot-capture" => "source_fork_to_template_ready_ack",
        "process-snapshot-restore" => "restore_request_to_child_ready_ack",
        "process-snapshot-first-write" => "restored_child_single_byte_memory_write",
        "process-snapshot-execute" => "restored_child_typed_check_call",
        _ => bail!("unknown process snapshot stage"),
    })
}

pub(super) fn identity() -> Result<Value> {
    let stat = std::fs::read_to_string("/proc/self/stat")?;
    // comm can contain spaces and parentheses. Fields after its final closing
    // parenthesis begin at field 3; starttime is field 22.
    let (_, fields) = stat
        .rsplit_once(") ")
        .ok_or_else(|| anyhow::anyhow!("invalid proc stat"))?;
    let ticks: u64 = fields
        .split_whitespace()
        .nth(19)
        .ok_or_else(|| anyhow::anyhow!("missing process birth"))?
        .parse()?;
    ensure!(ticks > 0);
    Ok(json!({"pid":unsafe { getpid() },"start_time_ticks":ticks.to_string()}))
}

pub(super) fn elapsed(start: Instant) -> Result<i64> {
    Ok(start.elapsed().as_nanos().try_into()?)
}

pub(super) fn send(stream: &mut UnixStream, value: &Value) -> Result<()> {
    let data = serde_json::to_vec(value)?;
    ensure!(data.len() <= 65536, "oversized worker proof");
    stream.write_all(&(data.len() as u32).to_le_bytes())?;
    stream.write_all(&data)?;
    Ok(())
}

pub(super) fn receive(stream: &mut UnixStream) -> Result<Value> {
    let mut size = [0; 4];
    stream.read_exact(&mut size)?;
    let size = u32::from_le_bytes(size) as usize;
    ensure!(size > 0 && size <= 65536, "invalid worker proof size");
    let mut data = vec![0; size];
    stream.read_exact(&mut data)?;
    Ok(serde_json::from_slice(&data)?)
}

fn ready(stream: &mut UnixStream) -> Result<()> {
    let mut byte = [0];
    stream.read_exact(&mut byte)?;
    ensure!(byte == *b"s", "invalid ready acknowledgment");
    Ok(())
}

fn released(stream: &mut UnixStream) -> Result<()> {
    let mut byte = [0];
    stream.read_exact(&mut byte)?;
    ensure!(byte == *b"g", "invalid post-timer release");
    Ok(())
}

fn memory_hash(state: &mut State) -> Result<String> {
    let memory = state
        .instance
        .get_memory(&mut state.store, "memory")
        .ok_or_else(|| anyhow::anyhow!("missing memory"))?;
    ensure!(memory.data_size(&state.store) == 3 * 65536);
    Ok(hex::encode(Sha256::digest(memory.data(&state.store))))
}

fn expected_hash(written: bool) -> String {
    let mut data = vec![0; 3 * 65536];
    data[0] = 7;
    if written {
        data[65535] = 22;
    }
    hex::encode(Sha256::digest(data))
}

fn restored(state: &mut State, mut output: UnixStream, stage: &str, diagnostic: bool) -> ! {
    let result = (|| -> Result<()> {
        let process = identity()?;
        let check = state
            .instance
            .get_typed_func::<(), i32>(&mut state.store, "check")
            .map_err(runtime_error)?;
        let memory = state
            .instance
            .get_memory(&mut state.store, "memory")
            .ok_or_else(|| anyhow::anyhow!("missing memory"))?;
        // Ready must precede all post-restore timed work and proof generation.
        output.write_all(b"s")?;
        if diagnostic {
            send(&mut output, &process)?;
        }
        // The source explicitly releases proof work after its ready-ack timer
        // stops; otherwise child hashing could overlap that parent timer.
        released(&mut output)?;
        let before = check.call(&mut state.store, ()).map_err(runtime_error)?;
        let before_hash = memory_hash(state)?;
        ensure!(before == 64 && before_hash == expected_hash(false));
        let start = Instant::now();
        memory.write(&mut state.store, 65535, &[22])?;
        let write_ns = elapsed(start)?;
        if diagnostic {
            output.write_all(b"w")?;
            released(&mut output)?;
        }
        let after_hash = memory_hash(state)?;
        let start = Instant::now();
        let after = check.call(&mut state.store, ()).map_err(runtime_error)?;
        let execute_ns = elapsed(start)?;
        if diagnostic {
            output.write_all(b"e")?;
            released(&mut output)?;
        }
        let pages = state.call("pages")?;
        let elements = state.call("elements")?;
        let probe = state.call("probe")?;
        ensure!(after == 64 && after_hash == expected_hash(true));
        ensure!((pages, elements, probe) == (3, 3, 127));
        let ns = if stage == "process-snapshot-execute" {
            execute_ns
        } else {
            write_ns
        };
        send(
            &mut output,
            &json!({"process":process,"elapsed_ns":ns,
            "restored_before_write":before,"restored_after_write":after,
            "passive_segment_probe":probe,"memory_pages":pages,"table_elements":elements,
            "memory_at_restore_sha256":before_hash,"memory_after_first_write_sha256":after_hash}),
        )?;
        Ok(())
    })();
    unsafe { _exit(if result.is_ok() { 0 } else { 110 }) }
}

fn template(mut state: State, mut control: UnixStream, stage: &str, diagnostic: bool) -> ! {
    let result = (|| -> Result<()> {
        let process = identity()?;
        control.write_all(b"s")?;
        released(&mut control)?;
        send(&mut control, &process)?;
        loop {
            let mut command = [0];
            control.read_exact(&mut command)?;
            if command == *b"q" {
                break;
            }
            ensure!(command == *b"r");
            single_threaded()?;
            let (mut input, output) = UnixStream::pair()?;
            input.set_read_timeout(Some(Duration::from_secs(2)))?;
            let parent_pid = unsafe { getpid() };
            // SAFETY: the dedicated executable owns this import-free guest and
            // procfs confirms a single-threaded embedding before each fork.
            let pid = unsafe { fork() };
            ensure!(pid >= 0, "restore fork failed");
            if pid == 0 {
                if bind_parent_lifetime(parent_pid).is_err() {
                    unsafe { _exit(102) }
                }
                drop(control);
                drop(input);
                restored(&mut state, output, stage, diagnostic);
            }
            drop(output);
            let mut child = OwnedChild { pid };
            ready(&mut input)?;
            // Forward readiness before reading/serializing the child proof.
            control.write_all(b"s")?;
            if diagnostic {
                let process = receive(&mut input)?;
                send(&mut control, &process)?;
            }
            released(&mut control)?;
            input.write_all(b"g")?;
            if diagnostic {
                for expected in [b'w', b'e'] {
                    let mut message = [0];
                    input.read_exact(&mut message)?;
                    ensure!(
                        message[0] == expected,
                        "unexpected diagnostic child barrier"
                    );
                    control.write_all(&message)?;
                    released(&mut control)?;
                    input.write_all(b"g")?;
                }
            }
            let proof = receive(&mut input)?;
            ensure!(proof["process"]["pid"].as_i64() == Some(i64::from(pid)));
            ensure!(
                child.wait(Duration::from_secs(2))? == 0,
                "restored child failed"
            );
            send(&mut control, &proof)?;
        }
        Ok(())
    })();
    unsafe { _exit(if result.is_ok() { 0 } else { 111 }) }
}

pub(super) fn collect(backend: &str, stage: &str, count: usize) -> Result<Value> {
    collect_observed(backend, stage, count, None)
}

pub(super) fn collect_observed(
    backend: &str,
    stage: &str,
    count: usize,
    mut observer: Option<&mut dyn FnMut(Value) -> Result<()>>,
) -> Result<Value> {
    let diagnostic = observer.is_some();
    let measurement_boundary = boundary(stage)?;
    ensure!((1..=1000).contains(&count), "samples must be 1..1000");
    let strategy = match backend {
        "cranelift" => Strategy::Cranelift,
        "winch" => Strategy::Winch,
        _ => bail!("unknown backend"),
    };
    ensure!(matches!(std::env::consts::ARCH, "aarch64" | "x86_64"));
    single_threaded()?;
    let source = identity()?;
    let wasm = wat::parse_str(FIXTURE)?;
    ensure!(hex::encode(Sha256::digest(&wasm)) == ARTIFACT);
    let mut samples = Vec::with_capacity(count);
    for index in 0..count {
        let mut config = Config::new();
        config.strategy(strategy).parallel_compilation(false);
        let engine = Engine::new(&config).map_err(runtime_error)?;
        let module = Module::new(&engine, &wasm).map_err(runtime_error)?;
        let mut store = Store::new(&engine, ());
        let instance = Instance::new(&mut store, &module, &[]).map_err(runtime_error)?;
        let mut state = State { store, instance };
        state.action("seed")?;
        ensure!(state.call("check")? == 64);
        let (parent, child) = UnixStream::pair()?;
        parent.set_read_timeout(Some(Duration::from_secs(5)))?;
        parent.set_write_timeout(Some(Duration::from_secs(5)))?;
        single_threaded()?;
        let parent_pid = unsafe { getpid() };
        let start = Instant::now();
        let pid = unsafe { fork() };
        ensure!(pid >= 0, "capture fork failed");
        if pid == 0 {
            if bind_parent_lifetime(parent_pid).is_err() {
                unsafe { _exit(102) }
            }
            drop(parent);
            template(state, child, stage, diagnostic);
        }
        drop(child);
        let mut captured = Template {
            child: OwnedChild { pid },
            control: parent,
        };
        ready(&mut captured.control)?;
        let capture_ns = elapsed(start)?;
        captured.control.write_all(b"g")?;
        let template_process = receive(&mut captured.control)?;
        ensure!(template_process["pid"].as_i64() == Some(i64::from(pid)));
        state.action("mutate")?;
        ensure!(state.call("check")? == 125);
        ensure!(state.call("probe").is_err());
        drop(state);
        drop(module);
        drop(engine);
        if let Some(observe) = observer.as_mut() {
            observe(
                json!({"version":"linux-process-snapshot-live-boundary-v1","sample_index":index,
                "stage":"template_after_source_release","restoration":-1,
                "source":source,"template":template_process,"restored":null}),
            )?;
        }
        let mut restorations = Vec::with_capacity(2);
        let mut restore_ns = 0;
        for restoration in 0..2 {
            let start = Instant::now();
            captured.control.write_all(b"r")?;
            ready(&mut captured.control)?;
            let ns = elapsed(start)?;
            let live_process = if diagnostic {
                receive(&mut captured.control)?
            } else {
                Value::Null
            };
            if let Some(observe) = observer.as_mut() {
                observe(
                    json!({"version":"linux-process-snapshot-live-boundary-v1","sample_index":index,
                    "stage":"restore_ready","restoration":restoration,
                    "source":source,"template":template_process,"restored":live_process}),
                )?;
            }
            captured.control.write_all(b"g")?;
            if restoration == 0 {
                restore_ns = ns;
            }
            if let Some(observe) = observer.as_mut() {
                for (expected, name) in [(b'w', "first_write_done"), (b'e', "execution_done")] {
                    let mut message = [0];
                    captured.control.read_exact(&mut message)?;
                    ensure!(
                        message[0] == expected,
                        "unexpected diagnostic template barrier"
                    );
                    observe(
                        json!({"version":"linux-process-snapshot-live-boundary-v1","sample_index":index,
                        "stage":name,"restoration":restoration,
                        "source":source,"template":template_process,"restored":live_process}),
                    )?;
                    captured.control.write_all(b"g")?;
                }
            }
            let proof = receive(&mut captured.control)?;
            if diagnostic {
                ensure!(
                    proof["process"] == live_process,
                    "live child differs from completed proof"
                );
            }
            ensure!(proof["restored_before_write"] == 64 && proof["restored_after_write"] == 64);
            ensure!(
                proof["passive_segment_probe"] == 127
                    && proof["memory_pages"] == 3
                    && proof["table_elements"] == 3
            );
            ensure!(proof["memory_at_restore_sha256"] == expected_hash(false));
            ensure!(proof["memory_after_first_write_sha256"] == expected_hash(true));
            restorations.push(proof);
        }
        captured.finish()?;
        let first = &restorations[0];
        let second = &restorations[1];
        ensure!(
            first["process"] != second["process"],
            "restoration identity reused"
        );
        let child_clock = matches!(
            stage,
            "process-snapshot-first-write" | "process-snapshot-execute"
        );
        let ns = match stage {
            "process-snapshot-capture" => capture_ns,
            "process-snapshot-restore" => restore_ns,
            _ => first["elapsed_ns"]
                .as_i64()
                .ok_or_else(|| anyhow::anyhow!("missing child clock"))?,
        };
        let proof = json!({"mode":MODE,"boundary":measurement_boundary,"pre_fork_threads":1,
            "source":source,"template":template_process,"restored":first["process"],"alternate_restored":second["process"],
            "clock_process":if child_clock { &first["process"] } else { &source },
            "source_released_before_restore":true,"children_reaped":true,"source_after_mutation":125,
            "restored_before_write":64,"restored_after_write":64,"passive_segment_probe":127,
            "memory_pages":3,"table_elements":3,"independent_restorations":2,
            "memory_at_restore_sha256":first["memory_at_restore_sha256"],
            "memory_after_first_write_sha256":first["memory_after_first_write_sha256"]});
        samples.push(json!({"index":index,"elapsed_ns":ns,"operations":1,"sample_type":"individual_operation",
            "warmup":false,"verified":true,"result":["64"],"process_snapshot_result":proof}));
    }
    Ok(
        json!({"version":"linux-process-snapshot-timing-worker-v1","runtime":"wasmtime",
        "runtime_version":"46.0.1","backend":backend,"scenario":stage,"wasm_sha256":ARTIFACT,
        "development_worker":true,"registered_adapter":false,"samples":samples}),
    )
}

pub(super) fn run(backend: &str, stage: &str, count: usize) -> Result<()> {
    println!("{}", collect(backend, stage, count)?);
    Ok(())
}
