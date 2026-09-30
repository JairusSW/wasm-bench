// Simultaneous process-COW worker shared by the memory-only density adapter
// and native qualification. Fault injection is restricted to a separate CLI.
// Children remain owned and live across every group inspection barrier.
use super::*;
use serde_json::{Value, json};

const MAX_INSTANCES: usize = 32;
const ARTIFACT: &str = "7a828c361cd0790044ac23e97c5c93838605c8f202088cabe990c4c1eb6d3381";
const OFFSETS: [usize; 3] = [65535, 131071, 196607];

fn hash(state: &mut State) -> Result<String> {
    let memory = state
        .instance
        .get_memory(&mut state.store, "memory")
        .ok_or_else(|| anyhow::anyhow!("missing density memory"))?;
    ensure!(memory.data_size(&state.store) == 3 * 65536);
    Ok(hex::encode(Sha256::digest(memory.data(&state.store))))
}

fn expected(index: Option<usize>, executed: bool) -> String {
    let mut bytes = vec![0; 3 * 65536];
    bytes[0] = 7;
    if let Some(index) = index {
        for offset in OFFSETS {
            bytes[offset] = (22 + index) as u8;
        }
    }
    if executed {
        bytes[128..131].copy_from_slice(b"xyz");
    }
    hex::encode(Sha256::digest(bytes))
}

struct HeldChild {
    owner: OwnedChild,
    control: UnixStream,
    process: Value,
    proof: Value,
}

fn child(state: &mut State, mut control: UnixStream, index: usize) -> ! {
    let result = (|| -> Result<()> {
        single_threaded()?;
        let process = timing::identity()?;
        let check = state
            .instance
            .get_typed_func::<(), i32>(&mut state.store, "check")
            .map_err(runtime_error)?;
        let memory = state
            .instance
            .get_memory(&mut state.store, "memory")
            .ok_or_else(|| anyhow::anyhow!("missing density memory"))?;
        // Resolve handles before reporting a usable, held child. No proof
        // hashing or guest mutation begins until the whole group is ready.
        timing::send(&mut control, &process)?;
        let mut command = [0];
        control.read_exact(&mut command)?;
        ensure!(command == *b"t");
        let before = check.call(&mut state.store, ()).map_err(runtime_error)?;
        let before_hash = hash(state)?;
        ensure!(before == 64 && before_hash == expected(None, false));
        ensure!(state.call("pages")? == 3 && state.call("elements")? == 3);
        let start = Instant::now();
        for offset in OFFSETS {
            memory.write(&mut state.store, offset, &[(22 + index) as u8])?;
        }
        let touch_ns = timing::elapsed(start)?;
        let touched_hash = hash(state)?;
        ensure!(touched_hash == expected(Some(index), false));
        let mut proof = json!({"index":index,"process":process,"before":before,
            "memory_before_sha256":before_hash,"memory_touched_sha256":touched_hash,
            "touch_byte":22+index,"touched_offsets":OFFSETS,
            "touch_elapsed_ns":touch_ns,"touch_clock_process":process,
            "memory_pages":3,"table_elements":3});
        timing::send(&mut control, &proof)?;
        control.read_exact(&mut command)?;
        ensure!(command == *b"e");
        let start = Instant::now();
        let after = check.call(&mut state.store, ()).map_err(runtime_error)?;
        let execute_ns = timing::elapsed(start)?;
        let probe = state.call("probe")?;
        ensure!(after == 64 && probe == 127);
        let executed_hash = hash(state)?;
        ensure!(executed_hash == expected(Some(index), true));
        proof["after"] = json!(after);
        proof["passive_segment_probe"] = json!(probe);
        proof["execute_elapsed_ns"] = json!(execute_ns);
        proof["execute_clock_process"] = process.clone();
        proof["memory_executed_sha256"] = json!(executed_hash);
        timing::send(&mut control, &proof)?;
        control.read_exact(&mut command)?;
        ensure!(command == *b"q");
        // Recheck after all siblings have written/executed, not just after this
        // child's own write. Distinct markers detect cross-child aliasing.
        ensure!(hash(state)? == expected(Some(index), true));
        ensure!(state.call("check")? == 64);
        timing::send(&mut control, &proof)?;
        Ok(())
    })();
    unsafe { _exit(if result.is_ok() { 0 } else { 112 }) }
}

fn live(children: &mut [HeldChild]) -> Result<()> {
    for child in children {
        ensure!(
            child.owner.poll()?.is_none(),
            "density child exited before group release"
        );
    }
    Ok(())
}

fn template(
    mut state: State,
    mut control: UnixStream,
    count: usize,
    fail_after: Option<usize>,
) -> ! {
    let result = (|| -> Result<()> {
        let process = timing::identity()?;
        timing::send(&mut control, &process)?;
        let mut command = [0];
        control.read_exact(&mut command)?;
        ensure!(command == *b"r");
        let mut children: Vec<HeldChild> = Vec::with_capacity(count);
        for index in 0..count {
            single_threaded()?;
            let (input, output) = UnixStream::pair()?;
            input.set_read_timeout(Some(Duration::from_secs(10)))?;
            input.set_write_timeout(Some(Duration::from_secs(10)))?;
            let parent_pid = unsafe { getpid() };
            // SAFETY: dedicated import-free, single-threaded embedding; every
            // child arms parent-death cleanup and owns an independent guest.
            let pid = unsafe { fork() };
            ensure!(pid >= 0, "density restore fork failed");
            if pid == 0 {
                if bind_parent_lifetime(parent_pid).is_err() {
                    unsafe { _exit(102) }
                }
                // The fork inherited parent-side socket descriptors and Rust
                // ownership guards for older siblings. Disarm these *copies*
                // before closing their descriptors; this child owns none of
                // those PIDs and must never reap/signal or shut down a sibling.
                for sibling in &mut children {
                    sibling.owner.pid = 0;
                }
                drop(children);
                drop(control);
                drop(input);
                child(&mut state, output, index);
            }
            drop(output);
            // Guard ownership before any fallible read, so partial provisioning
            // failure drops and reaps every child already created.
            children.push(HeldChild {
                owner: OwnedChild { pid },
                control: input,
                process: Value::Null,
                proof: Value::Null,
            });
            let held = children.last_mut().unwrap();
            held.process = timing::receive(&mut held.control)?;
            ensure!(held.process["pid"].as_i64() == Some(i64::from(pid)));
            if fail_after == Some(index + 1) {
                live(&mut children)?;
                let processes: Vec<Value> = children.iter().map(|c| c.process.clone()).collect();
                timing::send(
                    &mut control,
                    &json!({"stage":"partial_provisioning","restored":processes}),
                )?;
                control.read_exact(&mut command)?;
                ensure!(command == *b"f");
                // Qualification-only injection through the same closure error
                // path as fork/socket/ready failures. Every owned child must
                // drop and reap before this template exits; no success proof.
                bail!("injected partial provisioning failure");
            }
        }
        live(&mut children)?;
        let processes: Vec<Value> = children.iter().map(|c| c.process.clone()).collect();
        timing::send(&mut control, &json!({"stage":"idle","restored":processes}))?;
        for (expected_command, stage) in [(b't', "touched"), (b'e', "executed")] {
            control.read_exact(&mut command)?;
            ensure!(command[0] == expected_command);
            for child in &mut children {
                child.control.write_all(&command)?;
            }
            for (index, child) in children.iter_mut().enumerate() {
                let proof = timing::receive(&mut child.control)?;
                ensure!(proof["process"] == child.process && proof["index"] == index);
                child.proof = proof;
            }
            live(&mut children)?;
            timing::send(&mut control, &json!({"stage":stage,"restored":processes}))?;
        }
        control.read_exact(&mut command)?;
        ensure!(command == *b"q");
        for child in &mut children {
            child.control.write_all(b"q")?;
        }
        let mut proofs = Vec::with_capacity(count);
        for child in &mut children {
            let proof = timing::receive(&mut child.control)?;
            ensure!(
                proof == child.proof,
                "child state changed after siblings executed"
            );
            ensure!(
                child.owner.wait(Duration::from_secs(2))? == 0,
                "density child failed"
            );
            proofs.push(proof);
        }
        let template_check = state.call("check")?;
        let template_hash = hash(&mut state)?;
        ensure!(template_check == 64 && template_hash == expected(None, false));
        let template_probe = state.call("probe")?;
        ensure!(template_probe == 127);
        timing::send(
            &mut control,
            &json!({"children":proofs,"children_reaped":true,
            "template_check":template_check,"template_memory_sha256":template_hash,
            "template_passive_segment_probe":template_probe}),
        )?;
        Ok(())
    })();
    // The closure's owned-child guards are dropped before _exit, including all
    // error paths. Never unwind inherited source/engine state after the fork.
    unsafe { _exit(if result.is_ok() { 0 } else { 113 }) }
}

pub(super) fn collect(
    backend: &str,
    count: usize,
    observer: &mut dyn FnMut(Value) -> Result<()>,
) -> Result<Value> {
    collect_inner(backend, count, None, observer)
}

fn collect_inner(
    backend: &str,
    count: usize,
    fail_after: Option<usize>,
    observer: &mut dyn FnMut(Value) -> Result<()>,
) -> Result<Value> {
    ensure!(
        (1..=MAX_INSTANCES).contains(&count),
        "density instances must be 1..32"
    );
    if let Some(n) = fail_after {
        ensure!(
            n > 0 && n < count,
            "failure must precede complete provisioning"
        );
    }
    ensure!(matches!(std::env::consts::ARCH, "aarch64" | "x86_64"));
    let strategy = match backend {
        "cranelift" => Strategy::Cranelift,
        "winch" => Strategy::Winch,
        _ => bail!("unknown backend"),
    };
    single_threaded()?;
    let source = timing::identity()?;
    let wasm = wat::parse_str(FIXTURE)?;
    ensure!(hex::encode(Sha256::digest(&wasm)) == ARTIFACT);
    let mut config = Config::new();
    config.strategy(strategy).parallel_compilation(false);
    let engine = Engine::new(&config).map_err(runtime_error)?;
    let module = Module::new(&engine, &wasm).map_err(runtime_error)?;
    let mut store = Store::new(&engine, ());
    let instance = Instance::new(&mut store, &module, &[]).map_err(runtime_error)?;
    let mut state = State { store, instance };
    state.action("seed")?;
    ensure!(state.call("check")? == 64);
    let (parent, child_control) = UnixStream::pair()?;
    parent.set_read_timeout(Some(Duration::from_secs(30)))?;
    parent.set_write_timeout(Some(Duration::from_secs(10)))?;
    single_threaded()?;
    let parent_pid = unsafe { getpid() };
    let pid = unsafe { fork() };
    ensure!(pid >= 0, "density capture fork failed");
    if pid == 0 {
        if bind_parent_lifetime(parent_pid).is_err() {
            unsafe { _exit(102) }
        }
        drop(parent);
        template(state, child_control, count, fail_after);
    }
    drop(child_control);
    let mut captured = Template {
        child: OwnedChild { pid },
        control: parent,
    };
    let template_process = timing::receive(&mut captured.control)?;
    ensure!(template_process["pid"].as_i64() == Some(i64::from(pid)));
    state.action("mutate")?;
    ensure!(state.call("check")? == 125 && state.call("probe").is_err());
    drop(state);
    drop(module);
    drop(engine);
    let event = |stage: &str, restored: Value| {
        json!({"version":"linux-process-snapshot-density-boundary-v1",
        "stage":stage,"instances":count,"source":source,"template":template_process,"restored":restored})
    };
    observer(event("template_after_source_release", json!([])))?;
    let start = Instant::now();
    captured.control.write_all(b"r")?;
    let idle = timing::receive(&mut captured.control)?;
    let provision_ns = timing::elapsed(start)?;
    if let Some(n) = fail_after {
        ensure!(
            idle["stage"] == "partial_provisioning"
                && idle["restored"].as_array().map(Vec::len) == Some(n)
        );
        let mut partial = event("partial_provisioning", idle["restored"].clone());
        partial["version"] = json!("linux-process-snapshot-density-partial-v1");
        partial["fail_after"] = json!(n);
        observer(partial)?;
        captured.control.write_all(b"f")?;
        // EOF occurs only after the template's owned child guards have run.
        let unexpected = timing::receive(&mut captured.control);
        ensure!(
            unexpected.is_err(),
            "partial failure emitted a completion frame"
        );
        ensure!(
            captured.child.wait(Duration::from_secs(2))? == 113,
            "partial failure template did not exit through error cleanup"
        );
        bail!("injected partial provisioning failure cleaned up");
    }
    ensure!(idle["stage"] == "idle" && idle["restored"].as_array().map(Vec::len) == Some(count));
    let processes = idle["restored"].clone();
    observer(event("idle", processes.clone()))?;
    for (command, stage) in [(b't', "touched"), (b'e', "executed")] {
        captured.control.write_all(&[command])?;
        let frame = timing::receive(&mut captured.control)?;
        ensure!(frame["stage"] == stage && frame["restored"] == processes);
        observer(event(stage, processes.clone()))?;
    }
    captured.control.write_all(b"q")?;
    let proof = timing::receive(&mut captured.control)?;
    ensure!(
        captured.child.wait(Duration::from_secs(2))? == 0,
        "density template failed"
    );
    ensure!(
        proof["children_reaped"] == true
            && proof["children"].as_array().map(Vec::len) == Some(count)
    );
    Ok(
        json!({"version":"linux-process-snapshot-density-worker-v1","runtime":"wasmtime","runtime_version":"46.0.1",
        "backend":backend,"architecture":std::env::consts::ARCH,"wasm_sha256":ARTIFACT,
        "qualification_only":true,"registered_adapter":false,"latency_eligible":false,"profile":"memory",
        "mode":"simultaneous_linux_process_cow","instances":count,"pre_fork_threads":1,
        "source":source,"template":template_process,"source_released_before_restore":true,"source_after_mutation":125,
        "provision_clock_process":source,"provision_boundary":"restore_request_to_all_live_ready_identity_frames",
        "provision_elapsed_ns":provision_ns,"children_reaped":true,"template_reaped":true,
        "children":proof["children"],"template_check":proof["template_check"],
        "template_memory_sha256":proof["template_memory_sha256"],"template_passive_segment_probe":proof["template_passive_segment_probe"]}),
    )
}

pub(super) fn run(backend: &str, count: usize) -> Result<()> {
    run_worker(backend, count, None)
}

pub(super) fn run_failure(backend: &str, count: usize, fail_after: usize) -> Result<()> {
    run_worker(backend, count, Some(fail_after))
}

fn run_worker(backend: &str, count: usize, fail_after: Option<usize>) -> Result<()> {
    use std::io::BufRead;
    let stdin = std::io::stdin();
    let mut input = stdin.lock();
    let mut observe = |event: Value| -> Result<()> {
        println!("{}", event);
        std::io::stdout().flush()?;
        let mut line = String::new();
        input.read_line(&mut line)?;
        ensure!(
            line == "continue\n",
            "density inspection requires exact continuation"
        );
        Ok(())
    };
    let proof = collect_inner(backend, count, fail_after, &mut observe)?;
    println!("{}", proof);
    Ok(())
}
